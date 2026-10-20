// @gloss implements gmi:RollupEngine scope=file

package rollup

import (
	"sort"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
)

// Realizers maps each concept CURIE to the boundaries whose code derives rel
// (realizes or holds) for it, keeping only the deepest: a boundary is dropped
// when a boundary nested in it is in the set too, so an enclosing system
// does not count as a second implementation of what its component does.
// Every boundary in a set is therefore unrelated by part-of to the others.
func Realizers(g *graph.Graph, rel string) map[string][]string {
	raw := RealizersRaw(g, rel)
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

// RealizersRaw maps each concept CURIE to the set of boundary IRIs with a
// derived rel edge to it, enclosing boundaries included.
func RealizersRaw(g *graph.Graph, rel string) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, b := range g.NodesOf(model.NodeBoundary) {
		for _, e := range g.Out(b.ID, rel) {
			if e.Source != graph.SourceDerived {
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
