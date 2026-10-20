// @gloss represents gmi:CanonicalGraph scope=file
// @gloss represents gm:Artifact, gm:Concept, gm:Boundary scope=file

// Package graph is the canonical GlossMark graph: nodes and edges with
// provenance, IRIs with CURIE aliases, computed closures, and a deterministic
// newline-delimited JSON encoding that diffs cleanly in git.
package graph

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/version"
)

// ArtifactPrefix is the IRI scheme for artifacts: urn:gloss:artifact:<path>[#<symbol>].
const ArtifactPrefix = "urn:gloss:artifact:"

// Edge sources.
const (
	SourceDeclared = "declared" // ontology files and manifests
	SourceInline   = "inline"   // `@gloss` markers
	SourceDerived  = "derived"  // rollup rules
	SourceInferred = "inferred" // inferrers (test paths)
)

// Node is one graph node.
type Node struct {
	Type     string         `json:"type"`
	NodeType model.NodeType `json:"node"`
	ID       string         `json:"id"`
	CURIE    string         `json:"curie,omitempty"`
	NS       string         `json:"ns,omitempty"`
	Tier     string         `json:"tier,omitempty"`
	Kind     string         `json:"kind,omitempty"`
	// KindFrom is the CURIE of the is-a ancestor a concept's kind was
	// inherited from; empty when the kind is declared.
	KindFrom string `json:"kind-from,omitempty"`
	Label    string `json:"label,omitempty"`
	Doc      string `json:"doc,omitempty"`
	// Concept attributes.
	Published  string `json:"published-as,omitempty"`
	Internal   bool   `json:"internal,omitempty"`
	Deprecated bool   `json:"deprecated,omitempty"`
	ReplacedBy string `json:"replaced-by,omitempty"`
	Value      any    `json:"value,omitempty"`
	// ValueFrom is path:line of the code a published value is bound to;
	// LiveValue is that line, ValueFP its fingerprint (see model.ValueFrom).
	ValueFrom  string   `json:"value-from,omitempty"`
	LiveValue  string   `json:"live-value,omitempty"`
	ValueFP    string   `json:"value-fp,omitempty"`
	Imported   bool     `json:"imported,omitempty"`
	Proposed   bool     `json:"proposed,omitempty"`
	ProposedBy string   `json:"proposed-by,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
	// Criticality is the effective governance level (low | medium | high |
	// critical) of a concept, boundary or artifact; CriticalityFrom says where
	// it came from: "declared", or the CURIE it was inherited from.
	Criticality     string `json:"criticality,omitempty"`
	CriticalityFrom string `json:"criticality-from,omitempty"`
	// CriticalityFloor: the boundary's level is a floor for its artifacts.
	CriticalityFloor bool `json:"criticality-floor,omitempty"`
	// Namespace attributes.
	Version string `json:"version,omitempty"`
	Owner   string `json:"owner,omitempty"`
	// Artifact attributes.
	Path   string         `json:"path,omitempty"`
	Anchor *marker.Anchor `json:"anchor,omitempty"`
	// Boundary (IRI) that contains an artifact, from the nearest manifest.
	Boundary string `json:"boundary,omitempty"`
	// Boundary: directory of its manifest, and the date a deprecated
	// boundary is due to be retired (deprecated and replaced-by above).
	Dir      string `json:"dir,omitempty"`
	RetireBy string `json:"retire-by,omitempty"`
	// Source location of the declaration, for diagnostics.
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
}

// Prov is edge provenance.
type Prov struct {
	Path        string   `json:"path,omitempty"`
	Line        int      `json:"line,omitempty"`
	Col         int      `json:"col,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	Symbol      string   `json:"symbol,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	Proposed    bool     `json:"proposed,omitempty"`
	Conf        *float64 `json:"conf,omitempty"`
	ID          string   `json:"id,omitempty"`
	Note        string   `json:"note,omitempty"`
	Actor       string   `json:"actor,omitempty"`
	Sugar       bool     `json:"sugar,omitempty"`
	// Rule names the rollup rule or inferrer that produced a derived edge.
	Rule string `json:"rule,omitempty"`
	// Via lists the CURIEs the derivation passed through.
	Via []string `json:"via,omitempty"`
	// Raw is the original target text (e.g. ".Local", "impl:NEW:X", "pa:*").
	Raw string `json:"raw,omitempty"`
	// Alias is the relation as written when the marker used an alias that
	// was stored as its canonical relation ("tests", stored as verifies).
	Alias string `json:"alias,omitempty"`
}

