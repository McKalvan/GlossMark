// @gloss implements gmi:Stripper scope=file

package marker

import (
	"bytes"
	"strings"
)

// Strip returns src with every marker removed and how many it removed. It
// reads comments exactly as Scan does (the same comment style, block state,
// backtick quoting and Markdown fences), so what it removes is what the
// extractor would have read, and nothing else: a marker-shaped string
// literal, or `@gloss` quoted in prose, stays. Line endings and every
// other byte are kept.
//
// A comment that holds nothing but the marker goes whole: the line, when
// nothing else is on it (also a line inside a block comment, which stays
// well-formed), or the trailing comment after code. A comment with other
// text, or an opening or closing line of a multi-line block, loses only
// the marker text, so the comment and the code around it keep their shape.
func Strip(src []byte, style CommentStyle) ([]byte, int) {
	raw := splitKeep(src)
	lines := make([]string, len(raw))
	for i, l := range raw {
		lines[i] = strings.TrimRight(strings.TrimRight(l, "\n"), "\r")
	}
	bom := false
	if len(lines) > 0 && strings.HasPrefix(lines[0], "\ufeff") {
		bom = true
		lines[0] = strings.TrimPrefix(lines[0], "\ufeff")
		raw[0] = strings.TrimPrefix(raw[0], "\ufeff")
	}
	removed := 0
	drop := make([]bool, len(lines))
	eachComment(lines, style, func(i int, segs []segment) {
		line := lines[i]
		// Later segments first, so earlier offsets stay valid.
		for s := len(segs) - 1; s >= 0; s-- {
			seg := segs[s]
			masked := maskQuoted(seg.text)
			m, _ := ParseLine(masked, i+1, seg.col)
			if m == nil {
				continue
			}
			removed++
			at := strings.Index(masked, m.Raw)
			if at < 0 {
				continue
			}
			segStart, segEnd := seg.col-1, seg.col-1+len(seg.text)
			openerStart, opener := segStart, false
			for _, tok := range style.Line {
				if tok != "" && strings.HasSuffix(line[:segStart], tok) {
					openerStart, opener = segStart-len(tok), true
				}
			}
			for _, pair := range style.Block {
				if strings.HasSuffix(line[:segStart], pair[0]) {
					openerStart, opener = segStart-len(pair[0]), true
				}
			}
			closerEnd, closer := segEnd, false
			if seg.block {
				for _, pair := range style.Block {
					if strings.HasPrefix(line[segEnd:], pair[1]) {
						closerEnd, closer = segEnd+len(pair[1]), true
					}
				}
			}
			rest := seg.text[:at] + seg.text[at+len(m.Raw):]
			onlyMarker := strings.Trim(rest, " \t*") == ""
			selfContained := !seg.block || opener && closer
			interior := seg.block && !opener && !closer
			outside := strings.TrimSpace(line[:openerStart]) == "" && strings.TrimSpace(line[closerEnd:]) == ""
			switch {
			case onlyMarker && outside && (selfContained || interior):
				drop[i] = true
				line = ""
			case onlyMarker && selfContained:
				line = joinAround(line[:openerStart], line[closerEnd:])
			default:
				start := segStart + at
				line = joinAround(line[:start], line[start+len(m.Raw):])
			}
		}
		if line != lines[i] && !drop[i] {
			raw[i] = line + ending(raw[i])
		}
	})
	if removed == 0 {
		return src, 0
	}
	var out bytes.Buffer
	first := true
	for i, l := range raw {
		if drop[i] {
			continue
		}
		if first && bom {
			out.WriteString("\ufeff")
		}
		first = false
		out.WriteString(l)
	}
	return out.Bytes(), removed
}

// eachComment calls fn for every line outside a Markdown fence with the
// comment segments on it, tracking block-comment state as Scan does.
func eachComment(lines []string, style CommentStyle, fn func(i int, segs []segment)) {
	inBlock := -1
	inFence := ""
	for i, line := range lines {
		if style.Fences {
			if tok := fenceToken(line); tok != "" {
				if inFence == "" {
					inFence = tok
				} else if tok[0] == inFence[0] && len(tok) >= len(inFence) {
					inFence = ""
				}
				continue
			}
			if inFence != "" {
				continue
			}
		}
		fn(i, commentSegments(line, style, &inBlock))
	}
}

// splitKeep splits src into lines that keep their terminator (\n, \r\n or a
// lone \r, as scanLines ends them), so a file can be rebuilt byte for byte.
func splitKeep(src []byte) []string {
	var out []string
	for len(src) > 0 {
		adv, _, _ := scanLines(src, true)
		if adv <= 0 {
			break
		}
		out = append(out, string(src[:adv]))
		src = src[adv:]
	}
	return out
}

// joinAround joins what precedes and follows a removed span: trailing
// blanks before it go when nothing follows, leading blanks after it go when
// nothing precedes, so no line ends or starts with the space the marker had.
func joinAround(before, after string) string {
	if strings.TrimSpace(after) == "" {
		return strings.TrimRight(before, " \t")
	}
	if strings.TrimSpace(before) == "" {
		return before + strings.TrimLeft(after, " \t")
	}
	return strings.TrimRight(before, " \t") + " " + strings.TrimLeft(after, " \t")
}

// ending returns the line terminator a raw line carries, or "".
func ending(raw string) string {
	switch {
	case strings.HasSuffix(raw, "\r\n"):
		return "\r\n"
	case strings.HasSuffix(raw, "\n"):
		return "\n"
	case strings.HasSuffix(raw, "\r"):
		return "\r"
	}
	return ""
}
