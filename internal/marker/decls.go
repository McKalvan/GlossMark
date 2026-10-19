package marker

import (
	"strings"
)

// Decl is one top-level declaration found by the detector.
type Decl struct {
	Symbol string // name as the anchor would record it
	Kind   string // function, class, type, resource, table, ...
	Line   int    // 1-based line of the declaration itself
	Insert int    // 1-based line a marker goes above: the first decorator, else Line
	Indent string // leading whitespace of the insertion line
	Doc    string // first line of the docstring or the comment just above, if any
}

// Declarations lists the top-level declarations of a file with the regex
// detector the anchorer uses. A decorator stack is skipped and recorded as
// the insertion point, so a marker inserted at Insert anchors to the
// declaration. Only unindented declarations are returned: a marker belongs on
// a public surface, not on every method.
func Declarations(path string, lines []string, style CommentStyle) []Decl {
	rules := rulesFor(path)
	if rules == nil {
		return nil
	}
	var out []Decl
	decoStart := -1
	for j := 0; j < len(lines); j++ {
		ln := lines[j]
		t := strings.TrimSpace(ln)
		if t == "" || isCommentLine(ln, style) {
			if t == "" {
				decoStart = -1
			}
			continue
		}
		if indent(ln) > 0 {
			decoStart = -1
			continue
		}
		if isDecorator(t, path) {
			if decoStart < 0 {
				decoStart = j
			}
			j = decoratorEnd(lines, j)
			continue
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
			if sym == "" {
				break
			}
			ins := j
			if decoStart >= 0 {
				ins = decoStart
			}
			out = append(out, Decl{Symbol: sym, Kind: r.kind, Line: j + 1, Insert: ins + 1, Indent: lines[ins][:len(lines[ins])-len(strings.TrimLeft(lines[ins], " \t"))], Doc: docFor(lines, ins, j, style)})
			break
		}
		decoStart = -1
	}
	return out
}

// docFor returns the comment block above the insertion point, or a Python
// docstring after the declaration, as one line.
func docFor(lines []string, ins, decl int, style CommentStyle) string {
	var above []string
	for k := ins - 1; k >= 0 && k >= ins-6; k-- {
		if !isCommentLine(lines[k], style) {
			break
		}
		t := strings.TrimSpace(lines[k])
		for _, tok := range style.Line {
			if tok != "" {
				t = strings.TrimPrefix(t, tok)
			}
		}
		t = strings.TrimSpace(strings.Trim(strings.TrimSpace(t), "/*"))
		if strings.Contains(t, "@gloss") {
			continue
		}
		above = append([]string{t}, above...)
	}
	if len(above) > 0 {
		return strings.TrimSpace(strings.Join(above, " "))
	}
	if decl+1 < len(lines) {
		t := strings.TrimSpace(lines[decl+1])
		for _, q := range []string{`"""`, `'''`} {
			if strings.HasPrefix(t, q) {
				return strings.TrimSpace(strings.Trim(t, `"'`))
			}
		}
	}
	return ""
}
