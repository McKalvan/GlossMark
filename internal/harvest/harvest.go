// @gloss implements gmi:EvidenceHarvester scope=file

package harvest

import (
	"bytes"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/importers"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/scaffold"
)

// Options controls a harvest.
type Options struct {
	// Exclude lists globs (repository-relative) to leave out, on top of the
	// excludes of a glossmark.yaml at the root, if there is one. Excluding a
	// vocabulary the owner keeps is how a derivation is evaluated against it.
	Exclude []string
	// Units are the application directories; nil means the deployables
	// gloss init would plan (ADR 0016, decision 1).
	Units []string
	// Git adds rename evidence from the history.
	Git bool
}

// Harvest walks root and returns its evidence, sorted by path, line and signal.
func Harvest(root string, opt Options) ([]Evidence, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	exclude := append([]string{}, opt.Exclude...)
	if cfg, err := model.LoadConfig(filepath.Join(root, load.ConfigName)); err == nil {
		exclude = append(exclude, cfg.Exclude...)
	}
	units := opt.Units
	if units == nil {
		units = scaffold.SourceDirs(root)
	}
	h := &harvester{units: units}
	modules := map[string]bool{}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			n := d.Name()
			if load.SkipDir(n) || strings.HasPrefix(n, ".") || buildDirs[n] || model.MatchAnyGlob(exclude, rel) || model.MatchAnyGlob(exclude, rel+"/") {
				return filepath.SkipDir
			}
			return nil
		}
		if model.MatchAnyGlob(exclude, rel) || strings.HasPrefix(rel, load.StateDir+"/") {
			return nil
		}
		style, ok := marker.StyleFor(rel, nil)
		if !ok {
			return nil
		}
		if info, err := d.Info(); err != nil || info.Size() > load.MaxFileSize {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil || bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0 {
			return nil
		}
		test := model.MatchAnyGlob(load.DefaultTestPaths, rel)
		if !test && codeExt[strings.ToLower(path.Ext(rel))] {
			modules[path.Dir(rel)] = true
			h.module(rel, path.Base(rel))
		}
		h.file(rel, strings.Split(string(b), "\n"), style, test)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for dir := range modules {
		if dir != "." {
			h.module(dir, path.Base(dir))
		}
	}
	if opt.Git {
		h.out = append(h.out, gitRenames(root, h.unitFor, exclude)...)
	}
	sortEvidence(h.out)
	return h.out, nil
}

func sortEvidence(ev []Evidence) {
	sort.SliceStable(ev, func(i, j int) bool {
		a, b := ev[i].Source, ev[j].Source
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if ev[i].Signal != ev[j].Signal {
			return ev[i].Signal < ev[j].Signal
		}
		return ev[i].Surface < ev[j].Surface
	})
}

// buildDirs hold build output, not source.
var buildDirs = map[string]bool{"dist": true, "coverage": true, "target": true, "vendor": true}

// codeExt are the source languages whose files and directories name modules.
var codeExt = map[string]bool{".py": true, ".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".java": true, ".kt": true, ".cs": true, ".rs": true, ".rb": true, ".php": true, ".scala": true, ".swift": true}

type harvester struct {
	units []string
	out   []Evidence
}

func (h *harvester) unitFor(rel string) string {
	best := ""
	for _, u := range h.units {
		if (rel == u || strings.HasPrefix(rel, u+"/")) && len(u) > len(best) {
			best = u
		}
	}
	return best
}

func (h *harvester) add(signal, surface, rel string, line int, context ...string) {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return
	}
	h.out = append(h.out, newEvidence(h.unitFor(rel), signal, surface, rel, line, context...))
}

// genericModules name a layer or a convention, not a domain.
var genericModules = map[string]bool{"src": true, "lib": true, "libs": true, "app": true, "apps": true, "pkg": true, "internal": true, "cmd": true, "components": true,
	"pages": true, "utils": true, "util": true, "helpers": true, "common": true, "shared": true, "core": true, "scripts": true, "tools": true, "backend": true, "frontend": true,
	"index": true, "main": true, "__init__": true, "__main__": true, "init": true, "types": true, "constants": true, "config": true, "settings": true, "conftest": true,
	"setup": true, "server": true, "models": true, "schemas": true, "api": true, "routes": true, "views": true, "hooks": true, "styles": true, "assets": true, "public": true,
	"static": true, "templates": true, "migrations": true, "versions": true, "fixtures": true, "testdata": true, "mocks": true, "base": true, "errors": true, "db": true}

