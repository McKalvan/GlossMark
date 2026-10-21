// @gloss implements gmi:RollupEngine scope=file

// Package rollup derives higher-level facts from the code-level graph and
// compares them with what manifests declare. The comparison is the drift signal.
package rollup

import (
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
)

// Rule names stamped into derived edges.
const (
	RuleContainment  = "containment"
	RuleRealization  = "realization-lift"
	RuleImplied      = "implied-relationship"
	RuleRealizesCore = "realizes-core"
)

// LiftedRelations are the artifact relations that lift to boundaries.
var LiftedRelations = []string{"produces", "consumes", "publishes", "subscribes"}

// SurfaceRelations lift to boundaries as themselves and never to realizes:
// a page presents a concept, an app shell hosts one; neither computes it.
var SurfaceRelations = []string{"presents", "hosts"}

// InfraRelations lift to realizes when their target is a resource or a system-type.
var InfraRelations = []string{"provisions", "configures", "deploys"}

// ConfigRelations lift to boundaries as themselves on a concept of any kind:
// a page that edits a constraint's value configures it without computing it
// (ADR 0012).
var ConfigRelations = []string{"configures"}

// IsConcrete reports whether an object IRI names a real node (not NEW/glob/unresolved).
func IsConcrete(iri string) bool {
	return !strings.HasPrefix(iri, "urn:gloss:new:") && !strings.HasPrefix(iri, "urn:gloss:glob:") && !strings.HasPrefix(iri, "urn:gloss:unresolved:")
}

