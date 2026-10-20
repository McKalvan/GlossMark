// @gloss implements gmi:ViewProjector scope=file

// Package slice computes a feature slice: from one or more seed nodes, the
// concepts, artifacts, tests and boundaries that belong to the feature, and a
// faded ring of context around them. Each relation is followed in one fixed
// direction for one fixed reason, never as a blind N-hop neighbourhood, and
// every node records its role and the path of edges that brought it in. The
// rules are docs/adr/0023-feature-slices.md; the CLI (gloss slice), the MCP
// tool (slice_of), saved slices in views.yaml and the viewer all call Compute.
package slice

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// Roles, in the order the CLI prints them. Every role but context is a
// member of the slice; context nodes explain it and are never expanded.
const (
	RoleSeed        = "seed"        // what was asked for
	RoleHoused      = "housed"      // a concept a seed boundary realizes, holds, presents or hosts
	RoleKind        = "kind"        // is-a a member
	RolePart        = "part"        // part-of a member
	RoleRealization = "realization" // realizes a member
	RolePublished   = "published"   // published-as a member, or what a member is published as
	RoleFlow        = "flow"        // reached along a --follow relation
	RoleArtifact    = "artifact"    // code, docs, pages, infra linked to a member
	RoleTest        = "test"        // an artifact whose only links to members are verifies
	RoleBoundary    = "boundary"    // realizes, holds, flows a member, or contains a member artifact
	RoleContext     = "context"     // one hop of explanation, drawn faded
)

// Roles lists every role in display order.
var Roles = []string{RoleSeed, RoleHoused, RoleKind, RolePart, RoleRealization, RolePublished, RoleFlow, RoleArtifact, RoleTest, RoleBoundary, RoleContext}

func roleRank(r string) int {
	for i, x := range Roles {
		if x == r {
			return i
		}
	}
	return len(Roles)
}

// Flow relations, and the relation on the other side of each.
var flowRels = []string{"produces", "consumes", "publishes", "subscribes"}
var counterpart = map[string]string{"produces": "consumes", "consumes": "produces", "publishes": "subscribes", "subscribes": "publishes"}

// boundaryRels are the boundary → concept relations that put a boundary in
// the slice (stage 4); housingRels are the ones a seed boundary's concepts
// enter through.
var boundaryRels = []string{"realizes", "holds", "represents", "presents", "hosts", "produces", "consumes", "publishes", "subscribes"}
var housingRels = []string{"realizes", "holds", "represents", "presents", "hosts"}

// artifactRels are the inline relations that make an artifact part of the
// feature (stage 3). Flow markers on code (produces, publishes, ...) are not
// among them: they say what the code emits, which is context.
var artifactRels = map[string]bool{"implements": true, "represents": true, "verifies": true, "tests": true, "documents": true, "instructs": true,
	"presents": true, "hosts": true, "provisions": true, "configures": true, "deploys": true, "may-modify": true, "publishes-value": true}

// structural relations between concepts, which stages 1 and 2 (or other
// context rules) already handle; any other concept → concept relation
// (reverses, attaches-to, a domain relation) is one hop of context.
var structural = map[string]bool{"is-a": true, "part-of": true, "has-part": true, "realizes": true, "published-as": true,
	"produces": true, "consumes": true, "publishes": true, "subscribes": true, "constrains": true, "same-as": true, "derived-from": true}

// Options change what a slice includes.
type Options struct {
	// Context keeps stage 5, the faded ring of explanation. Off, the slice
	// holds only members and their boundaries.
	Context bool
	// Follow names flow relations (produces, consumes, publishes, subscribes)
	// that repeat with stages 1 and 2, for end-to-end flows: produces and
	// publishes go forward to what a member makes, consumes and subscribes go
	// to whoever takes a member in.
	Follow []string
}

