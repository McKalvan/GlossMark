// @gloss implements gmi:DeclarationAnchor scope=file

package marker

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Disambiguate gives distinct symbols to anchors in one file that name
// different declarations with the same bare symbol, such as process() on two
// classes. Each colliding symbol is qualified by its enclosing classes
// (InboundHandler.process), as Go methods already are; declarations that
// still collide (overloads, a redefinition) take an ordinal in file order
// (check#2). A renamed anchor's fingerprint covers the qualified name, so
// identical signatures in different classes differ. Symbols that do not
// collide are left alone, so their artifact IRIs stay stable.
func Disambiguate(path string, lines []string, anchors []Anchor) {
	changed := map[int]bool{}
	for _, idx := range collisions(anchors) {
		for _, i := range idx {
			if c := container(path, lines, anchors[i].StartLine-1); c != "" {
				anchors[i].Symbol = c + "." + anchors[i].Symbol
				changed[i] = true
			}
		}
	}
	for _, idx := range collisions(anchors) {
		starts := map[int]int{} // start line → ordinal
		var order []int
		for _, i := range idx {
			if _, ok := starts[anchors[i].StartLine]; !ok {
				order = append(order, anchors[i].StartLine)
				starts[anchors[i].StartLine] = 0
			}
		}
		sort.Ints(order)
		for n, s := range order {
			starts[s] = n + 1
		}
		for _, i := range idx {
			if n := starts[anchors[i].StartLine]; n > 1 {
				anchors[i].Symbol += fmt.Sprintf("#%d", n)
				changed[i] = true
			}
		}
	}
	for i := range changed {
		anchors[i].Fingerprint = fp(anchors[i].Symbol + " " + anchors[i].Signature)
	}
}

// collisions groups the symbol anchors whose symbol names more than one
// declaration (distinct start lines); several markers on one declaration
// are not a collision.
func collisions(anchors []Anchor) [][]int {
	bySym := map[string][]int{}
	var syms []string
	for i, a := range anchors {
		if a.Scope != "symbol" || a.Symbol == "" {
			continue
		}
		if _, ok := bySym[a.Symbol]; !ok {
			syms = append(syms, a.Symbol)
		}
		bySym[a.Symbol] = append(bySym[a.Symbol], i)
	}
	var out [][]int
	for _, s := range syms {
		idx := bySym[s]
		for _, i := range idx[1:] {
			if anchors[i].StartLine != anchors[idx[0]].StartLine {
				out = append(out, idx)
				break
			}
		}
	}
	return out
}

var (
	pyContainer = regexp.MustCompile(`^\s*(?:class|(?:async\s+)?def)\s+([A-Za-z_]\w*)`)
	rbContainer = regexp.MustCompile(`^\s*(?:class|module)\s+([A-Za-z_][\w:]*)`)
	// braceContainer matches a type-like declaration whose body holds
	// methods: class, interface, struct, object, namespace, ...
	braceContainer = regexp.MustCompile(`^\s*(?:(?:export|default|declare|abstract|public|private|protected|internal|static|final|sealed|open|data|partial|pub(?:\([^)]*\))?|unsafe)\s+)*(?:class|interface|enum|record|struct|trait|object|namespace|module|union)\s+([A-Za-z_$][\w$]*)`)
	// rustImpl names the type of an impl block: impl Foo, impl<T> Trait for Foo<T>.
	rustImpl = regexp.MustCompile(`^\s*(?:unsafe\s+)?impl\b(?:\s*<[^>]*>)?\s+(?:[\w:<>, ]+?\s+for\s+)?([A-Za-z_]\w*)`)
)

// container returns the dotted names of the classes (and, in Python,
// functions) enclosing the declaration on 0-based line j, or "".
func container(path string, lines []string, j int) string {
	if j < 0 || j >= len(lines) {
		return ""
	}
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".py", ".rb":
		re := pyContainer
		if ext == ".rb" {
			re = rbContainer
		}
		var names []string
		ind := indent(lines[j])
		for k := j - 1; k >= 0 && ind > 0; k-- {
			t := strings.TrimSpace(lines[k])
			if t == "" || strings.HasPrefix(t, "#") || indent(lines[k]) >= ind {
				continue
			}
			ind = indent(lines[k])
			if m := re.FindStringSubmatch(lines[k]); m != nil {
				names = append([]string{m[1]}, names...)
			}
		}
		return strings.Join(names, ".")
	case ".go", ".yaml", ".yml", ".md", ".mdx", ".sql", ".feature", ".tf", ".hcl", ".sh", ".bash", ".zsh", ".proto":
		return ""
	}
	if rulesFor(path) == nil {
		return ""
	}
	// Brace languages: track which declaration each open brace belongs to.
	var stack []string
	pending := ""
	rust := strings.EqualFold(filepath.Ext(path), ".rs") // 'a is a lifetime, not a quote
	for k := 0; k < j; k++ {
		ln := lines[k]
		if m := braceContainer.FindStringSubmatch(ln); m != nil {
			pending = m[1]
		} else if m := rustImpl.FindStringSubmatch(ln); m != nil && rust {
			pending = m[1]
		}
		var quote byte
		for i := 0; i < len(ln); i++ {
			ch := ln[i]
			if quote != 0 {
				if ch == '\\' {
					i++
				} else if ch == quote {
					quote = 0
				}
				continue
			}
			switch ch {
			case '"', '`':
				quote = ch
			case '\'':
				if !rust {
					quote = ch
				}
			case '/':
				if i+1 < len(ln) && ln[i+1] == '/' {
					i = len(ln)
				}
			case '{':
				stack = append(stack, pending)
				pending = ""
			case '}':
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
			case ';':
				pending = ""
			}
		}
	}
	var names []string
	for _, s := range stack {
		if s != "" {
			names = append(names, s)
		}
	}
	return strings.Join(names, ".")
}
