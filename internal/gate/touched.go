// @gloss implements gmi:Gate scope=file

package gate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// ConceptTouch describes one concept a change touches and what constrains it.
type ConceptTouch struct {
	Curie         string   `json:"curie"`
	Kind          string   `json:"kind,omitempty"`
	Tier          string   `json:"tier,omitempty"`
	Doc           string   `json:"doc,omitempty"`
	Published     string   `json:"published_as,omitempty"`   // this concept is published under that core name
	PublishedBy   []string `json:"published_by,omitempty"`   // impl concepts published under this core name
	Realizes      []string `json:"realizes,omitempty"`       // core concepts this stands for
	Constrains    []string `json:"constrains,omitempty"`     // what this constrains
	ConstrainedBy []string `json:"constrained_by,omitempty"` // constraints on this
	// ConstrainedCode lists the files linked to what this constrains.
	ConstrainedCode []string `json:"constrained_code,omitempty"`
	Guards          []string `json:"guards,omitempty"`       // what this mechanism guards (ADR 0015)
	GuardedBy       []string `json:"guarded_by,omitempty"`   // mechanisms guarding this, with their level
	GuardedCode     []string `json:"guarded_code,omitempty"` // files linked to what this guards
	// Applies lists the custom relations declared with surfaces: [impact]
	// that touch this concept, on either end.
	Applies    []AppliedTouch `json:"applies,omitempty"`
	Boundaries []string       `json:"boundaries,omitempty"` // boundaries that declare or derive it
	Tests      []string       `json:"tests,omitempty"`      // artifacts that test or verify it
	// TestsVia lists the tests of the concepts that realize this one, which
	// gloss drift counts for it.
	TestsVia       []ViaTests `json:"tests_via,omitempty"`
	OtherArtifacts []string   `json:"other_artifacts,omitempty"` // other files linked to it (blast radius)
	Proposed       bool       `json:"proposed,omitempty"`
	Deprecated     bool       `json:"deprecated,omitempty"`
	Internal       bool       `json:"internal,omitempty"`
	// Criticality is the concept's effective governance level and where it comes from.
	Criticality     string   `json:"criticality,omitempty"`
	CriticalityFrom string   `json:"criticality_from,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

// ViaTests is the tests of one concept that realizes a touched concept.
type ViaTests struct {
	Via   string   `json:"via"`
	Tests []string `json:"tests"`
}

// FileTouch is what one changed file touches.
type FileTouch struct {
	Path     string `json:"path"`
	Boundary string `json:"boundary,omitempty"`
	// GuardedBy names the mechanisms a manifest puts this file behind with
	// guarded-by, with their level (ADR 0015).
	GuardedBy []string       `json:"guarded_by,omitempty"`
	Manifest  string         `json:"manifest,omitempty"`
	Allowed   []string       `json:"allowed,omitempty"`
	Links     []LinkTouch    `json:"links"`
	Concepts  []ConceptTouch `json:"concepts"`
	Gate      Class          `json:"gate"`
	Hints     []string       `json:"hints,omitempty"`
	// Criticality is the highest level among the file's boundary and the
	// concepts it touches; CriticalityFrom names the node that carries it.
	Criticality     string `json:"criticality,omitempty"`
	CriticalityFrom string `json:"criticality_from,omitempty"`
}

// AppliedTouch is a custom relation with surfaces: [impact] on a concept:
// what the concept applies it to, what applies it to the concept, and the
// files linked to what it applies to.
type AppliedTouch struct {
	Relation string   `json:"relation"`
	To       []string `json:"to,omitempty"`
	By       []string `json:"by,omitempty"`
	Code     []string `json:"code,omitempty"`
}

// LinkTouch is one marker in a changed file.
type LinkTouch struct {
	Line     int    `json:"line"`
	Symbol   string `json:"symbol,omitempty"`
	Relation string `json:"relation"`
	Target   string `json:"target"`
	Proposed bool   `json:"proposed,omitempty"`
	MarkerID string `json:"marker_id,omitempty"`
}

// Touched answers "what does a change to these files touch, and what
// constrains it": the concepts the files link to, their published status,
// constraints, boundaries, tests and the other files sharing the concept.
// Ontology and manifest files are expanded to the concepts and boundaries they define.
func Touched(g *graph.Graph, ws *load.Workspace, files []string) []FileTouch {
	var out []FileTouch
	pol := ws.Config.EffectivePolicy()
	for _, f := range files {
		ft := FileTouch{Path: f, Gate: AUTO}
		if m := ws.ManifestFor(f); m != nil {
			ft.Boundary, ft.Manifest, ft.Allowed = m.Boundary, m.Path, rollup.AllowedFor(g, ws, m).Patterns
			if b := g.NodeByCurie(m.Boundary); b != nil {
				if f := rollup.FloorFor(g, b.ID); f != nil {
					ft.Criticality, ft.CriticalityFrom = f.Criticality, f.CURIE
				}
			}
		}
		for _, id := range rollup.PathGuards(g, f) {
			if gn := g.Node(id); gn != nil {
				label := gn.CURIE
				if gn.Criticality != "" {
					label += " (" + gn.Criticality + ")"
				}
				ft.GuardedBy = append(ft.GuardedBy, label)
			}
		}
		seen := map[string]bool{}
		code := false
		addConcept := func(id string) {
			n := g.Node(id)
			if n == nil || n.NodeType != model.NodeConcept || seen[id] {
				return
			}
			seen[id] = true
			ft.Concepts = append(ft.Concepts, describeTouch(g, ws, n, f))
		}
		switch {
		case strings.HasSuffix(f, ".gloss.yaml") && !strings.HasSuffix(f, "/.gloss.yaml") && f != ".gloss.yaml":
			// Ontology file: every concept defined in it.
			ft.Gate = HARD
			ft.Hints = append(ft.Hints, "ontology edits are HARD: a human reviews them in the PR")
			for _, n := range g.NodesOf(model.NodeConcept) {
				if n.File == f {
					addConcept(n.ID)
				}
			}
		case strings.HasSuffix(f, ".gloss.yaml"):
			ft.Gate = HARD
			ft.Hints = append(ft.Hints, "manifest edits are HARD")
			for _, b := range g.NodesOf(model.NodeBoundary) {
				if b.File == f {
					for _, e := range g.Out(b.ID, "") {
						if e.Source == graph.SourceDeclared {
							addConcept(e.O)
						}
					}
				}
			}
		case f == load.ConfigName:
			ft.Gate = HARD
			ft.Hints = append(ft.Hints, "namespace changes are HARD")
		default:
			code = true
			for _, a := range g.NodesOf(model.NodeArtifact) {
				if a.Path != f {
					continue
				}
				// The file is rated at the level the graph stamps on its
				// artifacts, so impact, diff and the graph agree.
				if model.CriticalityRank(a.Criticality) > model.CriticalityRank(ft.Criticality) {
					ft.Criticality, ft.CriticalityFrom = a.Criticality, a.CriticalityFrom
				}
				for _, e := range g.Out(a.ID, "") {
					if e.Source != graph.SourceInline || e.Prov == nil {
						continue
					}
					ft.Links = append(ft.Links, LinkTouch{Line: e.Prov.Line, Symbol: e.Prov.Symbol, Relation: e.P, Target: g.Curie(e.O), Proposed: e.Prov.Proposed, MarkerID: e.Prov.ID})
					// Each link is gated as gloss diff gates it.
					cls, _ := ClassifyLink(g, ws, e, pol)
					ft.Gate = Max(ft.Gate, cls)
					// A guard raises the file's review level, not its own (ADR 0015).
					if gn := topGuard(g, e.P, e.O); gn != nil && model.CriticalityRank(gn.Criticality) > model.CriticalityRank(ft.Criticality) {
						ft.Criticality, ft.CriticalityFrom = gn.Criticality, gn.CURIE+" (guard)"
					}
					if gn := topPathGuard(g, e); gn != nil && model.CriticalityRank(gn.Criticality) > model.CriticalityRank(ft.Criticality) {
						ft.Criticality, ft.CriticalityFrom = gn.Criticality, gn.CURIE+" (guard)"
					}
					if on := g.Node(e.O); on != nil && on.Criticality != "" && !rollup.CarriesCriticality(e.P) {
						ft.Hints = append(ft.Hints, fmt.Sprintf("%s %s (%s): this file shows a %s concept; it is not rated by it", e.P, on.CURIE, on.Criticality, on.Criticality))
					}
					addConcept(e.O)
					for _, core := range realizesChain(g, e.O) {
						addConcept(core)
					}
				}
			}
			if len(ft.Links) == 0 {
				ft.Hints = append(ft.Hints, "no @gloss links in this file; add one so the change is attributable to a concept")
			}
		}
		for _, c := range ft.Concepts {
			if c.Published != "" || len(c.PublishedBy) > 0 {
				ft.Gate = HARD
				ft.Hints = append(ft.Hints, c.Curie+" is on the published seam: changes alter the business view (HARD)")
			}
			if c.Proposed {
				ft.Gate = HARD
			}
			if !code && model.CriticalityRank(c.Criticality) > model.CriticalityRank(ft.Criticality) {
				ft.Criticality, ft.CriticalityFrom = c.Criticality, c.Curie
			}
		}
		if ft.Gate == AUTO && len(ft.Links) > 0 {
			ft.Gate = SOFT
		}
		if ft.Criticality != "" {
			if want := Class(pol.GateForCriticality(ft.Criticality)); want != "" && rank(want) > rank(ft.Gate) {
				ft.Gate = want
			}
			if model.CriticalityRank(ft.Criticality) >= model.CriticalityRank("high") {
				ft.Hints = append(ft.Hints, fmt.Sprintf("criticality %s (via %s): a reviewer who owns this area should see the change", ft.Criticality, ft.CriticalityFrom))
			}
		}
		ft.Hints = uniq(ft.Hints)
		sort.Slice(ft.Links, func(i, j int) bool { return ft.Links[i].Line < ft.Links[j].Line })
		sort.Slice(ft.Concepts, func(i, j int) bool { return ft.Concepts[i].Curie < ft.Concepts[j].Curie })
		out = append(out, ft)
	}
	return out
}

// testsVia collects the tests of the concepts that realize id, directly or
// through each other, by realizer.
func testsVia(g *graph.Graph, id string) []ViaTests {
	var out []ViaTests
	seen := map[string]bool{id: true}
	var walk func(x string, d int)
	walk = func(x string, d int) {
		if d > 8 {
			return
		}
		for _, e := range g.In(x, "realizes") {
			sn := g.Node(e.S)
			if e.Source == graph.SourceDerived || sn == nil || sn.NodeType != model.NodeConcept || seen[e.S] {
				continue
			}
			seen[e.S] = true
			var ts []string
			for _, te := range g.In(e.S, "") {
				if te.P != "verifies" && te.P != "tests" {
					continue
				}
				if a := g.Node(te.S); a != nil && a.NodeType == model.NodeArtifact {
					ts = append(ts, a.Path)
				}
			}
			if len(ts) > 0 {
				sort.Strings(ts)
				out = append(out, ViaTests{Via: sn.CURIE, Tests: uniq(ts)})
			}
			walk(e.S, d+1)
		}
	}
	walk(id, 0)
	sort.Slice(out, func(i, j int) bool { return out[i].Via < out[j].Via })
	return out
}

func without(xs []string, x string) []string {
	var out []string
	for _, y := range xs {
		if y != x {
			out = append(out, y)
		}
	}
	return out
}

func realizesChain(g *graph.Graph, id string) []string {
	var out []string
	seen := map[string]bool{}
	var walk func(x string, d int)
	walk = func(x string, d int) {
		if d > 8 {
			return
		}
		for _, p := range []string{"realizes", "published-as"} {
			for _, e := range g.Out(x, p) {
				if e.Source != graph.SourceDerived && !seen[e.O] {
					seen[e.O] = true
					out = append(out, e.O)
					walk(e.O, d+1)
				}
			}
		}
	}
	walk(id, 0)
	return out
}

func describeTouch(g *graph.Graph, ws *load.Workspace, n *graph.Node, self string) ConceptTouch {
	c := ConceptTouch{Curie: n.CURIE, Kind: n.Kind, Tier: n.Tier, Doc: n.Doc, Published: n.Published, Proposed: n.Proposed, Deprecated: n.Deprecated, Internal: n.Internal, Criticality: n.Criticality, CriticalityFrom: n.CriticalityFrom}
	for _, e := range g.In(n.ID, "published-as") {
		c.PublishedBy = append(c.PublishedBy, g.Curie(e.S))
	}
	for _, o := range g.Objects(n.ID, "realizes") {
		c.Realizes = append(c.Realizes, g.Curie(o))
	}
	for _, o := range g.Objects(n.ID, "constrains") {
		c.Constrains = append(c.Constrains, g.Curie(o))
	}
	for _, s := range g.Subjects(n.ID, "constrains") {
		c.ConstrainedBy = append(c.ConstrainedBy, g.Curie(s))
	}
	for _, o := range g.Objects(n.ID, "guards") {
		c.Guards = append(c.Guards, g.Curie(o))
	}
	for _, s := range g.Subjects(n.ID, "guards") {
		label := g.Curie(s)
		if sn := g.Node(s); sn != nil && sn.Criticality != "" {
			label += " (" + sn.Criticality + ")"
		}
		c.GuardedBy = append(c.GuardedBy, label)
	}
	c.GuardedCode = without(uniq(append(rollup.AppliedCode(g, n.ID, "guards"), rollup.ManifestGuardedCode(g, n.ID)...)), self)
	sort.Strings(c.GuardedCode)
	for _, rel := range rollup.AppliesRelationsIn(ws.Vocab, "impact")[len(rollup.AppliesRelations):] {
		var to, by []string
		for _, o := range g.Objects(n.ID, rel) {
			to = append(to, g.Curie(o))
		}
		for _, s := range g.Subjects(n.ID, rel) {
			by = append(by, g.Curie(s))
		}
		code := without(rollup.AppliedCode(g, n.ID, rel), self)
		if len(to)+len(by)+len(code) > 0 {
			sort.Strings(to)
			sort.Strings(by)
			c.Applies = append(c.Applies, AppliedTouch{Relation: rel, To: to, By: by, Code: code})
		}
	}
	c.ConstrainedCode = without(rollup.AppliedCode(g, n.ID, "constrains"), self)
	sort.Strings(c.Guards)
	sort.Strings(c.GuardedBy)
	bset := map[string]bool{}
	others := map[string]bool{}
	for _, e := range g.In(n.ID, "") {
		sn := g.Node(e.S)
		if sn == nil {
			continue
		}
		switch sn.NodeType {
		case model.NodeBoundary:
			if e.P != "part-of" {
				bset[sn.CURIE+" "+e.P] = true
			}
		case model.NodeArtifact:
			if e.P == "tests" || e.P == "verifies" {
				c.Tests = append(c.Tests, sn.Path)
			} else if sn.Path != self && e.P != "may-modify" && e.P != "instructs" {
				others[sn.Path] = true
			}
		}
	}
	for k := range bset {
		c.Boundaries = append(c.Boundaries, k)
	}
	for k := range others {
		c.OtherArtifacts = append(c.OtherArtifacts, k)
	}
	sort.Strings(c.Boundaries)
	sort.Strings(c.OtherArtifacts)
	sort.Strings(c.Tests)
	c.Tests = uniq(c.Tests)
	if c.Deprecated {
		c.Warnings = append(c.Warnings, "deprecated; use "+n.ReplacedBy)
	}
	if n.Tier == model.TierCore && n.NodeType == model.NodeConcept && n.Imported {
		c.Warnings = append(c.Warnings, "imported core concept: extend it in your namespace, never edit it here")
	}
	c.TestsVia = testsVia(g, n.ID)
	// Warn exactly when gloss drift would call the concept untested.
	if len(c.Tests) == 0 && (n.Kind == "process" || n.Kind == "capability") && !rollup.Tested(g, n.ID) {
		c.Warnings = append(c.Warnings, "no tests or verifies link; consider adding one")
	}
	return c
}

// PlanTouch is what a change does to the plan (ADR 0034): the plan items
// the changed files close, and the open ones they work towards.
type PlanTouch struct {
	Closes []string `json:"closes,omitempty"`
	Open   []string `json:"open,omitempty"`
}

// PlanItemsTouched sorts the plan items of a drift report by whether the
// touched files close them (planned-closed) or still leave them open: the
// items on the concepts the files reach, on the boundaries they sit in, and,
// for an open planned concept, on the concepts those boundaries allow code
// to link to. Nil when the change touches no plan item.
func PlanItemsTouched(g *graph.Graph, ws *load.Workspace, rep *rollup.Report, ts []FileTouch) *PlanTouch {
	if rep == nil {
		return nil
	}
	concepts, bounds := map[string]bool{}, map[string]bool{}
	var allowed []rollup.Allowed
	for _, t := range ts {
		if t.Boundary != "" && !bounds[t.Boundary] {
			bounds[t.Boundary] = true
			if m := ws.ManifestFor(t.Path); m != nil {
				allowed = append(allowed, rollup.AllowedFor(g, ws, m))
			}
		}
		for _, c := range t.Concepts {
			concepts[c.Curie] = true
		}
	}
	allows := func(c string) bool {
		for _, a := range allowed {
			if len(a.Patterns) > 0 && a.Matches(c) {
				return true
			}
		}
		return false
	}
	p := &PlanTouch{}
	for _, f := range rep.Findings {
		reached := f.Concept != "" && concepts[f.Concept] || f.Boundary != "" && bounds[f.Boundary]
		if !reached && !(f.Kind != rollup.KindPlannedClosed && f.Concept != "" && allows(f.Concept)) {
			continue
		}
		switch f.Kind {
		case rollup.KindPlannedClosed:
			p.Closes = append(p.Closes, f.Msg)
		case rollup.KindPlanned, rollup.KindPlanOverdue:
			p.Open = append(p.Open, f.Msg)
		}
	}
	if len(p.Closes)+len(p.Open) == 0 {
		return nil
	}
	return p
}
