// @gloss implements gmi:SpecInference scope=file

package infer

import (
	"bufio"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// What the built-in sources share: reading a spec as a YAML tree with
// line numbers, the shape of a YAML block, and the routes and literal
// bindings code declares.

// kindOf reports whether the first lines of a YAML or JSON file declare
// an AsyncAPI or OpenAPI document.
func kindOf(lines []string) string {
	for i, l := range lines {
		if i > 30 {
			break
		}
		t := strings.TrimSpace(strings.Trim(strings.TrimSpace(l), `{,`))
		switch {
		case strings.HasPrefix(t, "asyncapi:"), strings.HasPrefix(t, `"asyncapi"`):
			return "asyncapi"
		case strings.HasPrefix(t, "openapi:"), strings.HasPrefix(t, `"openapi"`), strings.HasPrefix(t, "swagger:"), strings.HasPrefix(t, `"swagger"`):
			return "openapi"
		}
	}
	return ""
}

// readSpec reads a YAML or JSON file as lines and a YAML tree, or nil
// when it is not a document of the given kind.
func readSpec(path, kind string) ([]string, *yaml.Node) {
	lines, err := readLines(path)
	if err != nil || kindOf(lines) != kind {
		return nil, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(strings.Join(lines, "\n")), &doc); err != nil || len(doc.Content) == 0 {
		return nil, nil
	}
	return lines, doc.Content[0]
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out, sc.Err()
}

func mapValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// scalar is a node's value, or "" for nil.
func scalar(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	return n.Value
}

func indentOf(lines []string, line int) string {
	if line < 1 || line > len(lines) {
		return ""
	}
	l := lines[line-1]
	return l[:len(l)-len(strings.TrimLeft(l, " \t"))]
}

// blockEnd is the last line of the YAML block that starts at line
// (1-based): before the next non-blank line indented no deeper.
func blockEnd(lines []string, line int) int {
	if line < 1 || line > len(lines) {
		return line
	}
	ind := len(indentOf(lines, line))
	end := line
	for k := line; k < len(lines); k++ {
		t := strings.TrimSpace(lines[k])
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if len(indentOf(lines, k+1)) <= ind {
			break
		}
		end = k + 1
	}
	return end
}

// item builds the Item for the key at line.
func item(lines []string, line int, symbol, role, relation, doc string, t Target) Item {
	return Item{Symbol: symbol, Role: role, Relation: relation, Line: line, End: blockEnd(lines, line), Indent: indentOf(lines, line), Doc: strings.TrimSpace(doc), Target: t}
}

// LastPart is the last dot-, colon- or slash-separated part of a name.
func LastPart(sym string) string {
	if i := strings.LastIndexAny(sym, ".:/"); i >= 0 {
		return sym[i+1:]
	}
	return sym
}

var versionSuffix = regexp.MustCompile(`[.\-_/]?v\d+$`)

// StripVersion drops a trailing version from a topic or path segment
// (claims.filed.v1: claims.filed).
func StripVersion(s string) string { return versionSuffix.ReplaceAllString(s, "") }

var (
	ident  = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	quoted = regexp.MustCompile(`["']([A-Za-z][\w.\-/]*)["']`)
	// assignment: an identifier bound to a literal, a list or tuple of
	// literals, or a bare word (YAML, properties), in any of the languages
	// and config formats a service is written in (filed_topic: str =
	// "claims.filed.v1", const val FILED_TOPIC = "...", filedTopic =
	// "...", topic: payouts.v1). An expression or a keyword argument is
	// not a binding.
	assignment = regexp.MustCompile(`^\s*(?:(?:export|const|val|var|let|final|static|public|private|protected|readonly)\s+)*([A-Za-z_][\w.]*?)\s*(?::\s*[A-Za-z_][\w\[\], ]*)?\s*[:=]\s*((?:[\[(]\s*)?["'][A-Za-z][\w.\-/]*["'](?:\s*,\s*["'][A-Za-z][\w.\-/]*["'])*(?:\s*[\])])?|[A-Za-z][\w.\-]*)\s*;?\s*$`)
)

// Binding is an identifier a line binds to literal values.
type Binding struct {
	Ident  string
	Values []string
}

// ParseBinding reads a line as an identifier bound to literals.
func ParseBinding(line string) (Binding, bool) {
	m := assignment.FindStringSubmatch(line)
	if m == nil {
		return Binding{}, false
	}
	b := Binding{Ident: LastPart(m[1]), Values: []string{m[2]}}
	if q := quoted.FindAllStringSubmatch(m[2], -1); len(q) > 0 {
		b.Values = nil
		for _, v := range q {
			b.Values = append(b.Values, v[1])
		}
	}
	return b, true
}

// configExts are the configuration files read for bindings, on top of
// the code files.
var configExts = map[string]bool{".conf": true, ".properties": true, ".toml": true, ".ini": true, ".env": true, ".json": true, ".cfg": true, ".yaml": true, ".yml": true}

// codeExts are the source files a binding or a declaration can sit in.
var codeExts = map[string]bool{".py": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".go": true, ".rs": true,
	".java": true, ".kt": true, ".kts": true, ".rb": true, ".cs": true, ".php": true, ".swift": true, ".scala": true, ".tf": true, ".sh": true}

var (
	// routeCall: a framework route declaration, in a decorator or a call:
	// @app.route("/p", methods=["POST"]), @router.get("/p"), @GetMapping("/p"),
	// @PostMapping(value = "/p"), @Get('/p'), app.post('/p', ...), r.HandleFunc("/p", ...),
	// post("/p") { ... } (Ktor).
	routeCall    = regexp.MustCompile(`(?i)(?:@(?:\w+\.)?(route|get|post|put|patch|delete|GetMapping|PostMapping|PutMapping|PatchMapping|DeleteMapping|RequestMapping)|\b(?:\w+\.)?(get|post|put|patch|delete|route|HandleFunc|Handle|Get|Post|Put|Patch|Delete))\s*\(\s*(?:value\s*=\s*|path\s*=\s*)?["'](/[^"']*)["']`)
	routeMethods = regexp.MustCompile(`(?i)methods\s*=\s*[\[(]\s*["'](\w+)["']`)
	pathParam    = regexp.MustCompile(`\{[^}]*\}|:[A-Za-z_]\w*|<[^>]*>`)
)

// Route is one HTTP route a declaration declares. Method is lower-case,
// or "" when the declaration does not say.
type Route struct {
	Method, Path string
}

// Routes finds the HTTP routes in a declaration's decorators and body.
func Routes(decorators []string, body string) []Route {
	var out []Route
	seen := map[Route]bool{}
	for _, ln := range append(append([]string{}, decorators...), strings.Split(body, "\n")...) {
		m := routeCall.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		method := strings.ToLower(m[1] + m[2])
		switch {
		case strings.HasSuffix(method, "mapping"):
			method = strings.TrimSuffix(method, "mapping")
			if method == "request" {
				method = ""
			}
		case method == "route", method == "handlefunc", method == "handle":
			method = ""
		}
		if mm := routeMethods.FindStringSubmatch(ln); mm != nil {
			method = strings.ToLower(mm[1])
		}
		r := Route{method, m[3]}
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}

// NormPath writes every path parameter as {} and drops the trailing slash.
func NormPath(p string) string {
	p = pathParam.ReplaceAllString(p, "{}")
	return strings.TrimSuffix(p, "/")
}

// PathWords are the literal segments of a path, parameters and version
// prefixes dropped, as one name.
func PathWords(p string) string {
	var out []string
	for _, seg := range strings.Split(NormPath(p), "/") {
		if seg == "" || seg == "{}" || versionSuffix.MatchString("/"+seg) || seg == "api" {
			continue
		}
		out = append(out, seg)
	}
	return strings.Join(out, " ")
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
