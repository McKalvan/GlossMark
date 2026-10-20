// @gloss implements gmi:RollupEngine scope=file

package rollup

import (
	"sort"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
)

// AppliesRelations are the ontology relations from a cross-cutting mechanism
// or a rule to the concepts it applies to: guards (ADR 0015) and constrains.
// Neither carries criticality into its targets.
var AppliesRelations = []string{"guards", "constrains"}

// AppliedBy returns the concepts that apply rel to c, or to a concept c
// stands for (RealizesStar), sorted by CURIE: the guards of a handler include
// those of the core process it realizes.
func AppliedBy(g *graph.Graph, c, rel string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range append([]string{c}, RealizesStar(g, c)...) {
		for _, e := range g.In(x, rel) {
			if e.Source == graph.SourceDerived || seen[e.S] {
				continue
			}
			if n := g.Node(e.S); n != nil && n.NodeType == model.NodeConcept {
				seen[e.S] = true
				out = append(out, e.S)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return g.Curie(out[i]) < g.Curie(out[j]) })
	return out
}

// AppliedCode returns the files whose links reach a concept that src applies
// rel to: an artifact linked to such a concept, or to a concept standing for
// it. Tests, documentation and agent files are left out; they are not the
// code the mechanism applies to.
func AppliedCode(g *graph.Graph, src, rel string) []string {
	targets := map[string]bool{}
	for _, e := range g.Out(src, rel) {
		if e.Source != graph.SourceDerived {
			targets[e.O] = true
		}
	}
	if len(targets) == 0 {
		return nil
	}
	reaches := map[string]bool{}
	hit := func(o string) bool {
		if v, ok := reaches[o]; ok {
			return v
		}
		v := targets[o]
		for _, x := range RealizesStar(g, o) {
			v = v || targets[x]
		}
		reaches[o] = v
		return v
	}
	files := map[string]bool{}
	for _, a := range g.NodesOf(model.NodeArtifact) {
		for _, e := range g.Out(a.ID, "") {
			if e.Source == graph.SourceDerived || !GuardedLink(e.P) {
				continue
			}
			if hit(e.O) {
				files[a.Path] = true
			}
		}
	}
	out := make([]string, 0, len(files))
	for f := range files {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// GuardedLink reports whether a link through relation p puts its artifact
// under what applies to the target: every link but tests, documentation,
// agent files and containment.
func GuardedLink(p string) bool {
	switch p {
	case "verifies", "tests", "documents", "instructs", "may-modify", "part-of":
		return false
	}
	return true
}