// Derive removes previous derived edges and recomputes rules 1, 2 and 4:
// containment, realization lift (including impl→core) and implied
// relationships. Rule 3 (taxonomic lift) is applied by Compare and by views;
// rule 5 (generalization) by views.
func Derive(g *graph.Graph, vocab *model.Vocabulary) {
	g.RemoveEdges(func(e *graph.Edge) bool { return e.Source == graph.SourceDerived })

	// Rule 1: containment. Artifact part-of its boundary.
	for _, a := range g.NodesOf(model.NodeArtifact) {
		if a.Boundary == "" {
			continue
		}
		g.AddEdge(&graph.Edge{S: a.ID, P: "part-of", O: a.Boundary, Source: graph.SourceDerived, Prov: &graph.Prov{Rule: RuleContainment, Path: a.Path}})
	}

	// Artifacts under each boundary, including descendants' artifacts.
	contained := map[string][]*graph.Node{}
	for _, a := range g.NodesOf(model.NodeArtifact) {
		if a.Boundary == "" {
			continue
		}
		contained[a.Boundary] = append(contained[a.Boundary], a)
		for _, b := range g.PartOfChain(a.Boundary) {
			contained[b] = append(contained[b], a)
		}
	}

	// Rule 2: realization lift.
	for _, b := range g.NodesOf(model.NodeBoundary) {
		arts := contained[b.ID]
		// direct adds b rel o, from artifact a's link e.
		direct := func(p, o string, a *graph.Node, e *graph.Edge) {
			g.AddEdge(&graph.Edge{S: b.ID, P: p, O: o, Source: graph.SourceDerived, Prov: &graph.Prov{Rule: RuleRealization, Via: []string{g.Curie(a.ID)}, Path: e.Prov.Path, Line: e.Prov.Line, Proposed: e.Prov.Proposed}})
		}
		// lift adds b rel o, reached from the concept e links along path.
		lift := func(p, o string, a *graph.Node, e *graph.Edge, path []Hop) {
			via, steps := liftVia(g, a.ID, e.O, path)
			g.AddEdge(&graph.Edge{S: b.ID, P: p, O: o, Source: graph.SourceDerived, Prov: &graph.Prov{Rule: RuleRealization, Via: via, Steps: steps, Path: e.Prov.Path, Line: e.Prov.Line, Proposed: e.Prov.Proposed}})
		}
		// liftStar lifts every concept e's object stands for under rel.
		liftStar := func(rel string, a *graph.Node, e *graph.Edge) {
			paths := RealizesPaths(g, e.O)
			for _, core := range sortedKeys(paths) {
				lift(rel, core, a, e, paths[core])
			}
		}
		for _, a := range arts {
			// publishes-value lifts as implements: the line holding a
			// published setting is the code that implements it (ADR 0017).
			for _, e := range append(g.Out(a.ID, "implements"), g.Out(a.ID, "publishes-value")...) {
				if !IsConcrete(e.O) {
					continue
				}
				direct("realizes", e.O, a, e)
				// Impl concepts lift to core through their own realizes (transitively).
				liftStar("realizes", a, e)
				// Intended relations declared on the implemented concept, and on the
				// core concepts it realizes, lift to the boundary. So do the surface
				// relations: code implementing pfi:FirePage, which presents
				// pf:FirePlanning, makes the boundary present it, not realize it.
				paths := RealizesPaths(g, e.O)
				paths[e.O] = nil
				for _, c := range sortedKeys(paths) {
					for _, rel := range append(append([]string{}, SurfaceRelations...), LiftedRelations...) {
						for _, x := range g.Objects(c, rel) {
							if IsConcrete(x) {
								lift(rel, x, a, e, append(append([]Hop{}, paths[c]...), Hop{rel, x}))
							}
						}
					}
				}
			}
			for _, e := range g.Out(a.ID, "represents") {
				if !IsConcrete(e.O) {
					continue
				}
				direct("represents", e.O, a, e)
				// A table, file or package that represents a concept means the
				// boundary holds its data: an entity's records, a measure's
				// stored values, an event log, a constraint's configured value.
				// A manifest declares holds: (ADR 0013).
				if on := g.Node(e.O); on != nil && on.NodeType == model.NodeConcept {
					direct("holds", e.O, a, e)
					liftStar("holds", a, e)
				}
			}
			for _, rel := range append(append([]string{}, SurfaceRelations...), ConfigRelations...) {
				for _, e := range g.Out(a.ID, rel) {
					if on := g.Node(e.O); IsConcrete(e.O) && on != nil && on.NodeType == model.NodeConcept {
						direct(rel, e.O, a, e)
					}
				}
			}
			// Infrastructure, configuration and deployment of a resource or a
			// system-type realize it: a Terraform file that provisions the
			// registry satisfies a manifest that declares realizes: [circ:LoanStore].
			for _, rel := range InfraRelations {
				for _, e := range g.Out(a.ID, rel) {
					on := g.Node(e.O)
					if !IsConcrete(e.O) || on == nil || on.NodeType != model.NodeConcept || (on.Kind != "resource" && on.Kind != "system-type") {
						continue
					}
					direct("realizes", e.O, a, e)
					liftStar("realizes", a, e)
				}
			}
			for _, rel := range LiftedRelations {
				for _, e := range g.Out(a.ID, rel) {
					if !IsConcrete(e.O) {
						continue
					}
					direct(rel, e.O, a, e)
					liftStar(rel, a, e)
				}
			}
			// Test relations lift too, so "implemented but untested" can be judged per boundary.
			for _, rel := range []string{"tests", "verifies"} {
				for _, e := range g.Out(a.ID, rel) {
					if IsConcrete(e.O) {
						g.AddEdge(&graph.Edge{S: b.ID, P: rel, O: e.O, Source: graph.SourceDerived, Prov: &graph.Prov{Rule: RuleRealization, Via: []string{g.Curie(a.ID)}, Path: e.Prov.Path, Line: e.Prov.Line}})
					}
				}
			}
		}
	}

	// Rule 4: implied relationships. A publishes/subscribes or produces/consumes
	// pair on the same concept between two boundaries implies depends-on
	// (consumer → producer) unless one is declared.
	type pair struct{ from, to string }
	implied := map[pair][]string{}
	for _, c := range g.NodesOf(model.NodeConcept) {
		producers := boundarySubjects(g, c.ID, "publishes", "produces")
		consumers := boundarySubjects(g, c.ID, "subscribes", "consumes")
		for _, s := range consumers {
			for _, p := range producers {
				if s == p || isAncestorBoundary(g, s, p) || isAncestorBoundary(g, p, s) {
					continue
				}
				implied[pair{s, p}] = append(implied[pair{s, p}], g.Curie(c.ID))
			}
		}
	}
	keys := make([]pair, 0, len(implied))
	for k := range implied {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].from != keys[j].from {
			return keys[i].from < keys[j].from
		}
		return keys[i].to < keys[j].to
	})
	for _, k := range keys {
		if len(g.Out(k.from, "depends-on")) > 0 {
			declared := false
			for _, e := range g.Out(k.from, "depends-on") {
				if e.O == k.to && e.Source == graph.SourceDeclared {
					declared = true
				}
			}
			if declared {
				continue
			}
		}
		via := implied[k]
		sort.Strings(via)
		g.AddEdge(&graph.Edge{S: k.from, P: "depends-on", O: k.to, Source: graph.SourceDerived, Prov: &graph.Prov{Rule: RuleImplied, Via: via}})
	}

	// Governance: effective criticality of every concept, boundary and artifact.
	Criticality(g)
}

