// @gloss implements gmi:ReviewTerminal scope=file

package tui

import (
	"strings"
	"unicode"
)

// runeWidth is the number of terminal cells a rune takes: 0 for combining
// marks and other zero-width runes, 2 for East Asian wide and fullwidth
// runes and emoji, 1 otherwise.
func runeWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		return 0
	case r < 0x1100:
		return 1
	}
	for _, rg := range wideRanges {
		if r >= rg[0] && r <= rg[1] {
			return 2
		}
	}
	return 1
}

// wideRanges are the East Asian Wide (W) and Fullwidth (F) blocks and the
// emoji planes, coarse enough to stay short and fine enough for paths and
// concept docs.
var wideRanges = [][2]rune{
	{0x1100, 0x115f}, {0x231a, 0x231b}, {0x2329, 0x232a}, {0x23e9, 0x23ec}, {0x23f0, 0x23f0}, {0x23f3, 0x23f3},
	{0x25fd, 0x25fe}, {0x2614, 0x2615}, {0x2648, 0x2653}, {0x267f, 0x267f}, {0x2693, 0x2693}, {0x26a1, 0x26a1},
	{0x26aa, 0x26ab}, {0x26bd, 0x26be}, {0x26c4, 0x26c5}, {0x26ce, 0x26ce}, {0x26d4, 0x26d4}, {0x26ea, 0x26ea},
	{0x26f2, 0x26f3}, {0x26f5, 0x26f5}, {0x26fa, 0x26fa}, {0x26fd, 0x26fd}, {0x2705, 0x2705}, {0x270a, 0x270b},
	{0x2728, 0x2728}, {0x274c, 0x274c}, {0x274e, 0x274e}, {0x2753, 0x2755}, {0x2757, 0x2757}, {0x2795, 0x2797},
	{0x27b0, 0x27b0}, {0x27bf, 0x27bf}, {0x2b1b, 0x2b1c}, {0x2b50, 0x2b50}, {0x2b55, 0x2b55},
	{0x2e80, 0x303e}, {0x3041, 0x33ff}, {0x3400, 0x4dbf}, {0x4e00, 0x9fff}, {0xa000, 0xa4cf}, {0xa960, 0xa97f},
	{0xac00, 0xd7a3}, {0xf900, 0xfaff}, {0xfe10, 0xfe19}, {0xfe30, 0xfe6f}, {0xff00, 0xff60}, {0xffe0, 0xffe6},
	{0x16fe0, 0x18aff}, {0x1b000, 0x1b2ff}, {0x1f004, 0x1f004}, {0x1f0cf, 0x1f0cf}, {0x1f18e, 0x1f18e},
	{0x1f191, 0x1f19a}, {0x1f200, 0x1f251}, {0x1f300, 0x1f64f}, {0x1f680, 0x1f6ff}, {0x1f7e0, 0x1f7eb},
	{0x1f90c, 0x1f9ff}, {0x1fa70, 0x1faff}, {0x20000, 0x3fffd},
}

// strWidth is the number of cells s takes on a terminal.
func strWidth(s string) int {
	n := 0
	for _, r := range s {
		n += runeWidth(r)
	}
	return n
}

// truncRight fits s into w cells, ending in … when it cut anything. A wide
// rune that would straddle the edge is dropped, so the result can be a
// cell short of w; it is never wider.
func truncRight(s string, w int) string {
	if strWidth(s) <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		rw := runeWidth(r)
		if n+rw > w-1 {
			break
		}
		b.WriteRune(r)
		n += rw
	}
	return b.String() + "…"
}

// truncLeft fits s into w cells by cutting from the left, which keeps the
// end of a path, the part that tells files apart. It cuts at a / when one
// is left, so a path reads …/loans/renew.py rather than …ns/renew.py.
func truncLeft(s string, w int) string {
	if strWidth(s) <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	n, i := 0, len(r)
	for i > 0 && n+runeWidth(r[i-1]) <= w-1 {
		i--
		n += runeWidth(r[i])
	}
	rest := string(r[i:])
	if j := strings.Index(rest, "/"); j > 0 && j < len(rest)-1 {
		rest = rest[j:]
	}
	return "…" + rest
}
