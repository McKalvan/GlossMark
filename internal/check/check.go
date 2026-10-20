// @gloss implements gmi:Linter scope=file

// Package check lints the graph: syntax, dangling references, domain/range,
// the own-subject rule, tier rules, the published seam and manifest allowed lists.
package check

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// Run returns lint diagnostics for a built graph. Codes:
//
//	GM2xx references   GM3xx ontology tiers   GM4xx own-subject   GM5xx markers/manifests   GM6xx governance
func Run(ws *load.Workspace, g *graph.Graph) []marker.Diagnostic {
	c := &checker{ws: ws, g: g}
	c.ontologyFiles()
	c.concepts()
	c.cycles()
	c.edges()
	c.markers()
	c.manifests()
	c.governance()
	c.waivers()
	sort.SliceStable(c.out, func(i, j int) bool {
		a, b := c.out[i], c.out[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Code < b.Code
	})
	return c.out
}

type checker struct {
	ws   *load.Workspace
	g    *graph.Graph
	out  []marker.Diagnostic
	seen map[marker.Diagnostic]bool
}

// add records a diagnostic once. Edge checks run per target, so a marker or
// ontology key with several targets would otherwise repeat one finding about
// its relation once per target.
func (c *checker) add(path string, line int, sev, code, msg string) {
	d := marker.Diagnostic{Path: path, Line: line, Severity: sev, Code: code, Msg: msg}
	if c.seen == nil {
		c.seen = map[marker.Diagnostic]bool{}
	}
	if c.seen[d] {
		return
	}
	c.seen[d] = true
	c.out = append(c.out, d)
}

func (c *checker) prefixOf(curie string) string {
	if i := strings.Index(curie, ":"); i > 0 {
		return curie[:i]
	}
	return ""
}

// ontologyFiles lints the YAML shape of owned ontology files. Inside a { }
// flow mapping an unquoted doc ends at the first comma and the rest of the
// sentence becomes null-valued keys; a null relation produces no edge, so
// nothing else would notice. GM305 names the truncated doc, GM304 any other
// null key.
func (c *checker) ontologyFiles() {
	for _, p := range sortedKeys(c.ws.Ontologies) {
		if !c.ws.Owned[p] {
			continue
		}
		o := c.ws.Ontologies[p]
		file := relFile(c.ws, o.Path)
		for _, name := range sortedKeys(o.Concepts) {
			con := o.Concepts[name]
			if con == nil || len(con.NullKeys) == 0 {
				continue
			}
			curie := p + ":" + name
			if con.Flow && con.Doc != "" && looksTruncated(con.Doc) {
				c.add(file, con.Line, "error", "GM305", fmt.Sprintf("%s: doc is cut at a comma inside { }: %q; the rest became null keys (%s). Quote the doc or write the entry in block style", curie, con.Doc, strings.Join(con.NullKeys, ", ")))
				continue
			}
			for _, k := range con.NullKeys {
				c.add(file, con.Line, "warning", "GM304", fmt.Sprintf("%s: key %q has no value, so it adds no edge; give it targets or remove it", curie, k))
			}
		}
	}
}

// looksTruncated reports whether a doc stops mid-sentence: no terminal
// punctuation, or a parenthesis or bracket left open.
func looksTruncated(doc string) bool {
	d := strings.TrimSpace(doc)
	if d == "" {
		return false
	}
	depth := 0
	for _, ch := range d {
		switch ch {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		}
	}
	if depth > 0 {
		return true
	}
	return !strings.ContainsAny(d[len(d)-1:], ".!?")
}

func (c *checker) concepts() {
	for _, n := range c.g.NodesOf(model.NodeConcept) {
		if n.Imported {
			continue
		}
		if n.Kind == "" {
			// kind is inherited through is-a when an ancestor declares one;
			// with no such ancestor the concept has none.
			c.add(n.File, n.Line, "error", "GM300", fmt.Sprintf("%s: kind is required (one of %s) unless an is-a ancestor declares one", n.CURIE, strings.Join(model.ConceptKinds, ", ")))
		} else if n.KindFrom != "" {
			// Inherited: the ancestor's own entry is checked. Parents that
			// disagree leave the kind ambiguous.
			kinds := map[string][]string{}
			for _, p := range c.g.Objects(n.ID, "is-a") {
				if pn := c.g.Node(p); pn != nil && pn.NodeType == model.NodeConcept && pn.Kind != "" {
					kinds[pn.Kind] = append(kinds[pn.Kind], pn.CURIE)
				}
			}
			if len(kinds) > 1 {
				var parts []string
				for _, k := range sortedKeys(kinds) {
					parts = append(parts, fmt.Sprintf("%s from %s", k, strings.Join(kinds[k], ", ")))
				}
				c.add(n.File, n.Line, "error", "GM306", fmt.Sprintf("%s: is-a parents give different kinds (%s); declare kind: on the concept", n.CURIE, strings.Join(parts, "; ")))
			}
		} else if !contains(model.ConceptKinds, n.Kind) {
			c.add(n.File, n.Line, "error", "GM301", fmt.Sprintf("%s: unknown kind %q; allowed: %s", n.CURIE, n.Kind, strings.Join(model.ConceptKinds, ", ")))
		}
		if !isPascal(strings.TrimPrefix(n.CURIE, n.NS+":")) {
			c.add(n.File, n.Line, "warning", "GM302", fmt.Sprintf("%s: concept names are PascalCase", n.CURIE))
		}
		if n.CriticalityFrom == "declared" && !model.ValidCriticality(n.Criticality) {
			c.add(n.File, n.Line, "error", "GM303", fmt.Sprintf("%s: unknown criticality %q; allowed: %s", n.CURIE, n.Criticality, strings.Join(model.CriticalityLevels, ", ")))
		}
		switch n.Tier {
		case model.TierCore:
			// Core may not reference any implementation namespace.
			for _, e := range c.g.Out(n.ID, "") {
				if e.Source != graph.SourceDeclared {
					continue
				}
				if on := c.g.Node(e.O); on != nil && on.Tier == model.TierImpl {
					c.add(n.File, n.Line, "error", "GM310", fmt.Sprintf("%s: core concept references implementation concept %s via %s; core is implementation-independent by construction", n.CURIE, on.CURIE, e.P))
				}
			}
			if n.Published != "" {
				c.add(n.File, n.Line, "error", "GM311", fmt.Sprintf("%s: published: is only meaningful on implementation concepts", n.CURIE))
			}
		case model.TierImpl:
			realizes := c.g.Objects(n.ID, "realizes")
			if !ImplComplete(c.g, n) {
				sev := "error"
				if n.Proposed {
					sev = "warning" // a proposal may be incomplete; acceptance requires the full entry
				}
				c.add(n.File, n.Line, sev, "GM320", fmt.Sprintf("%s: implementation concept must carry realizes: <core concept> (or presents:/hosts: for a page or a shell), is-a or part-of a concept that does, or internal: true", n.CURIE))
			}
			for _, r := range realizes {
				rn := c.g.Node(r)
				if rn == nil {
					continue
				}
				if rn.Tier != model.TierCore && rn.NodeType == model.NodeConcept {
					c.add(n.File, n.Line, "error", "GM321", fmt.Sprintf("%s: realizes must point at a core concept, %s is %s", n.CURIE, rn.CURIE, rn.Tier))
				}
			}
			if n.Published != "" {
				pn := c.g.NodeByCurie(n.Published)
				switch {
				case pn == nil:
					c.add(n.File, n.Line, "error", "GM330", fmt.Sprintf("%s: published as: %s names no existing core concept; propose it through a decision request first (publishing never mints a core name)", n.CURIE, n.Published))
				case pn.Tier != model.TierCore:
					c.add(n.File, n.Line, "error", "GM331", fmt.Sprintf("%s: published as: must name a core concept; %s is %s", n.CURIE, pn.CURIE, pn.Tier))
				}
				if n.Internal {
					c.add(n.File, n.Line, "error", "GM332", fmt.Sprintf("%s: a concept cannot be both internal and published", n.CURIE))
				}
			}
		}
		if n.Proposed && n.Imported {
			c.add(n.File, n.Line, "error", "GM350", fmt.Sprintf("%s: proposed concepts live in owned namespaces; propose upstream through a decision request", n.CURIE))
		}
		if n.Deprecated && n.ReplacedBy == "" {
			c.add(n.File, n.Line, "warning", "GM340", fmt.Sprintf("%s: deprecated concepts should carry replaced-by:", n.CURIE))
		}
		if n.ReplacedBy != "" && c.g.NodeByCurie(n.ReplacedBy) == nil {
			c.add(n.File, n.Line, "error", "GM341", fmt.Sprintf("%s: replaced-by %s does not exist", n.CURIE, n.ReplacedBy))
		}
	}
}

// standsFor reports whether an implementation concept, or a concept it is-a
// or is part of (transitively), realizes, is published as, presents or hosts
// something, is internal, or is a core concept. That is what GM320 asks of
// every implementation concept. Two concepts that are part of each other and
// realize nothing are not excused by the edge between them.
func standsFor(g *graph.Graph, id string) bool {
	seen := map[string]bool{id: true}
	queue := []string{id}
	for len(queue) > 0 {
		x := queue[0]
		queue = queue[1:]
		n := g.Node(x)
		if n == nil || n.NodeType != model.NodeConcept {
			continue
		}
		if x != id && (n.Tier == model.TierCore || n.Internal) {
			return true
		}
		for _, p := range []string{"realizes", "published-as", "presents", "hosts"} {
			for _, e := range g.Out(x, p) {
				if e.Source != graph.SourceDerived {
					return true
				}
			}
		}
		for _, p := range []string{"is-a", "part-of"} {
			for _, e := range g.Out(x, p) {
				if e.Source != graph.SourceDerived && !seen[e.O] {
					seen[e.O] = true
					queue = append(queue, e.O)
				}
			}
		}
	}
	return false
}

// cycles reports the cycles in the hierarchies: an is-a cycle (GM370, a
// self-loop included) makes every concept on it subsume every other and so
// widens every declaration that names one; a part-of or has-part cycle among
// concepts (GM371) makes each a part of itself; a part-of cycle among
// boundaries (GM510) makes each roll up the other's code. Each error names
// the members in order with the file and line of every edge.
func (c *checker) cycles() {
	var concepts, boundaries []string
	for _, n := range c.g.NodesOf(model.NodeConcept) {
		concepts = append(concepts, n.ID)
	}
	for _, n := range c.g.NodesOf(model.NodeBoundary) {
		boundaries = append(boundaries, n.ID)
	}
	forward := func(p string) func(string) []graph.Step {
		return func(id string) []graph.Step {
			var out []graph.Step
			for _, e := range c.g.Out(id, p) {
				if e.Source == graph.SourceDeclared {
					out = append(out, graph.Step{From: id, To: e.O, Edge: e})
				}
			}
			return out
		}
	}
	// W has-part P states P part-of W.
	partOf := func(id string) []graph.Step {
		out := forward("part-of")(id)
		for _, e := range c.g.In(id, "has-part") {
			if e.Source == graph.SourceDeclared {
				out = append(out, graph.Step{From: id, To: e.S, Edge: e})
			}
		}
		return out
	}
	report := func(code, what string, cycle []graph.Step) {
		var parts []string
		for _, s := range cycle {
			parts = append(parts, fmt.Sprintf("%s %s %s (%s:%d)", c.g.Curie(s.Edge.S), s.Edge.P, c.g.Curie(s.Edge.O), provPath(s.Edge), provLine(s.Edge)))
		}
		first := cycle[0].Edge
		c.add(provPath(first), provLine(first), "error", code, fmt.Sprintf("%s cycle: %s; %s", what, strings.Join(parts, " → "), cycleAdvice[code]))
	}
	for _, cy := range c.g.Cycles(concepts, forward("is-a")) {
		report("GM370", "is-a", cy)
	}
	for _, cy := range c.g.Cycles(concepts, partOf) {
		report("GM371", "part-of", cy)
	}
	for _, cy := range c.g.Cycles(boundaries, forward("part-of")) {
		report("GM510", "boundary part-of", cy)
	}
}

var cycleAdvice = map[string]string{
	"GM370": "every concept on it subsumes every other, which widens every declaration that names one; remove one edge",
	"GM371": "every concept on it is a part of itself; remove one edge",
	"GM510": "every boundary on it rolls up the others' code; remove one part-of",
}

func provPath(e *graph.Edge) string {
	if e.Prov == nil {
		return ""
	}
	return e.Prov.Path
}

func provLine(e *graph.Edge) int {
	if e.Prov == nil {
		return 0
	}
	return e.Prov.Line
}

// markers lints redundancy between inline markers. A symbol-level marker
// that repeats a file-level marker's relation and target states the same
// fact twice and makes two edges (GM130); the same marker on consecutive
// declarations, three or more in a row, usually wants one scope=block
// marker above the run (GM131).
// Both passed silently and inflated file coverage in the first adoptions.
func (c *checker) markers() {
	type fact struct{ path, rel, obj string }
	// decl is one anchor that markers bind to, in file order; facts maps
	// "relation\x00object" to the line of the marker that states it.
	type decl struct {
		symbol bool
		start  int // first line of the anchored declaration
		facts  map[string]int
	}
	fileLevel := map[fact]int{}
	byPath := map[string][]*graph.Edge{}
	for _, e := range c.g.Edges() {
		if e.Source != graph.SourceInline || e.Prov == nil || e.P == "may-modify" {
			continue
		}
		if e.Prov.Scope == "file" {
			fileLevel[fact{e.Prov.Path, e.P, e.O}] = e.Prov.Line
		}
		byPath[e.Prov.Path] = append(byPath[e.Prov.Path], e)
	}
	for _, path := range sortedKeys(byPath) {
		edges := byPath[path]
		sort.SliceStable(edges, func(i, j int) bool { return edges[i].Prov.Line < edges[j].Prov.Line })
		var ds []*decl
		index := map[string]*decl{}
		for _, e := range edges {
			if e.Prov.Scope == "file" {
				continue
			}
			if fl, ok := fileLevel[fact{path, e.P, e.O}]; ok && fl != e.Prov.Line {
				c.add(path, e.Prov.Line, "warning", "GM130", fmt.Sprintf("%s %s repeats the file-level marker at line %d; keep the file marker or the symbol markers, not both", e.P, c.g.Curie(e.O), fl))
			}
			d, ok := index[e.S]
			if !ok {
				d = &decl{symbol: e.Prov.Scope == "symbol", facts: map[string]int{}}
				if a := c.g.Node(e.S); a != nil && a.Anchor != nil {
					d.start = a.Anchor.StartLine
				}
				index[e.S] = d
				ds = append(ds, d)
			}
			k := e.P + "\x00" + e.O
			if l, ok := d.facts[k]; !ok || e.Prov.Line < l {
				d.facts[k] = e.Prov.Line
			}
		}
		// A run is one relation and target on consecutive declarations,
		// whatever else is stacked on them. Two in a row is often honest (two
		// handlers for one concept); three or more is the helper-by-helper
		// noise the first adoptions produced. Targets that repeat over the
		// same run are reported together.
		type run struct {
			rel        string
			start, end int
		}
		type finding struct {
			line, last int
			objs       []string
		}
		runs := map[run]*finding{}
		var order []run
		has := func(i int, k string) bool {
			if i < 0 || i >= len(ds) || !ds[i].symbol {
				return false
			}
			_, ok := ds[i].facts[k]
			return ok
		}
		for i, d := range ds {
			if !d.symbol {
				continue
			}
			for _, k := range sortedKeys(d.facts) {
				if has(i-1, k) {
					continue // not the start of its run
				}
				j := i + 1
				for has(j, k) {
					j++
				}
				if j-i < minRun {
					continue
				}
				rel, obj, _ := strings.Cut(k, "\x00")
				r := run{rel, i, j - 1}
				f, ok := runs[r]
				if !ok {
					f = &finding{line: d.facts[k], last: ds[j-1].facts[k]}
					runs[r] = f
					order = append(order, r)
				}
				f.line, f.last = min(f.line, d.facts[k]), max(f.last, ds[j-1].facts[k])
				f.objs = append(f.objs, c.g.Curie(obj))
			}
		}
		var lines []string
		for _, r := range order {
			f := runs[r]
			if lines == nil {
				b, _ := os.ReadFile(filepath.Join(c.ws.Root, filepath.FromSlash(path)))
				lines = strings.Split(string(b), "\n")
			}
			nested := true
			for _, d := range ds[r.start : r.end+1] {
				if d.start < 1 || d.start > len(lines) || strings.TrimLeft(lines[d.start-1], " \t") == lines[d.start-1] {
					nested = false
				}
			}
			c.add(path, f.line, "warning", "GM131", fmt.Sprintf("%s %s is repeated on %d consecutive declarations (lines %d-%d); %s", r.rel, strings.Join(f.objs, ", "), r.end-r.start+1, f.line, f.last, runRemedy(nested)))
		}
	}
}

// runRemedy is GM131's advice. scope=block anchors the next declaration and
// its body only, so it spans a run of methods when it sits above their
// enclosing class or block, never a run of top-level functions (#76).
func runRemedy(nested bool) string {
	if nested {
		return "one marker on the enclosing class or block (scope=block above it), or one on the public entry point, says the same with one edge"
	}
	return "one marker with scope=file at the top of the file, or one on the shared entry point, says the same with one edge (scope=block would cover only the next declaration); for a cross-cutting mechanism, declare guards: on its concept in the ontology instead (ADR 0015)"
}

// minRun is the shortest run of identical markers GM131 reports.
const minRun = 3

func (c *checker) edges() {
	for _, e := range c.g.Edges() {
		if e.Source == graph.SourceDerived {
			continue
		}
		path, line := "", 0
		if e.Prov != nil {
			path, line = e.Prov.Path, e.Prov.Line
		}
		sn := c.g.Node(e.S)
		rel := c.ws.Vocab.Get(e.P)
		if rel == nil {
			c.add(path, line, "error", "GM200", fmt.Sprintf("unknown relation %q; declare it under relations: in an ontology file or use one of: %s", e.P, strings.Join(c.ws.Vocab.Names(), ", ")))
			continue
		}
		// Where may the relation be written?
		switch e.Source {
		case graph.SourceInline:
			if !rel.Inline {
				c.add(path, line, "error", "GM201", fmt.Sprintf("relation %q cannot be used in an inline marker (ontology or manifest only)", e.P))
			}
			if e.Prov != nil && e.Prov.Alias == "tests" {
				c.add(path, line, "warning", "GM240", "prefer verifies: it is the one test relation; tests is an accepted alias that means the same")
			}
		case graph.SourceDeclared:
			if sn != nil && sn.NodeType == model.NodeConcept && !rel.Ontology {
				c.add(path, line, "error", "GM202", fmt.Sprintf("relation %q cannot be declared on a concept", e.P))
			}
			if sn != nil && sn.NodeType == model.NodeBoundary && !rel.Manifest && e.P != "part-of" {
				c.add(path, line, "error", "GM203", fmt.Sprintf("relation %q cannot be declared in a manifest", e.P))
			}
		}
		// Object resolution.
		switch {
		case strings.HasPrefix(e.O, "urn:gloss:unresolved:"):
			raw := strings.TrimPrefix(e.O, "urn:gloss:unresolved:")
			if p := c.prefixOf(raw); p != "" {
				c.add(path, line, "error", "GM210", fmt.Sprintf("unknown namespace prefix %q in %s; declare it under owns:, imports: or prefixes: in %s", p, raw, load.ConfigName))
			} else {
				c.add(path, line, "error", "GM211", fmt.Sprintf("unresolved target %q", raw))
			}
			continue
		case strings.HasPrefix(e.O, "urn:gloss:new:"):
			if e.Prov == nil || !e.Prov.Proposed {
				c.add(path, line, "error", "GM212", fmt.Sprintf("%s proposes a new concept and must be written as @gloss? (agents never invent concepts inline)", c.g.Curie(e.O)))
			}
			if !c.ws.Owned[c.prefixOf(strings.TrimPrefix(e.O, "urn:gloss:new:"))] {
				c.add(path, line, "error", "GM213", fmt.Sprintf("%s proposes a concept in a namespace this repo does not own; propose it upstream through a decision request", c.g.Curie(e.O)))
			}
			continue
		case strings.HasPrefix(e.O, "urn:gloss:glob:"):
			if e.P != "may-modify" {
				c.add(path, line, "error", "GM214", fmt.Sprintf("glob target %s is only allowed on may-modify", c.g.Curie(e.O)))
			}
			continue
		}
		on := c.g.Node(e.O)
		if on == nil {
			if sn != nil && sn.NodeType == model.NodeBoundary && e.P == "part-of" {
				continue
			}
			c.add(path, line, "error", "GM215", fmt.Sprintf("dangling reference: %s does not exist", c.g.Curie(e.O)))
			continue
		}
		if sn == nil {
			continue
		}
		// Domain / range by node type.
		if !rel.Allows(sn.NodeType, on.NodeType) {
			c.add(path, line, "error", "GM220", fmt.Sprintf("%s %s %s: relation %q expects %s → %s, got %s → %s", c.g.Curie(e.S), e.P, on.CURIE, e.P, types(rel.Domain), types(rel.Range), sn.NodeType, on.NodeType))
		}
		// Soft range kinds for inline links.
		if e.Source == graph.SourceInline && len(rel.RangeKinds) > 0 && on.NodeType == model.NodeConcept && on.Kind != "" && !contains(rel.RangeKinds, on.Kind) {
			c.add(path, line, "warning", "GM221", fmt.Sprintf("%s usually targets a %s; %s is a %s", e.P, strings.Join(rel.RangeKinds, "/"), on.CURIE, on.Kind))
		}
		// Deprecated targets.
		if on.Deprecated {
			msg := fmt.Sprintf("%s is deprecated", on.CURIE)
			if on.ReplacedBy != "" {
				msg += "; use " + on.ReplacedBy
			}
			c.add(path, line, "warning", "GM222", msg)
		}
		// The published seam: unpublished concepts of an imported implementation namespace are not referenceable.
		if on.NodeType == model.NodeConcept && on.Imported && on.Tier == model.TierImpl && on.Published == "" {
			c.add(path, line, "error", "GM230", fmt.Sprintf("%s is an unpublished concept of imported namespace %s: and cannot be referenced; only published concepts cross repos", on.CURIE, on.NS))
		}
		// Own-subject rule for declared concept edges.
		if e.Source == graph.SourceDeclared && sn.NodeType == model.NodeConcept {
			if !c.ws.Owned[sn.NS] && !sn.Imported {
				continue
			}
			if sn.Imported {
				// Edges inside an imported ontology are the upstream owner's business.
				continue
			}
			if e.P == "has-part" && on.NodeType == model.NodeConcept && !c.ws.Owned[on.NS] {
				// has-part is the inverse of part-of: on part-of sn, whose subject would be imported.
				c.add(path, line, "error", "GM400", fmt.Sprintf("%s has-part %s makes imported %s the subject of a part-of statement; extend, never mutate (own-subject rule). Write %s part-of %s instead", sn.CURIE, on.CURIE, on.CURIE, on.CURIE, sn.CURIE))
			}
		}
		// Tier direction on realizes / published-as.
		if e.Source == graph.SourceDeclared && (e.P == "realizes" || e.P == "published-as") && sn.NodeType == model.NodeConcept && on.NodeType == model.NodeConcept {
			if sn.Tier == model.TierCore {
				c.add(path, line, "error", "GM312", fmt.Sprintf("%s: core concepts do not realize anything; the dependency direction is implementation → core", sn.CURIE))
			}
		}
		// is-a across tiers.
		if e.P == "is-a" && sn.NodeType == model.NodeConcept && on.NodeType == model.NodeConcept && sn.Tier == model.TierCore && on.Tier == model.TierImpl {
			c.add(path, line, "error", "GM310", fmt.Sprintf("%s: core concept is-a implementation concept %s", sn.CURIE, on.CURIE))
		}
	}
	// Own-subject rule, stated directly: every declared edge's subject must be in an owned namespace.
	for _, p := range sortedKeys(c.ws.Ontologies) {
		o := c.ws.Ontologies[p]
		if !c.ws.Owned[p] {
			continue
		}
		for name, con := range o.Concepts {
			if strings.Contains(name, ":") && c.prefixOf(name) != p {
				c.add(relFile(c.ws, o.Path), con.Line, "error", "GM401", fmt.Sprintf("%s: concept defined in namespace %s: belongs to %s:, which this repo does not own (own-subject rule)", name, p, c.prefixOf(name)))
			}
		}
	}
}

// waivers: a waiver needs an ISO expires: date, or it would waive forever
// (GM610); one that names no finding waives nothing, usually a typo (GM611).
func (c *checker) waivers() {
	pol := c.ws.Config.EffectivePolicy()
	if len(pol.Waivers) == 0 {
		return
	}
	for _, w := range pol.Waivers {
		if _, err := time.Parse(rollup.WaiverDate, w.Expires); err != nil {
			c.add(load.ConfigName, 1, "error", "GM610", fmt.Sprintf("waiver for %s by %s: expires %q is not a YYYY-MM-DD date; a waiver must expire", w.Finding, w.Owner, w.Expires))
		}
	}
	rep := rollup.Compare(c.g, c.ws, rollup.Options{Policy: pol})
	for _, w := range rollup.UnmatchedWaivers(rep, pol) {
		c.add(load.ConfigName, 1, "warning", "GM611", fmt.Sprintf("waiver for %s by %s matches no finding (by id or concept); gloss drift --format json lists the ids", w.Finding, w.Owner))
	}
}

// governance: a boundary that declares a criticality below a concept its code
// realizes understates its risk; the concept's level wins in reviews.
func (c *checker) governance() {
	for _, u := range rollup.Understated(c.g) {
		c.add(u.File, 1, "warning", "GM600", fmt.Sprintf("%s declares criticality %s but realizes %s (%s); reviews use the higher level", u.Boundary, u.Declared, u.Concept, u.Level))
	}
}

func (c *checker) manifests() {
	for _, m := range c.ws.Manifests {
		b := c.g.NodeByCurie(m.Boundary)
		if b == nil {
			continue
		}
		if m.DefaultNamespace != "" {
			if _, ok := c.ws.Ontologies[m.DefaultNamespace]; !ok {
				c.add(m.Path, 1, "error", "GM500", fmt.Sprintf("default-namespace %q is not an owned or imported namespace", m.DefaultNamespace))
			}
		}
		for _, pat := range m.Allowed {
			if pat != model.DerivedMarker && !strings.Contains(pat, "*") && c.g.NodeByCurie(pat) == nil {
				c.add(m.Path, 1, "warning", "GM501", fmt.Sprintf("allowed lists %s, which does not exist", pat))
			}
		}
		// Inline links in this boundary must target the allowed set: the
		// manifest's own list, else the set derived from what it declares.
		allowed := rollup.AllowedFor(c.g, c.ws, m)
		if len(allowed.Patterns) == 0 {
			continue
		}
		for _, a := range c.g.NodesOf(model.NodeArtifact) {
			if a.Boundary != b.ID {
				continue
			}
			for _, e := range c.g.Out(a.ID, "") {
				if e.Source != graph.SourceInline || e.P == "may-modify" || e.P == "instructs" || e.P == "documents" {
					continue
				}
				if strings.HasPrefix(e.O, "urn:gloss:") {
					continue
				}
				on := c.g.Node(e.O)
				if on == nil || on.NodeType != model.NodeConcept {
					continue
				}
				if allowed.Matches(on.CURIE) {
					continue
				}
				if allowed.Derived {
					c.add(e.Prov.Path, e.Prov.Line, "warning", "GM503", fmt.Sprintf("%s %s is outside the allowed set derived from %s (HARD gate); declare the concept there, or write allowed: to widen it (gloss check --explain shows the set)", e.P, on.CURIE, m.Path))
					continue
				}
				sev := "warning"
				if e.Prov == nil || !e.Prov.Proposed {
					sev = "error"
				}
				c.add(e.Prov.Path, e.Prov.Line, sev, "GM502", fmt.Sprintf("%s %s is outside this boundary's allowed list in %s (HARD gate)", e.P, on.CURIE, m.Path))
			}
		}
	}
}

// ExplainAllowed renders the effective allowed set of every manifest, one
// line each, for gloss check --explain.
func ExplainAllowed(ws *load.Workspace, g *graph.Graph) string {
	var sb strings.Builder
	for _, m := range ws.Manifests {
		a := rollup.AllowedFor(g, ws, m)
		switch {
		case len(a.Patterns) == 0:
			fmt.Fprintf(&sb, "%s (%s): allowed = everything; the manifest declares nothing and has no default-namespace\n", m.Boundary, m.Path)
		case a.Derived:
			fmt.Fprintf(&sb, "%s (%s): allowed = %s  [derived: default-namespace, declared concepts, their descendants, realized cores, nested boundaries]\n", m.Boundary, m.Path, strings.Join(a.Patterns, ", "))
		default:
			fmt.Fprintf(&sb, "%s (%s): allowed = %s  [declared]\n", m.Boundary, m.Path, strings.Join(a.Patterns, ", "))
		}
	}
	return sb.String()
}

func relFile(ws *load.Workspace, path string) string {
	return ws.Rel(path)
}

func types(ts []model.NodeType) string {
	var s []string
	for _, t := range ts {
		s = append(s, string(t))
	}
	return strings.Join(s, "|")
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func isPascal(s string) bool {
	return s != "" && s[0] >= 'A' && s[0] <= 'Z' && !strings.ContainsAny(s, "_- ")
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ImplComplete reports whether an implementation concept stands for
// something, the GM320 rule that gloss accept --concept also applies.
// realizes is inherited through is-a and part-of, so only a concept whose
// is-a and part-of ancestors stand for nothing either is incomplete.
func ImplComplete(g *graph.Graph, n *graph.Node) bool {
	return n.Internal || n.Deprecated || standsFor(g, n.ID)
}
