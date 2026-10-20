// @gloss implements gmi:ViewProjector scope=file

// Package views projects the one graph into what a given observer sees: a
// declared view over tier × zoom, with implementation folded into what it
// realizes, part-of expanded to a depth, and sets lifted to their least
// common subsumer. PR impact is "which views changed".
package views

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// Projection is one rendered view.
// @gloss represents gm:View
type Projection struct {
	Name string `json:"name"`
	Doc  string `json:"doc,omitempty"`
	Zoom string `json:"zoom"`
	// Curated is true when the view names concepts in include: (not only
	// namespace globs). The viewer opens such a view expanded, and only
	// edges with at least one named end are kept.
	Curated bool    `json:"curated,omitempty"`
	Nodes   []PNode `json:"nodes"`
	Edges   []PEdge `json:"edges"`
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
	Line        int      `json:"line,omitempty"` // first line of an artifact's anchor
	// LiveValue is the code line a published value is bound to (value-from),
	// at ValueFrom (path:line).
	LiveValue string `json:"liveValue,omitempty"`
	ValueFrom string `json:"valueFrom,omitempty"`
	Imported  bool   `json:"imported,omitempty"`
	// Boundary is the boundary node (CURIE) this concept or artifact sits in,
	// resolved from what the boundary's code realizes or represents; the
	// viewer draws the boundary as a box around its members. Empty when no
	// boundary houses the node or several unrelated boundaries do.
	Boundary string `json:"boundary,omitempty"`
	// Placement says why a concept sits where it does: housed (inside
	// Boundary), or why it is unplaced (shared, flow, bound, surface,
	// unlinked); PlacementReason names what produces it, such as "produced by
	// acme:payments-api". Concepts only.
	Placement       string `json:"placement,omitempty"`
	PlacementReason string `json:"placementReason,omitempty"`
	// Criticality is the effective governance level of the node (the highest
	// of its own and of everything folded into it); CriticalityFrom says where
	// the level comes from. Owner is the boundary's owner.
	Criticality     string `json:"criticality,omitempty"`
	CriticalityFrom string `json:"criticalityFrom,omitempty"`
	Owner           string `json:"owner,omitempty"`
	// Deprecated, ReplacedBy and RetireBy (boundaries only) mark what is being
	// retired and its successor; the viewer draws such nodes dashed.
	Deprecated bool   `json:"deprecated,omitempty"`
	ReplacedBy string `json:"replacedBy,omitempty"`
	RetireBy   string `json:"retireBy,omitempty"`
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

// boundaryLevel is a boundary's depth by its level. A boundary with no level
// (a parent named only by part-of, with no manifest of its own) sits one
// level above its shallowest child; one with no children either is a component.
func boundaryLevel(g *graph.Graph, id string, seen map[string]bool) int {
	if n := g.Node(id); n != nil {
		if lvl, ok := levelIndex[n.Kind]; ok {
			return lvl
		}
	}
	if seen[id] {
		return 3
	}
	seen[id] = true
	lvl := 4
	for _, c := range g.Subjects(id, "part-of") {
		if cn := g.Node(c); cn != nil && cn.NodeType == model.NodeBoundary {
			if l := boundaryLevel(g, c, seen) - 1; l < lvl {
				lvl = l
			}
		}
	}
	if lvl == 4 {
		return 3
	}
	if lvl < 0 {
		return 0
	}
	return lvl
}

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
	pr := &Projection{Name: name, Doc: v.Doc, Zoom: zoom, Curated: curated(v)}
	p.curated = pr.Curated
	p.zoom = zoomIndex[zoom]
	p.concepts()
	p.boundaries()
	if p.zoom >= zoomIndex["code"] {
		p.artifacts()
	}
	p.house()
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
	curated  bool
}

