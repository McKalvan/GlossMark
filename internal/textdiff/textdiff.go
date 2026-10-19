// @gloss implements gmi:TextDiff scope=file

// Package textdiff renders a unified diff of two small texts (ontology
// files, manifests) so a tool can show a reviewable change before writing it.
package textdiff

import (
	"fmt"
	"strings"
)

// Unified returns a unified diff of a and b with three lines of context,
// labelled with path, or "" when they are equal.
func Unified(path, a, b string) string {
	if a == b {
		return ""
	}
	x, y := split(a), split(b)
	// LCS table; ontology files are a few thousand lines at most.
	n, m := len(x), len(y)
	lcs := make([][]int32, n+1)
	for i := range lcs {
		lcs[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	type op struct {
		kind byte // ' ', '-', '+'
		text string
		ai   int // index in a (for ' ' and '-')
		bi   int // index in b (for ' ' and '+')
	}
	var ops []op
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && x[i] == y[j]:
			ops = append(ops, op{' ', x[i], i, j})
			i++
			j++
		case i < n && (j == m || lcs[i+1][j] >= lcs[i][j+1]):
			ops = append(ops, op{'-', x[i], i, j})
			i++
		default:
			ops = append(ops, op{'+', y[j], i, j})
			j++
		}
	}
	const ctx = 3
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- a/%s\n+++ b/%s\n", path, path)
	for k := 0; k < len(ops); {
		if ops[k].kind == ' ' {
			k++
			continue
		}
		start := k - ctx
		if start < 0 {
			start = 0
		}
		end := k
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			run := 0
			for end+run < len(ops) && ops[end+run].kind == ' ' {
				run++
			}
			if end+run == len(ops) || run > 2*ctx {
				end += min(run, ctx)
				break
			}
			end += run
		}
		la, lb := 0, 0
		for _, o := range ops[start:end] {
			if o.kind != '+' {
				la++
			}
			if o.kind != '-' {
				lb++
			}
		}
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", ops[start].ai+1, la, ops[start].bi+1, lb)
		for _, o := range ops[start:end] {
			sb.WriteByte(o.kind)
			sb.WriteString(o.text)
			sb.WriteByte('\n')
		}
		k = end
	}
	return sb.String()
}

func split(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
