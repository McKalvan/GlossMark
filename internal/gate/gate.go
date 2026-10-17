// Package gate classifies ontology changes into AUTO, SOFT and HARD, computes
// per-view impact between a base and a head graph, and renders the PR comment.
package gate

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/views"
)

// Class is a gate class.
type Class string

// Gate classes, in increasing strictness.
const (
	AUTO Class = "AUTO"
	SOFT Class = "SOFT"
	HARD Class = "HARD"
)

func rank(c Class) int {
	switch c {
	case AUTO:
		return 0
	case SOFT:
		return 1
	}
	return 2
}

// Max returns the stricter class.
func Max(a, b Class) Class {
	if rank(a) >= rank(b) {
		return a
	}
	return b
}

// Change is one classified difference.
type Change struct {
	Gate     Class    `json:"gate"`
	Kind     string   `json:"kind"`
	Glyph    string   `json:"glyph"`
	Relation string   `json:"relation,omitempty"`
	Subject  string   `json:"subject,omitempty"`
	Object   string   `json:"object,omitempty"`
	Path     string   `json:"path,omitempty"`
	Line     int      `json:"line,omitempty"`
	Symbol   string   `json:"symbol,omitempty"`
	Proposed bool     `json:"proposed,omitempty"`
	Conf     float64  `json:"conf,omitempty"`
	Actor    string   `json:"actor,omitempty"`
	Reasons  []string `json:"reasons,omitempty"`
	Msg      string   `json:"msg"`
	MarkerID string   `json:"marker_id,omitempty"`
}

// Impact is the result of gloss diff.
type Impact struct {
	Repo         string          `json:"repo"`
	Base         string          `json:"base"`
	Gate         Class           `json:"gate"`
	Changes      []Change        `json:"changes"`
	Views        []*views.Delta  `json:"views"`
	ViewsChanged []string        `json:"views_changed"`
	Rollup       []string        `json:"rollup"`
	Before       rollup.Metrics  `json:"before"`
	After        rollup.Metrics  `json:"after"`
	Orphans      []string        `json:"orphans"`
	Versions     []VersionChange `json:"versions,omitempty"`
	Boundaries   []string        `json:"boundaries"`
}

// VersionChange records a namespace version bump and the bump the diff implies.
type VersionChange struct {
	Namespace string `json:"namespace"`
	From      string `json:"from"`
	To        string `json:"to"`
	Required  string `json:"required"` // MAJOR | MINOR | PATCH | none
	Actual    string `json:"actual"`
	OK        bool   `json:"ok"`
}

// Options for Classify.
type Options struct {
	Base       string
	Policy     *model.Policy
	HeadLedger map[string]*model.LedgerEntry
	// ChangedFiles are repo-relative paths git reports as changed; used for the HARD path rule.
	ChangedFiles []string
	BeforeReport *rollup.Report
	AfterReport  *rollup.Report
}

