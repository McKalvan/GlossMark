// @gloss implements gmi:DeclarationAnchor, gmi:MarkerSuggester scope=file

package marker

import (
	"strings"
)

// Decl is one declaration found by the detector.
type Decl struct {
	Symbol string // name as the anchor would record it
	Kind   string // function, class, type, resource, table, ...
	Line   int    // 1-based line of the declaration itself
	End    int    // 1-based last line of its block, as the anchorer estimates it
	Insert int    // 1-based line a marker goes above: the first decorator, else Line
	Indent string // leading whitespace of the insertion line
	Doc    string // the docstring or the comment block just above, if any, as one line
	// Parent is the enclosing class (or impl, module, object) of a member;
	// empty for a top-level declaration.
	Parent string
	// Decorators are the decorator or annotation lines above the
	// declaration, trimmed.
	Decorators []string
}

// Declarations lists the declarations of a file with the regex detector the
// anchorer uses: the top-level ones, and the members (methods) of top-level
// classes, each right after its class. A decorator stack is skipped and
// recorded as the insertion point, so a marker inserted at Insert anchors to
// the declaration.
func Declarations(path string, lines []string, style CommentStyle) []Decl {
	var out []Decl
	for _, d := range declsAt(path, lines, style, 0, len(lines), 0) {
		out = append(out, d)
		if !hasMembers(d, lines) {
			continue
		}
		// Members sit one indentation step inside the container's block.
		member := -1
		for k := d.Line; k < d.End && k < len(lines); k++ {
			t := strings.TrimSpace(lines[k])
			if t == "" || isCommentLine(lines[k], style) || t == "{" || t == "}" {
				continue
			}
			if indent(lines[k]) > 0 && !strings.HasPrefix(t, `"""`) && !strings.HasPrefix(t, `'''`) {
				member = indent(lines[k])
				break
			}
			if indent(lines[k]) == 0 {
				break
			}
		}
		if member <= 0 {
			continue
		}
		for _, m := range declsAt(path, lines, style, d.Line, d.End, member) {
			if m.Kind == "variable" {
				continue
			}
			m.Parent = d.Symbol
			out = append(out, m)
		}
	}
	return out
}

// hasMembers reports a declaration whose block holds methods: a class, an
// interface, a struct's impl block, a Ruby class or module, a Kotlin object.
func hasMembers(d Decl, lines []string) bool {
	if d.End <= d.Line {
		return false
	}
	switch d.Kind {
	case "class", "interface", "type", "definition":
		sig := strings.TrimSpace(lines[d.Line-1])
		for _, f := range []string{"def ", "fn ", "pub fn ", "pub(crate) fn ", "async fn ", "pub async fn "} {
			if strings.HasPrefix(sig, f) {
				return false
			}
		}
		return true
	}
	return false
}

// declsAt lists the declarations in lines[from:to] indented by exactly
// want columns.
func declsAt(path string, lines []string, style CommentStyle, from, to, want int) []Decl {
	rules := rulesFor(path)
	if rules == nil {
		return nil
	}
	var out []Decl
	decoStart := -1
	for j := from; j < to && j < len(lines); j++ {
		ln := lines[j]
		t := strings.TrimSpace(ln)
		if t == "" || isCommentLine(ln, style) {
			if t == "" {
				decoStart = -1
			}
			continue
		}
		if indent(ln) != want {
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
			d := Decl{Symbol: sym, Kind: r.kind, Line: j + 1, End: blockEnd(lines, j, path) + 1, Insert: ins + 1, Indent: lines[ins][:len(lines[ins])-len(strings.TrimLeft(lines[ins], " \t"))], Doc: docFor(lines, ins, j, style)}
			for k := ins; k < j; k++ {
				if t := strings.TrimSpace(lines[k]); t != "" && !isCommentLine(lines[k], style) {
					d.Decorators = append(d.Decorators, t)
				}
			}
			out = append(out, d)
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
	// A Python docstring: the first line after the signature (a multi-line
	// signature ends at the line that closes with a colon), up to the end of
	// its first paragraph.
	k := decl
	for k < len(lines) && k < decl+8 && !strings.HasSuffix(strings.TrimSpace(stripHash(lines[k])), ":") {
		k++
	}
	if k+1 >= len(lines) {
		return ""
	}
	t := strings.TrimSpace(lines[k+1])
	for _, q := range []string{`"""`, `'''`} {
		if !strings.HasPrefix(t, q) {
			continue
		}
		body := strings.TrimPrefix(t, q)
		var para []string
		for n := k + 1; n < len(lines) && n < k+10; n++ {
			if n > k+1 {
				body = strings.TrimSpace(lines[n])
			}
			end := strings.Contains(body, q)
			body = strings.TrimSpace(strings.Split(body, q)[0])
			if body == "" && len(para) > 0 {
				break
			}
			if body != "" {
				para = append(para, body)
			}
			if end {
				break
			}
		}
		return strings.Join(para, " ")
	}
	return ""
}

// stripHash drops a trailing # comment from a Python line.
func stripHash(s string) string {
	if i := strings.Index(s, " #"); i >= 0 {
		return s[:i]
	}
	return s
}