// curated reports whether a view's include list names at least one concept.
func curated(v *model.View) bool {
	for _, pat := range v.Include {
		if !strings.Contains(pat, "*") {
			return true
		}
	}
	return false
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
		pn := PNode{ID: n.CURIE, Label: p.g.Label(n.ID), Type: "concept", Kind: n.Kind, Tier: n.Tier, NS: n.NS, Depth: d, Internal: n.Internal, Doc: n.Doc, Value: n.Value, Imported: n.Imported, Criticality: n.Criticality, CriticalityFrom: n.CriticalityFrom,
			LiveValue: n.LiveValue, ValueFrom: n.ValueFrom, Deprecated: n.Deprecated, ReplacedBy: n.ReplacedBy}
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
		lvl := boundaryLevel(p.g, b.ID, map[string]bool{})
		if lvl > p.zoom {
			// Deeper than the zoom: fold into the nearest shown ancestor.
			for _, anc := range p.g.PartOfChain(b.ID) {
				if boundaryLevel(p.g, anc, map[string]bool{}) <= p.zoom {
					p.fold[b.ID] = []string{anc}
					break
				}
			}
			continue
		}
		p.included[b.ID] = true
		p.fold[b.ID] = []string{b.ID}
		pn := PNode{ID: b.CURIE, Label: p.g.Label(b.ID), Type: "boundary", Kind: b.Kind, NS: b.NS, Depth: lvl, Doc: b.Doc, Branch: "boundary", Criticality: b.Criticality, CriticalityFrom: b.CriticalityFrom, Owner: b.Owner, Deprecated: b.Deprecated, ReplacedBy: b.ReplacedBy, RetireBy: b.RetireBy}
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
		if p.curated && !p.linksNamed(a.ID) {
			continue // a curated view shows only the code behind the concepts it names
		}
		p.included[a.ID] = true
		p.fold[a.ID] = []string{a.ID}
		pn := PNode{ID: p.g.Curie(a.ID), Label: a.Label, Type: "artifact", Kind: a.Kind, Depth: 4, Path: a.Path, Branch: "artifact", Criticality: a.Criticality, CriticalityFrom: a.CriticalityFrom}
		if a.Anchor != nil && a.Anchor.Scope != "file" {
			pn.Line = a.Anchor.StartLine
		}
		if a.Boundary != "" {
			if t := p.fold[a.Boundary]; len(t) > 0 {
				pn.Parent = p.g.Curie(t[0])
				pn.Boundary = pn.Parent
			}
		}
		p.byID[pn.ID] = len(p.nodes)
		p.nodes = append(p.nodes, pn)
	}
}

// house places every concept node inside the boundary whose code realizes or
// represents it. Candidates come from the housing relations on boundaries
// (declared in manifests or derived from the artifacts beneath them), mapped
// through this view's folds. The deepest candidates win; when several
// unrelated boundaries house one concept it goes to their nearest common
// enclosing boundary, or stays unplaced when there is none. A part-of tree
// moves as one: its root takes the common boundary of its parts.
func (p *projector) house() {
	// Tie-break: a boundary whose own code implements or represents the
	// concept (a direct link) beats one whose manifest declares it, which beats
	// one that only reaches it by lifting through another concept. Only the
	// strongest candidates go to resolveBoundary, so a concept many boundaries
	// touch indirectly still lands in the box that computes it.
	cand := map[string]map[string]int{} // concept CURIE → boundary CURIE → strength
	for _, b := range p.g.NodesOf(model.NodeBoundary) {
		bt := p.fold[b.ID]
		if len(bt) == 0 || !p.included[bt[0]] {
			continue
		}
		bc := p.g.Curie(bt[0])
		for _, rel := range rollup.HousingRelations {
			for _, e := range p.g.Out(b.ID, rel) {
				strength := 0
				switch {
				case e.Source == graph.SourceDerived && e.Prov != nil && len(e.Prov.Via) == 1:
					strength = 2
				case e.Source == graph.SourceDeclared:
					strength = 1
				}
				for _, t := range p.target(e.O, 0) {
					if !p.included[t] {
						continue
					}
					tc := p.g.Curie(t)
					if i, ok := p.byID[tc]; !ok || p.nodes[i].Type != "concept" {
						continue
					}
					if cand[tc] == nil {
						cand[tc] = map[string]int{}
					}
					if old, ok := cand[tc][bc]; !ok || strength > old {
						cand[tc][bc] = strength
					}
				}
			}
		}
	}
	strongest := map[string]map[string]bool{}
	for cu, byB := range cand {
		best := -1
		for _, st := range byB {
			if st > best {
				best = st
			}
		}
		set := map[string]bool{}
		for b, st := range byB {
			if st == best {
				set[b] = true
			}
		}
		strongest[cu] = set
		p.nodes[p.byID[cu]].Boundary = p.resolveBoundary(set)
	}
	// Part-of trees: the root adopts the common boundary of its descendants
	// when it has none of its own; parts without one follow the root.
	rootOf := func(i int) int {
		cur := i
		for g := 0; g < 32; g++ {
			par := p.nodes[cur].Parent
			j, ok := p.byID[par]
			if par == "" || !ok || p.nodes[j].Type != "concept" {
				break
			}
			cur = j
		}
		return cur
	}
	trees := map[int]map[string]bool{}
	for i := range p.nodes {
		if p.nodes[i].Type != "concept" {
			continue
		}
		r := rootOf(i)
		if r == i {
			continue
		}
		if trees[r] == nil {
			trees[r] = map[string]bool{}
		}
		if p.nodes[i].Boundary != "" {
			trees[r][p.nodes[i].Boundary] = true
		}
	}
	for r, set := range trees {
		if p.nodes[r].Boundary == "" && len(set) > 0 {
			p.nodes[r].Boundary = p.resolveBoundary(set)
			if p.nodes[r].Boundary == "" && len(strongest[p.nodes[r].ID]) == 0 {
				strongest[p.nodes[r].ID] = set // its parts are housed apart
			}
		}
	}
	for i := range p.nodes {
		if p.nodes[i].Type == "concept" && p.nodes[i].Boundary == "" {
			if r := rootOf(i); r != i {
				p.nodes[i].Boundary = p.nodes[r].Boundary
			}
		}
	}
	p.place(strongest)
}

