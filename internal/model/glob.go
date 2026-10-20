// @gloss represents gmi:FileFormats scope=file

package model

import (
	"regexp"
	"strings"
	"sync"
)

var (
	globCache = map[string]*regexp.Regexp{}
	globMu    sync.Mutex
)

// MatchGlob matches a slash-separated path against a gitignore-style glob:
// `*` matches within a segment, `**` matches across segments, `?` one char.
// A pattern without a slash matches the basename anywhere in the tree.
func MatchGlob(pattern, path string) bool {
	pattern = strings.TrimPrefix(pattern, "./")
	path = strings.TrimPrefix(path, "./")
	if !strings.Contains(pattern, "/") {
		pattern = "**/" + pattern
	}
	return globRegexp(pattern).MatchString(path)
}

func globRegexp(pattern string) *regexp.Regexp {
	globMu.Lock()
	defer globMu.Unlock()
	if re, ok := globCache[pattern]; ok {
		return re
	}
	var sb strings.Builder
	sb.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					sb.WriteString("(?:.*/)?")
				} else {
					sb.WriteString(".*")
				}
			} else {
				sb.WriteString("[^/]*")
			}
		case '?':
			sb.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			sb.WriteByte('\\')
			sb.WriteByte(c)
		default:
			sb.WriteByte(c)
		}
	}
	sb.WriteString("$")
	re := regexp.MustCompile(sb.String())
	globCache[pattern] = re
	return re
}

// MatchCurie matches a CURIE against a pattern such as `circ:*`, `circ:Catalog*`
// or an exact CURIE.
func MatchCurie(pattern, curie string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == curie
	}
	var sb strings.Builder
	sb.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			sb.WriteString(".*")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\', '?':
			sb.WriteByte('\\')
			sb.WriteByte(c)
		default:
			sb.WriteByte(c)
		}
	}
	sb.WriteString("$")
	return regexp.MustCompile(sb.String()).MatchString(curie)
}

// MatchAnyCurie reports whether any pattern matches.
func MatchAnyCurie(patterns []string, curie string) bool {
	for _, p := range patterns {
		if MatchCurie(p, curie) {
			return true
		}
	}
	return false
}

// MatchAnyGlob reports whether any glob matches the path.
func MatchAnyGlob(patterns []string, path string) bool {
	for _, p := range patterns {
		if MatchGlob(p, path) {
			return true
		}
	}
	return false
}
