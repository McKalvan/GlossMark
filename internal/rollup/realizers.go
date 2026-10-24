// @gloss implements gmi:RollupEngine scope=file

package rollup

import (
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
)

// Realizers maps each concept CURIE to the boundaries whose code derives rel
// (realizes or holds) for it, keeping only the deepest: a boundary is dropped
// when a boundary nested in it is in the set too, so an enclosing system
// does not count as a second implementation of what its component does.
// Every boundary in a set is therefore unrelated by part-of to the others.
func Realizers(g *graph.Graph, rel string) map[string][]string {
	return deepest(g, RealizersRaw(g, rel))
}

// OwnRealizers is Realizers counting only what a boundary realizes itself:
// an edge lifted from a part to its whole (a step "part-of ...") is left out.
// A whole whose parts live in two boundaries is decomposed, not implemented
// twice, so it is no overlap (ADR 0019, as amended by ADR 0025 section 13).
func OwnRealizers(g *graph.Graph, rel string) map[string][]string {
	return deepest(g, realizersRaw(g, rel, func(e *graph.Edge) bool {
		if e.Prov != nil {
			for _, s := range e.Prov.Steps {
				if strings.HasPrefix(s, "part-of ") {
					return false
				}
			}
		}
		return true
	}))
}

// deepest keeps, per concept, the boundaries no other boundary in the set is
// nested in, as CURIEs.
func deepest(g *graph.Graph, raw map[string]map[string]bool) map[string][]string {
	out := map[string][]string{}
	for c, set := range raw {
		inner := map[string]bool{}
		for b := range set {
			for _, anc := range g.PartOfChain(b) {
				if anc != b && set[anc] {
					inner[anc] = true
				}
			}
		}
		var cs []string
		for b := range set {
			if !inner[b] {
				cs = append(cs, g.Curie(b))
			}
		}
		sort.Strings(cs)
		out[c] = cs
	}
	return out
}

// Holders maps each concept CURIE to the deepest boundaries whose manifest
// declares holds for it. holds is never derived (ADR 0037), so two of them
// are two claims to own the same data.
func Holders(g *graph.Graph) map[string][]string {
	out := map[string]map[string]bool{}
	for _, b := range g.NodesOf(model.NodeBoundary) {
		for _, e := range g.Out(b.ID, "holds") {
			if e.Source != graph.SourceDeclared {
				continue
			}
			n := g.Node(e.O)
			if n == nil || n.NodeType != model.NodeConcept {
				continue
			}
			if out[n.CURIE] == nil {
				out[n.CURIE] = map[string]bool{}
			}
			out[n.CURIE][b.ID] = true
		}
	}
	return deepest(g, out)
}

// RealizersRaw maps each concept CURIE to the set of boundary IRIs with a
// derived rel edge to it, enclosing boundaries included.
func RealizersRaw(g *graph.Graph, rel string) map[string]map[string]bool {
	return realizersRaw(g, rel, nil)
}

// realizersRaw is RealizersRaw over the derived edges keep accepts (all when nil).
func realizersRaw(g *graph.Graph, rel string, keep func(*graph.Edge) bool) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, b := range g.NodesOf(model.NodeBoundary) {
		for _, e := range g.Out(b.ID, rel) {
			if e.Source != graph.SourceDerived || (keep != nil && !keep(e)) {
				continue
			}
			n := g.Node(e.O)
			if n == nil || n.NodeType != model.NodeConcept {
				continue
			}
			if out[n.CURIE] == nil {
				out[n.CURIE] = map[string]bool{}
			}
			out[n.CURIE][b.ID] = true
		}
	}
	return out
}
