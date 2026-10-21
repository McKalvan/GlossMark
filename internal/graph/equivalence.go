// @gloss represents gmi:CanonicalGraph scope=file

package graph

import "sort"

// SetEquivalence declares classes of concepts that mean the same thing:
// accepted exact matches from mapping sets (ADR 0025 §6). Only gloss
// landscape sets it, so a single repository's own drift never changes. With
// it, Ancestors, Descendants and Subsumes treat a class as one concept, and
// Canonical names the class.
func (g *Graph) SetEquivalence(pairs [][2]string) {
	parent := map[string]string{}
	var find func(x string) string
	find = func(x string) string {
		if p, ok := parent[x]; ok && p != x {
			r := find(p)
			parent[x] = r
			return r
		}
		parent[x] = x
		return x
	}
	for _, p := range pairs {
		a, b := find(p[0]), find(p[1])
		if a == b {
			continue
		}
		if b < a {
			a, b = b, a
		}
		parent[b] = a // the lexically smallest IRI represents the class
	}
	g.equiv = map[string]string{}
	g.classes = map[string][]string{}
	for x := range parent {
		r := find(x)
		g.equiv[x] = r
		g.classes[r] = append(g.classes[r], x)
	}
	for r := range g.classes {
		sort.Strings(g.classes[r])
		if len(g.classes[r]) < 2 {
			delete(g.classes, r)
		}
	}
}

// Canonical returns the representative of id's equivalence class, or id.
func (g *Graph) Canonical(id string) string {
	if r, ok := g.equiv[id]; ok {
		return r
	}
	return id
}

// Equivalents returns the other members of id's equivalence class, sorted.
func (g *Graph) Equivalents(id string) []string {
	var out []string
	for _, m := range g.classes[g.Canonical(id)] {
		if m != id {
			out = append(out, m)
		}
	}
	return out
}

// HasEquivalence reports whether any class was declared.
func (g *Graph) HasEquivalence() bool { return len(g.classes) > 0 }

// equivClosure is Closure over is-a that crosses equivalence classes: the
// start's equivalents and every reached concept's equivalents are reached too.
func (g *Graph) equivClosure(start string, inverse bool) []string {
	seen := map[string]bool{}
	var stack []string
	push := func(x string) {
		for _, m := range append([]string{x}, g.Equivalents(x)...) {
			if m != start && !seen[m] {
				seen[m] = true
				stack = append(stack, m)
			}
		}
	}
	for _, m := range g.Equivalents(start) {
		push(m)
	}
	stack = append(stack, start)
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		var next []string
		if inverse {
			next = g.Subjects(cur, "is-a")
		} else {
			next = g.Objects(cur, "is-a")
		}
		for _, n := range next {
			push(n)
		}
	}
	return sortedKeys(seen)
}
