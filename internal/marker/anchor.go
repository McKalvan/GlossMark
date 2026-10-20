// @gloss implements gmi:DeclarationAnchor scope=file

package marker

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"strings"
)

// Anchor is where a marker binds in a file, stored three ways in the design:
// path plus line range, a symbol string (SCIP-shaped when an indexer exists;
// a declaration-detector name until then), and a content fingerprint of the
// anchored signature.
type Anchor struct {
	Scope       string `json:"scope"`                 // file | symbol | block | line | range
	Symbol      string `json:"symbol,omitempty"`      // declaration name, e.g. breed_stallion, resource.aws_sqs_queue.capture
	Kind        string `json:"kind,omitempty"`        // function, class, resource, table, feature, document, heading, ...
	StartLine   int    `json:"start"`                 // first line of the anchored region (1-based)
	EndLine     int    `json:"end"`                   // last line (inclusive)
	Signature   string `json:"signature,omitempty"`   // normalized signature line
	Fingerprint string `json:"fingerprint,omitempty"` // sha256: prefix of signature (symbol/block) or of the whole file (file)
}

type declRule struct {
	re   *regexp.Regexp
	kind string
	// sym builds the symbol from the submatches; nil → first non-empty group.
	sym func(m []string) string
}

func first(m []string) string {
	for _, g := range m[1:] {
		if g != "" {
			return g
		}
	}
	return ""
}

var (
	pyRules = []declRule{
		{regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_]\w*)`), "function", nil},
		{regexp.MustCompile(`^\s*class\s+([A-Za-z_]\w*)`), "class", nil},
		{regexp.MustCompile(`^([A-Za-z_]\w*)\s*(?::[^=]+)?=`), "variable", nil},
	}
	tsRules = []declRule{
		{regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:declare\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][\w$]*)`), "class", nil},
		{regexp.MustCompile(`^\s*(?:export\s+)?(?:declare\s+)?interface\s+([A-Za-z_$][\w$]*)`), "interface", nil},
		{regexp.MustCompile(`^\s*(?:export\s+)?(?:declare\s+)?type\s+([A-Za-z_$][\w$]*)`), "type", nil},
		{regexp.MustCompile(`^\s*(?:export\s+)?(?:declare\s+)?(?:const\s+)?enum\s+([A-Za-z_$][\w$]*)`), "enum", nil},
		{regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s*\*?\s*([A-Za-z_$][\w$]*)`), "function", nil},
		{regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)`), "variable", nil},
		{regexp.MustCompile(`^\s*(?:public|private|protected|static|async|readonly|\s)*\s*([A-Za-z_$][\w$]*)\s*\([^)]*\)\s*(?::\s*[^{]+)?\{`), "method", nil},
	}
	// ktRules come before the TS and C rules for Kotlin and Scala, where
	// "const val NAME" would otherwise read as the TS "const val".
	ktRules = []declRule{
		{regexp.MustCompile(`^\s*(?:(?:private|public|internal|protected|override|open|final|abstract|lateinit|const|lazy|implicit|inline)\s+)*(?:val|var)\s+([A-Za-z_]\w*)`), "variable", nil},
	}
	goRules = []declRule{
		{regexp.MustCompile(`^func\s+\(\s*\w+\s+\*?([A-Za-z_]\w*)\s*\)\s*([A-Za-z_]\w*)`), "method", func(m []string) string { return m[1] + "." + m[2] }},
		{regexp.MustCompile(`^func\s+([A-Za-z_]\w*)`), "function", nil},
		{regexp.MustCompile(`^type\s+([A-Za-z_]\w*)`), "type", nil},
		{regexp.MustCompile(`^package\s+([A-Za-z_]\w*)`), "package", nil},
		{regexp.MustCompile(`^(?:var|const)\s+([A-Za-z_]\w*)`), "variable", nil},
		{regexp.MustCompile(`^(?:var|const)\s*\(`), "variable", func(m []string) string { return "" }},
	}
	hclRules = []declRule{
		{regexp.MustCompile(`^\s*(resource|data)\s+"([^"]+)"\s+"([^"]+)"`), "resource", func(m []string) string { return m[1] + "." + m[2] + "." + m[3] }},
		{regexp.MustCompile(`^\s*(module|variable|output|provider)\s+"([^"]+)"`), "block", func(m []string) string { return m[1] + "." + m[2] }},
		{regexp.MustCompile(`^\s*(locals|terraform)\s*\{`), "block", func(m []string) string { return m[1] }},
	}
	yamlRules = []declRule{
		{regexp.MustCompile(`^([A-Za-z_][\w./-]*):`), "key", nil},
		{regexp.MustCompile(`^-\s+name:\s*["']?([^"'\s]+)`), "item", nil},
	}
	sqlRules = []declRule{
		{regexp.MustCompile(`(?i)^\s*create\s+(?:or\s+replace\s+)?(?:temp(?:orary)?\s+)?(?:unique\s+)?(table|view|materialized view|function|procedure|index|type|schema|trigger|sequence)\s+(?:if\s+not\s+exists\s+)?([\w."]+)`), "sql", func(m []string) string {
			return strings.ToLower(m[1]) + "." + strings.ReplaceAll(m[2], `"`, "")
		}},
		{regexp.MustCompile(`(?i)^\s*alter\s+table\s+(?:if\s+exists\s+)?([\w."]+)`), "alter", nil},
	}
	gherkinRules = []declRule{
		{regexp.MustCompile(`^\s*(Feature|Rule|Scenario Outline|Scenario|Background|Example):\s*(.*)`), "gherkin", func(m []string) string {
			return strings.ToLower(strings.ReplaceAll(m[1], " ", "-")) + ":" + strings.TrimSpace(m[2])
		}},
	}
	mdRules = []declRule{
		{regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*#*\s*$`), "heading", func(m []string) string { return strings.TrimSpace(m[2]) }},
	}
	cRules = []declRule{
		{regexp.MustCompile(`^\s*(?:public|private|protected|internal|static|final|abstract|sealed|open|data|export|pub(?:\([^)]*\))?|unsafe|async|\s)*\s*(class|interface|enum|record|struct|trait|impl|object|fn|union|typedef\s+struct)\s+([A-Za-z_]\w*)`), "type", func(m []string) string { return m[2] }},
		{regexp.MustCompile(`^\s*(?:public|private|protected|internal|static|final|override|virtual|async|\s)*\s*[\w<>\[\],.?]+\s+([A-Za-z_]\w*)\s*\([^;]*$`), "method", nil},
	}
	rbRules = []declRule{
		{regexp.MustCompile(`^\s*(?:def|class|module)\s+(?:self\.)?([A-Za-z_][\w.?!:]*)`), "definition", nil},
	}
	shRules = []declRule{
		{regexp.MustCompile(`^\s*(?:function\s+)?([A-Za-z_]\w*)\s*\(\)`), "function", nil},
		{regexp.MustCompile(`^\s*(FROM|RUN|ENTRYPOINT|CMD|COPY)\b`), "instruction", nil},
	}
	protoRules = []declRule{
		{regexp.MustCompile(`^\s*(message|service|enum|rpc)\s+([A-Za-z_]\w*)`), "proto", func(m []string) string { return m[2] }},
	}
)

