// @gloss implements gmi:EvidenceHarvester scope=file

package harvest

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/mckalvan/glossmark/internal/marker"
)

// Constraints and events are written as sentences and state changes, not as
// names (ADR 0025, section 12): harvesting finds the sentence or the change,
// and the derivation names the concept.

// ruleWords mark a sentence that states a rule.
var ruleWords = regexp.MustCompile(`(?i)\b(never|must|cannot|can't|may not|at most|at least|no more than|exactly (?:one|once)|forbidden|not allowed|always|fails? closed|is required|are required)\b`)

const (
	minRuleWords = 5
	maxRuleWords = 80
	maxRuleChars = 400
)

// ruleSentences returns the sentences of a paragraph that state a rule.
func ruleSentences(text string) []string {
	var out []string
	for _, s := range sentences(text) {
		n := len(strings.Fields(s))
		if n < minRuleWords || n > maxRuleWords || !ruleWords.MatchString(s) {
			continue
		}
		if len(s) > maxRuleChars {
			s = strings.TrimSpace(s[:maxRuleChars]) + "…"
		}
		out = append(out, s)
	}
	return out
}

// sentences splits prose at a full stop, question or exclamation mark that
// is followed by a space and a capital (or a digit, or emphasis), so "e.g.
// a key" and "v1.2" stay whole.
func sentences(text string) []string {
	text = strings.Join(strings.Fields(text), " ")
	var out []string
	start := 0
	rs := []rune(text)
	for i := 0; i < len(rs)-2; i++ {
		if (rs[i] != '.' && rs[i] != '!' && rs[i] != '?') || rs[i+1] != ' ' {
			continue
		}
		if next := rs[i+2]; unicode.IsUpper(next) || unicode.IsDigit(next) || next == '*' || next == '`' {
			out = append(out, strings.TrimSpace(string(rs[start:i+1])))
			start = i + 2
		}
	}
	if rest := strings.TrimSpace(string(rs[start:])); rest != "" {
		out = append(out, rest)
	}
	return out
}

// paragraphs groups Markdown prose into paragraphs, each with its first
// line: blank lines, headings, fences and list items break them, and every
// table cell is a paragraph of its own.
type paragraph struct {
	text string
	line int
}

func (h *harvester) ruleParagraphs(rel string, paras []paragraph) {
	for _, p := range paras {
		for _, s := range ruleSentences(cleanMarkdown(p.text)) {
			h.add("rule-sentence", s, rel, p.line)
		}
	}
}

var listItem = regexp.MustCompile(`^(?:[-*+]|\d+[.)])\s+`)

// markdownParagraphs returns the prose paragraphs of a Markdown file.
func markdownParagraphs(lines []string) []paragraph {
	var out []paragraph
	var cur []string
	start := 0
	flush := func() {
		if len(cur) > 0 {
			out = append(out, paragraph{strings.Join(cur, " "), start})
		}
		cur = nil
	}
	inFence := false
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~"):
			flush()
			inFence = !inFence
		case inFence:
		case t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "---"):
			flush()
		case strings.HasPrefix(t, "|"):
			flush()
			for _, cell := range strings.Split(strings.Trim(t, "|"), "|") {
				if c := strings.TrimSpace(cell); c != "" && strings.Trim(c, "-: ") != "" {
					out = append(out, paragraph{c, i + 1})
				}
			}
		default:
			if listItem.MatchString(t) {
				flush()
				t = listItem.ReplaceAllString(t, "")
			}
			if len(cur) == 0 {
				start = i + 1
			}
			cur = append(cur, t)
		}
	}
	flush()
	return out
}

var (
	stateAttr  = regexp.MustCompile(`\b(\w+)\.((?i:state|status|phase|stage))\s*=\s*([^=\s].*)$`)
	stateKw    = regexp.MustCompile(`\b(state|status|phase|stage)\s*=\s*["'](\w+)["']`)
	stateSQL   = regexp.MustCompile(`(?i)\bset\s+(state|status|phase|stage)\s*=\s*'(\w+)'`)
	stateValue = regexp.MustCompile(`^(["']?)((?:[A-Za-z_]\w*\.)*)([A-Za-z_]\w*)`)
	callee     = regexp.MustCompile(`\b([A-Z]\w*)\(`)
	notAValue  = map[string]bool{"None": true, "null": true, "nil": true, "True": true, "False": true, "true": true, "false": true, "undefined": true, "self": true, "this": true}
)