// Classify diffs base against head.
func Classify(base, head *graph.Graph, ws *load.Workspace, opt Options) *Impact {
	if opt.Policy == nil {
		opt.Policy = model.DefaultPolicy()
	}
	im := &Impact{Repo: filepath.Base(ws.Root), Base: opt.Base, Gate: AUTO}
	c := &classifier{base: base, head: head, ws: ws, opt: opt, im: im}
	c.ontology()
	c.manifests()
	c.links()
	c.stale()
	c.versions()
	c.paths()
	c.viewsImpact()
	c.metrics()
	sort.SliceStable(im.Changes, func(i, j int) bool {
		a, b := im.Changes[i], im.Changes[j]
		if rank(a.Gate) != rank(b.Gate) {
			return rank(a.Gate) > rank(b.Gate)
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Line < b.Line
	})
	for _, ch := range im.Changes {
		im.Gate = Max(im.Gate, ch.Gate)
	}
	return im
}

type classifier struct {
	base, head *graph.Graph
	ws         *load.Workspace
	opt        Options
	im         *Impact
	bounds     map[string]bool
}

func (c *classifier) add(ch Change) {
	if ch.Glyph == "" {
		ch.Glyph = "+"
	}
	c.im.Changes = append(c.im.Changes, ch)
}

func inlineEdges(g *graph.Graph) map[string]*graph.Edge {
	out := map[string]*graph.Edge{}
	for _, e := range g.Edges() {
		if e.Source != graph.SourceInline || e.Prov == nil {
			continue
		}
		out[e.Prov.ID+"|"+e.P+"|"+e.O] = e
	}
	return out
}

func declaredEdges(g *graph.Graph, nt model.NodeType) map[string]*graph.Edge {
	out := map[string]*graph.Edge{}
	for _, e := range g.Edges() {
		if e.Source != graph.SourceDeclared {
			continue
		}
		if n := g.Node(e.S); n == nil || n.NodeType != nt {
			continue
		}
		out[e.S+"|"+e.P+"|"+e.O] = e
	}
	return out
}

// isPublished reports whether a concept IRI is published or is the core name of a published concept.
func isPublished(g *graph.Graph, iri string) bool {
	n := g.Node(iri)
	if n == nil {
		return false
	}
	if n.Published != "" {
		return true
	}
	return len(g.In(iri, "published-as")) > 0
}

func (c *classifier) ontology() {
	// Concept nodes.
	bn, hn := map[string]*graph.Node{}, map[string]*graph.Node{}
	for _, n := range c.base.NodesOf(model.NodeConcept) {
		bn[n.ID] = n
	}
	for _, n := range c.head.NodesOf(model.NodeConcept) {
		hn[n.ID] = n
	}
	for id, h := range hn {
		b, ok := bn[id]
		if !ok {
			c.add(Change{Gate: HARD, Kind: "concept-added", Glyph: "!", Subject: h.CURIE, Path: h.File, Line: h.Line, Reasons: []string{"edit to ontology"}, Msg: fmt.Sprintf("concept %s added (%s)", h.CURIE, h.Kind)})
			continue
		}
		var diffs []string
		if b.Kind != h.Kind {
			diffs = append(diffs, fmt.Sprintf("kind %s → %s", b.Kind, h.Kind))
		}
		if fmt.Sprint(b.Value) != fmt.Sprint(h.Value) {
			diffs = append(diffs, fmt.Sprintf("value %v → %v", orNone(b.Value), orNone(h.Value)))
		}
		if b.Published != h.Published {
			diffs = append(diffs, fmt.Sprintf("published %s → %s", orNone(b.Published), orNone(h.Published)))
		}
		if b.Internal != h.Internal {
			diffs = append(diffs, fmt.Sprintf("internal %v → %v", b.Internal, h.Internal))
		}
		if b.Deprecated != h.Deprecated {
			diffs = append(diffs, fmt.Sprintf("deprecated %v → %v", b.Deprecated, h.Deprecated))
		}
		if b.Label != h.Label || b.Doc != h.Doc {
			diffs = append(diffs, "label/doc")
		}
		if len(diffs) == 0 {
			continue
		}
		pub := isPublished(c.head, id) || isPublished(c.base, id)
		gate := HARD
		reasons := []string{"edit to ontology"}
		if len(diffs) == 1 && diffs[0] == "label/doc" {
			gate = SOFT
			reasons = []string{"labels and docs only (PATCH)"}
		}
		if pub {
			reasons = append(reasons, "business view affected: published concept")
			gate = HARD
		}
		kind := "concept-changed"
		if pub {
			kind = "published-changed"
		}
		c.add(Change{Gate: gate, Kind: kind, Glyph: "!", Subject: h.CURIE, Path: h.File, Line: h.Line, Reasons: reasons, Msg: fmt.Sprintf("%s%s %s", pubPrefix(pub), h.CURIE, strings.Join(diffs, ", "))})
	}
	for id, b := range bn {
		if _, ok := hn[id]; !ok {
			c.add(Change{Gate: HARD, Kind: "concept-removed", Glyph: "-", Subject: b.CURIE, Path: b.File, Line: b.Line, Reasons: []string{"edit to ontology (MAJOR)"}, Msg: fmt.Sprintf("concept %s removed; IDs are permanent, prefer deprecated: true + replaced-by:", b.CURIE)})
		}
	}
	// Declared concept edges.
	be, he := declaredEdges(c.base, model.NodeConcept), declaredEdges(c.head, model.NodeConcept)
	for k, e := range he {
		if _, ok := be[k]; ok {
			continue
		}
		if _, existed := bn[e.S]; !existed {
			continue // reported with the concept
		}
		pub := isPublished(c.head, e.S) || isPublished(c.head, e.O)
		c.add(Change{Gate: HARD, Kind: "ontology-edge-added", Glyph: "!", Relation: e.P, Subject: c.head.Curie(e.S), Object: c.head.Curie(e.O), Path: provPath(e), Line: provLine(e),
			Reasons: reasons("edit to ontology", pub), Msg: fmt.Sprintf("%s%s %s %s added", pubPrefix(pub), c.head.Curie(e.S), e.P, c.head.Curie(e.O))})
	}
	for k, e := range be {
		if _, ok := he[k]; ok {
			continue
		}
		if _, exists := hn[e.S]; !exists {
			continue
		}
		pub := isPublished(c.base, e.S) || isPublished(c.base, e.O)
		c.add(Change{Gate: HARD, Kind: "ontology-edge-removed", Glyph: "-", Relation: e.P, Subject: c.base.Curie(e.S), Object: c.base.Curie(e.O), Path: provPath(e), Line: provLine(e),
			Reasons: reasons("edit to ontology", pub), Msg: fmt.Sprintf("%s%s %s %s removed", pubPrefix(pub), c.base.Curie(e.S), e.P, c.base.Curie(e.O))})
	}
	// Imports.
	for _, n := range c.head.NodesOf(model.NodeNamespace) {
		if b := c.base.Node(n.ID); b == nil {
			kind := "namespace-added"
			if n.Imported {
				kind = "import-added"
			}
			c.add(Change{Gate: HARD, Kind: kind, Glyph: "!", Subject: n.CURIE, Path: load.ConfigName, Reasons: []string{"cross-repo import"}, Msg: fmt.Sprintf("namespace %s %s", n.CURIE, strings.TrimPrefix(kind, "namespace-"))})
		}
	}
	for _, n := range c.base.NodesOf(model.NodeNamespace) {
		if h := c.head.Node(n.ID); h == nil {
			c.add(Change{Gate: HARD, Kind: "namespace-removed", Glyph: "-", Subject: n.CURIE, Path: load.ConfigName, Reasons: []string{"cross-repo import"}, Msg: fmt.Sprintf("namespace %s removed", n.CURIE)})
		}
	}
}

func pubPrefix(pub bool) string {
	if pub {
		return "published "
	}
	return ""
}

func reasons(base string, pub bool) []string {
	r := []string{base}
	if pub {
		r = append(r, "business view affected: published concept")
	}
	return r
}

func orNone(v any) string {
	s := fmt.Sprint(v)
	if s == "" || s == "<nil>" {
		return "none"
	}
	return s
}

func provPath(e *graph.Edge) string {
	if e.Prov != nil {
		return e.Prov.Path
	}
	return ""
}

func provLine(e *graph.Edge) int {
	if e.Prov != nil {
		return e.Prov.Line
	}
	return 0
}

func (c *classifier) manifests() {
	c.bounds = map[string]bool{}
	bn, hn := map[string]*graph.Node{}, map[string]*graph.Node{}
	for _, n := range c.base.NodesOf(model.NodeBoundary) {
		bn[n.ID] = n
	}
	for _, n := range c.head.NodesOf(model.NodeBoundary) {
		hn[n.ID] = n
	}
	for id, h := range hn {
		if _, ok := bn[id]; !ok {
			c.add(Change{Gate: HARD, Kind: "boundary-added", Glyph: "!", Subject: h.CURIE, Path: h.File, Reasons: []string{"edit to manifest"}, Msg: fmt.Sprintf("boundary %s added (%s)", h.CURIE, h.Kind)})
			c.bounds[h.CURIE] = true
		} else if bn[id].Kind != h.Kind {
			c.add(Change{Gate: HARD, Kind: "boundary-changed", Glyph: "!", Subject: h.CURIE, Path: h.File, Reasons: []string{"edit to manifest"}, Msg: fmt.Sprintf("boundary %s level %s → %s", h.CURIE, bn[id].Kind, h.Kind)})
		}
	}
	for id, b := range bn {
		if _, ok := hn[id]; !ok {
			c.add(Change{Gate: HARD, Kind: "boundary-removed", Glyph: "-", Subject: b.CURIE, Path: b.File, Reasons: []string{"edit to manifest"}, Msg: fmt.Sprintf("boundary %s removed", b.CURIE)})
		}
	}
	be, he := declaredEdges(c.base, model.NodeBoundary), declaredEdges(c.head, model.NodeBoundary)
	for k, e := range he {
		if _, ok := be[k]; !ok {
			if _, existed := bn[e.S]; !existed {
				continue
			}
			c.add(Change{Gate: HARD, Kind: "manifest-changed", Glyph: "!", Relation: e.P, Subject: c.head.Curie(e.S), Object: c.head.Curie(e.O), Path: provPath(e), Reasons: []string{"edit to manifest"}, Msg: fmt.Sprintf("%s declares %s %s", c.head.Curie(e.S), e.P, c.head.Curie(e.O))})
			c.bounds[c.head.Curie(e.S)] = true
		}
	}
	for k, e := range be {
		if _, ok := he[k]; !ok {
			if _, exists := hn[e.S]; !exists {
				continue
			}
			c.add(Change{Gate: HARD, Kind: "manifest-changed", Glyph: "-", Relation: e.P, Subject: c.base.Curie(e.S), Object: c.base.Curie(e.O), Path: provPath(e), Reasons: []string{"edit to manifest"}, Msg: fmt.Sprintf("%s no longer declares %s %s", c.base.Curie(e.S), e.P, c.base.Curie(e.O))})
			c.bounds[c.base.Curie(e.S)] = true
		}
	}
}

// ClassifyLink gates one inline link in the head graph against policy. It is
// shared with gloss accept --policy.
func ClassifyLink(g *graph.Graph, ws *load.Workspace, e *graph.Edge, pol *model.Policy) (Class, []string) {
	var reasons []string
	gate := SOFT
	p := e.Prov
	switch {
	case strings.HasPrefix(e.O, "urn:gloss:new:"):
		return HARD, []string{"new concept proposed; decision request"}
	case strings.HasPrefix(e.O, "urn:gloss:unresolved:"):
		return HARD, []string{"unresolved reference"}
	case strings.HasPrefix(e.O, "urn:gloss:glob:"):
		if e.P == "may-modify" {
			return HARD, []string{"agent remit changed"}
		}
	}
	on := g.Node(e.O)
	if on == nil {
		return HARD, []string{"target does not exist"}
	}
	rel := ws.Vocab.Get(e.P)
	sn := g.Node(e.S)
	if rel == nil || sn == nil || !rel.Allows(sn.NodeType, on.NodeType) {
		return HARD, []string{"invalid for (artifact kind, concept kind)"}
	}
	if e.P == "may-modify" || e.P == "instructs" {
		return HARD, []string{"agent remit or instructions changed"}
	}
	// Allowed list of the nearest manifest.
	if m := ws.ManifestFor(p.Path); m != nil && len(m.Allowed) > 0 && on.NodeType == model.NodeConcept && !model.MatchAnyCurie(m.Allowed, on.CURIE) {
		gate = HARD
		reasons = append(reasons, "outside allowed")
	}
	// Agent remit (may-modify) when the actor is a named agent with an agent file.
	if p.Actor != "" && strings.HasPrefix(p.Actor, "agent:") {
		if remit, ok := agentRemit(g, p.Actor); ok {
			if on.NodeType == model.NodeConcept && !model.MatchAnyCurie(remit, on.CURIE) {
				gate = HARD
				reasons = append(reasons, fmt.Sprintf("outside %s may-modify remit", agentName(p.Actor)))
			}
		}
	}
	if on.NodeType == model.NodeConcept && isPublished(g, e.O) {
		gate = HARD
		reasons = append(reasons, "business view affected: published concept")
	}
	if on.Deprecated {
		gate = HARD
		reasons = append(reasons, "target deprecated")
	}
	if p.Proposed {
		conf := -1.0
		if p.Conf != nil {
			conf = *p.Conf
		}
		if conf >= 0 && conf < pol.SoftConfThreshold {
			gate = HARD
			reasons = append(reasons, fmt.Sprintf("conf %.2f below threshold %.2f", conf, pol.SoftConfThreshold))
		} else if conf < 0 && strings.HasPrefix(p.Actor, "agent:") {
			gate = HARD
			reasons = append(reasons, "agent proposal without conf")
		}
	}
	if gate == SOFT {
		reasons = append(reasons, "existing concept, valid, inside allowed")
	}
	return gate, reasons
}

func agentName(actor string) string {
	n := strings.TrimPrefix(actor, "agent:")
	if i := strings.Index(n, "@"); i >= 0 {
		n = n[:i]
	}
	return n
}

// agentRemit finds the may-modify patterns of the agent file named like the actor.
func agentRemit(g *graph.Graph, actor string) ([]string, bool) {
	name := agentName(actor)
	var remit []string
	found := false
	for _, a := range g.NodesOf(model.NodeArtifact) {
		base := strings.TrimPrefix(strings.TrimSuffix(filepath.Base(a.Path), filepath.Ext(a.Path)), ".")
		if base != name {
			continue
		}
		for _, e := range g.Out(a.ID, "may-modify") {
			found = true
			remit = append(remit, g.Curie(e.O))
		}
	}
	return remit, found
}

func (c *classifier) links() {
	be, he := inlineEdges(c.base), inlineEdges(c.head)
	for k, e := range he {
		b, ok := be[k]
		if ok {
			// Same marker, same target: fingerprint-only or ?-flip.
			if b.Prov.Proposed && !e.Prov.Proposed {
				c.add(Change{Gate: AUTO, Kind: "link-accepted", Glyph: "+", Relation: e.P, Subject: c.head.Curie(e.S), Object: c.head.Curie(e.O), Path: e.Prov.Path, Line: e.Prov.Line, Symbol: e.Prov.Symbol, Actor: e.Prov.Actor, MarkerID: e.Prov.ID,
					Reasons: []string{"proposal accepted (@gloss? → @gloss)"}, Msg: fmt.Sprintf("%s %s accepted", e.P, c.head.Curie(e.O))})
			} else if b.Prov.Fingerprint != e.Prov.Fingerprint {
				c.add(Change{Gate: AUTO, Kind: "fingerprint-changed", Glyph: "~", Relation: e.P, Subject: c.head.Curie(e.S), Object: c.head.Curie(e.O), Path: e.Prov.Path, Line: e.Prov.Line, Symbol: e.Prov.Symbol, MarkerID: e.Prov.ID,
					Reasons: []string{"fingerprint-only change on an unchanged concept"}, Msg: fmt.Sprintf("%s %s anchor signature changed", e.P, c.head.Curie(e.O))})
			}
			continue
		}
		gate, why := ClassifyLink(c.head, c.ws, e, c.opt.Policy)
		glyph := "+"
		if e.Prov.Proposed {
			glyph = "?"
		}
		conf := 0.0
		if e.Prov.Conf != nil {
			conf = *e.Prov.Conf
		}
		kind := "link-added"
		if strings.HasPrefix(e.O, "urn:gloss:new:") {
			kind = "new-concept"
		}
		c.add(Change{Gate: gate, Kind: kind, Glyph: glyph, Relation: e.P, Subject: c.head.Curie(e.S), Object: c.head.Curie(e.O), Path: e.Prov.Path, Line: e.Prov.Line, Symbol: e.Prov.Symbol,
			Proposed: e.Prov.Proposed, Conf: conf, Actor: e.Prov.Actor, Reasons: why, MarkerID: e.Prov.ID, Msg: fmt.Sprintf("%s %s", e.P, c.head.Curie(e.O))})
		if m := c.ws.ManifestFor(e.Prov.Path); m != nil {
			c.bounds[m.Boundary] = true
		}
	}
	for k, e := range be {
		if _, ok := he[k]; ok {
			continue
		}
		pub := isPublished(c.base, e.O)
		gate := SOFT
		why := []string{"link removed; engineering view only"}
		if pub {
			gate = HARD
			why = []string{"business view affected: published concept"}
		}
		c.add(Change{Gate: gate, Kind: "link-removed", Glyph: "-", Relation: e.P, Subject: c.base.Curie(e.S), Object: c.base.Curie(e.O), Path: e.Prov.Path, Line: e.Prov.Line, Symbol: e.Prov.Symbol, MarkerID: e.Prov.ID, Reasons: why,
			Msg: fmt.Sprintf("%s %s removed", e.P, c.base.Curie(e.O))})
	}
}

func (c *classifier) stale() {
	for id, le := range c.opt.HeadLedger {
		if le.Status != model.StatusStale {
			continue
		}
		c.add(Change{Gate: AUTO, Kind: "stale", Glyph: "~", Relation: le.Relation, Object: strings.Join(le.Targets, ", "), Path: le.Path, MarkerID: id,
			Reasons: []string{"signature changed; re-confirm"}, Msg: fmt.Sprintf("stale %s %s", le.Relation, strings.Join(le.Targets, ", "))})
	}
}

// versions classifies the semver bump each owned namespace needs.
func (c *classifier) versions() {
	for _, n := range c.head.NodesOf(model.NodeNamespace) {
		if n.Imported {
			continue
		}
		b := c.base.Node(n.ID)
		if b == nil {
			continue
		}
		required := "none"
		for _, ch := range c.im.Changes {
			if !strings.HasPrefix(ch.Subject, n.NS+":") {
				continue
			}
			switch ch.Kind {
			case "concept-removed", "ontology-edge-removed":
				required = "MAJOR"
			case "concept-changed", "published-changed":
				if strings.Contains(ch.Msg, "kind ") && required != "MAJOR" {
					required = "MAJOR"
				} else if required == "none" {
					required = "PATCH"
				}
			case "ontology-edge-added":
				if (ch.Relation == "is-a" || ch.Relation == "part-of") && required != "MAJOR" {
					required = "MAJOR"
				} else if required == "none" || required == "PATCH" {
					required = "MINOR"
				}
			case "concept-added":
				if required == "none" || required == "PATCH" {
					required = "MINOR"
				}
			}
		}
		actual := bumpOf(b.Version, n.Version)
		if required == "none" && actual == "none" {
			continue
		}
		ok := rankBump(actual) >= rankBump(required)
		c.im.Versions = append(c.im.Versions, VersionChange{Namespace: n.NS, From: b.Version, To: n.Version, Required: required, Actual: actual, OK: ok})
		if !ok {
			c.add(Change{Gate: SOFT, Kind: "version-bump", Glyph: "!", Subject: n.NS + ":", Path: n.File, Reasons: []string{"semver: bump the namespace version"},
				Msg: fmt.Sprintf("namespace %s: changes require a %s bump but version went %s → %s (%s)", n.NS, required, orNone(b.Version), orNone(n.Version), actual)})
		}
		if required == "MAJOR" {
			c.add(Change{Gate: HARD, Kind: "major-bump", Glyph: "!", Subject: n.NS + ":", Path: n.File, Reasons: []string{"MAJOR bump"}, Msg: fmt.Sprintf("namespace %s: MAJOR change (removal, retype, or is-a/part-of change)", n.NS)})
		}
	}
}

func rankBump(b string) int {
	switch b {
	case "MAJOR":
		return 3
	case "MINOR":
		return 2
	case "PATCH":
		return 1
	}
	return 0
}

func bumpOf(from, to string) string {
	if from == to {
		return "none"
	}
	f, t := semver(from), semver(to)
	if f == nil || t == nil {
		return "unknown"
	}
	switch {
	case t[0] > f[0]:
		return "MAJOR"
	case t[1] > f[1]:
		return "MINOR"
	case t[2] > f[2]:
		return "PATCH"
	}
	return "downgrade"
}

func semver(s string) []int {
	s = strings.TrimPrefix(s, "v")
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return nil
	}
	out := make([]int, 3)
	for i, p := range parts {
		var n int
		if _, err := fmt.Sscanf(p, "%d", &n); err != nil {
			return nil
		}
		out[i] = n
	}
	return out
}

