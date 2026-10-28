// @gloss implements gmi:Linter scope=file

package check

import "strings"

// Distance is the edit distance between a and b counting an adjacent
// transposition as one edit (optimal string alignment), so the commonest
// typos ("Retrun", "chekc") are one edit from what was meant.
func Distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}

// Closest returns the candidate nearest to word, ignoring case, when it is
// close enough to be the likely intent: at most one edit in four characters
// (at least one). Ties go to the earlier candidate. It returns "" when no
// candidate is that close.
func Closest(word string, candidates []string) string {
	w := strings.ToLower(word)
	limit := max(1, len([]rune(w))/4)
	best, bestD := "", limit+1
	for _, c := range candidates {
		if d := Distance(w, strings.ToLower(c)); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}
