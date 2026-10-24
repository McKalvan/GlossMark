// @gloss implements gmi:RollupEngine scope=file

package rollup

import (
	"fmt"
	"slices"
	"sort"
	"strings"

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

// ManifestGuard is one guarded-by entry: every artifact under Dir, the
// directory of Boundary's manifest, is guarded by Guard (ADR 0015 amendment).
type ManifestGuard struct {
	Boundary, Dir, Guard string // node IDs, and the manifest's directory ("" at the root)
}

// ManifestGuards lists the guarded-by declarations of every boundary.
func ManifestGuards(g *graph.Graph) []ManifestGuard {
	var out []ManifestGuard
	for _, b := range g.NodesOf(model.NodeBoundary) {
		for _, e := range g.Out(b.ID, "guarded-by") {
			if n := g.Node(e.O); n != nil && n.NodeType == model.NodeConcept {
				out = append(out, ManifestGuard{Boundary: b.ID, Dir: cleanDir(b.Dir), Guard: e.O})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir < out[j].Dir
		}
		return g.Curie(out[i].Guard) < g.Curie(out[j].Guard)
	})
	return out
}

// Under reports whether path is in the directory dir ("" is the root).
func Under(path, dir string) bool {
	return dir == "" || path == dir || len(path) > len(dir) && path[:len(dir)] == dir && path[len(dir)] == '/'
}

func cleanDir(d string) string {
	if d == "." || d == "./" {
		return ""
	}
	for len(d) > 0 && d[len(d)-1] == '/' {
		d = d[:len(d)-1]
	}
	return d
}

// PathGuards returns the guards a manifest declares over path with
// guarded-by, each once, sorted by CURIE.
func PathGuards(g *graph.Graph, path string) []string {
	seen := map[string]bool{}
	var out []string
	for _, mg := range ManifestGuards(g) {
		if Under(path, mg.Dir) && !seen[mg.Guard] {
			seen[mg.Guard] = true
			out = append(out, mg.Guard)
		}
	}
	sort.Slice(out, func(i, j int) bool { return g.Curie(out[i]) < g.Curie(out[j]) })
	return out
}

// ManifestGuardedCode returns the files a manifest puts under guard with
// guarded-by: every artifact under its directory, tests and documents
// excepted as for the ontology form.
func ManifestGuardedCode(g *graph.Graph, guard string) []string {
	var dirs []string
	for _, mg := range ManifestGuards(g) {
		if mg.Guard == guard {
			dirs = append(dirs, mg.Dir)
		}
	}
	if len(dirs) == 0 {
		return nil
	}
	files := map[string]bool{}
	for _, a := range g.NodesOf(model.NodeArtifact) {
		in := false
		for _, d := range dirs {
			in = in || Under(a.Path, d)
		}
		if !in || !guardedArtifact(g, a.ID) {
			continue
		}
		files[a.Path] = true
	}
	out := make([]string, 0, len(files))
	for f := range files {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// guardedArtifact reports whether an artifact has a link that puts it under
// what applies to its target (GuardedLink): a test or a document is not
// guarded code.
func guardedArtifact(g *graph.Graph, id string) bool {
	for _, e := range g.Out(id, "") {
		if e.Source != graph.SourceDerived && GuardedLink(e.P) {
			return true
		}
	}
	return false
}

// AppliesRelationsIn are AppliesRelations plus the custom relations an
// ontology opts into the same treatment with surfaces: [impact, context]:
// the ones whose surfaces include surface.
func AppliesRelationsIn(v *model.Vocabulary, surface string) []string {
	out := append([]string{}, AppliesRelations...)
	if v == nil {
		return out
	}
	for _, name := range v.Names() {
		r := v.Get(name)
		if r == nil || name == "guards" || name == "constrains" {
			continue
		}
		for _, s := range r.Surfaces {
			if s == surface {
				out = append(out, name)
				break
			}
		}
	}
	return out
}

// unguarded reports, for each guarded-by declaration, a handler under the
// boundary's directory that carries none of the guard's decorators while
// another handler there does. The guard's decorators are named by the
// symbols of the artifacts that implement it (or a concept realizing it). A
// guard applied by middleware decorates no handler, so it gets no finding:
// the declaration is taken at its word.
func (c *comparer) unguarded() {
	for _, mg := range ManifestGuards(c.g) {
		names := guardSymbols(c.g, mg.Guard)
		if len(names) == 0 {
			continue
		}
		var with []string
		var without []*graph.Node
		for _, a := range c.g.NodesOf(model.NodeArtifact) {
			if !Under(a.Path, mg.Dir) || a.Anchor == nil || a.Anchor.Scope != "symbol" || a.Anchor.Symbol == "" || !handler(c.g, a.ID, mg.Guard) {
				continue
			}
			if decoratedBy(a.Anchor.Decorators, names) {
				with = append(with, a.Path+"#"+a.Anchor.Symbol)
			} else {
				without = append(without, a)
			}
		}
		if len(with) == 0 {
			continue
		}
		sort.Strings(with)
		guard, b := c.g.Curie(mg.Guard), c.g.Curie(mg.Boundary)
		for _, a := range without {
			sym := a.Path + "#" + a.Anchor.Symbol
			c.add(Finding{ID: KindUnguarded + ":" + b + ":" + guard + ":" + sym, Kind: KindUnguarded, Severity: "warning", Boundary: b, Concept: guard, Path: a.Path, Line: a.Anchor.StartLine, Via: with,
				Msg: fmt.Sprintf("%s is under %s, guarded-by %s, but carries none of its decorators (%s) that %d other handler(s) there carry: add the guard, or move the handler out of the boundary", sym, b, guard, strings.Join(names, ", "), len(with))})
		}
	}
}

// guardSymbols are the bare names of the declarations that implement a
// guard concept or a concept realizing it: the names its decorators use.
func guardSymbols(g *graph.Graph, guard string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range append([]string{guard}, g.Subjects(guard, "realizes")...) {
		for _, e := range g.In(x, "implements") {
			a := g.Node(e.S)
			if a == nil || a.NodeType != model.NodeArtifact || a.Anchor == nil || a.Anchor.Symbol == "" {
				continue
			}
			s := a.Anchor.Symbol
			if i := strings.LastIndexAny(s, ".:"); i >= 0 {
				s = s[i+1:]
			}
			if s != "" && !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	sort.Strings(out)
	return out
}

// handler reports whether an artifact is guarded code with respect to
// guard: it links to a concept through a guarded relation, and is not the
// guard's own implementation.
func handler(g *graph.Graph, id, guard string) bool {
	ok := false
	for _, e := range g.Out(id, "") {
		if e.Source == graph.SourceDerived || !GuardedLink(e.P) {
			continue
		}
		if e.O == guard || slices.Contains(RealizesStar(g, e.O), guard) {
			return false
		}
		if n := g.Node(e.O); n != nil && n.NodeType == model.NodeConcept {
			ok = true
		}
	}
	return ok
}

// decoratedBy reports whether a decorator stack names one of names as a
// whole identifier (@require_bearer, @auth.require_bearer(scopes=...)).
func decoratedBy(decorators, names []string) bool {
	for _, d := range decorators {
		for _, n := range names {
			for i := strings.Index(d, n); i >= 0; {
				end := i + len(n)
				if (i == 0 || !identRune(d[i-1])) && (end == len(d) || !identRune(d[end])) {
					return true
				}
				j := strings.Index(d[i+1:], n)
				if j < 0 {
					break
				}
				i += 1 + j
			}
		}
	}
	return false
}

func identRune(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}
