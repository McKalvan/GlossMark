// @gloss implements gmi:ViewProjector scope=file

package views

import (
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/slice"
)

// SliceMembers returns the CURIEs in a saved slice (a view with slice: seeds),
// computed with the same rules as gloss slice and with context on, or nil
// when the view is not a saved slice. A seed may be a CURIE glob.
func SliceMembers(g *graph.Graph, v *model.View) map[string]bool {
	if len(v.Slice) == 0 {
		return nil
	}
	var seeds []string
	for _, t := range []model.NodeType{model.NodeConcept, model.NodeBoundary} {
		for _, n := range g.NodesOf(t) {
			if model.MatchAnyCurie(v.Slice, n.CURIE) {
				seeds = append(seeds, n.ID)
			}
		}
	}
	set := map[string]bool{}
	if len(seeds) == 0 {
		return set
	}
	for _, n := range slice.Compute(g, seeds, slice.Options{Context: true}).Nodes {
		set[n.ID] = true
	}
	return set
}
