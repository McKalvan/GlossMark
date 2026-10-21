// @gloss implements gmi:LandscapeAssembler scope=file
// @gloss implements gmi:MappingSets scope=file

package rollup

import (
	"fmt"
	"sort"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/mapping"
	"github.com/mckalvan/glossmark/internal/model"
)

// RuleMappingLift names the is-a edges an accepted broad or narrow match
// contributes when mapping-lift is on.
const RuleMappingLift = "mapping-lift"

// MappingSummary is what gloss landscape reports about mappings.
type MappingSummary struct {
	Accepted, Proposed, Stale int
	Classes                   [][]string // equivalence classes from exact matches, CURIEs
	Lifted                    int        // is-a edges from broad/narrow matches
	Dangling                  []string   // "subject predicate object: endpoint" whose endpoint is missing
	StaleMappings             []string
}

// ApplyMappings gives accepted mappings their meaning on a landscape graph
// (ADR 0025 §6): exact matches become equivalence classes (subsumption,
// drift coverage and context-map edges treat the class as one concept), and,
// with lift, broad and narrow matches become is-a edges for taxonomic lift.
// Proposed and rejected mappings mean nothing yet. Call before Derive.
func ApplyMappings(g *graph.Graph, lift bool) MappingSummary {
	var sum MappingSummary
	var pairs [][2]string
	for _, e := range g.Edges() {
		if e.Source != graph.SourceMapping || e.Prov == nil {
			continue
		}
		mapped := fmt.Sprintf("%s %s %s", g.Curie(e.S), e.Prov.Raw, g.Curie(e.O))
		for _, end := range []string{e.S, e.O} {
			if g.Node(end) == nil {
				if ns := g.Node(nsIRI(g, end)); ns != nil {
					sum.Dangling = append(sum.Dangling, mapped+": "+g.Curie(end)+" does not exist")
				}
			}
		}
		if e.Prov.Proposed {
			sum.Proposed++
			continue
		}
		sum.Accepted++
		if e.Prov.Review == model.StatusStale {
			sum.Stale++
			sum.StaleMappings = append(sum.StaleMappings, mapped)
		}
		sn, on := g.Node(e.S), g.Node(e.O)
		if sn == nil || on == nil || sn.NodeType != model.NodeConcept || on.NodeType != model.NodeConcept {
			continue
		}
		switch e.P {
		case mapping.RelExact:
			pairs = append(pairs, [2]string{e.S, e.O})
		case mapping.RelBroad, mapping.RelNarrow:
			if !lift {
				continue
			}
			sub, sup := e.S, e.O // subject broadMatch object: the object is broader
			if e.P == mapping.RelNarrow {
				sub, sup = e.O, e.S
			}
			if g.AddEdge(&graph.Edge{S: sub, P: "is-a", O: sup, Source: graph.SourceMapping, Prov: &graph.Prov{Rule: RuleMappingLift, Path: e.Prov.Path, Line: e.Prov.Line, Set: e.Prov.Set, ID: e.Prov.ID + "#lift"}}) {
				sum.Lifted++
			}
		}
	}
	if len(pairs) > 0 {
		g.SetEquivalence(pairs)
		seen := map[string]bool{}
		for _, p := range pairs {
			r := g.Canonical(p[0])
			if seen[r] {
				continue
			}
			seen[r] = true
			cls := []string{g.Curie(r)}
			for _, m := range g.Equivalents(r) {
				cls = append(cls, g.Curie(m))
			}
			sort.Strings(cls)
			sum.Classes = append(sum.Classes, cls)
		}
		sort.Slice(sum.Classes, func(i, j int) bool { return sum.Classes[i][0] < sum.Classes[j][0] })
	}
	sort.Strings(sum.Dangling)
	sort.Strings(sum.StaleMappings)
	return sum
}

// nsIRI returns the IRI of the namespace node an IRI falls under, or "".
func nsIRI(g *graph.Graph, iri string) string {
	best := ""
	for _, n := range g.NodesOf(model.NodeNamespace) {
		if len(n.ID) > len(best) && len(iri) > len(n.ID) && iri[:len(n.ID)] == n.ID {
			best = n.ID
		}
	}
	return best
}
