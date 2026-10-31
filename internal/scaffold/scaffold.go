// @gloss implements gmi:Scaffolder scope=file

// Package scaffold plans and writes the files gloss init creates: the
// config, two tier files, a root manifest and one per deployable directory
// (a top-level directory with source, schema, infrastructure or config files,
// or each project inside a grouping directory such as apps/ or services/;
// a utility directory such as utils/ or core/ with no entry point gets none),
// views, .gitignore lines, a task-runner target and a CI job (written only
// when the workspace is the git root). It never overwrites a file; existing
// ones are skipped or appended to.
package scaffold

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/mckalvan/glossmark/internal/model"
)

// Options tunes the plan.
type Options struct {
	Name    string // repository name (default: the directory name)
	Core    string // core namespace prefix (default: guessed from the name)
	Impl    string // implementation namespace prefix (default: core + "i")
	IRIBase string // base for namespace IRIs (default https://onto.example.com/<name>/)
	Version string // gloss release the CI job pins (default v0.3.0)
	// BoundaryPrefix is the prefix of boundary names (default: the
	// repository's own core prefix + "b").
	BoundaryPrefix string
	// Imports are sibling workspaces (paths relative to the root, or
	// absolute) whose core namespaces this repository imports instead of
	// owning: init writes imports: entries and no core tier file.
	Imports []string
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
	// Guessed is set when the core prefix was derived from the name, not
	// given or imported; the plan then says how to choose it (issue #229).
	Guessed bool
	Dirs    []string // manifest directories, slash-separated and relative to Root
	// Utility lists directories named like shared code (utils/, core/, ...)
	// with no entry point: no manifest, they stay under their parent.
	Utility []string
	Files   []File
	// GitRoot is the enclosing git repository's root when it differs from
	// Root; the CI job is then printed (Workflow), not written.
	GitRoot  string
	Workflow string
	// Imported lists the core prefixes taken from Options.Imports; the
	// caller runs gloss lock after Apply when it is non-empty.
	Imported []string
	// Vocabulary lists gloss import commands for a glossary or an enums
	// file found in the tree.
	Vocabulary []string
}

// SuggestBlock is the commented suggest: block gloss init writes into
// glossmark.yaml: the repository defaults of gloss suggest and the
// precision auto-accept must meet, at their defaults (ADR 0024).
const SuggestBlock = `# suggest:                          # gloss suggest's defaults for this repository; flags override them
#   auto-accept-precision: 0.9      # auto-accept (SOFT) only proposals whose evidence measured at least this precision;
#                                   # the conf threshold is derived from it (gloss doctor prints it)
#   min-conf: 0.3                   # lowest conf that becomes a proposal
#   stopwords: []                   # more programming words that never match a concept on their own
#   labels: labels.tsv              # what gloss suggest --calibrate scores against (default: this repository's own links)
#   calibration: glossmark.calibration.tsv  # this repository's measurement (gloss suggest --calibrate --write);
#                                   # the threshold is derived from it instead of the shipped calibration
`

// skipDirs are never scaffolded as boundaries.
var skipDirs = map[string]bool{"node_modules": true, "vendor": true, "dist": true, "build": true, "target": true, "out": true,
	"docs": true, "doc": true, "test": true, "tests": true, "__pycache__": true, "venv": true, "env": true, "coverage": true,
	"ontology": true, "examples": true, "scripts": true, "third_party": true}

// sourceExts mark a directory as holding source code.
var sourceExts = map[string]bool{".py": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".go": true, ".rs": true,
	".java": true, ".kt": true, ".rb": true, ".cs": true, ".php": true, ".swift": true, ".scala": true, ".c": true, ".cc": true, ".cpp": true,
	".h": true, ".tf": true, ".sql": true, ".vue": true, ".svelte": true, ".dart": true, ".ex": true, ".exs": true}

// schemaExts and configExts also mark a directory as worth a manifest: a
// directory of Avro or protobuf schemas, Kubernetes or Helm YAML, or
// configuration is a boundary too (a data contract, a deployment).
var schemaExts = map[string]bool{".avsc": true, ".avdl": true, ".proto": true, ".graphql": true, ".gql": true, ".thrift": true, ".prisma": true,
	".hcl": true, ".tfvars": true, ".bicep": true}

