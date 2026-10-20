// @gloss implements gmi:WorkspaceLoader scope=file

// Package load reads a GlossMark workspace: glossmark.yaml, the owned and
// imported ontologies, every boundary manifest, views.yaml, and the list of
// files to scan for markers.
package load

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/version"
)

// ConfigName is the repository-level file.
const ConfigName = "glossmark.yaml"

// ManifestName is the per-directory boundary manifest.
const ManifestName = ".gloss.yaml"

// StateDir holds tool-maintained files (graph.jsonl, ledger.jsonl).
const StateDir = ".glossmark"

// Workspace is a loaded repository.
type Workspace struct {
	Root       string
	Config     *model.Config
	Ontologies map[string]*model.Ontology // prefix → ontology, owned and imported
	Owned      map[string]bool
	Imported   map[string]*model.ImportDecl
	Manifests  []*model.Manifest // sorted by directory depth then name
	Views      *model.Views
	Vocab      *model.Vocabulary
	Files      []string // repo-relative, slash-separated, sorted
	Diags      []marker.Diagnostic
	// Prefixes is the effective CURIE table: owned, imported and config prefixes.
	Prefixes map[string]string
	// Resolved records how each import was resolved (version, source, hash).
	Resolved map[string]*Resolved
	// Lock is glossmark.lock when present.
	Lock    *Lock
	lockErr error
}

// FindRoot walks up from start until it finds glossmark.yaml.
func FindRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if st, err := os.Stat(filepath.Join(dir, ConfigName)); err == nil && !st.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s found in %s or any parent", ConfigName, start)
		}
		dir = parent
	}
}

