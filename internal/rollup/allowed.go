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
	// Entries says, for a derived set, why each pattern is in it, in the
	// order of Patterns.
	Entries []AllowedEntry
}

// Reasons a pattern is in a derived allowed set.
const (
	AllowedNamespace = "default-namespace" // the manifest's default-namespace glob
	AllowedDeclared  = "declared"          // a concept the manifest declares
	AllowedCovered   = "covered"           // covered by the declared concept From: an is-a or part-of descendant, or a concept that realizes it
	AllowedRealized  = "realized"          // a core concept the declared implementation concept From realizes
	AllowedListed    = "listed"            // written next to derived in allowed:
	AllowedNested    = "nested"            // allowed in the nested boundary From
)

// AllowedEntry is one pattern of a derived allowed set and why it is there.
type AllowedEntry struct {
	Pattern string `json:"pattern"`
	Reason  string `json:"reason"`
	// From is the declared concept a covered or realized entry comes from,
	// or the nested boundary a nested entry comes from.
	From string `json:"from,omitempty"`
}

// Matches reports whether a concept CURIE is inside the set. An empty set
// restricts nothing.
func (a Allowed) Matches(curie string) bool {
	return len(a.Patterns) == 0 || model.MatchAnyCurie(a.Patterns, curie)
}

// AllowedFor returns a manifest's effective allowed set. When the manifest
// writes allowed: that list is used as a deliberate narrowing. Otherwise the
// set is derived: the default namespace, every concept the manifest declares
// (realizes, holds, presents, hosts, produces, consumes, publishes,
// subscribes), every concept a declared one covers in drift (its is-a and
// part-of descendants along mixed chains, and the concepts that realize it
// or one of those), the core concepts a declared implementation concept
// realizes, and everything the boundaries nested inside it allow (a
// container holds its components). A manifest that declares nothing, has no
// default namespace and no children restricts nothing.
func AllowedFor(g *graph.Graph, ws *load.Workspace, m *model.Manifest) Allowed {
	if m == nil {
		return Allowed{}
	}
	if len(m.Allowed) > 0 && !m.ExtendsDerivedAllowed() {
		return Allowed{Patterns: []string(m.Allowed)}
	}
	d := &deriver{g: g, ws: ws, seen: map[*model.Manifest]bool{}}
	entries := d.derive(m)
	if len(entries) == 0 {
		return Allowed{}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		// Namespace globs first, then CURIEs alphabetically.
		pi, pj := entries[i].Pattern, entries[j].Pattern
		gi, gj := strings.HasSuffix(pi, ":*"), strings.HasSuffix(pj, ":*")
		if gi != gj {
			return gi
		}
		return pi < pj
	})
	out := Allowed{Derived: true, Entries: entries}
	for _, e := range entries {
		out.Patterns = append(out.Patterns, e.Pattern)
	}
	return out
}

type deriver struct {
	g    *graph.Graph
	ws   *load.Workspace
	seen map[*model.Manifest]bool
	// up maps a concept to every concept that covers it: itself, its is-a
	// ancestors, and everything it realizes (RealizesStar) with their is-a
	// ancestors. Built on first use.
	up map[string]map[string]bool
}

// derive returns m's own entries followed by its children's, one per pattern.
func (d *deriver) derive(m *model.Manifest) []AllowedEntry {
	if d.seen[m] {
		return nil
	}
	d.seen[m] = true
	set := map[string]bool{}
	var out []AllowedEntry
	add := func(e AllowedEntry) {
		if !set[e.Pattern] {
			set[e.Pattern] = true
			out = append(out, e)
		}
	}
	if m.DefaultNamespace != "" {
		add(AllowedEntry{Pattern: m.DefaultNamespace + ":*", Reason: AllowedNamespace})
	}
	for _, e := range d.expandDeclared(m) {
		add(e)
	}
	// allowed: [derived, x, y] adds x and y to the derived set.
	for _, p := range m.Allowed {
		if p != model.DerivedMarker {
			add(AllowedEntry{Pattern: p, Reason: AllowedListed})
		}
	}
	for _, c := range Children(d.ws, m) {
		if d.seen[c] {
			continue
		}
		var child []AllowedEntry
		if len(c.Allowed) > 0 && !c.ExtendsDerivedAllowed() {
			// A written list narrows the child; what the child declares still
			// belongs to the parent.
			for _, p := range c.Allowed {
				child = append(child, AllowedEntry{Pattern: p})
			}
			child = append(child, d.expandDeclared(c)...)
		} else {
			child = d.derive(c)
		}
		for _, e := range child {
			add(AllowedEntry{Pattern: e.Pattern, Reason: AllowedNested, From: c.Boundary})
		}
	}
	return out
}

// expandDeclared returns the concepts a manifest declares, the concepts each
// covers (the relation drift compares with, so check and drift agree) and
// the cores a declared implementation concept realizes.
func (d *deriver) expandDeclared(m *model.Manifest) []AllowedEntry {
	var out []AllowedEntry
	sets := m.DeclaredSets()
	rels := make([]string, 0, len(sets))
	for rel := range sets {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	// Every declared concept first, so one declared below another keeps its
	// own reason.
	var declared []*graph.Node
	for _, rel := range rels {
		for _, t := range sets[rel] {
			iri, _ := d.g.Expand(t)
			n := d.g.Node(iri)
			if n == nil {
				out = append(out, AllowedEntry{Pattern: t, Reason: AllowedDeclared})
				continue
			}
			out = append(out, AllowedEntry{Pattern: n.CURIE, Reason: AllowedDeclared})
			declared = append(declared, n)
		}
	}
	for _, n := range declared {
		for _, x := range d.coveredBy(n.ID) {
			out = append(out, AllowedEntry{Pattern: d.g.Curie(x), Reason: AllowedCovered, From: n.CURIE})
		}
		// What it realizes, not the wholes it is part of: declaring a part
		// does not admit links to the whole (ADR 0009).
		for _, core := range realizedBy(d.g, n.ID) {
			if cn := d.g.Node(core); cn != nil {
				out = append(out, AllowedEntry{Pattern: cn.CURIE, Reason: AllowedRealized, From: n.CURIE})
			}
		}
	}
	return out
}

// coveredBy returns the concepts other than s that a declared s covers: x
// with s == x or s an is-a ancestor of x or of something x realizes. That is
// Covers(s, x) without its clause for what s itself realizes, which
// expandDeclared adds as realized. Sorted.
func (d *deriver) coveredBy(s string) []string {
	if d.up == nil {
		d.up = map[string]map[string]bool{}
		for _, n := range d.g.NodesOf(model.NodeConcept) {
			up := map[string]bool{n.ID: true}
			for _, a := range d.g.Ancestors(n.ID) {
				up[a] = true
			}
			for _, core := range RealizesStar(d.g, n.ID) {
				up[core] = true
				for _, a := range d.g.Ancestors(core) {
					up[a] = true
				}
			}
			d.up[n.ID] = up
		}
	}
	var out []string
	for x, up := range d.up {
		if x != s && up[s] {
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

// Children returns the manifests nested inside m: in a directory under m's
// (unless they opt out with part-of: [], ADR 0026), or declaring part-of
// m's boundary.
func Children(ws *load.Workspace, m *model.Manifest) []*model.Manifest {
	var out []*model.Manifest
	for _, c := range ws.Manifests {
		if c == m {
			continue
		}
		optOut := len(c.PartOf) == 0 && c.Empty["part-of"]
		nested := !optOut && (m.Dir == "" && c.Dir != "" || m.Dir != "" && strings.HasPrefix(c.Dir, m.Dir+"/"))
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