// transition returns the lifecycle value a line assigns, the attribute it
// assigns it to, and what it belongs to (an object or the type being
// built), or "" when the line assigns none.
func transition(ln string) (value, attr, of string) {
	if m := stateAttr.FindStringSubmatch(ln); m != nil {
		v := stateValue.FindStringSubmatch(m[3])
		if v == nil {
			return "", "", ""
		}
		quote, prefix, val := v[1], v[2], v[3]
		if (val == "value" || val == "name") && prefix != "" {
			segs := strings.Split(strings.TrimSuffix(prefix, "."), ".")
			val, prefix = segs[len(segs)-1], strings.Join(segs[:len(segs)-1], ".")
		}
		literal := quote != "" || prefix != "" || strings.ToUpper(val) == val || unicode.IsUpper(rune(val[0]))
		if !literal || notAValue[val] {
			return "", "", ""
		}
		return val, m[2], m[1]
	}
	if loc := stateKw.FindStringSubmatchIndex(ln); loc != nil {
		of := ""
		for _, c := range callee.FindAllStringSubmatchIndex(ln[:loc[0]], -1) {
			of = ln[c[2]:c[3]]
		}
		return ln[loc[4]:loc[5]], ln[loc[2]:loc[3]], of
	}
	if m := stateSQL.FindStringSubmatch(ln); m != nil {
		return m[2], m[1], ""
	}
	return "", "", ""
}

// enclosing returns the innermost function or method around a line, or "".
func enclosing(decls []marker.Decl, line int) string {
	best, at := "", 0
	for _, d := range decls {
		if (d.Kind == "function" || d.Kind == "method") && d.Line <= line && line <= d.End && d.Line > at {
			best, at = d.Symbol, d.Line
		}
	}
	return best
}

var (
	valueSetStart = regexp.MustCompile(`^(?:export\s+)?(?:const\s+)?([A-Z][A-Z0-9_]+)\s*(?::[^=]+)?=\s*(?:frozenset\(|set\(|tuple\(|new Set\(|Object\.freeze\()?\s*([\[{(])`)
	setMember     = regexp.MustCompile(`["']([^"']+)["']|\b(?:[A-Za-z_]\w*\.)+([A-Za-z_]\w*)\b`)
)

// maxValueSetLines bounds how far a named collection is read.
const maxValueSetLines = 60

// valueSet reads a module-level named collection of literals or enum members
// starting at line i (LIQUID_TYPES = {AccountType.CHECKING, ...},
// CATEGORIES = ["Income", ...]): a classification rule or a catalogue the
// business keeps. It returns the name, the members and the last line read.
func valueSet(lines []string, i int) (name string, members []string, end int) {
	m := valueSetStart.FindStringSubmatch(lines[i])
	if m == nil {
		return "", nil, i
	}
	closer := map[string]string{"[": "]", "{": "}", "(": ")"}[m[2]]
	body := lines[i][len(m[0]):]
	end = i
	for !strings.Contains(body, closer) && end+1 < len(lines) && end-i < maxValueSetLines {
		end++
		body += " " + strings.SplitN(lines[end], "#", 2)[0]
	}
	body = strings.SplitN(body, closer, 2)[0]
	for _, mm := range setMember.FindAllStringSubmatch(body, -1) {
		// A quote inside a string ("Trader Joe's") can pair the wrong
		// quotes; what is left between them has no letter.
		if v := strings.TrimSpace(mm[1] + mm[2]); strings.IndexFunc(v, unicode.IsLetter) >= 0 {
			members = append(members, v)
		}
	}
	if len(members) < 2 {
		return "", nil, i
	}
	return m[1], members, end
}

// moduleDocstring returns a Python module's leading docstring, or "".
func moduleDocstring(lines []string) string {
	i := 0
	for i < len(lines) && (strings.TrimSpace(lines[i]) == "" || strings.HasPrefix(strings.TrimSpace(lines[i]), "#")) {
		i++
	}
	if i == len(lines) {
		return ""
	}
	t := strings.TrimSpace(lines[i])
	q := ""
	for _, p := range []string{`"""`, `'''`} {
		if strings.HasPrefix(t, p) {
			q = p
		}
	}
	if q == "" {
		return ""
	}
	t = strings.TrimPrefix(t, q)
	if strings.Contains(t, q) {
		return strings.SplitN(t, q, 2)[0]
	}
	var b []string
	b = append(b, t)
	for i++; i < len(lines); i++ {
		if strings.Contains(lines[i], q) {
			b = append(b, strings.SplitN(lines[i], q, 2)[0])
			break
		}
		b = append(b, strings.TrimSpace(lines[i]))
	}
	return strings.Join(b, " ")
}