func boundarySubjects(g *graph.Graph, o string, preds ...string) []string {
	set := map[string]bool{}
	for _, p := range preds {
		for _, e := range g.In(o, p) {
			if n := g.Node(e.S); n != nil && n.NodeType == model.NodeBoundary {
				set[e.S] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func isAncestorBoundary(g *graph.Graph, anc, b string) bool {
	for _, x := range g.PartOfChain(b) {
		if x == anc {
			return true
		}
	}
	return false
}

// RealizesStar returns every concept c stands for: what it realizes or is
// published as (transitively), the wholes it is part of and what they
// realize (implementing a part implements the whole), and what its is-a
// ancestors realize (a CsvImporter is-a Importer realizes pf:Importing
// without repeating the line). Neither is-a ancestors nor c itself are
// included.
func RealizesStar(g *graph.Graph, c string) []string { return standsFor(g, c, allWholes) }

// realizedBy is RealizesStar without the wholes: what c realizes or is
// published as, directly or through its is-a ancestors. It is the declared
// side of coverage, where a part never answers for its whole.
func realizedBy(g *graph.Graph, c string) []string { return standsFor(g, c, noWholes) }

// realizedByMechanism is realizedBy plus what a mechanism's wholes realize: a
// part of an implementation concept realizes what that concept realizes, but
// a core concept reached on the way does not stand for the areas it is part
// of. It is the set an untested implementation concept is reported under, so
// an untested domain area never hides an untested part (ADR 0025, section 13).
func realizedByMechanism(g *graph.Graph, c string) []string {
	return standsFor(g, c, func(x string) bool { n := g.Node(x); return n != nil && n.Tier == model.TierImpl })
}

// Which concepts' part-of wholes a realization walk climbs.
var (
	allWholes = func(string) bool { return true }
	noWholes  = func(string) bool { return false }
)

func standsFor(g *graph.Graph, c string, wholes func(string) bool) []string {
	paths := standsForPaths(g, c, wholes)
	out := make([]string, 0, len(paths))
	for k := range paths {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Hop is one step of a derivation: the relation followed and the concept it
// leads to.
type Hop struct{ Rel, To string }

// RealizesPaths is RealizesStar with, for every concept c stands for, the
// shortest chain of hops from c to it: is-a steps up to the ancestor that
// realizes, part-of steps up to the whole, and the realizes or published-as
// step itself. Among chains of one length the first by relation and IRI wins.
func RealizesPaths(g *graph.Graph, c string) map[string][]Hop { return standsForPaths(g, c, allWholes) }

func standsForPaths(g *graph.Graph, c string, wholes func(string) bool) map[string][]Hop {
	out := map[string][]Hop{}
	// walked: each concept is expanded once (breadth first, so its path is a
	// shortest one), and an is-a or part-of cycle ends the walk.
	walked := map[string][]Hop{c: nil}
	queue := []string{c}
	extend := func(path []Hop, rel, to string) []Hop {
		return append(append([]Hop{}, path...), Hop{rel, to})
	}
	for len(queue) > 0 {
		x := queue[0]
		queue = queue[1:]
		path := walked[x]
		if len(path) > 16 {
			continue
		}
		visit := func(rel, to string, output bool) {
			p := extend(path, rel, to)
			if output && to != c {
				if _, ok := out[to]; !ok {
					out[to] = p
				}
			}
			if _, ok := walked[to]; !ok {
				walked[to] = p
				queue = append(queue, to)
			}
		}
		for _, p := range []string{"realizes", "published-as"} {
			for _, o := range g.Objects(x, p) {
				if IsConcrete(o) && declaredEdge(g, x, p, o) {
					visit(p, o, true)
				}
			}
		}
		// A part of a mechanism realizes the mechanism and what it realizes.
		if wholes(x) {
			for _, o := range g.Objects(x, "part-of") {
				if n := g.Node(o); n != nil && n.NodeType == model.NodeConcept && declaredEdge(g, x, "part-of", o) {
					visit("part-of", o, true)
				}
			}
		}
		// A kind of a mechanism realizes what the mechanism realizes.
		for _, o := range g.Objects(x, "is-a") {
			if n := g.Node(o); n != nil && n.NodeType == model.NodeConcept && declaredEdge(g, x, "is-a", o) {
				visit("is-a", o, false)
			}
		}
	}
	return out
}

// declaredEdge reports whether (s, p, o) has an edge that is not derived.
func declaredEdge(g *graph.Graph, s, p, o string) bool {
	for _, e := range g.Out(s, p) {
		if e.O == o && e.Source != graph.SourceDerived {
			return true
		}
	}
	return false
}

// liftVia returns the provenance of a fact derived from artifact a linking
// to concept linked and reaching its target along path: via lists the
// artifact, the linked concept and every concept passed through (not the
// target), steps the hops from linked to the target as "rel CURIE".
func liftVia(g *graph.Graph, a, linked string, path []Hop) (via, steps []string) {
	via = []string{g.Curie(a), g.Curie(linked)}
	for i, h := range path {
		if i < len(path)-1 {
			via = append(via, g.Curie(h.To))
		}
		steps = append(steps, h.Rel+" "+g.Curie(h.To))
	}
	return via, steps
}

// Covers reports whether declared concept s accounts for derived concept d:
// identity, taxonomic lift (s subsumes d through is-a*), realization lift
// (d realizes* s, where a part realizes its whole), or s subsumes something
// d realizes. Both hierarchies point one way: a declared whole covers a
// derived part, as a declared supertype covers a derived subtype, and a
// declared part never covers a derived whole (ADR 0009).
func Covers(g *graph.Graph, s, d string) bool {
	if s == d || g.Subsumes(s, d) {
		return true
	}
	for _, core := range RealizesStar(g, d) {
		if core == s || g.Subsumes(s, core) {
			return true
		}
	}
	// A declared impl concept covers the core concept it realizes when
	// derived names the core; not the wholes it is part of.
	for _, core := range realizedBy(g, s) {
		if core == d {
			return true
		}
	}
	return false
}
