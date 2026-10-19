// @gloss implements gmi:WorkspaceLoader scope=file

// Package load reads a GlossMark workspace: glossmark.yaml, the owned and
// imported ontologies, every boundary manifest, views.yaml, and the list of
// files to scan for markers.
package load

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	Lock *Lock
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
		if o.Version == "" {
			o.Version = decl.Version
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
	for _, p := range iprefixes {
		im := cfg.Imports[p]
		ws.Imported[p] = im
		ws.Prefixes[p] = im.IRI
		o, diag := resolveImport(root, p, im)
		if diag != nil {
			ws.Diags = append(ws.Diags, *diag)
		}
		if o == nil {
			o = &model.Ontology{Concepts: map[string]*model.Concept{}}
		} else {
			ws.Resolved[p] = &Resolved{Prefix: p, Version: o.Version, Range: im.Version, Source: im.Source, Hash: hashFile(o.Path), Path: o.Path}
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
	if v, err := model.LoadViews(vpath); err == nil {
		ws.Views = v
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

// isLinkedWorktree reports whether dir is the root of a linked git worktree
// (git worktree add, or an agent harness's .claude/worktrees/<name>): a
// second checkout of a repository, whose files would otherwise be read twice.
// Its .git is a file that points into a .../worktrees/<name> git directory.
// Submodules also have a .git file, pointing into .../modules/<name>, and
// stay part of the walk as before.
func isLinkedWorktree(dir string) bool {
	b, err := os.ReadFile(filepath.Join(dir, ".git"))
	if err != nil {
		return false
	}
	line := strings.TrimSpace(string(b))
	if !strings.HasPrefix(line, "gitdir:") {
		return false
	}
	gitdir := filepath.ToSlash(strings.TrimSpace(strings.TrimPrefix(line, "gitdir:")))
	return strings.Contains(gitdir, "/worktrees/")
}

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
			if rel != "" && isLinkedWorktree(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ManifestName {
			m, err := model.LoadManifest(path)
			if err != nil {
				ws.Diags = append(ws.Diags, marker.Diagnostic{Path: rel, Line: 1, Severity: "error", Code: "GM110", Msg: err.Error()})
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
		m := ws.ManifestFor(f)
		if m != nil && len(m.Generated) > 0 {
			sub := f
			if m.Dir != "" {
				sub = strings.TrimPrefix(f, m.Dir+"/")
			}
			if model.MatchAnyGlob(m.Generated, sub) {
				continue
			}
		}
		kept = append(kept, f)
	}
	sort.Strings(kept)
	ws.Files = kept
	return nil
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
