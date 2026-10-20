// @gloss implements gmi:ValueBinder scope=file

package extract

import (
	"path/filepath"
	"regexp"
	"strings"
)

// keyPathFile reports whether value-from symbol: names a key path in path
// (YAML, TOML or JSON) rather than an assignment in code.
func keyPathFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml", ".toml", ".json":
		return true
	}
	return false
}

var (
	yamlKey  = regexp.MustCompile(`^\s*(?:-\s+)?("[^"]*"|'[^']*'|[^\s:#'"-][^:#]*?)\s*:(?:\s|$)`)
	tomlHead = regexp.MustCompile(`^\s*\[\[?\s*([^\]]+?)\s*\]\]?\s*(?:#.*)?$`)
	tomlKey  = regexp.MustCompile(`^\s*("[^"]*"|'[^']*'|[A-Za-z0-9_.-]+)\s*=`)
	jsonKey  = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"\s*:`)
)

// keyAt is one key a configuration file sets: its dotted path and 1-based line.
type keyAt struct {
	line int
	path string
}

// keyPaths returns the dotted path of every key a YAML, TOML or JSON file
// sets, in file order. The parse is line by line, enough for configuration:
// YAML nesting by indentation (a list item's keys nest under the list),
// TOML [table] headers and dotted keys, JSON objects by braces.
func keyPaths(path string, lines []string) []keyAt {
	var out []keyAt
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		type level struct {
			indent int
			key    string
		}
		var stack []level
		for i, l := range lines {
			t := strings.TrimSpace(l)
			if t == "---" {
				stack = nil
			}
			if t == "" || strings.HasPrefix(t, "#") || t == "---" {
				continue
			}
			m := yamlKey.FindStringSubmatchIndex(l)
			if m == nil {
				continue
			}
			ind := m[2] // the key's column, past any "- "
			for len(stack) > 0 && stack[len(stack)-1].indent >= ind {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, level{ind, unquote(l[m[2]:m[3]])})
			keys := make([]string, len(stack))
			for k, s := range stack {
				keys[k] = s.key
			}
			out = append(out, keyAt{i + 1, strings.Join(keys, ".")})
		}
	case ".toml":
		table := ""
		for i, l := range lines {
			if m := tomlHead.FindStringSubmatch(l); m != nil {
				table = dotted(m[1])
				continue
			}
			if m := tomlKey.FindStringSubmatch(l); m != nil {
				k := dotted(m[1])
				if table != "" {
					k = table + "." + k
				}
				out = append(out, keyAt{i + 1, k})
			}
		}
	case ".json":
		var stack []string
		pending := ""
		for i, l := range lines {
			// Keys and braces in order along the line; a key match skips its
			// string, so only a brace inside a string value is misread.
			rest := l
			for len(rest) > 0 {
				loc := jsonKey.FindStringSubmatchIndex(rest)
				brace := strings.IndexAny(rest, "{}")
				if loc != nil && (brace < 0 || loc[0] < brace) {
					key := rest[loc[2]:loc[3]]
					// The outermost object has no key.
					out = append(out, keyAt{i + 1, strings.TrimLeft(strings.Join(append(append([]string{}, stack...), key), "."), ".")})
					pending = key
					rest = rest[loc[1]:]
					continue
				}
				if brace < 0 {
					break
				}
				if rest[brace] == '{' {
					stack = append(stack, pending)
				} else if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
				pending = ""
				rest = rest[brace+1:]
			}
		}
	}
	return out
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// dotted normalizes a TOML key or table name: a.b, "a".b, a . b → a.b.
func dotted(s string) string {
	parts := strings.Split(s, ".")
	for i, p := range parts {
		parts[i] = unquote(p)
	}
	return strings.Join(parts, ".")
}

// findKey resolves a value-from symbol in a key-path file: the line whose
// dotted path equals symbol, else the one line whose path ends with it
// (a bare key, or a tail such as claims-write.per_minute). It returns the
// candidates when several lines end with symbol.
func findKey(path string, lines []string, symbol string) (int, []string) {
	var tail []keyAt
	for _, k := range keyPaths(path, lines) {
		if k.path == symbol {
			return k.line, nil
		}
		if strings.HasSuffix(k.path, "."+symbol) {
			tail = append(tail, k)
		}
	}
	if len(tail) == 1 {
		return tail[0].line, nil
	}
	var cands []string
	for _, k := range tail {
		cands = append(cands, k.path)
	}
	return 0, cands
}
