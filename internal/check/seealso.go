// @gloss implements gmi:Linter scope=file

package check

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/mckalvan/glossmark/internal/model"
)

var scheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

// seeAlso lints a concept's see-also entries (ADR 0050): one that is neither
// an http(s) URL nor a repository-relative path is GM309; a path whose file
// is missing, or whose #anchor names no heading of a Markdown file, is GM313.
// URLs are never fetched.
func (c *checker) seeAlso(file, curie string, con *model.Concept) {
	for _, e := range con.SeeAlso {
		p, anchor, _ := strings.Cut(e, "#")
		switch {
		case strings.TrimSpace(e) == "":
			c.add(file, con.Line, "error", "GM309", fmt.Sprintf("%s: see-also has an empty entry", curie))
			continue
		case scheme.MatchString(e):
			if l := strings.ToLower(e); !strings.HasPrefix(l, "http://") && !strings.HasPrefix(l, "https://") {
				c.add(file, con.Line, "error", "GM309", fmt.Sprintf("%s: see-also %q: only http(s) URLs and repository paths are allowed", curie, e))
			}
			continue
		case strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`):
			c.add(file, con.Line, "error", "GM309", fmt.Sprintf("%s: see-also %q is an absolute path; write it relative to the repository root", curie, e))
			continue
		case p == "" || path.Clean(p) == ".." || strings.HasPrefix(path.Clean(p), "../"):
			c.add(file, con.Line, "error", "GM309", fmt.Sprintf("%s: see-also %q names no file in the repository", curie, e))
			continue
		}
		b, err := os.ReadFile(filepath.Join(c.ws.Root, filepath.FromSlash(p)))
		if err != nil {
			c.add(file, con.Line, "warning", "GM313", fmt.Sprintf("%s: see-also %q: no such file in the repository", curie, e))
			continue
		}
		if anchor != "" && isMarkdown(p) && !markdownAnchors(b)[anchor] {
			c.add(file, con.Line, "warning", "GM313", fmt.Sprintf("%s: see-also %q: %s has no heading #%s", curie, e, p, anchor))
		}
	}
}

func isMarkdown(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".md", ".markdown", ".mdx":
		return true
	}
	return false
}

// markdownAnchors returns the anchors a code host gives a Markdown file's
// headings: lower case, punctuation dropped, spaces as hyphens, and -1, -2
// on repeats. Headings inside fenced code blocks do not count.
func markdownAnchors(b []byte) map[string]bool {
	out := map[string]bool{}
	seen := map[string]int{}
	fence := ""
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		t := strings.TrimSpace(sc.Text())
		if fence != "" {
			if strings.HasPrefix(t, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fence = t[:3]
			continue
		}
		h := strings.TrimLeft(t, "#")
		if n := len(t) - len(h); n == 0 || n > 6 || (h != "" && h[0] != ' ') {
			continue
		}
		s := slugify(strings.TrimRight(strings.TrimSpace(h), "#"))
		if k := seen[s]; k > 0 {
			out[fmt.Sprintf("%s-%d", s, k)] = true
		} else {
			out[s] = true
		}
		seen[s]++
	}
	return out
}

func slugify(h string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(h)) {
		switch {
		case r == ' ':
			sb.WriteRune('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r):
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
