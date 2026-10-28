// @gloss implements gmi:GraphBrowser scope=file

// Package browse is the read-only graph browser behind gloss review node,
// the Browse tab of the review terminal and the browse_node MCP tool (ADR
// 0033): any node, concept, boundary or artifact, with every edge in and out
// grouped by relation, and the drift findings on it. It changes nothing.
package browse

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// Node is one node of the graph with its neighbourhood.
type Node struct {
	Ref         string    `json:"ref"` // what to pass back to Lookup: a CURIE, path or path#symbol
	IRI         string    `json:"iri"`
	Type        string    `json:"type"` // concept | boundary | artifact | namespace | ...
	Kind        string    `json:"kind,omitempty"`
	Tier        string    `json:"tier,omitempty"`
	Label       string    `json:"label,omitempty"`
	Doc         string    `json:"doc,omitempty"`
	Path        string    `json:"path,omitempty"`
	Line        int       `json:"line,omitempty"`
	Criticality string    `json:"criticality,omitempty"`
	Proposed    bool      `json:"proposed,omitempty"`
	Out         []Group   `json:"out,omitempty"`
	In          []Group   `json:"in,omitempty"`
	Findings    []Finding `json:"findings,omitempty"`
}

// Group is the neighbours one relation reaches, in one direction.
type Group struct {
	Relation string     `json:"relation"`
	Nodes    []Neighbor `json:"nodes"`
}

