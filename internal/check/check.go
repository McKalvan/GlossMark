// @gloss implements gmi:Linter scope=file

// Package check lints the graph: syntax, dangling references, domain/range,
// the own-subject rule, tier rules, the published seam and manifest allowed lists.
package check

import (
	"fmt"
	"sort"
	"strings"

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
	c.edges()
	c.markers()
	c.manifests()
	c.governance()
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
	ws  *load.Workspace
	g   *graph.Graph
	out []marker.Diagnostic
}

func (c *checker) add(path string, line int, sev, code, msg string) {
	c.out = append(c.out, marker.Diagnostic{Path: path, Line: line, Severity: sev, Code: code, Msg: msg})
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
			if len(c.g.Out(n.ID, "is-a")) > 0 {
				// kind is inherited from the parent; fine.
			} else {
				c.add(n.File, n.Line, "error", "GM300", fmt.Sprintf("%s: kind is required (one of %s) unless the concept has is-a", n.CURIE, strings.Join(model.ConceptKinds, ", ")))
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

// markers lints redundancy between inline markers. A symbol-level marker
// that repeats a file-level marker's relation and target states the same
// fact twice and makes two edges (GM130); the same marker on consecutive
// declarations, three or more in a row, usually wants one scope=block
// marker above the run (GM131).
// Both passed silently and inflated file coverage in the first adoptions.
func (c *checker) markers() {
	type fact struct{ path, rel, obj string }
	type mk struct {
		id, scope, rel string
		line           int
		objs           []string
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
		var ms []*mk
		index := map[string]*mk{}
		for _, e := range edges {
			if e.Prov.Scope != "file" {
				if fl, ok := fileLevel[fact{path, e.P, e.O}]; ok && fl != e.Prov.Line {
					c.add(path, e.Prov.Line, "warning", "GM130", fmt.Sprintf("%s %s repeats the file-level marker at line %d; keep the file marker or the symbol markers, not both", e.P, c.g.Curie(e.O), fl))
				}
			}
			m, ok := index[e.Prov.ID]
			if !ok {
				m = &mk{id: e.Prov.ID, scope: e.Prov.Scope, rel: e.P, line: e.Prov.Line}
				index[e.Prov.ID] = m
				ms = append(ms, m)
			}
			m.objs = append(m.objs, e.O)
		}
		// A run is the same symbol-level marker on consecutive declarations.
		// Two in a row is often honest (two handlers for one concept); three
		// or more is the helper-by-helper noise the first adoptions produced.
		for i := 0; i < len(ms); {
			j := i + 1
			for j < len(ms) && ms[i].scope == "symbol" && ms[j].scope == "symbol" && ms[j].rel == ms[i].rel && sameSet(ms[j].objs, ms[i].objs) {
				j++
			}
			if n := j - i; n >= minRun {
				a := ms[i]
				c.add(path, a.line, "warning", "GM131", fmt.Sprintf("%s %s is repeated on %d consecutive declarations (lines %d-%d); one marker with scope=block above the run, or one on the public entry point, says the same with one edge", a.rel, c.g.Curie(a.objs[0]), n, a.line, ms[j-1].line))
			}
			i = j
		}
	}
}

// minRun is the shortest run of identical markers GM131 reports.
const minRun = 3

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := append([]string{}, a...), append([]string{}, b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

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
			if e.P == "tests" {
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
	return strings.TrimPrefix(strings.TrimPrefix(path, ws.Root), "/")
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
// realizes is inherited through is-a and part-of (RealizesStar), so only a
// concept that stands for nothing at all is incomplete.
func ImplComplete(g *graph.Graph, n *graph.Node) bool {
	if n.Internal || n.Deprecated {
		return true
	}
	if len(g.Objects(n.ID, "realizes")) > 0 || len(g.Out(n.ID, "presents"))+len(g.Out(n.ID, "hosts")) > 0 || len(g.Out(n.ID, "is-a")) > 0 {
		return true
	}
	return len(rollup.RealizesStar(g, n.ID)) > 0
}
