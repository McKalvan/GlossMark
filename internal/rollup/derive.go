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
		add := func(p, o string, via []string, path string, line int, proposed bool) {
			g.AddEdge(&graph.Edge{S: b.ID, P: p, O: o, Source: graph.SourceDerived, Prov: &graph.Prov{Rule: RuleRealization, Via: via, Path: path, Line: line, Proposed: proposed}})
		}
		for _, a := range arts {
			for _, e := range g.Out(a.ID, "implements") {
				if !IsConcrete(e.O) {
					continue
				}
				add("realizes", e.O, []string{g.Curie(a.ID)}, e.Prov.Path, e.Prov.Line, e.Prov.Proposed)
				// Impl concepts lift to core through their own realizes (transitively).
				for _, core := range RealizesStar(g, e.O) {
					add("realizes", core, []string{g.Curie(a.ID), g.Curie(e.O)}, e.Prov.Path, e.Prov.Line, e.Prov.Proposed)
				}
				// Intended relations declared on the implemented concept, and on the
				// core concepts it realizes, lift to the boundary. So do the surface
				// relations: code implementing pfi:FirePage, which presents
				// pf:FirePlanning, makes the boundary present it, not realize it.
				for _, c := range append([]string{e.O}, RealizesStar(g, e.O)...) {
					for _, rel := range SurfaceRelations {
						for _, x := range g.Objects(c, rel) {
							if IsConcrete(x) {
								g.AddEdge(&graph.Edge{S: b.ID, P: rel, O: x, Source: graph.SourceDerived, Prov: &graph.Prov{Rule: RuleRealization, Via: []string{g.Curie(a.ID), g.Curie(c)}, Path: e.Prov.Path, Line: e.Prov.Line, Proposed: e.Prov.Proposed}})
							}
						}
					}
					for _, rel := range LiftedRelations {
						for _, x := range g.Objects(c, rel) {
							if IsConcrete(x) {
								via := []string{g.Curie(a.ID), g.Curie(e.O)}
								if c != e.O {
									via = append(via, g.Curie(c))
								}
								g.AddEdge(&graph.Edge{S: b.ID, P: rel, O: x, Source: graph.SourceDerived, Prov: &graph.Prov{Rule: RuleRealization, Via: via, Path: e.Prov.Path, Line: e.Prov.Line, Proposed: e.Prov.Proposed}})
							}
						}
					}
				}
			}
			for _, e := range g.Out(a.ID, "represents") {
				if !IsConcrete(e.O) {
					continue
				}
				add("represents", e.O, []string{g.Curie(a.ID)}, e.Prov.Path, e.Prov.Line, e.Prov.Proposed)
				// A table, file or package that represents a concept means the
				// boundary holds its data: an entity's records, a measure's
				// stored values, an event log, a constraint's configured value.
				// A manifest declares holds: (ADR 0013).
				if on := g.Node(e.O); on != nil && on.NodeType == model.NodeConcept {
					add("holds", e.O, []string{g.Curie(a.ID)}, e.Prov.Path, e.Prov.Line, e.Prov.Proposed)
					for _, core := range RealizesStar(g, e.O) {
						add("holds", core, []string{g.Curie(a.ID), g.Curie(e.O)}, e.Prov.Path, e.Prov.Line, e.Prov.Proposed)
					}
				}
			}
			for _, rel := range SurfaceRelations {
				for _, e := range g.Out(a.ID, rel) {
					if IsConcrete(e.O) {
						add(rel, e.O, []string{g.Curie(a.ID)}, e.Prov.Path, e.Prov.Line, e.Prov.Proposed)
					}
				}
			}
			// Infrastructure, configuration and deployment of a resource or a
			// system-type realize it: a Terraform file that provisions the
			// registry satisfies a manifest that declares realizes: [impl:FoalRegistry].
			for _, rel := range InfraRelations {
				for _, e := range g.Out(a.ID, rel) {
					on := g.Node(e.O)
					if !IsConcrete(e.O) || on == nil || on.NodeType != model.NodeConcept || (on.Kind != "resource" && on.Kind != "system-type") {
						continue
					}
					add("realizes", e.O, []string{g.Curie(a.ID)}, e.Prov.Path, e.Prov.Line, e.Prov.Proposed)
					for _, core := range RealizesStar(g, e.O) {
						add("realizes", core, []string{g.Curie(a.ID), g.Curie(e.O)}, e.Prov.Path, e.Prov.Line, e.Prov.Proposed)
					}
				}
			}
			for _, rel := range LiftedRelations {
				for _, e := range g.Out(a.ID, rel) {
					if !IsConcrete(e.O) {
						continue
					}
					g.AddEdge(&graph.Edge{S: b.ID, P: rel, O: e.O, Source: graph.SourceDerived, Prov: &graph.Prov{Rule: RuleRealization, Via: []string{g.Curie(a.ID)}, Path: e.Prov.Path, Line: e.Prov.Line, Proposed: e.Prov.Proposed}})
					for _, core := range RealizesStar(g, e.O) {
						g.AddEdge(&graph.Edge{S: b.ID, P: rel, O: core, Source: graph.SourceDerived, Prov: &graph.Prov{Rule: RuleRealization, Via: []string{g.Curie(a.ID), g.Curie(e.O)}, Path: e.Prov.Path, Line: e.Prov.Line, Proposed: e.Prov.Proposed}})
					}
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
func RealizesStar(g *graph.Graph, c string) []string { return standsFor(g, c, true) }

// realizedBy is RealizesStar without the wholes: what c realizes or is
// published as, directly or through its is-a ancestors. It is the declared
// side of coverage, where a part never answers for its whole.
func realizedBy(g *graph.Graph, c string) []string { return standsFor(g, c, false) }

func standsFor(g *graph.Graph, c string, wholes bool) []string {
	seen := map[string]bool{c: true}
	// walked: each concept is expanded once, so an is-a or part-of cycle
	// ends the walk instead of re-entering it until the depth limit.
	walked := map[string]bool{}
	var out []string
	var walk func(x string, depth int)
	walk = func(x string, depth int) {
		if depth > 16 || walked[x] {
			return
		}
		walked[x] = true
		for _, p := range []string{"realizes", "published-as"} {
			for _, e := range g.Out(x, p) {
				if e.Source == graph.SourceDerived || !IsConcrete(e.O) {
					continue
				}
				if !seen[e.O] {
					seen[e.O] = true
					out = append(out, e.O)
					walk(e.O, depth+1)
				}
			}
		}
		// A part of a mechanism realizes the mechanism and what it realizes.
		for _, e := range g.Out(x, "part-of") {
			if n := g.Node(e.O); wholes && n != nil && n.NodeType == model.NodeConcept && e.Source != graph.SourceDerived && !seen[e.O] {
				seen[e.O] = true
				out = append(out, e.O)
				walk(e.O, depth+1)
			}
		}
		// A kind of a mechanism realizes what the mechanism realizes.
		for _, e := range g.Out(x, "is-a") {
			if n := g.Node(e.O); n != nil && n.NodeType == model.NodeConcept && e.Source != graph.SourceDerived {
				walk(e.O, depth+1)
			}
		}
	}
	walk(c, 0)
	sort.Strings(out)
	return out
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