func rulesFor(path string) []declRule {
	base := filepath.Base(path)
	switch strings.ToLower(filepath.Ext(base)) {
	case ".py":
		return pyRules
	case ".kt", ".kts", ".scala":
		return append(append(append([]declRule{}, ktRules...), tsRules...), cRules...)
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".dart", ".swift", ".groovy":
		return append(append([]declRule{}, tsRules...), cRules...)
	case ".go":
		return goRules
	case ".tf", ".hcl", ".tfvars":
		return hclRules
	case ".yaml", ".yml", ".toml", ".ini", ".cfg", ".properties", ".env", ".conf":
		return yamlRules
	case ".sql", ".ddl", ".psql":
		return sqlRules
	case ".feature":
		return gherkinRules
	case ".md", ".mdx":
		return mdRules
	case ".java", ".cs", ".rs", ".c", ".h", ".cc", ".cpp", ".hpp", ".php", ".zig", ".m", ".mm":
		return cRules
	case ".rb":
		return rbRules
	case ".sh", ".bash", ".zsh":
		return shRules
	case ".proto":
		return protoRules
	}
	if base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile.") {
		return shRules
	}
	return nil
}

// Resolve computes the anchor for a marker found at f, given the file's lines.
// Precedence follows the design: explicit scope=, else the next declaration,
// else the file when the marker sits in the header comment, else a block
// anchor on the next non-blank line.
func Resolve(path string, f Found, lines []string, style CommentStyle) Anchor {
	scope := f.Marker.Scope()
	idx := f.Marker.Line - 1 // 0-based index of the marker line
	switch scope {
	case "file":
		return fileAnchor(lines)
	case "line":
		ln := clamp(idx+1, len(lines))
		return Anchor{Scope: "line", StartLine: ln + 1, EndLine: ln + 1, Signature: norm(at(lines, ln)), Fingerprint: fp(norm(at(lines, ln)))}
	}
	// YAML documents: a marker before "---" or at the top binds to the document.
	rules := rulesFor(path)
	isYAML := strings.HasSuffix(strings.ToLower(path), ".yaml") || strings.HasSuffix(strings.ToLower(path), ".yml")
	// Look ahead for a declaration within a window, skipping blanks, comments and decorators.
	window := 12
	for j := idx + 1; j < len(lines) && j <= idx+window; j++ {
		ln := lines[j]
		t := strings.TrimSpace(ln)
		if t == "" || isCommentLine(ln, style) {
			continue
		}
		if isDecorator(t, path) && !isYAML {
			// A decorator or annotation stack sits between the marker and the
			// declaration. Skip it whole, including arguments that span lines,
			// without spending the look-ahead window on it.
			if k := decoratorEnd(lines, j); k > j {
				window += k - j
				j = k
			}
			continue
		}
		if isYAML && t == "---" {
			continue
		}
		if isYAML {
			if a, ok := yamlDocAnchor(lines, j); ok {
				if scope == "" || scope == "symbol" || scope == "block" || scope == "range" {
					return a
				}
			}
		}
		for _, r := range rules {
			m := r.re.FindStringSubmatch(ln)
			if m == nil {
				continue
			}
			sym := ""
			if r.sym != nil {
				sym = r.sym(m)
			} else {
				sym = first(m)
			}
			end := blockEnd(lines, j, path)
			a := Anchor{Scope: "symbol", Symbol: sym, Kind: r.kind, StartLine: j + 1, EndLine: end + 1, Signature: norm(ln)}
			a.Fingerprint = fp(a.Signature)
			if scope == "block" || scope == "range" {
				a.Scope = scope
			}
			return a
		}
		// First code line is not a recognized declaration.
		if scope == "symbol" {
			// Caller asked for a symbol but none is detectable; bind to the block and let check warn.
			end := blockEnd(lines, j, path)
			return Anchor{Scope: "block", Kind: "unresolved-symbol", StartLine: j + 1, EndLine: end + 1, Signature: norm(ln), Fingerprint: fp(norm(ln))}
		}
		if scope == "" && f.Header {
			return fileAnchor(lines)
		}
		end := blockEnd(lines, j, path)
		return Anchor{Scope: "block", StartLine: j + 1, EndLine: end + 1, Signature: norm(ln), Fingerprint: fp(norm(ln))}
	}
	// Nothing follows: header comment or trailing comment → file, keeping an
	// explicit scope (sidecars carry range/block scopes with JSON Pointers).
	a := fileAnchor(lines)
	if scope != "" {
		a.Scope = scope
	}
	return a
}

