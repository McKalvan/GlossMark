// @gloss implements gmi:ViewProjector scope=file

// Package views projects the one graph into what a given observer sees: a
// declared view over tier × zoom, with implementation folded into what it
// realizes, part-of expanded to a depth, and sets lifted to their least
// common subsumer. PR impact is "which views changed".
package views

import (
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
)

// Projection is one rendered view.
type Projection struct {
	Name  string  `json:"name"`
	Doc   string  `json:"doc,omitempty"`
	Zoom  string  `json:"zoom"`
	Nodes []PNode `json:"nodes"`
	Edges []PEdge `json:"edges"`
}

// PNode is a node as the view shows it.
type PNode struct {
	ID          string   `json:"id"` // CURIE
	Label       string   `json:"label"`
	Type        string   `json:"type"` // concept | boundary | artifact
	Kind        string   `json:"kind,omitempty"`
	Tier        string   `json:"tier,omitempty"`
	NS          string   `json:"ns,omitempty"`
	Branch      string   `json:"branch,omitempty"` // is-a root, for colouring
	Parent      string   `json:"parent,omitempty"` // part-of parent shown in this view
	Depth       int      `json:"depth"`
	Published   bool     `json:"published,omitempty"`
	PublishedAs string   `json:"publishedAs,omitempty"`
	Folded      []string `json:"folded,omitempty"` // CURIEs collapsed into this node
	Internal    bool     `json:"internal,omitempty"`
	Doc         string   `json:"doc,omitempty"`
	Value       any      `json:"value,omitempty"`
	Path        string   `json:"path,omitempty"`
	Imported    bool     `json:"imported,omitempty"`
}

// PEdge is an edge as the view shows it.
type PEdge struct {
	S        string   `json:"s"`
	P        string   `json:"p"`
	O        string   `json:"o"`
	Source   string   `json:"source"` // declared | inline | derived | mixed
	Proposed bool     `json:"proposed,omitempty"`
	Count    int      `json:"count,omitempty"`
	Lifted   bool     `json:"lifted,omitempty"`
	Via      []string `json:"via,omitempty"`
}

func (e PEdge) key() string { return e.S + "|" + e.P + "|" + e.O }

var zoomIndex = map[string]int{"landscape": 0, "system": 1, "container": 2, "component": 3, "code": 4}
var levelIndex = map[string]int{"domain": 0, "system": 1, "container": 2, "component": 3}