// Edge is one triple with provenance.
type Edge struct {
	Type   string `json:"type"`
	S      string `json:"s"`
	P      string `json:"p"`
	O      string `json:"o"`
	Source string `json:"source"`
	Prov   *Prov  `json:"prov,omitempty"`
}

// Key identifies an edge for dedupe: same triple from the same place.
func (e *Edge) Key() string {
	k := e.S + "|" + e.P + "|" + e.O + "|" + e.Source
	if e.Prov != nil {
		k += "|" + e.Prov.Path + "|" + fmt.Sprint(e.Prov.Line) + "|" + e.Prov.Rule
	}
	return k
}

// Header is the first JSONL line.
type Header struct {
	Type      string            `json:"type"`
	Format    int               `json:"format"`
	Generator string            `json:"generator"`
	Prefixes  map[string]string `json:"prefixes"`
	Context   string            `json:"@context,omitempty"`
}

// Graph holds nodes, edges and indexes.
type Graph struct {
	Generator string
	Prefixes  map[string]string
	nodes     map[string]*Node
	edges     []*Edge
	keys      map[string]bool
	out       map[string]map[string][]*Edge
	in        map[string]map[string][]*Edge
}

// New returns an empty graph with the given prefix table.
func New(prefixes map[string]string) *Graph {
	g := &Graph{Prefixes: map[string]string{}, nodes: map[string]*Node{}, keys: map[string]bool{},
		out: map[string]map[string][]*Edge{}, in: map[string]map[string][]*Edge{}}
	for k, v := range prefixes {
		g.Prefixes[k] = v
	}
	return g
}

// Expand turns a CURIE into an IRI. Unknown prefixes yield an unresolved URN and ok=false.
func (g *Graph) Expand(curie string) (string, bool) {
	if strings.Contains(curie, "://") || strings.HasPrefix(curie, "urn:") {
		return curie, true
	}
	i := strings.Index(curie, ":")
	if i <= 0 {
		return "urn:gloss:unresolved:" + curie, false
	}
	base, ok := g.Prefixes[curie[:i]]
	if !ok {
		return "urn:gloss:unresolved:" + curie, false
	}
	return base + curie[i+1:], true
}

// Curie turns an IRI back into a CURIE when a prefix matches (longest wins).
func (g *Graph) Curie(iri string) string {
	if n, ok := g.nodes[iri]; ok && n.CURIE != "" {
		return n.CURIE
	}
	if strings.HasPrefix(iri, "urn:gloss:unresolved:") {
		return strings.TrimPrefix(iri, "urn:gloss:unresolved:")
	}
	if strings.HasPrefix(iri, "urn:gloss:glob:") {
		return strings.TrimPrefix(iri, "urn:gloss:glob:")
	}
	if strings.HasPrefix(iri, "urn:gloss:new:") {
		return strings.TrimPrefix(iri, "urn:gloss:new:")
	}
	if strings.HasPrefix(iri, ArtifactPrefix) {
		return strings.TrimPrefix(iri, ArtifactPrefix)
	}
	best, bestLen := "", -1
	for p, base := range g.Prefixes {
		if strings.HasPrefix(iri, base) && len(base) > bestLen {
			best, bestLen = p, len(base)
		}
	}
	if bestLen < 0 {
		return iri
	}
	return best + ":" + iri[bestLen:]
}

// AddNode inserts or merges a node by ID.
func (g *Graph) AddNode(n *Node) *Node {
	n.Type = "node"
	if ex, ok := g.nodes[n.ID]; ok {
		// Merge: keep the first declaration's fields, fill blanks.
		if ex.Kind == "" {
			ex.Kind, ex.KindFrom = n.Kind, n.KindFrom
		}
		if ex.Label == "" {
			ex.Label = n.Label
		}
		if ex.Tier == "" {
			ex.Tier = n.Tier
		}
		if ex.NS == "" {
			ex.NS = n.NS
		}
		if ex.CURIE == "" {
			ex.CURIE = n.CURIE
		}
		if ex.File == "" {
			ex.File, ex.Line = n.File, n.Line
		}
		if ex.Owner == "" {
			ex.Owner = n.Owner
		}
		if ex.Criticality == "" {
			ex.Criticality, ex.CriticalityFrom = n.Criticality, n.CriticalityFrom
		}
		return ex
	}
	g.nodes[n.ID] = n
	return n
}