// Load reads the workspace rooted at root (the directory holding glossmark.yaml).
func Load(root string) (*Workspace, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	cfg, err := model.LoadConfig(filepath.Join(root, ConfigName))
	if err != nil {
		return nil, err
	}
	ws := &Workspace{Root: root, Config: cfg, Ontologies: map[string]*model.Ontology{}, Owned: map[string]bool{},
		Imported: map[string]*model.ImportDecl{}, Vocab: model.NewVocabulary(), Prefixes: map[string]string{}, Resolved: map[string]*Resolved{}}
	if !version.SupportsGrammar(cfg.Grammar) {
		return nil, fmt.Errorf("%s: grammar %d is not supported by gloss %s (supported: %v); upgrade gloss or lower the directive", ConfigName, cfg.Grammar, version.String(), version.SupportedGrammars)
	}
	for p, v := range cfg.Prefixes {
		ws.Prefixes[p] = v
	}
	// Owned namespaces.
	prefixes := make([]string, 0, len(cfg.Owns))
	for p := range cfg.Owns {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)
	for _, p := range prefixes {
		decl := cfg.Owns[p]
		ws.Owned[p] = true
		ws.Prefixes[p] = decl.IRI
		path := filepath.Join(root, filepath.FromSlash(decl.File))
		o, err := model.LoadOntology(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				ws.Diags = append(ws.Diags, marker.Diagnostic{Path: decl.File, Line: 1, Severity: "error", Code: "GM100",
					Msg: fmt.Sprintf("ontology file for owned namespace %s: not found", p)})
				o = &model.Ontology{Concepts: map[string]*model.Concept{}, Path: path}
			} else {
				return nil, err
			}
		}
		o.Namespace = p
		if o.IRI == "" {
			o.IRI = decl.IRI
		}
		if o.Tier == "" {
			o.Tier = decl.Tier
		}
		// The ontology file's version: is the namespace version, because it is
		// what importers resolve; owns.<prefix>.version may only repeat it.
		switch {
		case decl.Version != "" && o.Version == "" && !errors.Is(err, fs.ErrNotExist):
			ws.Diags = append(ws.Diags, marker.Diagnostic{Path: ConfigName, Line: 1, Severity: "warning", Code: "GM114",
				Msg: fmt.Sprintf("owns.%s.version is %s but %s has no version:, which is what importers and gloss diff read; move it into the ontology file", p, decl.Version, decl.File)})
		case decl.Version != "" && o.Version != "" && decl.Version != o.Version:
			ws.Diags = append(ws.Diags, marker.Diagnostic{Path: ConfigName, Line: 1, Severity: "error", Code: "GM115",
				Msg: fmt.Sprintf("owns.%s.version is %s but %s says version: %s; the ontology file is the version importers resolve, so make them agree (or drop owns.%s.version)", p, decl.Version, decl.File, o.Version, p)})
		}
		if decl.Version == "" {
			decl.Version = o.Version
		}
		ws.Ontologies[p] = o
	}
	// Imported namespaces.
	iprefixes := make([]string, 0, len(cfg.Imports))
	for p := range cfg.Imports {
		iprefixes = append(iprefixes, p)
	}
	sort.Strings(iprefixes)
	ws.Lock, ws.lockErr = ReadLock(root)
	for _, p := range iprefixes {
		im := cfg.Imports[p]
		ws.Imported[p] = im
		ws.Prefixes[p] = im.IRI
		o, diag := resolveImport(root, p, im)
		var up *Resolved
		if o != nil {
			up = &Resolved{Prefix: p, Version: o.Version, Range: im.Version, Source: im.Source, Hash: hashFile(o.Path), Path: o.Path}
		}
		// A locked import resolves from its vendored copy; the upstream is
		// only compared, so a newer one is reported instead of taken.
		if po := pinned(root, p, im, ws.Lock); po != nil {
			r := &Resolved{Prefix: p, Version: po.Version, Range: im.Version, Source: im.Source, Hash: hashFile(po.Path), Path: po.Path, Pinned: true, Upstream: up}
			if diag != nil {
				diag.Severity = "warning"
				diag.Msg += "; resolving the pinned copy " + ws.Rel(po.Path)
			}
			if up != nil && up.Hash != r.Hash {
				ws.Diags = append(ws.Diags, upstreamAvailable(r, up))
			}
			o, ws.Resolved[p] = po, r
		} else if o != nil {
			ws.Resolved[p] = up
		}
		if diag != nil {
			ws.Diags = append(ws.Diags, *diag)
		}
		if o == nil {
			o = &model.Ontology{Concepts: map[string]*model.Concept{}}
		}
		o.Namespace = p
		if o.IRI == "" {
			o.IRI = im.IRI
		}
		if o.Tier == "" {
			o.Tier = im.Tier
		}
		if o.Tier == "" {
			o.Tier = model.TierCore
			if looksImplementation(o) {
				ws.Diags = append(ws.Diags, marker.Diagnostic{Path: ConfigName, Line: 1, Severity: "warning", Code: "GM113",
					Msg: fmt.Sprintf("imports.%s: neither the import nor the imported ontology states a tier, so %s: is read as core and only-published-concepts (GM230) is not enforced although it has published or realizing concepts; add tier: implementation to imports.%s, or import from gloss publish output", p, p, p)})
			}
		}
		ws.Ontologies[p] = o
	}
	// Relation overrides from every ontology (owned first, then imported).
	for _, p := range append(prefixes, iprefixes...) {
		o := ws.Ontologies[p]
		names := make([]string, 0, len(o.Relations))
		for n := range o.Relations {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			ws.Vocab.Override(n, o.Relations[n])
		}
	}
	// Views.
	vpath := filepath.Join(root, filepath.FromSlash(cfg.Views))
	if v, unknown, err := model.LoadViews(vpath); err == nil {
		ws.Views = v
		for _, k := range unknown {
			ws.Diags = append(ws.Diags, marker.Diagnostic{Path: filepath.ToSlash(cfg.Views), Line: k.Line, Severity: "error", Code: "GM118", Msg: fmt.Sprintf("unknown key %q in the views file; the rest of the file still applies", k.Key)})
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	} else {
		var core, impl []string
		for _, p := range append(prefixes, iprefixes...) {
			if ws.Ontologies[p].Tier == model.TierImpl {
				impl = append(impl, p)
			} else {
				core = append(core, p)
			}
		}
		ws.Views = model.DefaultViews(core, impl)
	}
	// Import version ranges and the lockfile.
	ws.Diags = append(ws.Diags, verifyImports(ws)...)
	// Walk the tree for manifests and candidate files.
	if err := ws.walk(); err != nil {
		return nil, err
	}
	return ws, nil
}