var configExts = map[string]bool{".yaml": true, ".yml": true, ".json": true, ".toml": true, ".properties": true, ".ini": true, ".conf": true, ".cfg": true, ".env": true, ".xml": true}

// projectFiles mark a directory as a project of its own (a deployable or a
// package); a top-level directory without one whose children have one is a
// grouping (apps/, services/), and each child gets the manifest instead.
var projectFiles = []string{"package.json", "go.mod", "pyproject.toml", "setup.py", "setup.cfg", "requirements.txt", "Cargo.toml", "pom.xml",
	"build.gradle", "build.gradle.kts", "Dockerfile", "Containerfile", "Gemfile", "composer.json", "mix.exs", "pubspec.yaml", "Chart.yaml",
	"kustomization.yaml", "main.tf", "deno.json", "Package.swift"}

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

// SourceDirs lists the directories that get a container manifest, relative
// to root and slash-separated: each top-level directory holding source,
// schema, infrastructure or configuration files, except that a grouping
// directory (no project file of its own, at least one child with one) is
// replaced by its children that hold such files, and a utility directory
// (utils/, helpers/, common/, core/, lib/, ...) with no entry point is left
// to its parent (issue #64).
func SourceDirs(root string) []string {
	dirs, _ := planDirs(root)
	return dirs
}

// planDirs returns the manifest directories and the utility directories
// that hold content but are not a boundary.
func planDirs(root string) (dirs, utility []string) {
	consider := func(rel string) {
		dir := filepath.Join(root, filepath.FromSlash(rel))
		if !hasContent(dir) {
			return
		}
		if utilityNames[strings.ToLower(path.Base(rel))] && !hasEntryPoint(dir) {
			utility = append(utility, rel)
			return
		}
		dirs = append(dirs, rel)
	}
	for _, n := range subdirs(root) {
		dir := filepath.Join(root, n)
		if !isProject(dir) {
			var kids []string
			grouping := false
			for _, c := range subdirs(dir) {
				if isProject(filepath.Join(dir, c)) {
					grouping = true
				}
				kids = append(kids, n+"/"+c)
			}
			if grouping {
				for _, k := range kids {
					consider(k)
				}
				continue
			}
		}
		consider(n)
	}
	sort.Strings(dirs)
	sort.Strings(utility)
	return dirs, utility
}

// utilityNames are directory names that conventionally hold shared code (a
// utility folder, a package of base classes) rather than a deployable; such a
// directory is a boundary only when it holds an entry point.
var utilityNames = map[string]bool{"util": true, "utils": true, "utilities": true, "helper": true, "helpers": true, "common": true,
	"commons": true, "core": true, "lib": true, "libs": true, "shared": true, "misc": true}

// entryStems are file names (without extension, lower-cased) that start a
// program or serve requests: main.go, __main__.py, Main.java, server.ts,
// wsgi.py, routes.rb, Program.cs.
var entryStems = map[string]bool{"main": true, "__main__": true, "server": true, "wsgi": true, "asgi": true, "routes": true, "program": true}

// buildFiles mark a directory with its own build as a deployable.
var buildFiles = map[string]bool{"Makefile": true, "CMakeLists.txt": true, "BUILD": true, "BUILD.bazel": true, "Procfile": true}

// hasEntryPoint reports whether dir is a project, or holds (outside hidden
// and skipped directories) a project or build file, a Dockerfile, or a source
// file named like an entry point.
func hasEntryPoint(dir string) bool {
	if isProject(dir) {
		return true
	}
	found := false
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || found {
			return filepath.SkipDir
		}
		if d.IsDir() {
			if p != dir && (strings.HasPrefix(d.Name(), ".") || skipDirs[strings.ToLower(d.Name())]) {
				return filepath.SkipDir
			}
			if p != dir && isProject(p) {
				found = true
				return filepath.SkipAll
			}
			return nil
		}
		n := d.Name()
		ext := strings.ToLower(filepath.Ext(n))
		if buildFiles[n] || (sourceExts[ext] && entryStems[strings.ToLower(strings.TrimSuffix(n, filepath.Ext(n)))]) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// subdirs lists the directories directly under dir that are not hidden or
// skipped, sorted.
func subdirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() && !strings.HasPrefix(n, ".") && !skipDirs[strings.ToLower(n)] {
			out = append(out, n)
		}
	}
	return out
}

