// @gloss implements gmi:Scaffolder scope=file

// Package scaffold plans and writes the files gloss init creates: the
// config, two tier files, a root manifest and one per top-level source
// directory, views, .gitignore lines, a task-runner target and a CI job.
// It never overwrites a file; existing ones are skipped or appended to.
package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Options tunes the plan.
type Options struct {
	Name    string // repository name (default: the directory name)
	Core    string // core namespace prefix (default: guessed from the name)
	Impl    string // implementation namespace prefix (default: core + "i")
	IRIBase string // base for namespace IRIs (default https://onto.example.com/<name>/)
	Version string // gloss release the CI job pins (default v0.1.0)
}

// Action is what happens to one file.
type Action string

const (
	Create Action = "create"
	Append Action = "append"
	Skip   Action = "skip"
)

// File is one planned write.
type File struct {
	Path    string // repo-relative
	Action  Action
	Content string // full content (create) or text to add (append)
	Note    string
	// insertAfter, when set on an append, inserts Content after the first
	// line containing it instead of at the end (package.json scripts).
	insertAfter string
}

// Plan is the full set of writes.
type Plan struct {
	Root     string
	Name     string
	Core     string
	Impl     string
	Boundary string // boundary prefix
	Dirs     []string
	Files    []File
}

// skipDirs are never scaffolded as boundaries.
var skipDirs = map[string]bool{"node_modules": true, "vendor": true, "dist": true, "build": true, "target": true, "out": true,
	"docs": true, "doc": true, "test": true, "tests": true, "__pycache__": true, "venv": true, "env": true, "coverage": true,
	"ontology": true, "examples": true, "scripts": true, "third_party": true}

// sourceExts mark a directory as holding source code.
var sourceExts = map[string]bool{".py": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".go": true, ".rs": true,
	".java": true, ".kt": true, ".rb": true, ".cs": true, ".php": true, ".swift": true, ".scala": true, ".c": true, ".cc": true, ".cpp": true,
	".h": true, ".tf": true, ".sql": true, ".vue": true, ".svelte": true, ".dart": true, ".ex": true, ".exs": true}

// words splits a repository name into lower-case words.
func words(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.ToLower(string(cur)))
			cur = nil
		}
	}
	rs := []rune(s)
	for i, r := range rs {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if unicode.IsUpper(r) && i > 0 && unicode.IsLower(rs[i-1]) {
				flush()
			}
			cur = append(cur, r)
		default:
			flush()
		}
	}
	flush()
	return out
}

// GuessPrefixes derives the core and implementation prefixes from a name:
// initials for several words (agent-arena → aa), else the first two letters
// (pfinance → pf); the implementation prefix adds an i.
func GuessPrefixes(name string) (string, string) {
	ws := words(name)
	core := ""
	if len(ws) >= 2 {
		for _, w := range ws {
			core += w[:1]
		}
	} else if len(ws) == 1 {
		core = ws[0]
		if len(core) > 2 {
			core = core[:2]
		}
	}
	if len(core) < 2 {
		core = "app"
	}
	if !unicode.IsLetter(rune(core[0])) {
		core = "x" + core
	}
	return core, core + "i"
}

func slug(name string) string {
	ws := words(name)
	if len(ws) == 0 {
		return "repo"
	}
	return strings.Join(ws, "-")
}

