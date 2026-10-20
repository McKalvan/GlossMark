// @gloss implements gmi:CriticalityStamper scope=file

package rollup

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
)

// HousingRelations are the boundary → concept relations that say "this
// concept's implementation lives inside this boundary": what the boundary's
// code implements (lifted to realizes) and the data shapes it holds. Views use
// them to place concepts inside boundary boxes; criticality rolls up along them.
var HousingRelations = []string{"realizes", "represents"}

// RatingRelations are the boundary → concept relations a boundary without a
// level of its own is rated by: what it houses, and what it configures (a
// value editor carries the level of the rule it edits; ADR 0012).
var RatingRelations = []string{"realizes", "represents", "configures"}

// Criticality stamps every concept, boundary and artifact with its effective
// governance level (see docs/GOVERNANCE.md). A declared level always wins.
//
//   - A concept without one inherits the highest level of what it realizes or
//     is published as, else of the concept it is part of, else of its is-a
//     ancestors.
//   - A boundary without one takes the highest level of the concepts its code
//     realizes, represents or configures (declared or derived), else its
//     parent's, unless it is a surface: a boundary that presents or hosts and
//     is rated by nothing else stays unclassified (ADR 0012).
//   - An artifact takes the highest level of the concepts it links to (not
//     those it presents or hosts). A boundary's level is a floor for the
//     artifacts beneath it only when its manifest sets criticality-floor: true.
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
		for _, p := range RatingRelations {
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
	// Parent fallback, walking up the part-of chain to the nearest classified
	// ancestor. A surface takes no level from its neighbours: a BFF that only
	// presents order placement is not as critical as the system around it.
	for _, b := range bounds {
		if b.Criticality != "" || surfaceOnly(g, b.ID) {
			continue
		}
		for _, anc := range g.PartOfChain(b.ID) {
			if a := g.Node(anc); a != nil && a.Criticality != "" {
				b.Criticality, b.CriticalityFrom = a.Criticality, a.CURIE
				break
			}
		}
	}

	// Artifacts: the concepts they link to, raised to a floor boundary's level.
	for _, a := range g.NodesOf(model.NodeArtifact) {
		best := ""
		var bestNode *graph.Node
		if f := FloorFor(g, a.Boundary); f != nil {
			best, bestNode = f.Criticality, f
		}
		for _, e := range g.Out(a.ID, "") {
			o := g.Node(e.O)
			if o == nil || o.NodeType != model.NodeConcept || !CarriesCriticality(e.P) {
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

// surfaceOnly reports whether boundary b presents or hosts something and
// realizes, represents or configures nothing.
func surfaceOnly(g *graph.Graph, b string) bool {
	for _, p := range RatingRelations {
		if len(g.Out(b, p)) > 0 {
			return false
		}
	}
	for _, p := range SurfaceRelations {
		if len(g.Out(b, p)) > 0 {
			return true
		}
	}
	return false
}

// CarriesCriticality reports whether an artifact takes the level of a concept
// it links to with relation p. A page that presents a critical concept, or a
// shell that hosts it, is not itself critical: criticality follows the
// mechanism (ADR 0001 rule 3). The gate applies the same rule.
func CarriesCriticality(p string) bool {
	return p != "part-of" && p != "presents" && p != "hosts"
}

// FloorFor returns the nearest boundary at or above b that declares
// criticality-floor: true and has a level, or nil.
func FloorFor(g *graph.Graph, b string) *graph.Node {
	if b == "" {
		return nil
	}
	var best *graph.Node
	for _, id := range append([]string{b}, g.PartOfChain(b)...) {
		n := g.Node(id)
		if n == nil || !n.CriticalityFloor || n.Criticality == "" {
			continue
		}
		if best == nil || model.CriticalityRank(n.Criticality) > model.CriticalityRank(best.Criticality) {
			best = n
		}
	}
	return best
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
// concept their code realizes, represents or configures: the governance
// drift signal.
func Understated(g *graph.Graph) []Understatement {
	var out []Understatement
	seen := map[string]bool{}
	for _, b := range g.NodesOf(model.NodeBoundary) {
		if b.CriticalityFrom != "declared" {
			continue
		}
		for _, p := range RatingRelations {
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

// CriticalitySuggestion is one proposed classification.
type CriticalitySuggestion struct {
	Concept string `json:"concept"`
	Level   string `json:"level"`
	Reason  string `json:"reason"`
}

// securityTest matches test paths and symbols that name a security concern.
var securityTest = regexp.MustCompile(`(?i)(secur|auth|permission|access.?control|acl|rbac|csrf|xss|inject|crypt|privacy|pii|gdpr|secret|token|sanitiz|tamper|fraud)`)

// SuggestCriticality proposes a first classification for owned concepts that
// have none, from facts already in the graph: a constraint is a rule the
// business relies on, a published concept crosses the seam into the business
// view, and a concept whose only verifiers are security tests guards
// something sensitive. Each is high. Everything else stays unclassified; the
// suggestion is a starting point a human edits, not a policy.
//
// A declared level is the review: a concept that has one, or inherits one
// from a concept that declares it, is never suggested, so lowering a core
// concept to medium on purpose sticks. An implementation concept that
// realizes an unclassified core concept of this repository is not suggested
// either; the suggestion goes to the core concept it realizes, from which
// it would inherit.
func SuggestCriticality(g *graph.Graph) []CriticalitySuggestion {
	reasons := map[string]string{}
	var order []string
	suggest := func(n *graph.Node, reason string) {
		if _, ok := reasons[n.CURIE]; !ok {
			order = append(order, n.CURIE)
			reasons[n.CURIE] = reason
		}
	}
	unclassified := func(n *graph.Node) bool {
		return n != nil && n.NodeType == model.NodeConcept && !n.Imported && !n.Deprecated && n.Criticality == ""
	}
	var deferred []*graph.Node
	var deferredReason []string
	for _, n := range g.NodesOf(model.NodeConcept) {
		if !unclassified(n) {
			continue // declared, inherited from a declaration, imported or deprecated
		}
		reason := ""
		switch {
		case n.Kind == "constraint":
			reason = "constraint: a rule the business relies on"
		case n.Published != "" || len(g.In(n.ID, "published-as")) > 0:
			reason = "published: crosses the seam into the business view"
		default:
			total, sec := 0, 0
			for _, e := range g.In(n.ID, "") {
				if e.P != "verifies" && e.P != "tests" {
					continue
				}
				a := g.Node(e.S)
				if a == nil || a.NodeType != model.NodeArtifact {
					continue
				}
				total++
				if securityTest.MatchString(a.Path) || (a.Anchor != nil && securityTest.MatchString(a.Anchor.Symbol)) {
					sec++
				}
			}
			if total > 0 && sec == total {
				reason = fmt.Sprintf("verified only by security tests (%d)", total)
			}
		}
		if reason == "" {
			continue
		}
		if n.Tier == model.TierImpl && len(coresOf(g, n, unclassified)) > 0 {
			deferred, deferredReason = append(deferred, n), append(deferredReason, reason)
			continue
		}
		suggest(n, reason)
	}
	for i, n := range deferred {
		for _, core := range coresOf(g, n, unclassified) {
			suggest(core, fmt.Sprintf("realized by %s (%s), which would inherit it", n.CURIE, deferredReason[i]))
		}
	}
	var out []CriticalitySuggestion
	for _, c := range order {
		out = append(out, CriticalitySuggestion{Concept: c, Level: "high", Reason: reasons[c]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Concept < out[j].Concept })
	return out
}

// coresOf returns the core concepts n realizes that pass keep: its declared
// realizes targets, or, when it declares none, the core concepts its
// realizes closure reaches (through is-a).
func coresOf(g *graph.Graph, n *graph.Node, keep func(*graph.Node) bool) []*graph.Node {
	var out []*graph.Node
	targets := g.Objects(n.ID, "realizes")
	if len(targets) == 0 {
		targets = RealizesStar(g, n.ID)
	}
	for _, id := range targets {
		if c := g.Node(id); keep(c) && c.Tier == model.TierCore {
			out = append(out, c)
		}
	}
	return out
}
