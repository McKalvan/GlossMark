// @gloss implements gmi:DriftComparator scope=file

package rollup

import (
	"fmt"
	"sort"
	"time"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
)

// Landscape-only finding kinds: gloss check reports the same facts per
// repository (GM106, GM222); the landscape reports them across all of them.
const (
	// KindOutOfRange: an importer resolves a namespace version outside its range.
	KindOutOfRange = "out-of-range"
	// KindDeprecatedUse: a marker, manifest or ontology references a deprecated concept.
	KindDeprecatedUse = "deprecated-use"
)

// DeprecatedUse lists every reference to a deprecated concept from code, a
// manifest or another concept, with the replacement, so an owner can see
// who still uses it before removing it. A deprecated concept's own edges and
// replaced-by edges are not uses.
func DeprecatedUse(g *graph.Graph) []Finding {
	var out []Finding
	seen := map[string]bool{}
	for _, e := range g.Edges() {
		if e.Source == graph.SourceDerived || e.P == "replaced-by" || e.Prov == nil {
			continue
		}
		on, sn := g.Node(e.O), g.Node(e.S)
		if on == nil || sn == nil || on.NodeType != model.NodeConcept || !on.Deprecated || sn.Deprecated {
			continue
		}
		id := fmt.Sprintf("%s:%s:%s:%d", KindDeprecatedUse, on.CURIE, e.Prov.Path, e.Prov.Line)
		if seen[id] {
			continue
		}
		seen[id] = true
		who := g.Curie(e.S)
		if sn.NodeType == model.NodeArtifact {
			who = fmt.Sprintf("%s:%d", e.Prov.Path, e.Prov.Line)
		}
		msg := fmt.Sprintf("%s %s %s: deprecated", who, e.P, on.CURIE)
		if on.ReplacedBy != "" {
			msg += "; replaced-by " + on.ReplacedBy
		}
		out = append(out, Finding{ID: id, Kind: KindDeprecatedUse, Severity: "warning", Concept: on.CURIE, Relation: e.P, Path: e.Prov.Path, Line: e.Prov.Line, Msg: msg})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AddDeprecatedUse appends the deprecated-use findings to a drift report,
// with the policy's waivers applied, so gloss drift names code still on a
// deprecated concept as gloss check (GM222) and gloss landscape do. The
// kind does not fail by default: a deprecation is a migration under way,
// and overdue is the finding that enforces its deadline.
func AddDeprecatedUse(r *Report, g *graph.Graph, pol *model.Policy, now time.Time) {
	fs := DeprecatedUse(g)
	if len(fs) == 0 {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	at := len(r.Findings)
	r.Findings = append(r.Findings, fs...)
	c := &comparer{g: g, opt: Options{Policy: pol, Now: now}, r: &Report{Findings: r.Findings[at:]}}
	c.waive()
}