func isProject(dir string) bool {
	for _, f := range projectFiles {
		if st, err := os.Stat(filepath.Join(dir, f)); err == nil && !st.IsDir() {
			return true
		}
	}
	m, _ := filepath.Glob(filepath.Join(dir, "*.csproj"))
	return len(m) > 0
}

// hasContent reports whether dir holds, at any depth outside hidden and
// skipped directories, a source, schema, infrastructure or config file.
func hasContent(dir string) bool {
	found := false
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || found {
			return filepath.SkipDir
		}
		if d.IsDir() && (strings.HasPrefix(d.Name(), ".") || skipDirs[strings.ToLower(d.Name())]) && p != dir {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			ext := strings.ToLower(filepath.Ext(p))
			if sourceExts[ext] || schemaExts[ext] || configExts[ext] || d.Name() == "Dockerfile" {
				found = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	return found
}

// GitRoot returns the root of the git repository enclosing dir (the nearest
// ancestor holding .git), or "" when there is none.
func GitRoot(dir string) string {
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// vocabularyFiles finds a markdown glossary and an enums file an importer
// can read, at most three directories deep.
func vocabularyFiles(root string) (glossaries, enums []string) {
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if p != root && (strings.HasPrefix(d.Name(), ".") || (skipDirs[strings.ToLower(d.Name())] && !strings.HasPrefix(strings.ToLower(d.Name()), "doc")) || strings.Count(rel, "/") >= 3) {
				return filepath.SkipDir
			}
			return nil
		}
		lower := strings.ToLower(d.Name())
		switch {
		case lower == "glossary.md" || lower == "glossary.markdown":
			glossaries = append(glossaries, rel)
		case lower == "enums.yaml" || lower == "enums.yml":
			enums = append(enums, rel)
		}
		return nil
	})
	return glossaries, enums
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
	own, _ := GuessPrefixes(name)
	core, impl := own, own+"i"
	imports, err := readImports(root, opt.Imports)
	if err != nil {
		return nil, err
	}
	var imported []string
	for _, im := range imports {
		imported = append(imported, im.prefix)
	}
	switch {
	case opt.Core != "":
		core = opt.Core
		if len(imports) == 0 {
			impl = core + "i"
		}
	case len(imports) == 1:
		core = imports[0].prefix // the shared core this repository builds on
	case len(imports) > 1:
		return nil, fmt.Errorf("--import names %d core namespaces (%s); pass --core to say which one this repository's implementation tier realizes", len(imports), strings.Join(imported, ", "))
	}
	if opt.Impl != "" {
		impl = opt.Impl
	}
	if core == impl {
		return nil, fmt.Errorf("core and implementation prefixes must differ (both %q)", core)
	}
	coreImported := false
	for _, im := range imports {
		if im.prefix == impl {
			return nil, fmt.Errorf("--impl %s: is imported from %s", impl, im.dir)
		}
		if im.prefix == core {
			coreImported = true
		}
	}
	if len(imports) > 0 && !coreImported {
		return nil, fmt.Errorf("--core %s: is not one of the imported namespaces (%s)", core, strings.Join(imported, ", "))
	}
	// --iri is an organisation's base, shared by sibling repositories: each
	// namespace takes <base><prefix>/ and the repository <base><name>/, so
	// two siblings never mint the same namespace IRI. Without --iri the base
	// is already per repository.
	base := opt.IRIBase
	repoIRI := ""
	if base == "" {
		base = "https://onto.example.com/" + slug(name) + "/"
		repoIRI = base
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	if repoIRI == "" {
		repoIRI = base + slug(name) + "/"
	}
	version := opt.Version
	if version == "" {
		version = "v0.3.0"
	}
	bprefix := own + "b"
	if len(imports) == 0 {
		bprefix = core + "b"
	}
	if opt.BoundaryPrefix != "" {
		bprefix = strings.TrimSuffix(opt.BoundaryPrefix, ":")
	}
	if bprefix == core || bprefix == impl {
		return nil, fmt.Errorf("the boundary prefix must differ from the namespace prefixes (%q)", bprefix)
	}
	dirs, utility := planDirs(root)
	p := &Plan{Root: root, Name: name, Core: core, Impl: impl, Boundary: bprefix, Dirs: dirs, Utility: utility, Imported: imported,
		Guessed: opt.Core == "" && len(imports) == 0}
	if g := GitRoot(root); g != "" && g != root {
		p.GitRoot = g
	}
	add := func(f File) {
		if f.Action == Create && exists(root, f.Path) {
			f.Action, f.Note = Skip, "exists; left as is"
		}
		p.Files = append(p.Files, f)
	}
	var cfg strings.Builder
	fmt.Fprintf(&cfg, `# glossmark.yaml: the namespaces this repository owns (docs/FILE-FORMATS.md).
grammar: 1
relations: starter           # implements, represents, verifies, documents, produces, consumes, part-of, is-a; full unlocks the rest (docs/ADVANCED.md)
repo-iri: %s                 # boundary prefixes such as %s: expand under it; no prefixes: needed
owns:
`, repoIRI, p.Boundary)
	if !coreImported {
		fmt.Fprintf(&cfg, "  %s: { iri: %s%s/, tier: core,           version: 0.1.0 }   # what the business recognises\n", core, base, core)
	}
	fmt.Fprintf(&cfg, "  %s: { iri: %s%s/, tier: implementation, version: 0.1.0 }   # the mechanisms that realize it\n", impl, base, impl)
	if len(imports) > 0 {
		cfg.WriteString("imports:                     # pinned by gloss lock (glossmark.lock)\n")
		for _, im := range imports {
			ver := ""
			if im.version != "" {
				ver = fmt.Sprintf(" version: %q,", im.version)
			}
			fmt.Fprintf(&cfg, "  %s: { iri: %s,%s source: %s }\n", im.prefix, im.iri, ver, im.source)
		}
	}
	cfg.WriteString(SuggestBlock)
	add(File{Path: "glossmark.yaml", Action: Create, Content: cfg.String()})
	if !coreImported {
		add(File{Path: "ontology/" + core + ".gloss.yaml", Action: Create, Content: fmt.Sprintf(`# ontology/%s.gloss.yaml: the core tier.
# Litmus test: if a domain expert would nod at the term, it is core; if only an
# engineer would, it belongs in %s:. Core concepts never mention %s:.
# Write docs quoted: doc: "One account at an institution (checking, savings)."
namespace: %s
version: 0.1.0
concepts: {}
`, core, impl, impl, core)})
	}
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
	used := map[string]bool{}
	for _, d := range p.Dirs {
		child := slug(path.Base(d))
		if used[child] {
			child = slug(strings.ReplaceAll(d, "/", "-")) // apps/api and services/api
		}
		if p.Boundary+":"+child == rootB {
			child += "-lib" // the package named like the repository (pfinance/pfinance)
		}
		used[child] = true
		add(File{Path: d + "/.gloss.yaml", Action: Create, Content: fmt.Sprintf(`# %s/.gloss.yaml: a container boundary. Fill in realizes: (or realizes: inherit).
boundary: %s:%s
level: container
part-of: %s
default-namespace: %s
`, d, p.Boundary, child, rootB, impl)})
	}
	// .gitignore: the viewer, and the per-machine hook gloss hook install
	// writes when PATH does not find gloss. The graph and the ledger are
	// committed.
	ignore := []string{".glossmark/viewer.html", ".glossmark/views.js", ".glossmark/views.json", ".claude/settings.local.json"}
	gi, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	var missing []string
	for _, l := range ignore {
		if !strings.Contains(string(gi), l) {
			missing = append(missing, l)
		}
	}
	switch {
	case len(missing) == 0:
		add(File{Path: ".gitignore", Action: Skip, Note: "already ignores the viewer and .claude/settings.local.json"})
	case len(gi) == 0:
		add(File{Path: ".gitignore", Action: Create, Content: "# GlossMark: the viewer is generated, settings.local.json is per machine; graph.jsonl and ledger.jsonl are committed\n" + strings.Join(missing, "\n") + "\n"})
	default:
		pre := "\n"
		if strings.HasSuffix(string(gi), "\n") {
			pre = ""
		}
		add(File{Path: ".gitignore", Action: Append, Content: pre + "# GlossMark: the viewer is generated, settings.local.json is per machine; graph.jsonl and ledger.jsonl are committed\n" + strings.Join(missing, "\n") + "\n"})
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
	workflow := fmt.Sprintf(`# GlossMark: lint markers, report drift, and comment the per-view impact on PRs.
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
`, version)
	if p.GitRoot != "" {
		// GitHub reads workflows only at the git root: print the job with
		// root: set instead of writing a file nobody runs.
		rel, _ := filepath.Rel(p.GitRoot, root)
		rel = filepath.ToSlash(rel)
		p.Workflow = strings.Replace(workflow, "fail-on: HARD,", "root: "+rel+", fail-on: HARD,", 1)
		add(File{Path: ".github/workflows/gloss.yml", Action: Skip, Note: "the git root is " + p.GitRoot + "; GitHub runs workflows only there, so add the job printed below to a workflow at the git root"})
	} else {
		add(File{Path: ".github/workflows/gloss.yml", Action: Create, Content: workflow})
	}
	if !coreImported {
		glossaries, enums := vocabularyFiles(root)
		for _, g := range glossaries {
			p.Vocabulary = append(p.Vocabulary, fmt.Sprintf("gloss import --from markdown-glossary %s --namespace %s", g, core))
		}
		for _, e := range enums {
			p.Vocabulary = append(p.Vocabulary, fmt.Sprintf("gloss import --from enums %s --namespace %s --values", e, core))
		}
	}
	return p, nil
}

// String renders the plan, one line per file.
func (p *Plan) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "repository %s: core %s:, implementation %s:, boundaries %s:\n", p.Name, p.Core, p.Impl, p.Boundary)
	if p.Guessed {
		fmt.Fprintf(&sb, "  (prefixes guessed from the name %q; every marker will carry them. Choose your own with --namespace <ns> (implementation <ns>i:, boundaries <ns>b:), --impl and --boundary-prefix; try a choice with --dry-run)\n", p.Name)
	}
	for _, f := range p.Files {
		fmt.Fprintf(&sb, "  %-6s %s", f.Action, f.Path)
		if f.Note != "" {
			fmt.Fprintf(&sb, "  (%s)", f.Note)
		}
		sb.WriteString("\n")
	}
	for _, d := range p.Utility {
		fmt.Fprintf(&sb, "  %-6s %s/  (not a boundary: shared code with no entry point; it stays under its parent's manifest)\n", "-", d)
	}
	if p.Workflow != "" {
		fmt.Fprintf(&sb, "CI job for a workflow at the git root (%s):\n", p.GitRoot)
		for _, l := range strings.Split(strings.TrimRight(p.Workflow, "\n"), "\n") {
			sb.WriteString("    " + l + "\n")
		}
	}
	if len(p.Vocabulary) > 0 {
		sb.WriteString("vocabulary found; after init, import it:\n")
		for _, c := range p.Vocabulary {
			sb.WriteString("    " + c + "\n")
		}
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

// importedNS is one core namespace a sibling workspace owns.
type importedNS struct {
	prefix, iri, version, source, dir string
}

// readImports reads each sibling's glossmark.yaml and returns its core-tier
// namespaces as imports: the IRI, a caret range on the current version and
// a source path relative to root.
func readImports(root string, dirs []string) ([]importedNS, error) {
	var out []importedNS
	seen := map[string]bool{}
	for _, d := range dirs {
		dir := d
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, dir)
		}
		cfg, err := model.LoadConfig(filepath.Join(dir, "glossmark.yaml"))
		if err != nil {
			return nil, fmt.Errorf("--import %s: %w", d, err)
		}
		n := 0
		for _, prefix := range sortedKeys(cfg.Owns) {
			ns := cfg.Owns[prefix]
			if ns.Tier != "core" {
				continue
			}
			if seen[prefix] {
				return nil, fmt.Errorf("--import %s: namespace %s: is imported twice", d, prefix)
			}
			seen[prefix] = true
			n++
			file := ns.File
			if file == "" {
				file = "ontology/" + prefix + ".gloss.yaml"
			}
			src, err := filepath.Rel(root, filepath.Join(dir, filepath.FromSlash(file)))
			if err != nil {
				src = filepath.Join(dir, file)
			}
			rng := ""
			if ns.Version != "" {
				rng = "^" + ns.Version
			}
			out = append(out, importedNS{prefix: prefix, iri: ns.IRI, version: rng, source: filepath.ToSlash(src), dir: d})
		}
		if n == 0 {
			return nil, fmt.Errorf("--import %s: owns no core namespace", d)
		}
	}
	return out, nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
