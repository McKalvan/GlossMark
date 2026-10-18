package gate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
)

// ConceptTouch describes one concept a change touches and what constrains it.
type ConceptTouch struct {
	Curie          string   `json:"curie"`
	Kind           string   `json:"kind,omitempty"`
	Tier           string   `json:"tier,omitempty"`
	Doc            string   `json:"doc,omitempty"`
	Published      string   `json:"published_as,omitempty"`    // this concept is published under that core name
	PublishedBy    []string `json:"published_by,omitempty"`    // impl concepts published under this core name
	Realizes       []string `json:"realizes,omitempty"`        // core concepts this stands for
	Constrains     []string `json:"constrains,omitempty"`      // what this constrains
	ConstrainedBy  []string `json:"constrained_by,omitempty"`  // constraints on this
	Boundaries     []string `json:"boundaries,omitempty"`      // boundaries that declare or derive it
	Tests          []string `json:"tests,omitempty"`           // artifacts that test or verify it
	OtherArtifacts []string `json:"other_artifacts,omitempty"` // other files linked to it (blast radius)
	Proposed       bool     `json:"proposed,omitempty"`
	Deprecated     bool     `json:"deprecated,omitempty"`
	Internal       bool     `json:"internal,omitempty"`
	// Criticality is the concept's effective governance level and where it comes from.
	Criticality     string   `json:"criticality,omitempty"`
	CriticalityFrom string   `json:"criticality_from,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

// FileTouch is what one changed file touches.
type FileTouch struct {
	Path     string         `json:"path"`
	Boundary string         `json:"boundary,omitempty"`
	Manifest string         `json:"manifest,omitempty"`
	Allowed  []string       `json:"allowed,omitempty"`
	Links    []LinkTouch    `json:"links"`
	Concepts []ConceptTouch `json:"concepts"`
	Gate     Class          `json:"gate"`
	Hints    []string       `json:"hints,omitempty"`
	// Criticality is the highest level among the file's boundary and the
	// concepts it touches; CriticalityFrom names the node that carries it.
	Criticality     string `json:"criticality,omitempty"`
	CriticalityFrom string `json:"criticality_from,omitempty"`
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
			ft.Boundary, ft.Manifest, ft.Allowed = m.Boundary, m.Path, m.Allowed
			if b := g.NodeByCurie(m.Boundary); b != nil && b.Criticality != "" {
				ft.Criticality, ft.CriticalityFrom = b.Criticality, b.CURIE
			}
		}
		seen := map[string]bool{}
		addConcept := func(id string) {
			n := g.Node(id)
			if n == nil || n.NodeType != model.NodeConcept || seen[id] {
				return
			}
			seen[id] = true
			ft.Concepts = append(ft.Concepts, describeTouch(g, n, f))
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
			for _, a := range g.NodesOf(model.NodeArtifact) {
				if a.Path != f {
					continue
				}
				for _, e := range g.Out(a.ID, "") {
					if e.Source != graph.SourceInline || e.Prov == nil {
						continue
					}
					ft.Links = append(ft.Links, LinkTouch{Line: e.Prov.Line, Symbol: e.Prov.Symbol, Relation: e.P, Target: g.Curie(e.O), Proposed: e.Prov.Proposed, MarkerID: e.Prov.ID})
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
			if model.CriticalityRank(c.Criticality) > model.CriticalityRank(ft.Criticality) {
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
		sort.Slice(ft.Links, func(i, j int) bool { return ft.Links[i].Line < ft.Links[j].Line })
		sort.Slice(ft.Concepts, func(i, j int) bool { return ft.Concepts[i].Curie < ft.Concepts[j].Curie })
		out = append(out, ft)
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

func describeTouch(g *graph.Graph, n *graph.Node, self string) ConceptTouch {
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
	if len(c.Tests) == 0 && (n.Kind == "process" || n.Kind == "capability") {
		c.Warnings = append(c.Warnings, "no tests or verifies link; consider adding one")
	}
	return c
}
