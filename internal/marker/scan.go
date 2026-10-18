// @gloss implements gmi:MarkerGrammar scope=file

package marker

import (
	"bufio"
	"io"
	"sort"
	"strings"
)

func sortStrings(s []string) { sort.Strings(s) }

// Found is a marker together with its physical position and comment context.
type Found struct {
	Marker  *Marker
	InBlock bool // marker sits inside a block comment
	Header  bool // every line before the marker is comment or blank
}

// Scan reads a file and returns every marker found in a comment, in order,
// with diagnostics for malformed ones. Only text inside a comment is
// considered, so a marker-shaped string literal is never a marker. Inside a
// comment, text between a pair of backticks is quoted prose (so a comment can
// talk about `@gloss` without being one), and for Markdown-like styles the
// body of a fenced code block is quoted as well.
// It also returns the file's lines for anchoring.
func Scan(r io.Reader, style CommentStyle) ([]Found, []Diagnostic, []string) {
	var lines []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	var found []Found
	var diags []Diagnostic
	inBlock := -1 // index into style.Block when inside a block comment
	header := true
	inFence := "" // the fence token that opened the current Markdown code block
	for i, line := range lines {
		lineNo := i + 1
		if style.Fences {
			if tok := fenceToken(line); tok != "" {
				if inFence == "" {
					inFence = tok
				} else if tok[0] == inFence[0] && len(tok) >= len(inFence) {
					inFence = ""
				}
				header = false
				continue
			}
			if inFence != "" {
				header = false
				continue
			}
		}
		comments := commentSegments(line, style, &inBlock)
		codeBefore := hasCode(line, style, comments)
		for _, seg := range comments {
			m, d := ParseLine(maskQuoted(seg.text), lineNo, seg.col)
			for j := range d {
				d[j].Line = lineNo
			}
			diags = append(diags, d...)
			if m != nil {
				found = append(found, Found{Marker: m, InBlock: seg.block, Header: header && !codeBefore})
			}
		}
		if strings.TrimSpace(line) != "" && codeBefore {
			header = false
		}
	}
	return found, diags, lines
}

// maskQuoted blanks every span between a pair of backticks, keeping the
// text's length so columns stay physical. An unpaired backtick is left alone.
func maskQuoted(text string) string {
	if strings.Count(text, "`") < 2 {
		return text
	}
	b := []byte(text)
	open := -1
	for i, ch := range b {
		if ch != '`' {
			continue
		}
		if open < 0 {
			open = i
			continue
		}
		for j := open; j <= i; j++ {
			b[j] = ' '
		}
		open = -1
	}
	return string(b)
}

// fenceToken returns the fence (``` or ~~~, possibly longer) that a Markdown
// line opens or closes, or "" when the line is not a fence.
func fenceToken(line string) string {
	t := strings.TrimLeft(line, " \t")
	for _, ch := range []byte{'`', '~'} {
		n := 0
		for n < len(t) && t[n] == ch {
			n++
		}
		if n >= 3 {
			return t[:n]
		}
	}
	return ""
}

type segment struct {
	text  string
	col   int // 1-based column where text begins
	block bool
}

// commentSegments extracts the comment bodies in a line, tracking block state.
func commentSegments(line string, style CommentStyle, inBlock *int) []segment {
	var segs []segment
	pos := 0
	for pos <= len(line) {
		if *inBlock >= 0 {
			closer := style.Block[*inBlock][1]
			end := strings.Index(line[pos:], closer)
			if end < 0 {
				segs = append(segs, segment{text: line[pos:], col: pos + 1, block: true})
				return segs
			}
			segs = append(segs, segment{text: line[pos : pos+end], col: pos + 1, block: true})
			pos += end + len(closer)
			*inBlock = -1
			continue
		}
		// Find the earliest comment opener from pos.
		bestIdx, bestLen, bestBlock := -1, 0, -1
		for _, tok := range style.Line {
			if tok == "" {
				// whole line is a comment (sidecar files)
				if bestIdx < 0 || pos < bestIdx {
					bestIdx, bestLen, bestBlock = pos, 0, -1
				}
				continue
			}
			if i := indexOutsideString(line, pos, tok); i >= 0 && (bestIdx < 0 || i < bestIdx) {
				bestIdx, bestLen, bestBlock = i, len(tok), -1
			}
		}
		for bi, pair := range style.Block {
			if i := indexOutsideString(line, pos, pair[0]); i >= 0 && (bestIdx < 0 || i < bestIdx) {
				bestIdx, bestLen, bestBlock = i, len(pair[0]), bi
			}
		}
		if bestIdx < 0 {
			return segs
		}
		start := bestIdx + bestLen
		if bestBlock < 0 {
			segs = append(segs, segment{text: line[start:], col: start + 1})
			return segs
		}
		closer := style.Block[bestBlock][1]
		end := strings.Index(line[start:], closer)
		if end < 0 {
			segs = append(segs, segment{text: line[start:], col: start + 1, block: true})
			*inBlock = bestBlock
			return segs
		}
		segs = append(segs, segment{text: line[start : start+end], col: start + 1, block: true})
		pos = start + end + len(closer)
	}
	return segs
}

// indexOutsideString finds tok in line from pos, skipping occurrences inside
// simple quoted strings. Good enough for pass 1; tree-sitter replaces it.
func indexOutsideString(line string, pos int, tok string) int {
	var quote byte
	for i := pos; i < len(line); i++ {
		ch := line[i]
		if quote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' || ch == '`' {
			quote = ch
			continue
		}
		if strings.HasPrefix(line[i:], tok) {
			return i
		}
	}
	return -1
}

// hasCode reports whether the line has non-comment, non-blank content.
func hasCode(line string, style CommentStyle, segs []segment) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if len(segs) == 0 {
		return true
	}
	// Code exists if anything non-blank precedes the first comment opener.
	first := segs[0]
	openerStart := first.col - 1
	for _, tok := range style.Line {
		if strings.HasSuffix(line[:first.col-1], tok) {
			openerStart = first.col - 1 - len(tok)
		}
	}
	for _, pair := range style.Block {
		if strings.HasSuffix(line[:first.col-1], pair[0]) {
			openerStart = first.col - 1 - len(pair[0])
		}
	}
	if openerStart < 0 {
		openerStart = 0
	}
	if strings.TrimSpace(line[:openerStart]) != "" {
		return true
	}
	// Content after a closed block comment counts as code.
	last := segs[len(segs)-1]
	if last.block {
		after := last.col - 1 + len(last.text)
		for _, pair := range style.Block {
			if strings.HasPrefix(line[after:], pair[1]) {
				rest := strings.TrimSpace(line[after+len(pair[1]):])
				if rest != "" {
					return true
				}
			}
		}
	}
	return false
}