// upstreamAvailable is the GM116 warning for a pinned import whose source
// now resolves to something else.
func upstreamAvailable(pin, up *Resolved) marker.Diagnostic {
	msg := fmt.Sprintf("imports.%s: upstream content changed under the same version %s (it should bump); still resolving the pinned copy. Review it, then run gloss lock --update", pin.Prefix, orNone(pin.Version))
	if up.Version != pin.Version {
		where := "inside"
		if code, _ := RangeProblem(up); code != "" {
			where = "outside"
		}
		msg = fmt.Sprintf("imports.%s: %s available, %s %s; still resolving the pinned %s. Review it, then run gloss lock --update", pin.Prefix, orNone(up.Version), where, orNone(pin.Range), orNone(pin.Version))
	}
	return marker.Diagnostic{Path: LockName, Line: 1, Severity: "warning", Code: "GM116", Msg: msg}
}

// looksImplementation reports whether an ontology has concepts only an
// implementation namespace carries: published: or realizes:.
func looksImplementation(o *model.Ontology) bool {
	for _, c := range o.Concepts {
		if c.Published != nil || len(c.Realizes) > 0 {
			return true
		}
	}
	return false
}

// resolveImport loads an imported ontology from a local source.
func resolveImport(root, prefix string, im *model.ImportDecl) (*model.Ontology, *marker.Diagnostic) {
	src := im.Source
	if strings.HasPrefix(src, "git+") || strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		// Remote fetch is Phase 3. Accept a vendored copy under ontology/<prefix>.gloss.yaml if present.
		vend := filepath.Join(root, "ontology", prefix+".gloss.yaml")
		if o, err := model.LoadOntology(vend); err == nil {
			return o, &marker.Diagnostic{Path: ConfigName, Line: 1, Severity: "warning", Code: "GM101",
				Msg: fmt.Sprintf("imports.%s: remote source %q is not fetched yet; using vendored copy %s", prefix, src, filepath.ToSlash(filepath.Join("ontology", prefix+".gloss.yaml")))}
		}
		return nil, &marker.Diagnostic{Path: ConfigName, Line: 1, Severity: "error", Code: "GM102",
			Msg: fmt.Sprintf("imports.%s: remote sources (%q) are not fetched yet; vendor the ontology directory and set source to a local path", prefix, src)}
	}
	if src == "" {
		src = filepath.ToSlash(filepath.Join("ontology", prefix+".gloss.yaml"))
	}
	path := src
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, filepath.FromSlash(src))
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, &marker.Diagnostic{Path: ConfigName, Line: 1, Severity: "error", Code: "GM103",
			Msg: fmt.Sprintf("imports.%s: source %q not found", prefix, src)}
	}
	if st.IsDir() {
		cands := []string{filepath.Join(path, prefix+".gloss.yaml"), filepath.Join(path, "ontology", prefix+".gloss.yaml")}
		for _, c := range cands {
			if o, err := model.LoadOntology(c); err == nil {
				return o, nil
			}
		}
		return nil, &marker.Diagnostic{Path: ConfigName, Line: 1, Severity: "error", Code: "GM103",
			Msg: fmt.Sprintf("imports.%s: no %s.gloss.yaml under %q", prefix, prefix, src)}
	}
	o, err := model.LoadOntology(path)
	if err != nil {
		return nil, &marker.Diagnostic{Path: ConfigName, Line: 1, Severity: "error", Code: "GM104", Msg: fmt.Sprintf("imports.%s: %v", prefix, err)}
	}
	return o, nil
}

