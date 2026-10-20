// @gloss implements gmi:HtmlViewer scope=file

package viewer

import (
	"sort"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/slice"
)

// SliceIndex is every concept's and boundary's feature slice, precomputed by
// gloss view so the page filters without porting the rules to JavaScript
// (docs/adr/0023-feature-slices.md). Node IDs and via steps are interned:
// each member of Of[seed] is [node, role, step...], indexes into IDs, Roles
// and Steps.
type SliceIndex struct {
	IDs   []string           `json:"ids"`
	Roles []string           `json:"roles"`
	Steps []string           `json:"steps"`
	Of    map[string][][]int `json:"of"`
	// Omitted counts seeds left out because the index reached its size cap;
	// the page tells the reader to run gloss slice for them.
	Omitted int `json:"omitted,omitempty"`
}

// SliceBudget caps the precomputed index at about this many integers (each a
// few bytes of JSON, about 2 MB in all). GlossMark's own map needs about
// 4,200; ADR 0023 records the measurement.
const SliceBudget = 300000

// Slices precomputes the slice (context on) of every concept and boundary
// in g, until the budget is spent. Seeds go in criticality order, highest
// first, then by CURIE, so a capped index keeps the slices that matter most.
func Slices(g *graph.Graph, budget int) *SliceIndex {
	idx := &SliceIndex{Roles: slice.Roles, Of: map[string][][]int{}}
	ids, steps := map[string]int{}, map[string]int{}
	roles := map[string]int{}
	for i, r := range slice.Roles {
		roles[r] = i
	}
	intern := func(m map[string]int, list *[]string, s string) int {
		if i, ok := m[s]; ok {
			return i
		}
		m[s] = len(*list)
		*list = append(*list, s)
		return m[s]
	}
	var seeds []*graph.Node
	seeds = append(seeds, g.NodesOf(model.NodeConcept)...)
	seeds = append(seeds, g.NodesOf(model.NodeBoundary)...)
	sort.SliceStable(seeds, func(i, j int) bool {
		ri, rj := model.CriticalityRank(seeds[i].Criticality), model.CriticalityRank(seeds[j].Criticality)
		if ri != rj {
			return ri > rj
		}
		return seeds[i].CURIE < seeds[j].CURIE
	})
	used := 0
	for _, n := range seeds {
		if used >= budget {
			idx.Omitted++
			continue
		}
		s := slice.Compute(g, []string{n.ID}, slice.Options{Context: true})
		var members [][]int
		for _, sn := range s.Nodes {
			m := []int{intern(ids, &idx.IDs, sn.ID), roles[sn.Role]}
			for _, st := range sn.Via {
				m = append(m, intern(steps, &idx.Steps, st))
			}
			used += len(m)
			members = append(members, m)
		}
		idx.Of[n.CURIE] = members
	}
	return idx
}