// module records a source file's stem or a source directory's name.
func (h *harvester) module(rel, name string) {
	name = strings.TrimSuffix(name, path.Ext(name))
	if genericModules[strings.ToLower(name)] || strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "_test") || strings.HasPrefix(name, "_") {
		return
	}
	h.out = append(h.out, newEvidence(h.unitFor(rel), "module", name, rel, 0))
}

func (h *harvester) file(rel string, lines []string, style marker.CommentStyle, test bool) {
	ext := strings.ToLower(path.Ext(rel))
	base := strings.ToLower(path.Base(rel))
	switch {
	case ext == ".md" || ext == ".mdx":
		if strings.Contains(base, "glossary") {
			for _, e := range importers.ParseGlossary(strings.Join(lines, "\n")) {
				h.add("glossary-entry", e.Name, rel, e.Line, aliasContext(e.Aliases)...)
			}
			return
		}
		h.markdown(rel, lines)
		return
	case (ext == ".yaml" || ext == ".yml") && (strings.Contains(base, "backlog") || strings.Contains(base, "roadmap") || strings.Contains(base, "issues")):
		// An issue tracker kept in the repository.
		for i, ln := range lines {
			if m := yamlTitle.FindStringSubmatch(ln); m != nil {
				h.add("issue-title", strings.Trim(m[1], `"'`), rel, i+1)
			}
		}
		return
	case (ext == ".yaml" || ext == ".yml") && strings.Contains(base, "enum"):
		if entries, err := importers.ParseEnums([]byte(strings.Join(lines, "\n")), true); err == nil {
			for _, e := range entries {
				h.add("glossary-entry", e.Name, rel, e.Line, aliasContext(e.Aliases)...)
			}
		}
		return
	case ext == ".sql" || ext == ".ddl" || ext == ".psql":
		h.sql(rel, lines)
		return
	case ext == ".feature":
		for _, d := range marker.Declarations(rel, lines, style) {
			if i := strings.Index(d.Symbol, ":"); i >= 0 {
				h.add("test-name", d.Symbol[i+1:], rel, d.Line)
			}
		}
		return
	case !codeExt[ext] && ext != ".proto":
		return
	}
	for _, d := range marker.Declarations(rel, lines, style) {
		h.decl(rel, lines, d, test)
	}
	h.lineSignals(rel, lines, test, ext)
}

func aliasContext(aliases []string) []string {
	var out []string
	for _, a := range aliases {
		out = append(out, "alias:"+a)
	}
	return out
}