// Node is one node of a slice.
type Node struct {
	ID    string `json:"id"` // CURIE (artifacts: path#symbol)
	Type  string `json:"type"`
	Kind  string `json:"kind,omitempty"`
	Tier  string `json:"tier,omitempty"`
	Label string `json:"label,omitempty"`
	Role  string `json:"role"`
	// Via is the path of edges from a seed to this node, each written as the
	// edge reads ("pa:CaptureHandler realizes pay:Capture"); empty for a seed.
	Via         []string `json:"via,omitempty"`
	Path        string   `json:"path,omitempty"`
	Line        int      `json:"line,omitempty"`
	Criticality string   `json:"criticality,omitempty"`
	Internal    bool     `json:"internal,omitempty"`
	Imported    bool     `json:"imported,omitempty"`
	// Findings are the drift findings on this concept (Annotate fills them).
	Findings []string `json:"findings,omitempty"`
	iri      string
}

// Edge is one edge with both ends in the slice.
type Edge struct {
	S      string `json:"s"`
	P      string `json:"p"`
	O      string `json:"o"`
	Source string `json:"source"`
}

// Summary counts a slice and says what in it is untested.
type Summary struct {
	Members    int      `json:"members"`
	Concepts   int      `json:"concepts"`
	Artifacts  int      `json:"artifacts"`
	Tests      int      `json:"tests"`
	Boundaries int      `json:"boundaries"`
	Context    int      `json:"context"`
	Untested   []string `json:"untested,omitempty"`
	// Critical is the highest criticality among the members.
	Criticality string `json:"criticality,omitempty"`
}

// Slice is the result of Compute.
type Slice struct {
	Seeds   []string `json:"seeds"`
	Context bool     `json:"context"`
	Follow  []string `json:"follow,omitempty"`
	Nodes   []Node   `json:"nodes"`
	Edges   []Edge   `json:"edges"`
	Summary Summary  `json:"summary"`
}

// Members returns the IDs of the nodes with a role other than context.
func (s *Slice) Members() []string {
	var out []string
	for _, n := range s.Nodes {
		if n.Role != RoleContext {
			out = append(out, n.ID)
		}
	}
	return out
}

// Node returns the node with the given ID, or nil.
func (s *Slice) Node(id string) *Node {
	for i := range s.Nodes {
		if s.Nodes[i].ID == id {
			return &s.Nodes[i]
		}
	}
	return nil
}

// ValidFollow is the set of relations --follow accepts.
var ValidFollow = flowRels

// ParseFollow splits a comma-separated --follow value and checks each name.
func ParseFollow(s string) ([]string, error) {
	var out []string
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if counterpart[f] == "" {
			return nil, fmt.Errorf("--follow %q: want one of %s", f, strings.Join(flowRels, ", "))
		}
		out = append(out, f)
	}
	return out, nil
}

// Resolve turns a command-line seed into node IRIs: a concept or boundary
// CURIE, an artifact ID (path#symbol), or a repo-relative path whose
// artifacts' linked concepts become the seeds (concepts_for_path).
func Resolve(g *graph.Graph, root, arg string) ([]string, error) {
	if n := g.NodeByCurie(arg); n != nil && (n.NodeType == model.NodeConcept || n.NodeType == model.NodeBoundary) {
		return []string{n.ID}, nil
	}
	if n := g.Node(graph.ArtifactPrefix + arg); n != nil {
		return []string{n.ID}, nil
	}
	p := filepath.ToSlash(arg)
	if root != "" && filepath.IsAbs(arg) {
		if r, err := filepath.Rel(root, arg); err == nil {
			p = filepath.ToSlash(r)
		}
	}
	p = strings.TrimSuffix(strings.TrimPrefix(p, "./"), "/")
	set := map[string]bool{}
	found := false
	for _, a := range g.NodesOf(model.NodeArtifact) {
		if a.Path != p && !strings.HasPrefix(a.Path, p+"/") {
			continue
		}
		found = true
		for _, e := range g.Out(a.ID, "") {
			if e.Source != graph.SourceInline && e.Source != graph.SourceInferred {
				continue
			}
			if o := g.Node(e.O); o != nil && (o.NodeType == model.NodeConcept || o.NodeType == model.NodeBoundary) {
				set[e.O] = true
			}
		}
	}
	if len(set) > 0 {
		return sortedKeys(set), nil
	}
	if found {
		return nil, fmt.Errorf("%s: no @gloss link in it to start a slice from", arg)
	}
	if root != "" {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err == nil {
			return nil, fmt.Errorf("%s: no @gloss link in it to start a slice from", arg)
		}
	}
	return nil, fmt.Errorf("no concept, boundary, artifact or linked path %q (try gloss mcp search_concepts or a full CURIE)", arg)
}