// Node returns the node with the given IRI or nil.
func (g *Graph) Node(id string) *Node { return g.nodes[id] }

// NodeByCurie looks up a node by CURIE.
func (g *Graph) NodeByCurie(curie string) *Node {
	iri, _ := g.Expand(curie)
	return g.nodes[iri]
}

// Nodes returns all nodes sorted by (type, id).
func (g *Graph) Nodes() []*Node {
	out := make([]*Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].NodeType != out[j].NodeType {
			return out[i].NodeType < out[j].NodeType
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// NodesOf returns nodes of one type, sorted.
func (g *Graph) NodesOf(t model.NodeType) []*Node {
	var out []*Node
	for _, n := range g.Nodes() {
		if n.NodeType == t {
			out = append(out, n)
		}
	}
	return out
}

// AddEdge inserts an edge unless an identical one exists. Returns true when added.
func (g *Graph) AddEdge(e *Edge) bool {
	e.Type = "edge"
	k := e.Key()
	if g.keys[k] {
		return false
	}
	g.keys[k] = true
	g.edges = append(g.edges, e)
	if g.out[e.S] == nil {
		g.out[e.S] = map[string][]*Edge{}
	}
	g.out[e.S][e.P] = append(g.out[e.S][e.P], e)
	if g.in[e.O] == nil {
		g.in[e.O] = map[string][]*Edge{}
	}
	g.in[e.O][e.P] = append(g.in[e.O][e.P], e)
	return true
}

// RemoveEdges drops edges matching the predicate (used to rebuild derived edges).
func (g *Graph) RemoveEdges(pred func(*Edge) bool) int {
	var keep []*Edge
	n := 0
	for _, e := range g.edges {
		if pred(e) {
			n++
			continue
		}
		keep = append(keep, e)
	}
	g.edges = nil
	g.keys = map[string]bool{}
	g.out = map[string]map[string][]*Edge{}
	g.in = map[string]map[string][]*Edge{}
	for _, e := range keep {
		g.AddEdge(e)
	}
	return n
}

// Edges returns all edges, sorted deterministically.
func (g *Graph) Edges() []*Edge {
	out := append([]*Edge{}, g.edges...)
	sort.Slice(out, func(i, j int) bool { return edgeLess(out[i], out[j]) })
	return out
}

func edgeLess(a, b *Edge) bool {
	if a.S != b.S {
		return a.S < b.S
	}
	if a.P != b.P {
		return a.P < b.P
	}
	if a.O != b.O {
		return a.O < b.O
	}
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	ap, bp := "", ""
	al, bl := 0, 0
	if a.Prov != nil {
		ap, al = a.Prov.Path, a.Prov.Line
	}
	if b.Prov != nil {
		bp, bl = b.Prov.Path, b.Prov.Line
	}
	if ap != bp {
		return ap < bp
	}
	return al < bl
}

// Out returns edges with subject s and predicate p ("" for any predicate).
func (g *Graph) Out(s, p string) []*Edge {
	m := g.out[s]
	if m == nil {
		return nil
	}
	if p != "" {
		return m[p]
	}
	var out []*Edge
	for _, es := range m {
		out = append(out, es...)
	}
	sort.Slice(out, func(i, j int) bool { return edgeLess(out[i], out[j]) })
	return out
}

// In returns edges with object o and predicate p ("" for any predicate).
func (g *Graph) In(o, p string) []*Edge {
	m := g.in[o]
	if m == nil {
		return nil
	}
	if p != "" {
		return m[p]
	}
	var out []*Edge
	for _, es := range m {
		out = append(out, es...)
	}
	sort.Slice(out, func(i, j int) bool { return edgeLess(out[i], out[j]) })
	return out
}

// Objects returns the distinct, sorted objects of (s, p).
func (g *Graph) Objects(s, p string) []string {
	set := map[string]bool{}
	for _, e := range g.Out(s, p) {
		set[e.O] = true
	}
	return sortedKeys(set)
}

// Subjects returns the distinct, sorted subjects of (p, o).
func (g *Graph) Subjects(o, p string) []string {
	set := map[string]bool{}
	for _, e := range g.In(o, p) {
		set[e.S] = true
	}
	return sortedKeys(set)
}

// Closure follows predicate p from start (forward when inverse is false,
// backward otherwise) and returns every reachable node, excluding start,
// sorted. Cycles are tolerated.
func (g *Graph) Closure(start, p string, inverse bool) []string {
	seen := map[string]bool{}
	stack := []string{start}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		var next []string
		if inverse {
			next = g.Subjects(cur, p)
		} else {
			next = g.Objects(cur, p)
		}
		for _, n := range next {
			if n == start || seen[n] {
				continue
			}
			seen[n] = true
			stack = append(stack, n)
		}
	}
	return sortedKeys(seen)
}

// Ancestors returns the is-a closure of a concept (its superclasses).
func (g *Graph) Ancestors(c string) []string { return g.Closure(c, "is-a", false) }

// Descendants returns the inverse is-a closure.
func (g *Graph) Descendants(c string) []string { return g.Closure(c, "is-a", true) }

// Subsumes reports whether general is-a* covers specific (reflexive).
func (g *Graph) Subsumes(general, specific string) bool {
	if general == specific {
		return true
	}
	for _, a := range g.Ancestors(specific) {
		if a == general {
			return true
		}
	}
	return false
}

// LCS returns the least common subsumers of a set of concepts: the deepest
// concepts that is-a* every input. A single input returns itself. When no
// common ancestor exists the inputs are returned unchanged.
func (g *Graph) LCS(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	ids = uniqueSorted(ids)
	if len(ids) == 1 {
		return ids
	}
	var common map[string]bool
	for _, id := range ids {
		anc := map[string]bool{id: true}
		for _, a := range g.Ancestors(id) {
			anc[a] = true
		}
		if common == nil {
			common = anc
			continue
		}
		for k := range common {
			if !anc[k] {
				delete(common, k)
			}
		}
	}
	if len(common) == 0 {
		return ids
	}
	// Keep the most specific: drop any that is a strict ancestor of another common one.
	var out []string
	for c := range common {
		dominated := false
		for d := range common {
			if d != c && g.Subsumes(c, d) {
				dominated = true
				break
			}
		}
		if !dominated {
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

// Step is one edge of a cycle, followed from From to To. Edge is the edge
// as written: for an inverse relation (has-part read as part-of) its
// subject is To.
type Step struct {
	From, To string
	Edge     *Edge
}

// Cycles returns the cycles that next forms among nodes: one per strongly
// connected component of two or more nodes, and one per self-loop. Each
// cycle starts at the component's smallest IRI and lists its steps in
// order, along a shortest way back. Cycles are sorted by their first node.
func (g *Graph) Cycles(nodes []string, next func(id string) []Step) [][]Step {
	in := map[string]bool{}
	for _, n := range nodes {
		in[n] = true
	}
	steps := func(id string) []Step {
		var out []Step
		for _, s := range next(id) {
			if in[s.To] {
				out = append(out, s)
			}
		}
		return out
	}
	// Tarjan's strongly connected components.
	index, low := map[string]int{}, map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	var comps [][]string
	var strong func(v string)
	strong = func(v string) {
		index[v], low[v] = len(index), len(index)
		stack = append(stack, v)
		onStack[v] = true
		for _, s := range steps(v) {
			if _, seen := index[s.To]; !seen {
				strong(s.To)
				low[v] = min(low[v], low[s.To])
			} else if onStack[s.To] {
				low[v] = min(low[v], index[s.To])
			}
		}
		if low[v] == index[v] {
			var comp []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				comp = append(comp, w)
				if w == v {
					break
				}
			}
			comps = append(comps, comp)
		}
	}
	for _, n := range uniqueSorted(nodes) {
		if _, seen := index[n]; !seen {
			strong(n)
		}
	}
	var out [][]Step
	for _, comp := range comps {
		sort.Strings(comp)
		start := comp[0]
		member := map[string]bool{}
		for _, c := range comp {
			member[c] = true
		}
		if len(comp) == 1 {
			for _, s := range steps(start) {
				if s.To == start {
					out = append(out, []Step{s})
					break
				}
			}
			continue
		}
		// Shortest way from start back to start inside the component.
		prev := map[string]Step{}
		queue := []string{start}
		visited := map[string]bool{}
		var last *Step
		for len(queue) > 0 && last == nil {
			cur := queue[0]
			queue = queue[1:]
			for _, s := range steps(cur) {
				if !member[s.To] {
					continue
				}
				if s.To == start {
					s := s
					last = &s
					break
				}
				if !visited[s.To] {
					visited[s.To] = true
					prev[s.To] = s
					queue = append(queue, s.To)
				}
			}
		}
		if last == nil {
			continue
		}
		cycle := []Step{*last}
		for at := last.From; at != start; at = prev[at].From {
			cycle = append([]Step{prev[at]}, cycle...)
		}
		out = append(out, cycle)
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0].From < out[j][0].From })
	return out
}

// Boundaries returns the part-of chain of a node upward (nearest first).
func (g *Graph) PartOfChain(id string) []string {
	var out []string
	seen := map[string]bool{id: true}
	cur := id
	for {
		ps := g.Objects(cur, "part-of")
		if len(ps) == 0 {
			return out
		}
		next := ps[0]
		if seen[next] {
			return out
		}
		seen[next] = true
		out = append(out, next)
		cur = next
	}
}

// WriteJSONL writes the header, nodes and edges deterministically.
func (g *Graph) WriteJSONL(w io.Writer) error {
	bw := bufio.NewWriter(w)
	enc := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		bw.Write(b)
		return bw.WriteByte('\n')
	}
	if err := enc(Header{Type: "context", Format: version.GraphFormat, Generator: g.Generator, Prefixes: g.Prefixes, Context: "./context.jsonld"}); err != nil {
		return err
	}
	for _, n := range g.Nodes() {
		if err := enc(n); err != nil {
			return err
		}
	}
	for _, e := range g.Edges() {
		if err := enc(e); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// ReadJSONL loads a graph written by WriteJSONL.
func ReadJSONL(r io.Reader) (*Graph, error) {
	g := New(nil)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			return nil, fmt.Errorf("graph.jsonl:%d: %w", lineNo, err)
		}
		switch probe.Type {
		case "context":
			var h Header
			if err := json.Unmarshal([]byte(line), &h); err != nil {
				return nil, err
			}
			g.Generator = h.Generator
			if h.Format > version.GraphFormat {
				return nil, fmt.Errorf("graph.jsonl: format %d was written by a newer gloss (%s); this release reads format %d. Upgrade gloss or rebuild with: gloss build", h.Format, h.Generator, version.GraphFormat)
			}
			for k, v := range h.Prefixes {
				g.Prefixes[k] = v
			}
		case "node":
			var n Node
			if err := json.Unmarshal([]byte(line), &n); err != nil {
				return nil, fmt.Errorf("graph.jsonl:%d: %w", lineNo, err)
			}
			g.AddNode(&n)
		case "edge":
			var e Edge
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				return nil, fmt.Errorf("graph.jsonl:%d: %w", lineNo, err)
			}
			g.AddEdge(&e)
		default:
			return nil, fmt.Errorf("graph.jsonl:%d: unknown record type %q", lineNo, probe.Type)
		}
	}
	return g, sc.Err()
}

// Stats summarizes the graph.
func (g *Graph) Stats() map[string]int {
	s := map[string]int{"nodes": len(g.nodes), "edges": len(g.edges)}
	for _, n := range g.nodes {
		s["nodes."+string(n.NodeType)]++
	}
	for _, e := range g.edges {
		s["edges."+e.Source]++
	}
	return s
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
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return sortedKeys(m)
}

// Label returns a display label: the node label, else its CURIE, else the IRI.
func (g *Graph) Label(id string) string {
	if n := g.nodes[id]; n != nil {
		if n.Label != "" {
			return n.Label
		}
		if n.CURIE != "" {
			return n.CURIE
		}
	}
	return g.Curie(id)
}