var (
	pyParents   = regexp.MustCompile(`^\s*class\s+\w+\s*\(([^)]*)\)`)
	tsEnumStart = regexp.MustCompile(`\benum\s+\w+`)
	memberLine  = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*(?:=|,|$|\()`)
	tableName   = regexp.MustCompile(`^\s*__tablename__\s*=\s*["']([^"']+)["']`)
	foreignKey  = regexp.MustCompile(`ForeignKey\(\s*["']([\w.]+)["']`)
	ormColumn   = regexp.MustCompile(`^\s+([a-z_]\w*)\s*(?::\s*Mapped\[[^\]]*\]+)?\s*=\s*(?:mapped_column|Column|sa\.Column|Field)\(`)
	literalType = regexp.MustCompile(`^\s*(\w+)\s*(?::\s*[\w\[\], ]+)?\s*=\s*(?:typing\.)?Literal\[(.*)\]\s*$`)
	tsUnion     = regexp.MustCompile(`^\s*(?:export\s+)?type\s+(\w+)\s*=\s*((?:\s*\|?\s*["'][^"']+["'])+)\s*;?\s*$`)
	quoted      = regexp.MustCompile(`["']([^"']+)["']`)
	fieldLine   = regexp.MustCompile(`^\s*(?:readonly\s+)?([A-Za-z_]\w*)\??\s*:\s*[^\s=(]`)
	goField     = regexp.MustCompile(`^\s*([A-Z]\w*)\s+[*\[\]\w.]+`)
	yamlTitle   = regexp.MustCompile(`^\s*-?\s*title:\s*(.+?)\s*$`)
)

// frameworkParents are base classes that say how a type is built, not what it is.
var frameworkParents = map[string]bool{"object": true, "BaseModel": true, "Base": true, "Enum": true, "StrEnum": true, "IntEnum": true, "str": true, "int": true,
	"Protocol": true, "ABC": true, "Exception": true, "TypedDict": true, "NamedTuple": true, "Generic": true, "SQLModel": true, "DeclarativeBase": true, "TestCase": true,
	"BaseSettings": true, "RuntimeError": true, "ValueError": true, "Error": true}

// typeKinds are the declaration kinds that name a type.
var typeKinds = map[string]bool{"class": true, "interface": true, "type": true, "enum": true, "proto": true, "definition": true}

func (h *harvester) decl(rel string, lines []string, d marker.Decl, test bool) {
	if test || strings.HasPrefix(d.Symbol, "_") {
		return
	}
	sig := ""
	if d.Line-1 < len(lines) {
		sig = lines[d.Line-1]
	}
	switch {
	case typeKinds[d.Kind]:
		h.typeDecl(rel, lines, d, sig)
	case d.Kind == "function" || d.Kind == "method":
		name := d.Symbol
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[i+1:]
		}
		if strings.HasPrefix(name, "_") || strings.HasPrefix(name, "test") || strings.HasPrefix(name, "Test") {
			return
		}
		var ctx []string
		if d.Parent != "" {
			ctx = append(ctx, "parent:"+d.Parent)
		}
		h.add("function-decl", name, rel, d.Line, ctx...)
	case d.Kind == "variable" || d.Kind == "type":
		h.literal(rel, d.Line, sig)
	}
}

func (h *harvester) typeDecl(rel string, lines []string, d marker.Decl, sig string) {
	t := strings.TrimSpace(sig)
	if d.Kind == "proto" {
		switch {
		case strings.HasPrefix(t, "message"):
			h.add("schema-type", d.Symbol, rel, d.Line)
		case strings.HasPrefix(t, "enum"):
			h.add("enum-type", d.Symbol, rel, d.Line)
			h.members(rel, lines, d)
		case strings.HasPrefix(t, "rpc"):
			h.add("function-decl", d.Symbol, rel, d.Line)
		default:
			h.add("type-decl", d.Symbol, rel, d.Line)
		}
		return
	}
	if d.Kind == "type" && h.literal(rel, d.Line, sig) {
		return
	}
	var parents []string
	if m := pyParents.FindStringSubmatch(sig); m != nil {
		for _, p := range strings.Split(m[1], ",") {
			p = strings.TrimSpace(p)
			if i := strings.IndexAny(p, "[("); i >= 0 {
				p = p[:i]
			}
			if p != "" && !strings.Contains(p, "=") {
				parents = append(parents, p[strings.LastIndex(p, ".")+1:])
			}
		}
	}
	enum := tsEnumStart.MatchString(sig) && d.Kind != "class"
	for _, p := range parents {
		if strings.HasSuffix(p, "Enum") {
			enum = true
		}
	}
	if enum {
		h.add("enum-type", d.Symbol, rel, d.Line)
		h.members(rel, lines, d)
		return
	}
	var ctx []string
	for _, p := range parents {
		ctx = append(ctx, "is-a:"+p)
	}
	h.add("type-decl", d.Symbol, rel, d.Line, ctx...)
	for _, p := range parents {
		if !frameworkParents[p] {
			h.add("inherits", d.Symbol, rel, d.Line, "is-a:"+p)
		}
	}
	// An ORM model: its table, columns and foreign keys are data evidence.
	table := ""
	for k := d.Line; k < d.End && k < len(lines); k++ {
		if m := tableName.FindStringSubmatch(lines[k]); m != nil {
			table = m[1]
			h.add("table", table, rel, k+1, "class:"+d.Symbol)
		}
	}
	// Fields: an ORM column is data, any other attribute is code. Only lines
	// at the block's member indentation count, so a method's locals are not
	// fields.
	member := -1
	for k := d.Line; k < d.End && k < len(lines); k++ {
		t := strings.TrimSpace(lines[k])
		if t == "" || t == "{" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "//") || strings.HasPrefix(t, `"""`) || strings.HasPrefix(t, "@") {
			continue
		}
		ind := len(lines[k]) - len(strings.TrimLeft(lines[k], " \t"))
		if member < 0 {
			member = ind
		}
		if ind != member {
			continue
		}
		if table != "" {
			if m := ormColumn.FindStringSubmatch(lines[k]); m != nil {
				h.add("column", m[1], rel, k+1, "table:"+table)
				continue
			}
		}
		re := fieldLine
		if strings.HasSuffix(rel, ".go") {
			re = goField
		}
		if m := re.FindStringSubmatch(lines[k]); m != nil && !strings.HasPrefix(m[1], "_") {
			h.add("field", m[1], rel, k+1, "of:"+d.Symbol)
		}
	}
	for k := d.Line; table != "" && k < d.End && k < len(lines); k++ {
		for _, m := range foreignKey.FindAllStringSubmatch(lines[k], -1) {
			h.add("foreign-key", table, rel, k+1, "fk:"+strings.SplitN(m[1], ".", 2)[0])
		}
	}
}

// members records the values of an enum declared as a block.
func (h *harvester) members(rel string, lines []string, d marker.Decl) {
	for k := d.Line; k < d.End && k < len(lines); k++ {
		t := strings.TrimSpace(lines[k])
		if t == "" || t == "{" || t == "}" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "//") || strings.HasPrefix(t, `"""`) {
			continue
		}
		if m := memberLine.FindStringSubmatch(lines[k]); m != nil && m[1] != "def" && m[1] != "option" {
			h.add("enum-value", m[1], rel, k+1, "enum:"+d.Symbol)
		}
	}
}

