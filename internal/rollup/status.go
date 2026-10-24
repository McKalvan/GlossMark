// @gloss implements gmi:DriftComparator scope=file

package rollup

import (
	"sort"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
)

// Status values for ConceptStatus, in precedence order.
const (
	StatusProposed      = "proposed"      // proposed: true, awaiting a human
	StatusPlanned       = "planned"       // planned, nothing implements it yet (ADR 0034)
	StatusOrphan        = "orphan"        // no artifact links to it
	StatusUnimplemented = "unimplemented" // linked (config, a page, a consumer), but no artifact implements it
	StatusUntested      = "untested"      // implemented, nothing verifies it
	StatusImplemented   = "implemented"   // implemented, and verified where it applies
)

// ConceptStatusRow is one concept in gloss export --status.
type ConceptStatusRow struct {
	Concept   string   `json:"concept"`
	Tier      string   `json:"tier"`
	Kind      string   `json:"kind,omitempty"`
	Status    string   `json:"status"`
	Artifacts int      `json:"artifacts"` // implementing artifacts (see implementing)
	Verifiers int      `json:"verifiers"`
	Findings  []string `json:"findings,omitempty"` // every finding kind on it (presented-only, stale, deprecated-use, ...)
	// Deprecated marks a concept the ontology retires, with its successor:
	// an external checker can flag every use without parsing the ontology.
	Deprecated bool   `json:"deprecated,omitempty"`
	ReplacedBy string `json:"replaced-by,omitempty"`
	// Planned marks a plan item, with its deadline when it has one.
	Planned bool   `json:"planned,omitempty"`
	PlanBy  string `json:"plan-by,omitempty"`
	Doc     string `json:"doc,omitempty"`
}

// ConceptStatus summarizes every owned concept for an external checker (a
// glossary's Status column, an ontology linter): proposed, planned, orphan,
// unimplemented, untested or implemented, plus counts and the finding kinds
// that apply.
func ConceptStatus(g *graph.Graph, rep *Report) []ConceptStatusRow {
	kinds := map[string][]string{}
	for _, f := range rep.Findings {
		if f.Concept != "" && f.Kind != KindConsistent {
			kinds[f.Concept] = append(kinds[f.Concept], f.Kind)
		}
	}
	var out []ConceptStatusRow
	for _, n := range g.NodesOf(model.NodeConcept) {
		if n.Imported {
			continue
		}
		r := ConceptStatusRow{Concept: n.CURIE, Tier: n.Tier, Kind: n.Kind, Doc: n.Doc, Findings: uniq(kinds[n.CURIE]), Deprecated: n.Deprecated, ReplacedBy: n.ReplacedBy, Planned: n.Planned, PlanBy: n.PlanBy}
		// The concept and the implementation concepts that realize it.
		ids := []string{n.ID}
		for _, s := range g.Subjects(n.ID, "realizes") {
			if sn := g.Node(s); sn != nil && sn.NodeType == model.NodeConcept {
				ids = append(ids, s)
			}
		}
		others := 0 // links that do not implement it: config, surfaces, uses
		for _, id := range ids {
			for _, e := range g.In(id, "") {
				if e.Source != graph.SourceInline && e.Source != graph.SourceInferred {
					continue
				}
				if a := g.Node(e.S); a == nil || a.NodeType != model.NodeArtifact {
					continue
				}
				switch {
				case e.P == "documents" || e.P == "instructs" || e.P == "may-modify":
					// prose and agent files are not the concept's code
				case e.P == "verifies" || e.P == "tests":
					r.Verifiers++
				case implementing(g, e):
					r.Artifacts++
				default:
					others++
				}
			}
		}
		has := func(k string) bool {
			for _, x := range r.Findings {
				if x == k {
					return true
				}
			}
			return false
		}
		switch {
		case n.Proposed:
			r.Status = StatusProposed
		case has(KindPlanned), has(KindPlanOverdue):
			r.Status = StatusPlanned
		case has(KindOrphan):
			r.Status = StatusOrphan
		case r.Artifacts == 0 && others > 0 && n.ValueFrom == "":
			r.Status = StatusUnimplemented
		case has(KindUntested):
			r.Status = StatusUntested
		default:
			r.Status = StatusImplemented
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Concept < out[j].Concept })
	return out
}

// implementing reports whether an artifact edge is the concept's code:
// implements, represents, a published value, production of it, or
// infrastructure that lifts to realizes (on a resource or system-type).
// documents, configures (of anything else), presents, consumes and the like
// link to a concept without implementing it.
func implementing(g *graph.Graph, e *graph.Edge) bool {
	switch e.P {
	case "implements", "represents", "publishes-value", "produces", "publishes":
		return true
	}
	for _, rel := range InfraRelations {
		if e.P == rel {
			on := g.Node(e.O)
			return on != nil && (on.Kind == "resource" || on.Kind == "system-type")
		}
	}
	return false
}