// ProjectAll renders every declared view, sorted by name.
func ProjectAll(g *graph.Graph, vs *model.Views) []*Projection {
	names := make([]string, 0, len(vs.Views))
	for n := range vs.Views {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []*Projection
	for _, n := range names {
		out = append(out, Project(g, n, vs.Views[n]))
	}
	return out
}

// Project renders one view.
func Project(g *graph.Graph, name string, v *model.View) *Projection {
	p := &projector{g: g, v: v, fold: map[string][]string{}, included: map[string]bool{}}
	zoom := v.Zoom
	if zoom == "" {
		zoom = "component"
	}
	pr := &Projection{Name: name, Doc: v.Doc, Zoom: zoom}
	p.zoom = zoomIndex[zoom]
	p.concepts()
	p.boundaries()
	if p.zoom >= zoomIndex["code"] {
		p.artifacts()
	}
	p.edges()
	pr.Nodes = p.nodes
	pr.Edges = p.outEdges
	sort.Slice(pr.Nodes, func(i, j int) bool { return pr.Nodes[i].ID < pr.Nodes[j].ID })
	sort.Slice(pr.Edges, func(i, j int) bool { return pr.Edges[i].key() < pr.Edges[j].key() })
	return pr
}

type projector struct {
	g        *graph.Graph
	v        *model.View
	zoom     int
	fold     map[string][]string // IRI → IRIs it is shown as (empty = hidden)
	included map[string]bool     // IRI → shown as itself
	nodes    []PNode
	byID     map[string]int
	outEdges []PEdge
}

func (p *projector) matchesInclude(curie string) bool {
	if len(p.v.Include) == 0 {
		return true
	}
	if model.MatchAnyCurie(p.v.Exclude, curie) {
		return false
	}
	return model.MatchAnyCurie(p.v.Include, curie)
}

func (p *projector) collapseRule(curie string) string {
	for pat, how := range p.v.Collapse {
		if model.MatchCurie(pat, curie) {
			return how
		}
	}
	return ""
}

// target returns the IRIs a concept is shown as in this view.
func (p *projector) target(iri string, depth int) []string {
	if t, ok := p.fold[iri]; ok {
		return t
	}
	if depth > 16 {
		return nil
	}
	n := p.g.Node(iri)
	if n == nil {
		return nil
	}
	p.fold[iri] = nil // cycle guard
	var out []string
	rule := p.collapseRule(n.CURIE)
	switch {
	case rule != "":
		// via realizes: fold into what it realizes (published-as first).
		var ups []string
		for _, e := range p.g.Out(iri, "published-as") {
			ups = append(ups, e.O)
		}
		if len(ups) == 0 {
			for _, e := range p.g.Out(iri, "realizes") {
				if e.Source != graph.SourceDerived {
					ups = append(ups, e.O)
				}
			}
		}
		if len(ups) == 0 {
			// A part of a mechanism folds where the mechanism folds.
			for _, e := range p.g.Out(iri, "part-of") {
				if pn := p.g.Node(e.O); pn != nil && pn.NodeType == model.NodeConcept {
					ups = append(ups, e.O)
				}
			}
		}
		for _, u := range ups {
			out = append(out, p.target(u, depth+1)...)
		}
	case p.matchesInclude(n.CURIE):
		out = []string{iri}
	default:
		// Not included and not collapsed: hidden. Published impl concepts still surface under their core name.
		if n.Published != "" {
			if pn := p.g.NodeByCurie(n.Published); pn != nil {
				out = append(out, p.target(pn.ID, depth+1)...)
			}
		}
	}
	out = uniq(out)
	p.fold[iri] = out
	return out
}

func (p *projector) concepts() {
	p.byID = map[string]int{}
	maxDepth := -1
	if p.v.Expand != nil {
		if d, ok := p.v.Expand["part-of"]; ok {
			maxDepth = d
		}
	}
	// Concept part-of depth and folding for expand.
	conceptDepth := func(iri string) (int, string) {
		d := 0
		cur := iri
		var anc []string
		for {
			var parent string
			for _, e := range p.g.Out(cur, "part-of") {
				if pn := p.g.Node(e.O); pn != nil && pn.NodeType == model.NodeConcept && e.Source != graph.SourceDerived {
					parent = e.O
					break
				}
			}
			if parent == "" || d > 16 {
				break
			}
			anc = append(anc, parent)
			cur = parent
			d++
		}
		if maxDepth >= 0 && d > maxDepth {
			// Fold into the ancestor at maxDepth.
			return d, anc[d-maxDepth-1]
		}
		return d, ""
	}
	for _, n := range p.g.NodesOf(model.NodeConcept) {
		if !p.matchesInclude(n.CURIE) || p.collapseRule(n.CURIE) != "" {
			continue
		}
		d, foldInto := conceptDepth(n.ID)
		if foldInto != "" {
			p.fold[n.ID] = p.target(foldInto, 0)
			continue
		}
		p.included[n.ID] = true
		p.fold[n.ID] = []string{n.ID}
		pn := PNode{ID: n.CURIE, Label: p.g.Label(n.ID), Type: "concept", Kind: n.Kind, Tier: n.Tier, NS: n.NS, Depth: d, Internal: n.Internal, Doc: n.Doc, Value: n.Value, Imported: n.Imported}
		pn.Branch = p.branch(n.ID)
		if n.Published != "" {
			pn.Published = true
			pn.PublishedAs = n.Published
		}
		for _, e := range p.g.Out(n.ID, "part-of") {
			if o := p.g.Node(e.O); o != nil && o.NodeType == model.NodeConcept && e.Source != graph.SourceDerived {
				pn.Parent = o.CURIE
			}
		}
		p.byID[n.CURIE] = len(p.nodes)
		p.nodes = append(p.nodes, pn)
	}
	// Resolve folds and attach folded lists + published badges.
	for _, n := range p.g.NodesOf(model.NodeConcept) {
		if p.included[n.ID] {
			continue
		}
		for _, t := range p.target(n.ID, 0) {
			if i, ok := p.byID[p.g.Curie(t)]; ok {
				p.nodes[i].Folded = append(p.nodes[i].Folded, n.CURIE)
				if n.Published != "" && p.g.Curie(t) == n.Published && (p.v.ShowPublished == nil || *p.v.ShowPublished) {
					p.nodes[i].Published = true
					p.nodes[i].PublishedAs = n.CURIE
					if p.nodes[i].Value == nil {
						p.nodes[i].Value = n.Value
					}
				}
			}
		}
	}
	for i := range p.nodes {
		sort.Strings(p.nodes[i].Folded)
	}
}

func (p *projector) branch(iri string) string {
	cur := iri
	for i := 0; i < 32; i++ {
		ups := p.g.Objects(cur, "is-a")
		if len(ups) == 0 {
			break
		}
		cur = ups[0]
	}
	return p.g.Curie(cur)
}

func (p *projector) boundaries() {
	if p.v.Boundaries != nil && !*p.v.Boundaries {
		return
	}
	for _, b := range p.g.NodesOf(model.NodeBoundary) {
		lvl, ok := levelIndex[b.Kind]
		if !ok {
			lvl = 3
		}
		if lvl > p.zoom {
			// Deeper than the zoom: fold into the nearest shown ancestor.
			for _, anc := range p.g.PartOfChain(b.ID) {
				if an := p.g.Node(anc); an != nil && levelIndex[an.Kind] <= p.zoom {
					p.fold[b.ID] = []string{anc}
					break
				}
			}
			continue
		}
		p.included[b.ID] = true
		p.fold[b.ID] = []string{b.ID}
		pn := PNode{ID: b.CURIE, Label: p.g.Label(b.ID), Type: "boundary", Kind: b.Kind, NS: b.NS, Depth: lvl, Doc: b.Doc, Branch: "boundary"}
		if chain := p.g.PartOfChain(b.ID); len(chain) > 0 {
			pn.Parent = p.g.Curie(chain[0])
		}
		p.byID[b.CURIE] = len(p.nodes)
		p.nodes = append(p.nodes, pn)
	}
	for _, b := range p.g.NodesOf(model.NodeBoundary) {
		if p.included[b.ID] {
			continue
		}
		for _, t := range p.fold[b.ID] {
			if i, ok := p.byID[p.g.Curie(t)]; ok {
				p.nodes[i].Folded = append(p.nodes[i].Folded, b.CURIE)
			}
		}
	}
}

func (p *projector) artifacts() {
	for _, a := range p.g.NodesOf(model.NodeArtifact) {
		p.included[a.ID] = true
		p.fold[a.ID] = []string{a.ID}
		pn := PNode{ID: p.g.Curie(a.ID), Label: a.Label, Type: "artifact", Kind: a.Kind, Depth: 4, Path: a.Path, Branch: "artifact"}
		if a.Boundary != "" {
			if t := p.fold[a.Boundary]; len(t) > 0 {
				pn.Parent = p.g.Curie(t[0])
			}
		}
		p.byID[pn.ID] = len(p.nodes)
		p.nodes = append(p.nodes, pn)
	}
}

func (p *projector) relationShown(rel string) bool {
	if len(p.v.Relations) == 0 {
		return true
	}
	for _, r := range p.v.Relations {
		if r == rel {
			return true
		}
	}
	return false
}

func (p *projector) edges() {
	type agg struct {
		e   PEdge
		src map[string]bool
		all bool // all contributing edges proposed
		n   int
	}
	aggs := map[string]*agg{}
	// Per (subject, relation) object sets for lifting.
	type sp struct{ s, p string }
	objs := map[sp]map[string]bool{}
	for _, e := range p.g.Edges() {
		if !p.relationShown(e.P) {
			continue
		}
		if e.P == "part-of" && e.Source == graph.SourceDerived {
			continue // containment is carried by Parent
		}
		ss := p.target(e.S, 0)
		if p.fold[e.S] != nil {
			ss = p.fold[e.S]
		}
		os := p.target(e.O, 0)
		if p.fold[e.O] != nil {
			os = p.fold[e.O]
		}
		for _, s := range ss {
			for _, o := range os {
				if s == o || !p.included[s] || !p.included[o] {
					continue
				}
				sc, oc := p.g.Curie(s), p.g.Curie(o)
				k := sc + "|" + e.P + "|" + oc
				a, ok := aggs[k]
				if !ok {
					a = &agg{e: PEdge{S: sc, P: e.P, O: oc}, src: map[string]bool{}, all: true}
					aggs[k] = a
				}
				a.src[e.Source] = true
				a.n++
				if e.Prov == nil || !e.Prov.Proposed {
					a.all = false
				}
				if e.Prov != nil && len(e.Prov.Via) > 0 {
					a.e.Via = uniq(append(a.e.Via, e.Prov.Via...))
				}
				key := sp{sc, e.P}
				if objs[key] == nil {
					objs[key] = map[string]bool{}
				}
				objs[key][o] = true
			}
		}
	}
	// Taxonomic lift on boundary subjects.
	lifted := map[string]PEdge{}
	dropped := map[string]bool{}
	if p.v.Lift == model.LiftRangeLCS {
		for key, set := range objs {
			if len(set) < 2 {
				continue
			}
			if i, ok := p.byID[key.s]; !ok || p.nodes[i].Type != "boundary" {
				continue
			}
			var ids []string
			for o := range set {
				if n := p.g.Node(o); n != nil && n.NodeType == model.NodeConcept {
					ids = append(ids, o)
				}
			}
			lcs := p.g.LCS(ids)
			if len(lcs) >= len(ids) {
				continue
			}
			var via []string
			anyProposed, allProposed := false, true
			srcs := map[string]bool{}
			for o := range set {
				oc := p.g.Curie(o)
				k := key.s + "|" + key.p + "|" + oc
				if a, ok := aggs[k]; ok {
					via = append(via, oc)
					dropped[k] = true
					for s := range a.src {
						srcs[s] = true
					}
					if a.all {
						anyProposed = true
					} else {
						allProposed = false
					}
				}
			}
			_ = anyProposed
			for _, l := range lcs {
				lc := p.g.Curie(l)
				if !p.included[l] {
					continue
				}
				lifted[key.s+"|"+key.p+"|"+lc] = PEdge{S: key.s, P: key.p, O: lc, Source: mixed(srcs), Lifted: true, Via: uniq(via), Count: len(via), Proposed: allProposed}
			}
		}
	}
	// A purely derived edge on a boundary that a visible descendant boundary
	// also carries is the same fact rolled up; keep it on the descendant only.
	isBoundary := func(id string) bool {
		i, ok := p.byID[id]
		return ok && p.nodes[i].Type == "boundary"
	}
	for k, a := range aggs {
		if dropped[k] || !isBoundary(a.e.S) || len(a.src) != 1 || !a.src[graph.SourceDerived] {
			continue
		}
		for k2, b := range aggs {
			if k2 == k || dropped[k2] || b.e.P != a.e.P || b.e.O != a.e.O || !isBoundary(b.e.S) {
				continue
			}
			if p.isDescendant(b.e.S, a.e.S) {
				dropped[k] = true
				break
			}
		}
	}
	for k, a := range aggs {
		if dropped[k] {
			continue
		}
		a.e.Source = mixed(a.src)
		a.e.Proposed = a.all
		a.e.Count = a.n
		p.outEdges = append(p.outEdges, a.e)
	}
	for _, e := range lifted {
		p.outEdges = append(p.outEdges, e)
	}
}

// isDescendant reports whether view node child sits under anc through Parent links.
func (p *projector) isDescendant(child, anc string) bool {
	cur := child
	for i := 0; i < 32; i++ {
		idx, ok := p.byID[cur]
		if !ok || p.nodes[idx].Parent == "" {
			return false
		}
		if p.nodes[idx].Parent == anc {
			return true
		}
		cur = p.nodes[idx].Parent
	}
	return false
}

func mixed(src map[string]bool) string {
	if len(src) == 1 {
		for k := range src {
			return k
		}
	}
	if src[graph.SourceInline] && !src[graph.SourceDeclared] && !src[graph.SourceDerived] {
		return graph.SourceInline
	}
	return "mixed"
}

func uniq(xs []string) []string {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Delta is the difference between two projections of the same view.
type Delta struct {
	View         string   `json:"view"`
	AddedNodes   []string `json:"added_nodes,omitempty"`
	RemovedNodes []string `json:"removed_nodes,omitempty"`
	AddedEdges   []PEdge  `json:"added_edges,omitempty"`
	RemovedEdges []PEdge  `json:"removed_edges,omitempty"`
	ChangedNodes []string `json:"changed_nodes,omitempty"` // published value / badge changed
}

// Changed reports whether the delta is non-empty.
func (d *Delta) Changed() bool {
	return len(d.AddedNodes)+len(d.RemovedNodes)+len(d.AddedEdges)+len(d.RemovedEdges)+len(d.ChangedNodes) > 0
}

// Diff compares two projections of one view.
func Diff(base, head *Projection) *Delta {
	d := &Delta{View: head.Name}
	bn := map[string]PNode{}
	for _, n := range base.Nodes {
		bn[n.ID] = n
	}
	hn := map[string]PNode{}
	for _, n := range head.Nodes {
		hn[n.ID] = n
	}
	for id, n := range hn {
		b, ok := bn[id]
		if !ok {
			d.AddedNodes = append(d.AddedNodes, id)
			continue
		}
		if valueString(b.Value) != valueString(n.Value) || b.Published != n.Published || b.PublishedAs != n.PublishedAs || b.Label != n.Label || b.Kind != n.Kind {
			d.ChangedNodes = append(d.ChangedNodes, id)
		}
	}
	for id := range bn {
		if _, ok := hn[id]; !ok {
			d.RemovedNodes = append(d.RemovedNodes, id)
		}
	}
	be := map[string]PEdge{}
	for _, e := range base.Edges {
		be[e.key()] = e
	}
	he := map[string]PEdge{}
	for _, e := range head.Edges {
		he[e.key()] = e
	}
	for k, e := range he {
		if _, ok := be[k]; !ok {
			d.AddedEdges = append(d.AddedEdges, e)
		}
	}
	for k, e := range be {
		if _, ok := he[k]; !ok {
			d.RemovedEdges = append(d.RemovedEdges, e)
		}
	}
	sort.Strings(d.AddedNodes)
	sort.Strings(d.RemovedNodes)
	sort.Strings(d.ChangedNodes)
	sort.Slice(d.AddedEdges, func(i, j int) bool { return d.AddedEdges[i].key() < d.AddedEdges[j].key() })
	sort.Slice(d.RemovedEdges, func(i, j int) bool { return d.RemovedEdges[i].key() < d.RemovedEdges[j].key() })
	return d
}

func valueString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(strings.Trim(toString(v), `"`))
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(sprint(x), "\n", " "), "  ", " "))
	}
}