// literal records a closed set written as a type alias: Python
// Literal["a", "b"] or a TypeScript union of string literals.
func (h *harvester) literal(rel string, line int, sig string) bool {
	name, body := "", ""
	if m := literalType.FindStringSubmatch(sig); m != nil {
		name, body = m[1], m[2]
	} else if m := tsUnion.FindStringSubmatch(sig); m != nil {
		name, body = m[1], m[2]
	} else {
		return false
	}
	h.add("enum-type", name, rel, line)
	for _, v := range quoted.FindAllStringSubmatch(body, -1) {
		h.add("enum-value", v[1], rel, line, "enum:"+name)
	}
	return true
}

var (
	pyTest     = regexp.MustCompile(`^\s*(?:async\s+)?def\s+test_(\w+)`)
	goTest     = regexp.MustCompile(`^func\s+Test(\w+)`)
	jsTest     = regexp.MustCompile(`\b(?:it|test|describe)\(\s*(['"` + "`" + `])(.{4,160}?)['"` + "`" + `]`)
	pyRoute    = regexp.MustCompile(`@\w+\.(?:get|post|put|patch|delete|websocket|api_route)\(\s*f?["']([^"']*)["']`)
	pyPrefix   = regexp.MustCompile(`APIRouter\([^)]*prefix\s*=\s*["']([^"']+)["']`)
	jsRoute    = regexp.MustCompile(`\b(?:app|router|server)\.(?:get|post|put|patch|delete)\(\s*['"` + "`" + `](/[^'"` + "`" + `]*)`)
	uiRoute    = regexp.MustCompile(`\bpath\s*[:=]\s*\{?\s*["'](/[^"']*)["']`)
	goRoute    = regexp.MustCompile(`(?:HandleFunc|Handle|\.(?:GET|POST|PUT|PATCH|DELETE))\(\s*"([^"]+)"`)
	inlineLit  = regexp.MustCompile(`\b(\w+)\s*:\s*(?:typing\.)?Literal\[([^\]]+)\]`)
	pyGuard    = regexp.MustCompile(`\braise\s+\w+\(\s*(?:status_code\s*=\s*)?(?:\d+\s*,\s*)?(?:detail\s*=\s*)?f?["']([^"']{6,160})["']`)
	httpDetail = regexp.MustCompile(`HTTPException\([^)]*detail\s*=\s*f?["']([^"']{6,160})["']`)
	jsGuard    = regexp.MustCompile(`\bthrow\s+new\s+\w*\(\s*['"` + "`" + `]([^'"` + "`" + `]{6,160})['"` + "`" + `]`)
	goGuard    = regexp.MustCompile(`(?:errors\.New|fmt\.Errorf)\(\s*"([^"%]{6,160})`)
	jsxText    = regexp.MustCompile(`>\s*([A-Z][A-Za-z'’ ,.!?-]{1,80}?)\s*<`)
	jsxAttr    = regexp.MustCompile(`\b(?:title|label|placeholder|aria-label|alt)=["']([^"'{}]{2,80})["']`)
	sqlRename  = regexp.MustCompile(`(?i)\brename\s+(?:column\s+)?(\w+)\s+to\s+(\w+)`)
	sqlRenameT = regexp.MustCompile(`(?i)\balter\s+table\s+(?:if\s+exists\s+)?([\w."]+)\s+rename\s+to\s+(\w+)`)
	opRename   = regexp.MustCompile(`op\.rename_table\(\s*["'](\w+)["']\s*,\s*["'](\w+)["']`)
	opCreate   = regexp.MustCompile(`op\.create_table\(\s*["'](\w+)["']`)
)