// SourceDirs lists top-level directories that hold source code.
func SourceDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() || strings.HasPrefix(n, ".") || skipDirs[strings.ToLower(n)] {
			continue
		}
		found := false
		filepath.WalkDir(filepath.Join(root, n), func(p string, d os.DirEntry, err error) error {
			if err != nil || found {
				return filepath.SkipDir
			}
			if d.IsDir() && (strings.HasPrefix(d.Name(), ".") || skipDirs[d.Name()]) && p != filepath.Join(root, n) {
				return filepath.SkipDir
			}
			if !d.IsDir() && sourceExts[strings.ToLower(filepath.Ext(p))] {
				found = true
				return filepath.SkipAll
			}
			return nil
		})
		if found {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func exists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

// Make builds the plan for root.
func Make(root string, opt Options) (*Plan, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	name := opt.Name
	if name == "" {
		name = filepath.Base(root)
	}
	core, impl := GuessPrefixes(name)
	if opt.Core != "" {
		core = opt.Core
		impl = core + "i"
	}
	if opt.Impl != "" {
		impl = opt.Impl
	}
	if core == impl {
		return nil, fmt.Errorf("core and implementation prefixes must differ (both %q)", core)
	}
	base := opt.IRIBase
	if base == "" {
		base = "https://onto.example.com/" + slug(name) + "/"
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	version := opt.Version
	if version == "" {
		version = "v0.1.0"
	}
	p := &Plan{Root: root, Name: name, Core: core, Impl: impl, Boundary: core + "b", Dirs: SourceDirs(root)}
	add := func(f File) {
		if f.Action == Create && exists(root, f.Path) {
			f.Action, f.Note = Skip, "exists; left as is"
		}
		p.Files = append(p.Files, f)
	}
	add(File{Path: "glossmark.yaml", Action: Create, Content: fmt.Sprintf(`# glossmark.yaml: the namespaces this repository owns (docs/FILE-FORMATS.md).
grammar: 1
repo-iri: %s                 # boundary prefixes such as %s: expand under it; no prefixes: needed
owns:
  %s: { iri: %score/, tier: core,           version: 0.1.0 }   # what the business recognises
  %s: { iri: %simpl/, tier: implementation, version: 0.1.0 }   # the mechanisms that realize it
policy:
  soft-conf-threshold: 0.7
`, base, p.Boundary, core, base, impl, base)})
	add(File{Path: "ontology/" + core + ".gloss.yaml", Action: Create, Content: fmt.Sprintf(`# ontology/%s.gloss.yaml: the core tier.
# Litmus test: if a domain expert would nod at the term, it is core; if only an
# engineer would, it belongs in %s:. Core concepts never mention %s:.
# Write docs quoted: doc: "One account at an institution (checking, savings)."
namespace: %s
version: 0.1.0
concepts: {}
`, core, impl, impl, core)})
	add(File{Path: "ontology/" + impl + ".gloss.yaml", Action: Create, Content: fmt.Sprintf(`# ontology/%s.gloss.yaml: the implementation tier.
# Every concept here carries realizes: <a %s: concept>, presents:/hosts: for a
# page or a shell, or internal: true for an engineering-private mechanism.
namespace: %s
version: 0.1.0
concepts: {}
`, impl, core, impl)})
	add(File{Path: "ontology/views.yaml", Action: Create, Content: fmt.Sprintf(`# ontology/views.yaml: declared projections over tier × zoom (docs/FILE-FORMATS.md).
views:
  business:
    doc: The core tier only; implementation folds into what it realizes.
    include: [%s:*]
    collapse: { "%s:*": via realizes }
    show-published: true
    relations: [produces, consumes, constrains, publishes, subscribes, reverses, is-a, realizes]
    lift: range-lcs
    zoom: system
  engineering:
    doc: Both tiers, two levels of parts, container zoom.
    include: [%s:*, %s:*]
    expand: { part-of: 2 }
    zoom: container
`, core, impl, core, impl)})
	rootB := p.Boundary + ":" + slug(name)
	add(File{Path: ".gloss.yaml", Action: Create, Content: fmt.Sprintf(`# .gloss.yaml: the repository's root boundary (docs/FILE-FORMATS.md).
# Declare realizes: once you know what the system stands for; until then
# drift reports orphans and coverage, and nothing else.
boundary: %s
level: system
default-namespace: %s
`, rootB, impl)})
	for _, d := range p.Dirs {
		child := slug(d)
		if p.Boundary+":"+child == rootB {
			child += "-lib" // the package named like the repository (pfinance/pfinance)
		}
		add(File{Path: d + "/.gloss.yaml", Action: Create, Content: fmt.Sprintf(`# %s/.gloss.yaml: a container boundary. Fill in realizes: (or realizes: inherit).
boundary: %s:%s
level: container
part-of: %s
default-namespace: %s
`, d, p.Boundary, child, rootB, impl)})
	}
	// .gitignore: the viewer only. The graph and the ledger are committed.
	ignore := []string{".glossmark/viewer.html", ".glossmark/views.js", ".glossmark/views.json"}
	gi, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	var missing []string
	for _, l := range ignore {
		if !strings.Contains(string(gi), l) {
			missing = append(missing, l)
		}
	}
	switch {
	case len(missing) == 0:
		add(File{Path: ".gitignore", Action: Skip, Note: "already ignores the viewer"})
	case len(gi) == 0:
		add(File{Path: ".gitignore", Action: Create, Content: "# GlossMark: the viewer is generated; graph.jsonl and ledger.jsonl are committed\n" + strings.Join(missing, "\n") + "\n"})
	default:
		pre := "\n"
		if strings.HasSuffix(string(gi), "\n") {
			pre = ""
		}
		add(File{Path: ".gitignore", Action: Append, Content: pre + "# GlossMark: the viewer is generated; graph.jsonl and ledger.jsonl are committed\n" + strings.Join(missing, "\n") + "\n"})
	}
	// Task runner.
	mk, mkErr := os.ReadFile(filepath.Join(root, "Makefile"))
	pkg, pkgErr := os.ReadFile(filepath.Join(root, "package.json"))
	switch {
	case mkErr == nil && strings.Contains(string(mk), "\ngloss:") || strings.HasPrefix(string(mk), "gloss:"):
		add(File{Path: "Makefile", Action: Skip, Note: "already has a gloss target"})
	case mkErr == nil:
		add(File{Path: "Makefile", Action: Append, Content: "\n.PHONY: gloss\ngloss: ## lint markers and refresh the committed graph\n\tgloss check\n\tgloss build -q\n"})
	case pkgErr == nil && strings.Contains(string(pkg), `"gloss"`):
		add(File{Path: "package.json", Action: Skip, Note: "already has a gloss script"})
	case pkgErr == nil && strings.Contains(string(pkg), `"scripts": {`):
		add(File{Path: "package.json", Action: Append, Content: `    "gloss": "gloss check && gloss build -q",`, insertAfter: `"scripts": {`})
	case pkgErr == nil:
		add(File{Path: "package.json", Action: Skip, Note: `no "scripts" block; add "gloss": "gloss check && gloss build -q" by hand`})
	default:
		add(File{Path: "Makefile", Action: Skip, Note: "no Makefile or package.json; run gloss check && gloss build -q from your task runner"})
	}
	add(File{Path: ".github/workflows/gloss.yml", Action: Create, Content: fmt.Sprintf(`# GlossMark: lint markers, report drift, and comment the per-view impact on PRs.
name: gloss
on: [pull_request, push]
permissions: { contents: read, pull-requests: write }
jobs:
  gloss:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: mckalvan/glossmark@%s
        with: { fail-on: HARD, coverage-fail-under: current }
`, version)})
	return p, nil
}

// String renders the plan, one line per file.
func (p *Plan) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "repository %s: core %s:, implementation %s:, boundaries %s:\n", p.Name, p.Core, p.Impl, p.Boundary)
	for _, f := range p.Files {
		fmt.Fprintf(&sb, "  %-6s %s", f.Action, f.Path)
		if f.Note != "" {
			fmt.Fprintf(&sb, "  (%s)", f.Note)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// Apply writes the plan.
func (p *Plan) Apply() error {
	for _, f := range p.Files {
		path := filepath.Join(p.Root, filepath.FromSlash(f.Path))
		switch f.Action {
		case Create:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(f.Content), 0o644); err != nil {
				return err
			}
		case Append:
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out := string(b) + f.Content
			if f.insertAfter != "" {
				i := strings.Index(string(b), f.insertAfter)
				if i < 0 {
					continue
				}
				j := i + len(f.insertAfter)
				out = string(b[:j]) + "\n" + f.Content + string(b[j:])
			}
			if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}