// isDecorator reports a Python/TS/Java-style @decorator line or a Rust #[attribute].
func isDecorator(trimmed, path string) bool {
	if strings.HasPrefix(trimmed, "@") {
		return true
	}
	return strings.HasPrefix(trimmed, "#[") && strings.EqualFold(filepath.Ext(path), ".rs")
}

// decoratorEnd returns the index of the last line of the decorator that
// starts at j: the line on which its brackets close, capped at 40 lines.
func decoratorEnd(lines []string, j int) int {
	depth := 0
	for k := j; k < len(lines) && k < j+40; k++ {
		var quote byte
		for i := 0; i < len(lines[k]); i++ {
			ch := lines[k][i]
			if quote != 0 {
				if ch == '\\' {
					i++
				} else if ch == quote {
					quote = 0
				}
				continue
			}
			switch ch {
			case '"', '\'':
				quote = ch
			case '#':
				i = len(lines[k])
			case '(', '[', '{':
				depth++
			case ')', ']', '}':
				depth--
			}
		}
		if depth <= 0 {
			return k
		}
	}
	return j
}

func yamlDocAnchor(lines []string, j int) (Anchor, bool) {
	// Kubernetes-style: kind + metadata.name within the document.
	kind, name := "", ""
	end := len(lines) - 1
	for k := j; k < len(lines); k++ {
		t := strings.TrimSpace(lines[k])
		if t == "---" && k > j {
			end = k - 1
			break
		}
		if strings.HasPrefix(t, "kind:") && !strings.HasPrefix(lines[k], " ") {
			kind = strings.TrimSpace(strings.TrimPrefix(t, "kind:"))
		}
		if strings.HasPrefix(t, "name:") && strings.HasPrefix(lines[k], "  ") && name == "" && kind != "" {
			name = strings.Trim(strings.TrimSpace(strings.TrimPrefix(t, "name:")), `"'`)
		}
	}
	if kind != "" {
		sym := kind
		if name != "" {
			sym = kind + "/" + name
		}
		sig := norm("kind: " + kind + " name: " + name)
		return Anchor{Scope: "symbol", Symbol: sym, Kind: "document", StartLine: j + 1, EndLine: end + 1, Signature: sig, Fingerprint: fp(sig)}, true
	}
	return Anchor{}, false
}