type builder struct {
	g     *graph.Graph
	opt   Options
	in    map[string]*Node
	order []string
}

func (b *builder) curie(id string) string { return b.g.Curie(id) }

func (b *builder) step(s, p, o string) string { return b.curie(s) + " " + p + " " + b.curie(o) }

// add puts id in the slice with a role, reached from parent by step. It
// returns false when id is already in (the first way in wins: rounds are
// breadth-first and sorted, so that is the shortest, stable path).
func (b *builder) add(id, role, parent, step string) bool {
	if _, ok := b.in[id]; ok {
		return false
	}
	n := b.g.Node(id)
	if n == nil || !(n.NodeType == model.NodeConcept || n.NodeType == model.NodeBoundary || n.NodeType == model.NodeArtifact) {
		return false
	}
	var via []string
	if p := b.in[parent]; p != nil {
		via = append(append([]string{}, p.Via...), step)
	} else if step != "" {
		via = []string{step}
	}
	sn := &Node{ID: b.curie(id), Type: string(n.NodeType), Kind: n.Kind, Tier: n.Tier, Label: b.g.Label(id), Role: role, Via: via,
		Criticality: n.Criticality, Internal: n.Internal, Imported: n.Imported, iri: id}
	if n.NodeType == model.NodeArtifact {
		sn.Path = n.Path
		if n.Anchor != nil && n.Anchor.Scope != "file" {
			sn.Line = n.Anchor.StartLine
		}
	}
	b.in[id] = sn
	b.order = append(b.order, id)
	return true
}

func (b *builder) isType(id string, t model.NodeType) bool {
	n := b.g.Node(id)
	return n != nil && n.NodeType == t
}

func (b *builder) concept(id string) bool { return b.isType(id, model.NodeConcept) }