var defaultSkipDirs = map[string]bool{".git": true, "node_modules": true, StateDir: true, ".hg": true, ".svn": true, "__pycache__": true, ".venv": true, "venv": true, ".terraform": true, ".idea": true, ".vscode": true}

const maxFileSize = 4 << 20

func (ws *Workspace) walk() error {
	var manifests []*model.Manifest
	var files []string
	err := filepath.WalkDir(ws.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(ws.Root, path)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			rel = ""
		}
		if d.IsDir() {
			if rel != "" && (defaultSkipDirs[d.Name()] || model.MatchAnyGlob(ws.Config.Exclude, rel) || model.MatchAnyGlob(ws.Config.Exclude, rel+"/")) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ManifestName {
			m, unknown, err := model.LoadManifest(path)
			for _, k := range unknown {
				ws.Diags = append(ws.Diags, marker.Diagnostic{Path: rel, Line: k.Line, Severity: "error", Code: "GM110", Msg: fmt.Sprintf("unknown manifest key %q; the other keys still apply", k.Key)})
			}
			if err != nil {
				// The diagnostic carries the workspace-relative path; drop the
				// absolute one LoadManifest prefixes.
				ws.Diags = append(ws.Diags, marker.Diagnostic{Path: rel, Line: 1, Severity: "error", Code: "GM110", Msg: strings.TrimPrefix(err.Error(), path+": ")})
				return nil
			}
			m.Path = rel
			m.Dir = strings.TrimSuffix(filepath.ToSlash(filepath.Dir(rel)), ".")
			if m.Dir == "." {
				m.Dir = ""
			}
			manifests = append(manifests, m)
			return nil
		}
		if rel == ConfigName || model.MatchAnyGlob(ws.Config.Exclude, rel) {
			return nil
		}
		if strings.HasSuffix(rel, ".gloss.yaml") && strings.HasPrefix(rel, "ontology/") {
			return nil // ontology files are not marker carriers
		}
		if info, err := d.Info(); err == nil && info.Size() > maxFileSize {
			return nil
		}
		if _, ok := marker.StyleFor(rel, ws.Config.CommentStyles); !ok {
			if line := unscannedMarker(path, rel); line > 0 {
				ws.Diags = append(ws.Diags, marker.Diagnostic{Path: rel, Line: line, Severity: "warning", Code: "GM117",
					Msg: fmt.Sprintf("this file has a @gloss marker but %s has no comment style, so it is not scanned; add it under comment-styles: in %s (JSON takes a sidecar, %s.gloss)", styleKey(rel), ConfigName, filepath.Base(rel))})
			}
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(manifests, func(i, j int) bool {
		di, dj := strings.Count(manifests[i].Dir, "/"), strings.Count(manifests[j].Dir, "/")
		if (manifests[i].Dir == "") != (manifests[j].Dir == "") {
			return manifests[i].Dir == ""
		}
		if di != dj {
			return di < dj
		}
		return manifests[i].Dir < manifests[j].Dir
	})
	ws.Manifests = manifests
	// Drop files matched by a manifest's generated globs.
	var kept []string
	for _, f := range files {
		if !ws.generated(f) {
			kept = append(kept, f)
		}
	}
	sort.Strings(kept)
	ws.Files = kept
	return nil
}

// markerLike matches text that reads as a marker: the marker keyword, an
// optional question mark, a relation word and the start of a target.
var markerLike = regexp.MustCompile(`@gloss\??[ \t]+[a-z][a-z-]*[ \t]+[A-Za-z.]`)

// unscannedMarker returns the first line of a file the walk skips (its type
// has no comment style) that reads as a marker, or 0. Binary files, and
// files a sidecar already annotates, are not reported.
func unscannedMarker(path, rel string) int {
	if _, err := os.Stat(path + ".gloss"); err == nil {
		return 0
	}
	b, err := os.ReadFile(path)
	if err != nil || bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0 {
		return 0
	}
	loc := markerLike.FindIndex(b)
	if loc == nil {
		return 0
	}
	return bytes.Count(b[:loc[0]], []byte("\n")) + 1
}

// styleKey is the comment-styles key that would cover rel: its extension,
// or its base name when it has none.
func styleKey(rel string) string {
	base := filepath.Base(rel)
	if ext := strings.ToLower(filepath.Ext(base)); ext != "" {
		return ext
	}
	return base
}

// generated reports whether a file matches its manifest's generated globs.
func (ws *Workspace) generated(f string) bool {
	m := ws.ManifestFor(f)
	if m == nil || len(m.Generated) == 0 {
		return false
	}
	sub := f
	if m.Dir != "" {
		sub = strings.TrimPrefix(f, m.Dir+"/")
	}
	return model.MatchAnyGlob(m.Generated, sub)
}

// InScope reports whether a repo-relative file path is one the walk would
// keep: outside skipped and excluded directories, not glossmark.yaml, a
// manifest or an ontology file, in a language with a comment style, and not
// generated. The size limit is not checked. gloss diff uses it to rebuild a
// base tree's file set from git when the base graph is read from a commit.
func (ws *Workspace) InScope(rel string) bool {
	parts := strings.Split(rel, "/")
	for i := 0; i < len(parts)-1; i++ {
		dir := strings.Join(parts[:i+1], "/")
		if defaultSkipDirs[parts[i]] || model.MatchAnyGlob(ws.Config.Exclude, dir) || model.MatchAnyGlob(ws.Config.Exclude, dir+"/") {
			return false
		}
	}
	if parts[len(parts)-1] == ManifestName || rel == ConfigName || model.MatchAnyGlob(ws.Config.Exclude, rel) {
		return false
	}
	if strings.HasSuffix(rel, ".gloss.yaml") && strings.HasPrefix(rel, "ontology/") {
		return false
	}
	if _, ok := marker.StyleFor(rel, ws.Config.CommentStyles); !ok {
		return false
	}
	return !ws.generated(rel)
}

// ManifestFor returns the nearest ancestor manifest for a repo-relative path, or nil.
func (ws *Workspace) ManifestFor(rel string) *model.Manifest {
	var best *model.Manifest
	for _, m := range ws.Manifests {
		if m.Dir == "" || rel == m.Dir || strings.HasPrefix(rel, m.Dir+"/") {
			if best == nil || len(m.Dir) > len(best.Dir) {
				best = m
			}
		}
	}
	return best
}

// Tier returns the tier of a prefix, or "".
func (ws *Workspace) Tier(prefix string) string {
	if o, ok := ws.Ontologies[prefix]; ok {
		return o.Tier
	}
	return ""
}

// StatePath returns the path of a tool-maintained file.
func (ws *Workspace) StatePath(name string) string {
	return filepath.Join(ws.Root, StateDir, name)
}

// Rel returns path relative to the root, slash-separated. A file outside the
// root, such as an import from a sibling checkout, keeps its ../ form, so
// nothing written to the graph depends on where the repository is checked out.
func (ws *Workspace) Rel(path string) string {
	if !filepath.IsAbs(path) {
		return filepath.ToSlash(path)
	}
	rel, err := filepath.Rel(ws.Root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// Abs returns the absolute path of a repo-relative file.
func (ws *Workspace) Abs(rel string) string { return filepath.Join(ws.Root, filepath.FromSlash(rel)) }

// IsTestPath reports whether a repo-relative path looks like a test.
func (ws *Workspace) IsTestPath(rel string) bool {
	pats := ws.Config.TestPaths
	if len(pats) == 0 {
		pats = DefaultTestPaths
	}
	return model.MatchAnyGlob(pats, rel)
}

// DefaultTestPaths are the layouts the inferrer treats as tests.
var DefaultTestPaths = []string{"**/test_*.py", "**/*_test.py", "**/tests/**", "**/test/**", "**/*_test.go", "**/*.test.ts", "**/*.test.tsx", "**/*.spec.ts", "**/*.spec.js", "**/*.test.js", "**/*.feature", "**/__tests__/**", "**/*Test.java", "**/*Tests.cs", "**/*_spec.rb", "**/spec/**"}
