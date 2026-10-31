// @gloss implements gmi:SpecInference scope=file

package suggest

import (
	"sort"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/infer"
)

// vocab is the read-only view of the ontology a source gets: the
// matcher's concepts without the matcher.
type vocab struct {
	m *matcher
	g *graph.Graph
}

func (v vocab) view(c *concept) infer.Concept {
	out := infer.Concept{CURIE: c.curie, Name: c.name, Kind: c.n.Kind, Aliases: c.alias, Impl: c.impl, Deprecated: c.n.Deprecated}
	for _, o := range v.g.Objects(c.n.ID, "is-a") {
		if cur := v.g.Curie(o); cur != "" {
			out.IsA = append(out.IsA, cur)
		}
	}
	for cur := range c.realizes {
		out.Realizes = append(out.Realizes, cur)
	}
	sort.Strings(out.Realizes)
	return out
}

func (v vocab) Concepts() []infer.Concept {
	out := make([]infer.Concept, 0, len(v.m.concepts))
	for _, c := range v.m.concepts {
		out = append(out, v.view(c))
	}
	return out
}

// Named: the concepts whose name or an alias is exactly name, as the
// matcher tokenizes it, core tier first.
func (v vocab) Named(name string, kinds ...string) []infer.Concept {
	var found []*concept
	for _, h := range v.m.nameHits(name) {
		if (h.kind == evNameExact || h.kind == evAliasExact) && (len(kinds) == 0 || contains(kinds, h.c.n.Kind)) {
			found = append(found, h.c)
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].impl != found[j].impl {
			return !found[i].impl
		}
		return found[i].curie < found[j].curie
	})
	out := make([]infer.Concept, 0, len(found))
	for _, c := range found {
		out = append(out, v.view(c))
	}
	return out
}

// CommonAncestor finds the concept every one of curies is-a (itself
// included) that is nearest to them all.
func (v vocab) CommonAncestor(curies []string) string {
	if len(curies) == 0 {
		return ""
	}
	dist := func(cur string) map[string]int {
		d := map[string]int{}
		start := v.g.NodeByCurie(cur)
		if start == nil {
			return d
		}
		d[cur] = 0
		queue := []*graph.Node{start}
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			for _, o := range v.g.Objects(n.ID, "is-a") {
				on := v.g.Node(o)
				if on == nil || on.CURIE == "" {
					continue
				}
				if _, seen := d[on.CURIE]; !seen {
					d[on.CURIE] = d[v.g.Curie(n.ID)] + 1
					queue = append(queue, on)
				}
			}
		}
		return d
	}
	common := dist(curies[0])
	for _, cur := range curies[1:] {
		d := dist(cur)
		for k, n := range common {
			if dn, ok := d[k]; ok {
				if dn > n {
					common[k] = dn
				}
			} else {
				delete(common, k)
			}
		}
	}
	best, bestD := "", -1
	for k, n := range common {
		if bestD < 0 || n < bestD || n == bestD && k < best {
			best, bestD = k, n
		}
	}
	return best
}