// Placement values, in precedence order: the first that applies wins.
const (
	PlacementHoused   = "housed"
	PlacementShared   = "shared"
	PlacementFlow     = "flow"
	PlacementBound    = "bound"
	PlacementSurface  = "surface"
	PlacementUnlinked = "unlinked"
)

// flowRels and surfaceRels are the boundary → concept relations that explain
// an unplaced concept, with the wording its reason uses.
var flowRels = [][2]string{{"produces", "produced by"}, {"consumes", "consumed by"}, {"publishes", "published by"}, {"subscribes", "subscribed to by"}}
var surfaceRels = [][2]string{{"presents", "presented by"}, {"hosts", "hosted by"}}

// place gives every concept node its placement and the reason for it.
// strongest holds each concept's strongest housing candidates; a concept left
// without a boundary that has any is shared.
func (p *projector) place(strongest map[string]map[string]bool) {
	links := map[string]map[string]map[string]bool{} // concept CURIE → relation → who
	add := func(iri, rel, who string) {
		for _, t := range p.target(iri, 0) {
			tc := p.g.Curie(t)
			if i, ok := p.byID[tc]; !p.included[t] || !ok || p.nodes[i].Type != "concept" {
				continue
			}
			if links[tc] == nil {
				links[tc] = map[string]map[string]bool{}
			}
			if links[tc][rel] == nil {
				links[tc][rel] = map[string]bool{}
			}
			links[tc][rel][who] = true
		}
	}
	explained := map[string]bool{}
	for _, r := range append(append([][2]string{}, flowRels...), surfaceRels...) {
		explained[r[0]] = true
	}
	for _, b := range p.g.NodesOf(model.NodeBoundary) {
		who := b.CURIE
		if bt := p.fold[b.ID]; len(bt) > 0 && p.included[bt[0]] {
			who = p.g.Curie(bt[0])
		}
		for _, e := range p.g.Out(b.ID, "") {
			rel := e.P
			if !explained[rel] {
				rel = "other:" + rel
			}
			add(e.O, rel, who)
		}
	}
	for _, a := range p.g.NodesOf(model.NodeArtifact) {
		for _, e := range p.g.Out(a.ID, "") {
			add(e.O, "other:"+e.P, "code")
		}
	}
	for _, n := range p.g.NodesOf(model.NodeConcept) {
		if n.ValueFrom != "" {
			add(n.ID, "value-from", n.ValueFrom)
		}
	}
	reason := func(l map[string]map[string]bool, rels [][2]string) string {
		var parts []string
		for _, r := range rels {
			if len(l[r[0]]) > 0 {
				parts = append(parts, r[1]+" "+list(p.deepest(l[r[0]])))
			}
		}
		return strings.Join(parts, "; ")
	}
	for i := range p.nodes {
		n := &p.nodes[i]
		if n.Type != "concept" {
			continue
		}
		l := links[n.ID]
		switch {
		case n.Boundary != "":
			n.Placement, n.PlacementReason = PlacementHoused, "inside "+n.Boundary
		case len(strongest[n.ID]) > 0:
			n.Placement, n.PlacementReason = PlacementShared, "realized or represented by "+list(p.deepest(strongest[n.ID]))+", and no boundary encloses them all"
		case reason(l, flowRels) != "":
			n.Placement, n.PlacementReason = PlacementFlow, reason(l, flowRels)
		case len(l["value-from"]) > 0:
			n.Placement, n.PlacementReason = PlacementBound, "value bound to code at "+list(keys(l["value-from"]))
		case reason(l, surfaceRels) != "":
			n.Placement, n.PlacementReason = PlacementSurface, reason(l, surfaceRels)
		default:
			var other []string
			for rel, who := range l {
				if strings.HasPrefix(rel, "other:") {
					other = append(other, strings.TrimPrefix(rel, "other:")+" from "+list(p.deepest(who)))
				}
			}
			sort.Strings(other)
			n.Placement, n.PlacementReason = PlacementUnlinked, "no boundary realizes, represents, produces, consumes, publishes, subscribes, presents or hosts it"
			if len(other) > 0 {
				n.PlacementReason += "; linked only by " + strings.Join(other, "; ")
			}
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// list joins names for a reason, naming at most three.
func list(xs []string) string {
	if len(xs) > 3 {
		return strings.Join(xs[:3], ", ") + fmt.Sprintf(" and %d more", len(xs)-3)
	}
	return strings.Join(xs, ", ")
}

// boundaryChain returns the boundary and its enclosing boundaries, nearest first.
func (p *projector) boundaryChain(curie string) []string {
	var out []string
	cur := curie
	for g := 0; g < 32; g++ {
		i, ok := p.byID[cur]
		if !ok || p.nodes[i].Type != "boundary" {
			break
		}
		out = append(out, cur)
		cur = p.nodes[i].Parent
	}
	return out
}

// deepest returns the candidates that enclose no other candidate, sorted.
func (p *projector) deepest(set map[string]bool) []string {
	var deepest []string
	for b := range set {
		enclosesOther := false
		for o := range set {
			if o == b {
				continue
			}
			if chain := p.boundaryChain(o); len(chain) > 1 {
				for _, anc := range chain[1:] {
					if anc == b {
						enclosesOther = true
					}
				}
			}
		}
		if !enclosesOther {
			deepest = append(deepest, b)
		}
	}
	sort.Strings(deepest)
	return deepest
}

// resolveBoundary picks one boundary for a set of candidates: drop any that
// encloses another candidate, then take the nearest common enclosing boundary.
func (p *projector) resolveBoundary(set map[string]bool) string {
	deepest := p.deepest(set)
	if len(deepest) == 0 {
		return ""
	}
	if len(deepest) == 1 {
		return deepest[0]
	}
	for _, anc := range p.boundaryChain(deepest[0]) {
		common := true
		for _, o := range deepest[1:] {
			found := false
			for _, x := range p.boundaryChain(o) {
				if x == anc {
					found = true
				}
			}
			if !found {
				common = false
				break
			}
		}
		if common {
			return anc
		}
	}
	return ""
}

// linksNamed reports whether an artifact links to a concept the view names.
func (p *projector) linksNamed(id string) bool {
	for _, e := range p.g.Out(id, "") {
		if n := p.g.Node(e.O); n != nil && n.NodeType == model.NodeConcept && p.matchesInclude(n.CURIE) {
			return true
		}
	}
	return false
}

// namesEnd reports whether a curated view's include list names either end of
// an edge (before folding), so a closed box does not carry edges between
// concepts the view never asked for.
func (p *projector) namesEnd(e *graph.Edge) bool {
	for _, id := range []string{e.S, e.O} {
		if n := p.g.Node(id); n != nil && n.NodeType == model.NodeConcept && p.matchesInclude(n.CURIE) {
			return true
		}
	}
	return false
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
		if p.curated && !p.namesEnd(e) {
			continue // neither end is something the view names
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
	ChangedNodes []string `json:"changed_nodes,omitempty"` // published value / badge / definition / criticality changed
	// Removed is set when the view exists in the base and not in the head.
	Removed bool `json:"removed,omitempty"`
}

// Changed reports whether the delta is non-empty.
func (d *Delta) Changed() bool {
	return d.Removed || len(d.AddedNodes)+len(d.RemovedNodes)+len(d.AddedEdges)+len(d.RemovedEdges)+len(d.ChangedNodes) > 0
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
		if valueString(b.Value) != valueString(n.Value) || b.Published != n.Published || b.PublishedAs != n.PublishedAs || b.Label != n.Label || b.Doc != n.Doc || b.Deprecated != n.Deprecated || b.ReplacedBy != n.ReplacedBy || b.Kind != n.Kind || b.Criticality != n.Criticality {
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