// Compute builds the slice of seeds (node IRIs) in g.
func Compute(g *graph.Graph, seeds []string, opt Options) *Slice {
	b := &builder{g: g, opt: opt, in: map[string]*Node{}}
	seeds = uniqueSorted(seeds)
	var frontier []string
	var seedBoundaries []string
	for _, s := range seeds {
		if !b.add(s, RoleSeed, "", "") {
			continue
		}
		switch g.Node(s).NodeType {
		case model.NodeConcept:
			frontier = append(frontier, s)
		case model.NodeBoundary:
			seedBoundaries = append(seedBoundaries, s)
		case model.NodeArtifact:
			// An artifact seed stands for the concepts it links to.
			for _, e := range g.Out(s, "") {
				if (e.Source == graph.SourceInline || e.Source == graph.SourceInferred) && b.concept(e.O) && b.add(e.O, RoleSeed, s, b.step(s, e.P, e.O)) {
					frontier = append(frontier, e.O)
				}
			}
		}
	}
	// A seed boundary: the boundaries inside it, then what they house.
	var inside []string
	for _, s := range seedBoundaries {
		inside = append(inside, s)
		inside = append(inside, b.nested(s)...)
	}
	for _, bd := range inside {
		for _, rel := range housingRels {
			for _, e := range g.Out(bd, rel) {
				if b.concept(e.O) && b.add(e.O, RoleHoused, bd, b.step(bd, rel, e.O)) {
					frontier = append(frontier, e.O)
				}
			}
		}
	}
	// Stages 1 and 2 (and --follow) repeat until nothing new is added.
	for len(frontier) > 0 {
		sort.Strings(frontier)
		var next []string
		push := func(id, role, parent, step string) {
			if b.concept(id) && b.add(id, role, parent, step) {
				next = append(next, id)
			}
		}
		for _, m := range frontier {
			for _, e := range g.In(m, "is-a") {
				push(e.S, RoleKind, m, b.step(e.S, "is-a", m))
			}
			for _, e := range g.In(m, "part-of") {
				push(e.S, RolePart, m, b.step(e.S, "part-of", m))
			}
			for _, e := range g.Out(m, "has-part") {
				push(e.O, RolePart, m, b.step(m, "has-part", e.O))
			}
			// published-as before realizes: a published mechanism usually
			// realizes its core name too, and published is the stronger fact.
			for _, e := range g.In(m, "published-as") {
				push(e.S, RolePublished, m, b.step(e.S, "published-as", m))
			}
			for _, e := range g.Out(m, "published-as") {
				push(e.O, RolePublished, m, b.step(m, "published-as", e.O))
			}
			for _, e := range g.In(m, "realizes") {
				push(e.S, RoleRealization, m, b.step(e.S, "realizes", m))
			}
			for _, f := range opt.Follow {
				switch f {
				case "produces", "publishes":
					for _, e := range g.Out(m, f) {
						push(e.O, RoleFlow, m, b.step(m, f, e.O))
					}
				default: // consumes, subscribes: whoever takes the member in
					for _, e := range g.In(m, f) {
						push(e.S, RoleFlow, m, b.step(e.S, f, m))
					}
				}
			}
		}
		frontier = next
	}
	concepts := b.ofType(model.NodeConcept)
	// Stage 3: artifacts linked to a member (or inside a seed boundary).
	artRels := map[string]map[string]bool{}
	link := func(a, rel string) {
		if artRels[a] == nil {
			artRels[a] = map[string]bool{}
		}
		artRels[a][rel] = true
	}
	for _, m := range append(append([]string{}, concepts...), inside...) {
		for _, e := range g.In(m, "") {
			if e.Source != graph.SourceInline && e.Source != graph.SourceInferred || !artifactRels[e.P] || !b.isType(e.S, model.NodeArtifact) {
				continue
			}
			b.add(e.S, RoleArtifact, m, b.step(e.S, e.P, m))
			link(e.S, e.P)
		}
	}
	if len(inside) > 0 {
		set := map[string]bool{}
		for _, bd := range inside {
			set[bd] = true
		}
		for _, a := range g.NodesOf(model.NodeArtifact) {
			if set[a.Boundary] {
				b.add(a.ID, RoleArtifact, a.Boundary, b.curie(a.ID)+" in "+b.curie(a.Boundary))
				link(a.ID, "in")
			}
		}
	}
	for a, rels := range artRels {
		if n := b.in[a]; n != nil && n.Role == RoleArtifact && len(rels) == 1 && rels["verifies"] {
			n.Role = RoleTest
		}
	}
	artifacts := b.ofType(model.NodeArtifact)
	// Stage 4: boundaries around the members.
	for _, m := range concepts {
		for _, rel := range boundaryRels {
			for _, e := range g.In(m, rel) {
				if b.isType(e.S, model.NodeBoundary) {
					b.add(e.S, RoleBoundary, m, b.step(e.S, rel, m))
				}
			}
		}
	}
	for _, a := range artifacts {
		if n := g.Node(a); n.Boundary != "" {
			b.add(n.Boundary, RoleBoundary, a, b.curie(a)+" in "+b.curie(n.Boundary))
		}
	}
	// The boxes those boundaries sit in, so the viewer can nest them.
	for _, bd := range b.ofType(model.NodeBoundary) {
		prev := bd
		for _, up := range g.PartOfChain(bd) {
			b.add(up, RoleBoundary, prev, b.step(prev, "part-of", up))
			prev = up
		}
	}
	// Stage 5: one hop of context, never expanded further.
	if opt.Context {
		ctx := func(id, parent, step string) {
			if b.isType(id, model.NodeConcept) || b.isType(id, model.NodeBoundary) {
				b.add(id, RoleContext, parent, step)
			}
		}
		for _, m := range append(append([]string{}, concepts...), artifacts...) {
			for _, rel := range flowRels {
				for _, e := range g.Out(m, rel) {
					if !b.concept(e.O) {
						continue
					}
					ctx(e.O, m, b.step(m, rel, e.O))
					// The producer or consumer on the other side.
					for _, x := range g.In(e.O, counterpart[rel]) {
						ctx(x.S, e.O, b.step(x.S, counterpart[rel], e.O))
					}
				}
			}
		}
		for _, m := range concepts {
			// Who produces, consumes, publishes or subscribes to a member.
			for _, rel := range flowRels {
				for _, e := range g.In(m, rel) {
					ctx(e.S, m, b.step(e.S, rel, m))
				}
			}
			for _, e := range g.In(m, "constrains") {
				ctx(e.S, m, b.step(e.S, "constrains", m))
			}
			for _, e := range g.Out(m, "constrains") {
				ctx(e.O, m, b.step(m, "constrains", e.O))
			}
			// What a member stands for and the wholes it is part of: upward
			// links that stages 1 and 2 never follow.
			for _, e := range g.Out(m, "realizes") {
				ctx(e.O, m, b.step(m, "realizes", e.O))
			}
			for _, e := range g.Out(m, "part-of") {
				if b.concept(e.O) {
					ctx(e.O, m, b.step(m, "part-of", e.O))
				}
			}
			for _, e := range g.In(m, "has-part") {
				ctx(e.S, m, b.step(e.S, "has-part", m))
			}
			for _, e := range g.Out(m, "") {
				if !structural[e.P] && b.concept(e.O) {
					ctx(e.O, m, b.step(m, e.P, e.O))
				}
			}
			for _, e := range g.In(m, "") {
				if !structural[e.P] && b.concept(e.S) {
					ctx(e.S, m, b.step(e.S, e.P, m))
				}
			}
		}
		// is-a ancestors, to show where the feature sits in the taxonomy.
		for _, m := range concepts {
			cur := []string{m}
			for depth := 0; len(cur) > 0 && depth < 16; depth++ {
				var up []string
				for _, c := range cur {
					for _, p := range g.Objects(c, "is-a") {
						if b.concept(p) {
							ctx(p, c, b.step(c, "is-a", p))
							if b.in[p] != nil && b.in[p].Role == RoleContext {
								up = append(up, p)
							}
						}
					}
				}
				cur = up
			}
		}
	}
	return b.result(seeds)
}