func fileAnchor(lines []string) Anchor {
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(strings.TrimRight(l, " \t\r")))
		h.Write([]byte{'\n'})
	}
	return Anchor{Scope: "file", StartLine: 1, EndLine: max(len(lines), 1), Fingerprint: "sha256:" + hex.EncodeToString(h.Sum(nil))[:16]}
}

// blockEnd estimates the last line of the declaration starting at j.
func blockEnd(lines []string, j int, path string) int {
	ext := strings.ToLower(filepath.Ext(path))
	start := lines[j]
	switch ext {
	case ".py", ".yaml", ".yml", ".feature", ".rb":
		// Indentation-based: the block ends before the next line with indentation <= start's that is not blank.
		ind := indent(start)
		end := j
		for k := j + 1; k < len(lines); k++ {
			t := strings.TrimSpace(lines[k])
			if t == "" {
				continue
			}
			if indent(lines[k]) <= ind && !(ext == ".rb" && t == "end") {
				if ext == ".feature" {
					// Scenarios inside a Feature are indented; stop at next same-level keyword.
					break
				}
				break
			}
			end = k
		}
		if ext == ".py" || ext == ".rb" {
			return end
		}
		if end == j && ext != ".feature" {
			return j
		}
		return end
	case ".md", ".mdx":
		lvl := 0
		for _, ch := range start {
			if ch == '#' {
				lvl++
			} else {
				break
			}
		}
		end := j
		for k := j + 1; k < len(lines); k++ {
			t := lines[k]
			if strings.HasPrefix(t, "#") {
				l := 0
				for _, ch := range t {
					if ch == '#' {
						l++
					} else {
						break
					}
				}
				if l <= lvl {
					break
				}
			}
			end = k
		}
		return end
	case ".sql":
		for k := j; k < len(lines); k++ {
			if strings.Contains(lines[k], ";") {
				return k
			}
		}
		return j
	}
	// Brace languages: match braces from the first '{' at or after j.
	depth := 0
	opened := false
	for k := j; k < len(lines); k++ {
		for _, ch := range lines[k] {
			switch ch {
			case '{':
				depth++
				opened = true
			case '}':
				depth--
			}
		}
		if opened && depth <= 0 {
			return k
		}
		if !opened && k > j+2 {
			// one-liner without braces
			return j
		}
	}
	return j
}

func indent(s string) int {
	n := 0
	for _, ch := range s {
		if ch == ' ' {
			n++
		} else if ch == '\t' {
			n += 4
		} else {
			break
		}
	}
	return n
}

func isCommentLine(line string, style CommentStyle) bool {
	t := strings.TrimSpace(line)
	for _, tok := range style.Line {
		if tok != "" && strings.HasPrefix(t, tok) {
			return true
		}
	}
	for _, pair := range style.Block {
		if strings.HasPrefix(t, pair[0]) || strings.HasPrefix(t, "*") || strings.HasSuffix(t, pair[1]) {
			return true
		}
	}
	return false
}

func at(lines []string, i int) string {
	if i < 0 || i >= len(lines) {
		return ""
	}
	return lines[i]
}

func clamp(i, n int) int {
	if n == 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	if i < 0 {
		return 0
	}
	return i
}

var ws = regexp.MustCompile(`\s+`)

// norm collapses whitespace so a reformat does not change a fingerprint.
func norm(s string) string { return strings.TrimSpace(ws.ReplaceAllString(s, " ")) }

func fp(s string) string {
	h := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(h[:])[:16]
}