// paths applies the HARD path rule for changed files the graph did not already explain.
func (c *classifier) paths() {
	seen := map[string]bool{}
	for _, ch := range c.im.Changes {
		seen[ch.Path] = true
	}
	for _, f := range c.opt.ChangedFiles {
		if seen[f] || !model.MatchAnyGlob(c.opt.Policy.HardPaths, f) {
			continue
		}
		c.add(Change{Gate: HARD, Kind: "hard-path", Glyph: "!", Path: f, Reasons: []string{"edit to ontology/ or manifests"}, Msg: fmt.Sprintf("%s changed", f)})
	}
}

func (c *classifier) viewsImpact() {
	bp := views.ProjectAll(c.base, c.ws.Views)
	hp := views.ProjectAll(c.head, c.ws.Views)
	bm := map[string]*views.Projection{}
	for _, p := range bp {
		bm[p.Name] = p
	}
	for _, h := range hp {
		b, ok := bm[h.Name]
		if !ok {
			b = &views.Projection{Name: h.Name}
		}
		d := views.Diff(b, h)
		c.im.Views = append(c.im.Views, d)
		if d.Changed() {
			c.im.ViewsChanged = append(c.im.ViewsChanged, h.Name)
		}
	}
	// A change that alters a core-only view is HARD; one that alters only engineering views is SOFT at most.
	for _, name := range c.im.ViewsChanged {
		v := c.ws.Views.Views[name]
		if v != nil && coreOnly(c.ws, v) {
			for i := range c.im.Changes {
				ch := &c.im.Changes[i]
				if ch.Gate == SOFT && (ch.Kind == "link-added" || ch.Kind == "link-removed") && c.touchesView(ch, c.im.Views, name) {
					ch.Gate = HARD
					ch.Reasons = append(ch.Reasons, "business view affected: "+name)
				}
			}
		}
	}
	// Collect boundaries touched.
	for b := range c.bounds {
		c.im.Boundaries = append(c.im.Boundaries, b)
	}
	sort.Strings(c.im.Boundaries)
}