// nested returns the boundaries inside bd (part-of, transitively), sorted.
func (b *builder) nested(bd string) []string {
	seen := map[string]bool{bd: true}
	var out []string
	stack := []string{bd}
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, e := range b.g.In(x, "part-of") {
			if b.isType(e.S, model.NodeBoundary) && !seen[e.S] {
				seen[e.S] = true
				out = append(out, e.S)
				stack = append(stack, e.S)
				b.add(e.S, RoleBoundary, x, b.step(e.S, "part-of", x))
			}
		}
	}
	sort.Strings(out)
	return out
}

// ofType returns the nodes of one type in the order they were added, so a
// later stage credits the member nearest a seed.
func (b *builder) ofType(t model.NodeType) []string {
	var out []string
	for _, id := range b.order {
		if b.isType(id, t) {
			out = append(out, id)
		}
	}
	return out
}

func (b *builder) result(seeds []string) *Slice {
	s := &Slice{Context: b.opt.Context, Follow: b.opt.Follow, Nodes: []Node{}, Edges: []Edge{}}
	for _, id := range seeds {
		if n := b.in[id]; n != nil {
			s.Seeds = append(s.Seeds, n.ID)
		}
	}
	for _, id := range b.order {
		s.Nodes = append(s.Nodes, *b.in[id])
	}
	sort.SliceStable(s.Nodes, func(i, j int) bool {
		if ri, rj := roleRank(s.Nodes[i].Role), roleRank(s.Nodes[j].Role); ri != rj {
			return ri < rj
		}
		return s.Nodes[i].ID < s.Nodes[j].ID
	})
	// Every edge with both ends in the slice, one per (s, p, o).
	src := map[string]map[string]bool{}
	var keys []string
	for _, id := range b.order {
		for _, e := range b.g.Out(id, "") {
			if b.in[e.O] == nil || e.S == e.O {
				continue
			}
			k := b.in[e.S].ID + "\x00" + e.P + "\x00" + b.in[e.O].ID
			if src[k] == nil {
				src[k] = map[string]bool{}
				keys = append(keys, k)
			}
			src[k][e.Source] = true
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts := strings.SplitN(k, "\x00", 3)
		source := "mixed"
		if len(src[k]) == 1 {
			for x := range src[k] {
				source = x
			}
		}
		s.Edges = append(s.Edges, Edge{S: parts[0], P: parts[1], O: parts[2], Source: source})
	}
	s.summarize()
	return s
}

func (s *Slice) summarize() {
	sum := Summary{}
	for _, n := range s.Nodes {
		switch {
		case n.Role == RoleContext:
			sum.Context++
			continue
		case n.Type == string(model.NodeConcept):
			sum.Concepts++
		case n.Role == RoleTest:
			sum.Tests++
		case n.Type == string(model.NodeArtifact):
			sum.Artifacts++
		case n.Type == string(model.NodeBoundary):
			sum.Boundaries++
		}
		if n.Type != string(model.NodeBoundary) {
			sum.Members++
		}
		sum.Criticality = model.MaxCriticality(sum.Criticality, n.Criticality)
	}
	sum.Untested = s.Summary.Untested
	s.Summary = sum
}

// Annotate attaches the drift findings (other than consistent ones) to the
// concepts in the slice, and lists the untested members in the summary.
func Annotate(s *Slice, findings []rollup.Finding) {
	idx := map[string]int{}
	for i, n := range s.Nodes {
		idx[n.ID] = i
	}
	unt := map[string]bool{}
	for _, f := range findings {
		i, ok := idx[f.Concept]
		if !ok || f.Kind == rollup.KindConsistent {
			continue
		}
		s.Nodes[i].Findings = append(s.Nodes[i].Findings, f.Kind+": "+f.Msg)
		if f.Kind == rollup.KindUntested && s.Nodes[i].Role != RoleContext {
			unt[f.Concept] = true
		}
	}
	for i := range s.Nodes {
		s.Nodes[i].Findings = uniqueSorted(s.Nodes[i].Findings)
	}
	s.Summary.Untested = sortedKeys(unt)
}

// Of resolves each argument (Resolve), computes the slice and annotates it
// with findings. The CLI and the MCP tool both call it, so they agree.
func Of(g *graph.Graph, root string, args []string, opt Options, findings []rollup.Finding) (*Slice, error) {
	var seeds []string
	for _, a := range args {
		ids, err := Resolve(g, root, a)
		if err != nil {
			return nil, err
		}
		seeds = append(seeds, ids...)
	}
	if len(seeds) == 0 {
		return nil, fmt.Errorf("no seed: pass a concept or boundary CURIE, an artifact or a path")
	}
	s := Compute(g, seeds, opt)
	Annotate(s, findings)
	return s, nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func uniqueSorted(xs []string) []string {
	if len(xs) == 0 {
		return xs
	}
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return sortedKeys(m)
}
