// @gloss implements gmi:CriticalityStamper scope=file

package rollup

import (
	"sort"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
)

// HousingRelations are the boundary → concept relations that say "this
// concept's implementation lives inside this boundary": what the boundary's
// code implements (lifted to realizes) and the data shapes it holds. Views use
// them to place concepts inside boundary boxes; criticality rolls up along them.
var HousingRelations = []string{"realizes", "represents"}

// Criticality stamps every concept, boundary and artifact with its effective
// governance level (see docs/GOVERNANCE.md). A declared level always wins.
//
//   - A concept without one inherits the highest level of what it realizes or
//     is published as, else of the concept it is part of, else of its is-a
//     ancestors.
//   - A boundary without one takes the highest level of the concepts its code
//     realizes or represents (declared or derived), else its parent's.
//   - An artifact takes the highest level of its boundary and of the concepts
//     it links to.
//
// CriticalityFrom records the provenance: "declared", or the CURIE of the node
// the level was inherited from.
func Criticality(g *graph.Graph) {
	// Reset inherited values so a rebuild is idempotent.
	for _, n := range g.Nodes() {
		if n.CriticalityFrom != "declared" {
			n.Criticality, n.CriticalityFrom = "", ""
		}
	}
	memo := map[string]bool{}
	var concept func(n *graph.Node, depth int) string
	concept = func(n *graph.Node, depth int) string {
		if n.CriticalityFrom == "declared" || memo[n.ID] || depth > 16 {
			return n.Criticality
		}
		memo[n.ID] = true
		best := ""
		var bestNode *graph.Node
		take := func(rels ...string) {
			for _, p := range rels {
				for _, e := range g.Out(n.ID, p) {
					o := g.Node(e.O)
					if o == nil || o.NodeType != model.NodeConcept || e.Source == graph.SourceDerived {
						continue
					}
					if lvl := concept(o, depth+1); better(lvl, o, best, bestNode) {
						best, bestNode = lvl, o
					}
				}
			}
		}
		take("realizes", "published-as")
		if best == "" {
			take("part-of")
		}
		if best == "" {
			take("is-a")
		}
		if best != "" {
			n.Criticality, n.CriticalityFrom = best, bestNode.CURIE
		}
		return n.Criticality
	}
	for _, n := range g.NodesOf(model.NodeConcept) {
		concept(n, 0)
	}

	// Boundaries: deepest first is not needed since inheritance goes up (parent) only when nothing is housed.
	bounds := g.NodesOf(model.NodeBoundary)
	housed := func(b *graph.Node) (string, string) {
		best := ""
		var bestNode *graph.Node
		for _, p := range HousingRelations {
			for _, e := range g.Out(b.ID, p) {
				o := g.Node(e.O)
				if o == nil || o.NodeType != model.NodeConcept {
					continue
				}
				if better(o.Criticality, o, best, bestNode) {
					best, bestNode = o.Criticality, o
				}
			}
		}
		if bestNode == nil {
			return "", ""
		}
		return best, bestNode.CURIE
	}
	for _, b := range bounds {
		if b.CriticalityFrom == "declared" {
			continue
		}
		if lvl, from := housed(b); lvl != "" {
			b.Criticality, b.CriticalityFrom = lvl, from
		}
	}
	// Parent fallback, walking up the part-of chain to the nearest classified ancestor.
	for _, b := range bounds {
		if b.Criticality != "" {
			continue
		}
		for _, anc := range g.PartOfChain(b.ID) {
			if a := g.Node(anc); a != nil && a.Criticality != "" {
				b.Criticality, b.CriticalityFrom = a.Criticality, a.CURIE
				break
			}
		}
	}

	// Artifacts: max of boundary and linked concepts.
	for _, a := range g.NodesOf(model.NodeArtifact) {
		best := ""
		var bestNode *graph.Node
		if b := g.Node(a.Boundary); b != nil && b.Criticality != "" {
			best, bestNode = b.Criticality, b
		}
		for _, e := range g.Out(a.ID, "") {
			o := g.Node(e.O)
			if o == nil || o.NodeType != model.NodeConcept || e.P == "part-of" {
				continue
			}
			if better(o.Criticality, o, best, bestNode) {
				best, bestNode = o.Criticality, o
			}
		}
		if best != "" {
			a.Criticality, a.CriticalityFrom = best, bestNode.CURIE
		}
	}
}

// better reports whether candidate node n at level lvl should replace the
// current best (bestNode at level best): a higher level wins; at equal level a
// declared source beats an inherited one, then the lower CURIE, so the
// recorded provenance is stable across runs.
func better(lvl string, n *graph.Node, best string, bestNode *graph.Node) bool {
	r, b := model.CriticalityRank(lvl), model.CriticalityRank(best)
	if r != b {
		return r > b
	}
	if r < 0 {
		return false
	}
	if bestNode == nil {
		return true
	}
	nd, bd := n.CriticalityFrom == "declared", bestNode.CriticalityFrom == "declared"
	if nd != bd {
		return nd
	}
	return n.CURIE < bestNode.CURIE
}

// Understated lists boundaries whose declared criticality is below that of a
// concept their code realizes or represents: the governance drift signal.
func Understated(g *graph.Graph) []Understatement {
	var out []Understatement
	seen := map[string]bool{}
	for _, b := range g.NodesOf(model.NodeBoundary) {
		if b.CriticalityFrom != "declared" {
			continue
		}
		for _, p := range HousingRelations {
			for _, e := range g.Out(b.ID, p) {
				o := g.Node(e.O)
				if o == nil || o.NodeType != model.NodeConcept || seen[b.ID+"|"+o.ID] {
					continue
				}
				if model.CriticalityRank(o.Criticality) > model.CriticalityRank(b.Criticality) {
					seen[b.ID+"|"+o.ID] = true
					out = append(out, Understatement{Boundary: b.CURIE, Declared: b.Criticality, Concept: o.CURIE, Level: o.Criticality, File: b.File})
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Boundary != out[j].Boundary {
			return out[i].Boundary < out[j].Boundary
		}
		return out[i].Concept < out[j].Concept
	})
	return out
}

// Understatement is one boundary declaring a lower criticality than a concept it houses.
type Understatement struct {
	Boundary string `json:"boundary"`
	Declared string `json:"declared"`
	Concept  string `json:"concept"`
	Level    string `json:"level"`
	File     string `json:"file,omitempty"`
}