var uiExt = map[string]bool{".tsx": true, ".jsx": true, ".vue": true, ".svelte": true, ".html": true}

// lineSignals are the signals read line by line rather than from declarations.
func (h *harvester) lineSignals(rel string, lines []string, test bool, ext string) {
	for i, ln := range lines {
		n := i + 1
		if test {
			if m := pyTest.FindStringSubmatch(ln); m != nil {
				h.add("test-name", strings.ReplaceAll(m[1], "_", " "), rel, n)
			} else if m := goTest.FindStringSubmatch(ln); m != nil {
				h.add("test-name", m[1], rel, n)
			} else if m := jsTest.FindStringSubmatch(ln); m != nil {
				h.add("test-name", m[2], rel, n)
			}
			continue
		}
		for _, re := range []*regexp.Regexp{pyRoute, pyPrefix, jsRoute, uiRoute, goRoute} {
			if m := re.FindStringSubmatch(ln); m != nil && m[1] != "" && m[1] != "/" {
				h.add("route", m[1], rel, n)
				break
			}
		}
		if ext == ".py" && literalType.FindStringSubmatch(ln) == nil {
			for _, m := range inlineLit.FindAllStringSubmatch(ln, -1) {
				for _, v := range quoted.FindAllStringSubmatch(m[2], -1) {
					h.add("enum-value", v[1], rel, n, "enum:"+m[1])
				}
			}
		}
		for _, re := range []*regexp.Regexp{pyGuard, httpDetail, jsGuard, goGuard} {
			if m := re.FindStringSubmatch(ln); m != nil {
				h.add("guard-message", m[1], rel, n)
				break
			}
		}
		if m := opRename.FindStringSubmatch(ln); m != nil {
			h.add("rename", m[1], rel, n, "renamed-to:"+m[2])
		}
		if m := opCreate.FindStringSubmatch(ln); m != nil {
			h.add("table", m[1], rel, n)
		}
		if uiExt[ext] {
			for _, m := range jsxText.FindAllStringSubmatch(ln, -1) {
				h.add("ui-string", m[1], rel, n)
			}
			for _, m := range jsxAttr.FindAllStringSubmatch(ln, -1) {
				h.add("ui-string", m[1], rel, n)
			}
		}
	}
}

var (
	sqlCreate = regexp.MustCompile(`(?i)^\s*create\s+(?:temp(?:orary)?\s+)?table\s+(?:if\s+not\s+exists\s+)?([\w."]+)`)
	sqlColumn = regexp.MustCompile(`^\s*"?([a-z_]\w*)"?\s+[A-Za-z]`)
	sqlRef    = regexp.MustCompile(`(?i)\breferences\s+([\w."]+)`)
	sqlNotCol = map[string]bool{"primary": true, "foreign": true, "constraint": true, "unique": true, "check": true, "key": true, "index": true}
)