// Neighbor is a node at the other end of an edge.
type Neighbor struct {
	Ref    string `json:"ref"`
	Label  string `json:"label"`
	Type   string `json:"type"`
	Kind   string `json:"kind,omitempty"`
	Source string `json:"source,omitempty"` // inline | manifest | declared | derived | inferred
	// From is where a manifest link is written (path:line of its links:
	// entry), since Path and Line name the code it maps (ADR 0041).
	From string `json:"from,omitempty"`
	// Proposed: the edge comes from a marker still awaiting review.
	Proposed bool   `json:"proposed,omitempty"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
}

// Finding is a drift finding on the node.
type Finding struct {
	Kind string `json:"kind"`
	Msg  string `json:"msg"`
}

// Ref is how a node is named for Lookup: a CURIE for concepts and
// boundaries, the path (or path#symbol) for artifacts, else the IRI.
func Ref(g *graph.Graph, id string) string {
	if strings.HasPrefix(id, graph.ArtifactPrefix) {
		return strings.TrimPrefix(id, graph.ArtifactPrefix)
	}
	if n := g.Node(id); n != nil && n.CURIE != "" {
		return n.CURIE
	}
	return g.Curie(id)
}

// resolve finds the node a ref names: a CURIE, an IRI, a repo-relative path
// (or path#symbol), or a path that holds artifacts below the file.
func resolve(g *graph.Graph, ref string) *graph.Node {
	ref = strings.TrimPrefix(strings.TrimSpace(ref), "./")
	if n := g.NodeByCurie(ref); n != nil {
		return n
	}
	if n := g.Node(ref); n != nil {
		return n
	}
	if n := g.Node(graph.ArtifactPrefix + ref); n != nil {
		return n
	}
	if iri, ok := g.Expand(ref); ok {
		if n := g.Node(iri); n != nil {
			return n
		}
	}
	if as := artifactsIn(g, ref); len(as) == 1 {
		return as[0]
	}
	return nil
}

// artifactsIn are the artifacts (declarations) of a file with no file node.
func artifactsIn(g *graph.Graph, path string) []*graph.Node {
	var out []*graph.Node
	for _, n := range g.NodesOf(model.NodeArtifact) {
		if n.Path == path {
			out = append(out, n)
		}
	}
	return out
}

// Lookup returns a node with its edges and findings; rep may be nil.
func Lookup(g *graph.Graph, rep *rollup.Report, ref string) (*Node, error) {
	n := resolve(g, ref)
	if n == nil {
		// A file whose links are on its declarations: list them.
		if as := artifactsIn(g, strings.TrimPrefix(ref, "./")); len(as) > 1 {
			f := &Node{Ref: as[0].Path, Type: "file", Path: as[0].Path, Label: as[0].Path}
			grp := Group{Relation: "declares"}
			for _, a := range as {
				grp.Nodes = append(grp.Nodes, Neighbor{Ref: Ref(g, a.ID), Label: g.Label(a.ID), Type: string(a.NodeType), Kind: a.Kind, Path: a.Path, Line: a.Line})
			}
			sort.Slice(grp.Nodes, func(i, j int) bool { return grp.Nodes[i].Line < grp.Nodes[j].Line })
			f.Out = []Group{grp}
			return f, nil
		}
		return nil, fmt.Errorf("no node %q: name a concept or boundary (ns:Name), a file (path or path#symbol), or an IRI", ref)
	}
	out := &Node{Ref: Ref(g, n.ID), IRI: n.ID, Type: string(n.NodeType), Kind: n.Kind, Tier: n.Tier, Label: g.Label(n.ID), Doc: n.Doc,
		Path: firstNonEmpty(n.Path, n.File), Line: n.Line, Criticality: n.Criticality, Proposed: n.Proposed}
	out.Out = groups(g, g.Out(n.ID, ""), func(e *graph.Edge) string { return e.O })
	out.In = groups(g, g.In(n.ID, ""), func(e *graph.Edge) string { return e.S })
	if rep != nil {
		for _, f := range rep.Findings {
			if f.Kind == rollup.KindConsistent || f.Kind == rollup.KindOverlap {
				continue
			}
			if (out.Ref != "" && (f.Concept == out.Ref || f.Boundary == out.Ref)) || (n.NodeType == model.NodeArtifact && f.Path != "" && f.Path == n.Path) {
				out.Findings = append(out.Findings, Finding{Kind: f.Kind, Msg: f.Msg})
			}
		}
	}
	return out, nil
}

func groups(g *graph.Graph, edges []*graph.Edge, other func(*graph.Edge) string) []Group {
	byRel := map[string]map[string]Neighbor{}
	for _, e := range edges {
		id := other(e)
		nb := Neighbor{Ref: Ref(g, id), Label: g.Label(id), Source: e.Source}
		if on := g.Node(id); on != nil {
			nb.Type, nb.Kind = string(on.NodeType), on.Kind
		}
		if e.Prov != nil {
			nb.Proposed, nb.Path, nb.Line, nb.From = e.Prov.Proposed, e.Prov.Path, e.Prov.Line, e.Prov.From
		}
		if byRel[e.P] == nil {
			byRel[e.P] = map[string]Neighbor{}
		}
		// One row per neighbour: the first edge, inline before derived,
		// stands for the rest.
		if prev, ok := byRel[e.P][id]; !ok || (prev.Source == graph.SourceDerived && e.Source != graph.SourceDerived) {
			byRel[e.P][id] = nb
		}
	}
	rels := make([]string, 0, len(byRel))
	for r := range byRel {
		rels = append(rels, r)
	}
	sort.Strings(rels)
	var out []Group
	for _, r := range rels {
		var ns []Neighbor
		for _, nb := range byRel[r] {
			ns = append(ns, nb)
		}
		sort.Slice(ns, func(i, j int) bool { return ns[i].Ref < ns[j].Ref })
		out = append(out, Group{Relation: r, Nodes: ns})
	}
	return out
}

// Index is where browsing starts: every boundary, then every concept the
// workspace owns (imported ones are reached through their links).
func Index(g *graph.Graph) []Neighbor {
	var bs, cs []Neighbor
	for _, n := range g.Nodes() {
		nb := Neighbor{Ref: Ref(g, n.ID), Label: g.Label(n.ID), Type: string(n.NodeType), Kind: n.Kind, Path: firstNonEmpty(n.File, n.Path), Line: n.Line}
		switch {
		case n.NodeType == model.NodeBoundary:
			bs = append(bs, nb)
		case n.NodeType == model.NodeConcept && !n.Imported:
			cs = append(cs, nb)
		}
	}
	sort.Slice(bs, func(i, j int) bool { return bs[i].Ref < bs[j].Ref })
	sort.Slice(cs, func(i, j int) bool { return cs[i].Ref < cs[j].Ref })
	return append(bs, cs...)
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}
