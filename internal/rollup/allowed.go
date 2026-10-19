// @gloss implements gmi:Linter scope=file

package rollup

import (
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
)

// Allowed is the effective allowed set of a boundary: the patterns links
// under it may target, and whether they were declared or derived.
type Allowed struct {
	Patterns []string
	// Derived is true when the manifest has no allowed: list and the set was
	// derived from what it declares; false when it was written by hand. An
	// empty Patterns with Derived false means no restriction.
	Derived bool
}

// Matches reports whether a concept CURIE is inside the set. An empty set
// restricts nothing.
func (a Allowed) Matches(curie string) bool {
	return len(a.Patterns) == 0 || model.MatchAnyCurie(a.Patterns, curie)
}

// AllowedFor returns a manifest's effective allowed set. When the manifest
// writes allowed: that list is used as a deliberate narrowing. Otherwise the
// set is derived: the default namespace, every concept the manifest declares
// (realizes, produces, consumes, publishes, subscribes), their part-of and
// is-a descendants, the core concepts they realize, and everything the
// boundaries nested inside it allow (a container holds its components). A
// manifest that declares nothing, has no default namespace and no children
// restricts nothing.
func AllowedFor(g *graph.Graph, ws *load.Workspace, m *model.Manifest) Allowed {
	if m == nil {
		return Allowed{}
	}
	if len(m.Allowed) > 0 {
		return Allowed{Patterns: []string(m.Allowed)}
	}
	set := map[string]bool{}
	seen := map[*model.Manifest]bool{}
	derive(g, ws, m, set, seen)
	if len(set) == 0 {
		return Allowed{}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		// Namespace globs first, then CURIEs alphabetically.
		gi, gj := strings.HasSuffix(out[i], ":*"), strings.HasSuffix(out[j], ":*")
		if gi != gj {
			return gi
		}
		return out[i] < out[j]
	})
	return Allowed{Patterns: out, Derived: true}
}

// derive adds m's declared set and its children's allowed sets to set.
func derive(g *graph.Graph, ws *load.Workspace, m *model.Manifest, set map[string]bool, seen map[*model.Manifest]bool) {
	if seen[m] {
		return
	}
	seen[m] = true
	if m.DefaultNamespace != "" {
		set[m.DefaultNamespace+":*"] = true
	}
	expandDeclared(g, m, set)
	for _, c := range Children(ws, m) {
		if seen[c] {
			continue
		}
		if len(c.Allowed) > 0 {
			for _, p := range c.Allowed {
				set[p] = true
			}
			// A written list narrows the child; what the child declares still
			// belongs to the parent.
			expandDeclared(g, c, set)
			continue
		}
		derive(g, ws, c, set, seen)
	}
}

// expandDeclared adds the concepts a manifest declares, their part-of and
// is-a descendants and the cores they realize to set.
func expandDeclared(g *graph.Graph, m *model.Manifest, set map[string]bool) {
	for _, targets := range m.DeclaredSets() {
		for _, t := range targets {
			iri, _ := g.Expand(t)
			n := g.Node(iri)
			if n == nil {
				set[t] = true
				continue
			}
			set[n.CURIE] = true
			for _, rel := range []string{"part-of", "is-a"} {
				for _, d := range g.Closure(n.ID, rel, true) {
					if dn := g.Node(d); dn != nil && dn.NodeType == model.NodeConcept {
						set[dn.CURIE] = true
					}
				}
			}
			for _, core := range RealizesStar(g, n.ID) {
				if cn := g.Node(core); cn != nil {
					set[cn.CURIE] = true
				}
			}
		}
	}
}

// Children returns the manifests nested inside m: in a directory under m's,
// or declaring part-of m's boundary.
func Children(ws *load.Workspace, m *model.Manifest) []*model.Manifest {
	var out []*model.Manifest
	for _, c := range ws.Manifests {
		if c == m {
			continue
		}
		nested := m.Dir == "" && c.Dir != "" || m.Dir != "" && strings.HasPrefix(c.Dir, m.Dir+"/")
		partOf := false
		for _, p := range c.PartOf {
			if p == m.Boundary {
				partOf = true
			}
		}
		if nested || partOf {
			out = append(out, c)
		}
	}
	return out
}