func (c *classifier) touchesView(ch *Change, deltas []*views.Delta, name string) bool {
	for _, d := range deltas {
		if d.View != name {
			continue
		}
		for _, e := range append(d.AddedEdges, d.RemovedEdges...) {
			if e.O == ch.Object || e.S == ch.Object {
				return true
			}
			// Lifted edges carry the raw objects in via.
			for _, v := range e.Via {
				if v == ch.Object {
					return true
				}
			}
		}
		for _, n := range append(d.AddedNodes, d.RemovedNodes...) {
			if n == ch.Object {
				return true
			}
		}
	}
	return false
}

// coreOnly reports whether a view includes no implementation namespace directly.
func coreOnly(ws *load.Workspace, v *model.View) bool {
	if len(v.Include) == 0 {
		return false
	}
	for _, pat := range v.Include {
		ns := strings.SplitN(pat, ":", 2)[0]
		if o, ok := ws.Ontologies[ns]; ok && o.Tier == model.TierImpl {
			return false
		}
		if ns == "*" {
			return false
		}
	}
	return true
}

func (c *classifier) metrics() {
	if c.opt.BeforeReport != nil {
		c.im.Before = c.opt.BeforeReport.Metrics
	}
	if c.opt.AfterReport != nil {
		c.im.After = c.opt.AfterReport.Metrics
		type key struct{ b, r string }
		viol := map[key][]string{}
		decl := map[key][]string{}
		var order []key
		for _, f := range c.opt.AfterReport.Findings {
			if f.Kind != rollup.KindViolation || !(len(c.bounds) == 0 || c.bounds[f.Boundary]) {
				continue
			}
			k := key{f.Boundary, f.Relation}
			if _, ok := viol[k]; !ok {
				order = append(order, k)
				decl[k] = f.Declared
			}
			cpt := f.Concept
			if f.Proposed {
				cpt += "?"
			}
			viol[k] = append(viol[k], cpt)
		}
		for _, k := range order {
			c.im.Rollup = append(c.im.Rollup, fmt.Sprintf("%s %s {%s} ⊄ declared {%s}  ✗", k.b, k.r, strings.Join(viol[k], ", "), strings.Join(decl[k], ", ")))
		}
		for _, f := range c.opt.AfterReport.Findings {
			switch f.Kind {
			case rollup.KindConsistent:
				if c.bounds[f.Boundary] {
					c.im.Rollup = append(c.im.Rollup, fmt.Sprintf("%s %s {%s} ⊑ declared  ✓", f.Boundary, f.Relation, strings.Join(f.Derived, ", ")))
				}
			case rollup.KindOrphan:
				c.im.Orphans = append(c.im.Orphans, f.Concept)
			}
		}
		c.im.Rollup = uniq(c.im.Rollup)
	}
}

func uniq(xs []string) []string {
	m := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !m[x] {
			m[x] = true
			out = append(out, x)
		}
	}
	return out
}

// Diagnostics converts HARD changes into diagnostics for CI output.
func (im *Impact) Diagnostics() []marker.Diagnostic {
	var out []marker.Diagnostic
	for _, ch := range im.Changes {
		if ch.Gate != HARD {
			continue
		}
		out = append(out, marker.Diagnostic{Path: ch.Path, Line: ch.Line, Severity: "error", Code: "GM900", Msg: ch.Msg + " [HARD: " + strings.Join(ch.Reasons, "; ") + "]"})
	}
	return out
}