func (h *harvester) sql(rel string, lines []string) {
	table := ""
	for i, ln := range lines {
		n := i + 1
		if m := sqlCreate.FindStringSubmatch(ln); m != nil {
			table = unquote(m[1])
			h.add("table", table, rel, n)
			continue
		}
		if m := sqlRenameT.FindStringSubmatch(ln); m != nil {
			h.add("rename", unquote(m[1]), rel, n, "renamed-to:"+m[2])
		} else if m := sqlRename.FindStringSubmatch(ln); m != nil {
			h.add("rename", m[1], rel, n, "renamed-to:"+m[2])
		}
		if table == "" {
			continue
		}
		if strings.Contains(ln, ");") || strings.HasPrefix(strings.TrimSpace(ln), ")") {
			table = ""
			continue
		}
		if m := sqlColumn.FindStringSubmatch(ln); m != nil && !sqlNotCol[strings.ToLower(m[1])] {
			h.add("column", m[1], rel, n, "table:"+table)
		}
		if m := sqlRef.FindStringSubmatch(ln); m != nil {
			h.add("foreign-key", table, rel, n, "fk:"+unquote(m[1]))
		}
	}
}

func unquote(s string) string {
	s = strings.ReplaceAll(s, `"`, "")
	return s[strings.LastIndex(s, ".")+1:]
}

var (
	mdHeading   = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*#*$`)
	mdBold      = regexp.MustCompile(`\*\*([A-Za-z][^*]{1,50}?)\*\*`)
	mdCode      = regexp.MustCompile("`[^`]*`")
	mdLink      = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdNumbering = regexp.MustCompile(`^(?:[A-Z]?\d+(?:\.\d+)*[.)]?|[A-Z][.)])\s+`)
	notTerm     = map[string]bool{"note": true, "warning": true, "example": true, "why": true, "tip": true, "important": true, "todo": true, "def.": true, "aliases": true, "status": true}
)

// markdown reads headings, bold terms (a definition list, a table's row
// labels) and the phrases of prose, outside fenced code.
func (h *harvester) markdown(rel string, lines []string) {
	inFence := false
	seen := map[string]bool{}
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := mdHeading.FindStringSubmatch(t); m != nil {
			if s := cleanMarkdown(m[1]); s != "" && len(strings.Fields(s)) <= 10 {
				h.add("doc-heading", s, rel, i+1)
			}
			continue
		}
		for _, m := range mdBold.FindAllStringSubmatch(ln, -1) {
			if term := cleanMarkdown(m[1]); !notTerm[strings.ToLower(term)] && len(strings.Fields(term)) <= 4 {
				h.add("doc-definition", term, rel, i+1)
			}
		}
		for _, p := range phrases(cleanMarkdown(mdCode.ReplaceAllString(ln, " "))) {
			if !seen[p] {
				seen[p] = true
				h.add("doc-phrase", p, rel, i+1)
			}
		}
	}
}

// phrases returns the two- and three-word runs of a line of prose that
// could name a concept: letters only, no stop word at either end. The
// clustering decides which of them recur enough to propose a concept.
func phrases(s string) []string {
	var out []string
	for _, chunk := range proseBreak.Split(strings.ToLower(s), -1) {
		ws := strings.Fields(chunk)
		for n := 2; n <= 3; n++ {
			for i := 0; i+n <= len(ws); i++ {
				run := ws[i : i+n]
				ok := !proseStop[run[0]] && !proseStop[run[n-1]]
				for _, w := range run {
					ok = ok && proseWord.MatchString(w)
				}
				if ok {
					out = append(out, strings.Join(run, " "))
				}
			}
		}
	}
	return out
}

var (
	proseBreak = regexp.MustCompile(`[^a-z\- ]+`)
	proseWord  = regexp.MustCompile(`^[a-z][a-z-]+$`)
	proseStop  = map[string]bool{}
)

func init() {
	for _, w := range strings.Fields(`a an the and or but if then else of to in on at by for from with without as is are was were be been being it its this that these those
		there here which who whom whose what when where why how not no nor so than too very can could may might must shall should will would do does did done has have had
		we you they he she i our your their his her my me us them one two three all any each every some such only own same other more most less least just also into onto over
		under about after before between through during per via up down out off again once both either neither new see use used using get gets set make makes eg ie etc`) {
		proseStop[w] = true
	}
}

func cleanMarkdown(s string) string {
	s = mdLink.ReplaceAllString(s, "$1")
	s = strings.NewReplacer("`", "", "**", "", "*", "", "_", " ").Replace(s)
	s = mdNumbering.ReplaceAllString(strings.TrimSpace(s), "")
	return strings.TrimRight(strings.TrimSpace(s), ":")
}
