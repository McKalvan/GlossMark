// @gloss implements gmi:ComplianceReporter scope=file
// @gloss consumes gm:Graph, gm:Ledger, gm:Drift scope=file

// Package report assembles audit reports from the graph, the ledger and the
// drift report: for each rule, the code that enforces it, the tests that
// verify it, its published value, who decided each link and when, and what
// is still open. It reads what the other packages built and parses nothing.
package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// Options selects the concepts and scopes the history.
type Options struct {
	// Concepts are the CURIEs to report on. Empty selects by Criticality,
	// or, when that is empty too, every constraint and every critical concept.
	Concepts []string
	// Criticality is the lowest level to include (high: high and critical).
	Criticality string
	// Since keeps only ledger events on or after this date; zero keeps all.
	Since time.Time
	// SourceURL is the link template for paths ({path}, {line}), or "".
	SourceURL string
	Now       time.Time
	Policy    *model.Policy
	Ledger    map[string]*model.LedgerEntry
	Drift     *rollup.Report
}

// Report is one compliance report.
type Report struct {
	Generated string     `json:"generated"`
	Generator string     `json:"generator,omitempty"`
	Selection string     `json:"selection"`
	Since     string     `json:"since,omitempty"`
	Concepts  []*Concept `json:"concepts"`
}

// Concept is one rule (or critical concept) with its evidence.
type Concept struct {
	CURIE           string     `json:"concept"`
	Label           string     `json:"label,omitempty"`
	Kind            string     `json:"kind,omitempty"`
	Tier            string     `json:"tier"`
	Doc             string     `json:"doc,omitempty"`
	Criticality     string     `json:"criticality,omitempty"`
	CriticalityFrom string     `json:"criticality_from,omitempty"`
	Status          string     `json:"status"`
	NamespaceOwner  string     `json:"namespace_owner,omitempty"`
	Boundaries      []Boundary `json:"boundaries,omitempty"`
	Realizers       []Realizer `json:"realizers,omitempty"`
	Artifacts       []Link     `json:"artifacts,omitempty"`
	Tests           []Link     `json:"tests,omitempty"`
	Values          []Value    `json:"values,omitempty"`
	History         []Event    `json:"history,omitempty"`
	Findings        []Finding  `json:"findings,omitempty"`
	Waivers         []Waiver   `json:"waivers,omitempty"`
}

// Boundary is a boundary whose code realizes the concept, with its owner.
type Boundary struct {
	CURIE    string `json:"boundary"`
	Label    string `json:"label,omitempty"`
	Owner    string `json:"owner,omitempty"`
	Manifest string `json:"manifest,omitempty"`
}

// Realizer is an implementation concept that stands for the rule in code.
type Realizer struct {
	CURIE string `json:"concept"`
	Doc   string `json:"doc,omitempty"`
}

// Link is one artifact linked to the concept, with its review state.
type Link struct {
	Relation string  `json:"relation"`
	Path     string  `json:"path"`
	Line     int     `json:"line,omitempty"`
	Symbol   string  `json:"symbol,omitempty"`
	URL      string  `json:"url,omitempty"`
	Via      string  `json:"via,omitempty"` // the realizer linked, when not the concept itself
	MarkerID string  `json:"marker_id,omitempty"`
	Status   string  `json:"status,omitempty"`
	Actor    string  `json:"actor,omitempty"`
	Conf     float64 `json:"conf,omitempty"`
	Proposed bool    `json:"proposed,omitempty"`
}

// Value is a published value and the code it is bound to.
type Value struct {
	Concept string `json:"concept"`
	Value   string `json:"value"`
	BoundTo string `json:"bound_to,omitempty"` // path:line
	URL     string `json:"url,omitempty"`
	Live    string `json:"live,omitempty"` // the bound line as it reads now
	Status  string `json:"status"`         // accepted | drifted | unbound
}

// Event is one entry of a link's history, from the ledger.
type Event struct {
	When     string  `json:"when"`
	Kind     string  `json:"kind"` // linked | proposed | accepted | rejected | stale | removed | value-bound | value-drifted
	Actor    string  `json:"actor,omitempty"`
	MarkerID string  `json:"marker_id"`
	Relation string  `json:"relation"`
	Target   string  `json:"target"`
	Path     string  `json:"path"`
	URL      string  `json:"url,omitempty"`
	Gate     string  `json:"gate,omitempty"`
	Conf     float64 `json:"conf,omitempty"`
	Reason   string  `json:"reason,omitempty"` // the reviewer's reason, or the author's note
}

// Finding is an open drift finding on the concept.
type Finding struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	URL      string `json:"url,omitempty"`
	Msg      string `json:"msg"`
	Waived   bool   `json:"waived,omitempty"`
	Waiver   string `json:"waiver,omitempty"`
}

// Waiver is a policy waiver that names the concept or one of its findings.
type Waiver struct {
	Finding string `json:"finding"`
	Owner   string `json:"owner"`
	Expires string `json:"expires"`
	Reason  string `json:"reason,omitempty"`
	Expired bool   `json:"expired,omitempty"`
}

// Compliance builds the report. Unknown CURIEs in opt.Concepts are an error.
func Compliance(g *graph.Graph, ws *load.Workspace, opt Options) (*Report, error) {
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	if opt.Drift == nil {
		opt.Drift = rollup.Compare(g, ws, rollup.Options{Ledger: opt.Ledger, Policy: opt.Policy, Now: opt.Now})
	}
	b := &builder{g: g, ws: ws, opt: opt}
	b.index()
	nodes, selection, err := b.selectConcepts()
	if err != nil {
		return nil, err
	}
	r := &Report{Generated: opt.Now.UTC().Format(time.RFC3339), Generator: g.Generator, Selection: selection}
	if !opt.Since.IsZero() {
		r.Since = opt.Since.Format(rollup.WaiverDate)
	}
	for _, n := range nodes {
		r.Concepts = append(r.Concepts, b.concept(n))
	}
	return r, nil
}

type builder struct {
	g   *graph.Graph
	ws  *load.Workspace
	opt Options
	// realizers: concept IRI → the concepts that stand for it in code.
	realizers map[string][]string
	// byMarker: marker id → the inline edges it produced.
	byMarker map[string][]*graph.Edge
	status   map[string]string
}

func (b *builder) index() {
	b.realizers = map[string][]string{}
	for _, n := range b.g.NodesOf(model.NodeConcept) {
		for _, c := range rollup.RealizesStar(b.g, n.ID) {
			b.realizers[c] = append(b.realizers[c], n.ID)
		}
	}
	b.byMarker = map[string][]*graph.Edge{}
	for _, e := range b.g.Edges() {
		if e.Prov != nil && e.Prov.ID != "" {
			b.byMarker[e.Prov.ID] = append(b.byMarker[e.Prov.ID], e)
		}
	}
	b.status = map[string]string{}
	for _, row := range rollup.ConceptStatus(b.g, b.opt.Drift) {
		b.status[row.Concept] = row.Status
	}
}

// selectConcepts applies Options: named CURIEs, a criticality floor, or the
// default (constraints and critical concepts), owned concepts only.
func (b *builder) selectConcepts() ([]*graph.Node, string, error) {
	var out []*graph.Node
	switch {
	case len(b.opt.Concepts) > 0:
		seen := map[string]bool{}
		for _, cur := range b.opt.Concepts {
			n := b.g.NodeByCurie(cur)
			if n == nil || n.NodeType != model.NodeConcept {
				names := make([]string, 0, 64)
				for _, c := range b.g.NodesOf(model.NodeConcept) {
					names = append(names, c.CURIE)
				}
				sort.Strings(names)
				if near := check.Closest(cur, names); near != "" {
					return nil, "", fmt.Errorf("unknown concept %q; did you mean %q?", cur, near)
				}
				return nil, "", fmt.Errorf("unknown concept %q; gloss export --status lists the concepts", cur)
			}
			if !seen[n.ID] {
				seen[n.ID] = true
				out = append(out, n)
			}
		}
		return out, "concepts " + strings.Join(b.opt.Concepts, ", "), nil
	case b.opt.Criticality != "":
		floor := model.CriticalityRank(b.opt.Criticality)
		for _, n := range b.g.NodesOf(model.NodeConcept) {
			if n.Imported || n.Proposed || model.CriticalityRank(n.Criticality) < floor {
				continue
			}
			out = append(out, n)
		}
		sortNodes(out)
		return out, "concepts at criticality " + b.opt.Criticality + " or above", nil
	default:
		for _, n := range b.g.NodesOf(model.NodeConcept) {
			if n.Imported || n.Proposed {
				continue
			}
			// An implementation constraint that stands for a core one is
			// reported under it, as a realizer.
			standsForCore := false
			for _, c := range rollup.RealizesStar(b.g, n.ID) {
				if cn := b.g.Node(c); cn != nil && cn.Kind == "constraint" {
					standsForCore = true
				}
			}
			if n.Kind == "constraint" && !standsForCore || n.Criticality == "critical" {
				out = append(out, n)
			}
		}
		sortNodes(out)
		return out, "every constraint and every critical concept", nil
	}
}

// sortNodes orders by criticality (highest first), then CURIE.
func sortNodes(ns []*graph.Node) {
	sort.SliceStable(ns, func(i, j int) bool {
		ri, rj := model.CriticalityRank(ns[i].Criticality), model.CriticalityRank(ns[j].Criticality)
		if ri != rj {
			return ri > rj
		}
		return ns[i].CURIE < ns[j].CURIE
	})
}

func (b *builder) concept(n *graph.Node) *Concept {
	c := &Concept{CURIE: n.CURIE, Label: n.Label, Kind: n.Kind, Tier: n.Tier, Doc: n.Doc, Criticality: n.Criticality, CriticalityFrom: n.CriticalityFrom, Status: b.status[n.CURIE]}
	if c.Label == "" {
		c.Label = nameOf(n.CURIE)
	}
	for _, ns := range b.g.NodesOf(model.NodeNamespace) {
		if ns.CURIE == n.NS || ns.Label == n.NS {
			c.NamespaceOwner = ns.Owner
		}
	}
	if c.NamespaceOwner == "" {
		if d := b.ws.Config.Owns[n.NS]; d != nil {
			c.NamespaceOwner = d.Owner
		}
	}
	// The concept and the implementation concepts that stand for it.
	ids := []string{n.ID}
	member := map[string]bool{n.ID: true}
	for _, r := range b.realizers[n.ID] {
		if member[r] {
			continue
		}
		member[r] = true
		ids = append(ids, r)
		if rn := b.g.Node(r); rn != nil {
			c.Realizers = append(c.Realizers, Realizer{CURIE: rn.CURIE, Doc: rn.Doc})
		}
	}
	sort.Slice(c.Realizers, func(i, j int) bool { return c.Realizers[i].CURIE < c.Realizers[j].CURIE })
	c.Boundaries = b.boundaries(ids)
	c.Artifacts, c.Tests = b.links(n, ids)
	c.Values = b.values(ids)
	c.History = b.history(ids)
	c.Findings, c.Waivers = b.findings(ids)
	return c
}

// boundaries lists the deepest boundaries that realize any of ids, declared
// or derived, with their owners.
func (b *builder) boundaries(ids []string) []Boundary {
	set := map[string]bool{}
	for _, id := range ids {
		for _, e := range b.g.In(id, "realizes") {
			if s := b.g.Node(e.S); s != nil && s.NodeType == model.NodeBoundary {
				set[s.ID] = true
			}
		}
	}
	inner := map[string]bool{}
	for id := range set {
		for _, anc := range b.g.PartOfChain(id) {
			if anc != id && set[anc] {
				inner[anc] = true
			}
		}
	}
	var out []Boundary
	for id := range set {
		if inner[id] {
			continue
		}
		bn := b.g.Node(id)
		out = append(out, Boundary{CURIE: bn.CURIE, Label: bn.Label, Owner: bn.Owner, Manifest: bn.File})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CURIE < out[j].CURIE })
	return out
}

// links collects the inline and inferred artifact links to the concept and
// its realizers, verifying ones apart.
func (b *builder) links(n *graph.Node, ids []string) (artifacts, tests []Link) {
	for _, id := range ids {
		via := ""
		if id != n.ID {
			via = b.g.Curie(id)
		}
		for _, e := range b.g.In(id, "") {
			if e.Source != graph.SourceInline && e.Source != graph.SourceInferred || e.Prov == nil {
				continue
			}
			if a := b.g.Node(e.S); a == nil || a.NodeType != model.NodeArtifact {
				continue
			}
			l := Link{Relation: e.P, Path: e.Prov.Path, Line: e.Prov.Line, Symbol: e.Prov.Symbol, Via: via, MarkerID: e.Prov.ID, Actor: e.Prov.Actor, Proposed: e.Prov.Proposed, URL: b.url(e.Prov.Path, e.Prov.Line)}
			if e.Prov.Conf != nil {
				l.Conf = *e.Prov.Conf
			}
			if le := b.opt.Ledger[e.Prov.ID]; le != nil {
				l.Status = le.Status
				if l.Actor == "" {
					l.Actor = le.Actor
				}
			} else if l.Proposed {
				l.Status = model.StatusProposed
			}
			if e.P == "verifies" || e.P == "tests" {
				tests = append(tests, l)
			} else {
				artifacts = append(artifacts, l)
			}
		}
	}
	sortLinks(artifacts)
	sortLinks(tests)
	return artifacts, tests
}

func sortLinks(ls []Link) {
	sort.SliceStable(ls, func(i, j int) bool {
		if ls[i].Path != ls[j].Path {
			return ls[i].Path < ls[j].Path
		}
		if ls[i].Line != ls[j].Line {
			return ls[i].Line < ls[j].Line
		}
		return ls[i].Relation < ls[j].Relation
	})
}

// values lists the published values among ids, with their binding state.
func (b *builder) values(ids []string) []Value {
	var out []Value
	for _, id := range ids {
		n := b.g.Node(id)
		if n == nil || n.Value == nil {
			continue
		}
		v := Value{Concept: n.CURIE, Value: fmt.Sprint(n.Value), BoundTo: n.ValueFrom, Live: n.LiveValue, Status: "unbound"}
		if n.ValueFrom != "" {
			path, line := splitLoc(n.ValueFrom)
			v.URL = b.url(path, line)
			v.Status = model.StatusAccepted
			if e := b.opt.Ledger["value:"+n.CURIE]; e != nil {
				v.Status = e.Status
				if e.Status == model.StatusStale { // older ledgers
					v.Status = model.StatusDrifted
				}
			}
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Concept < out[j].Concept })
	return out
}

// history turns the ledger entries whose targets resolve to ids into events,
// oldest first, scoped by Since.
func (b *builder) history(ids []string) []Event {
	member := map[string]bool{}
	for _, id := range ids {
		member[id] = true
	}
	var out []Event
	for _, e := range b.opt.Ledger {
		target := b.target(e, member)
		if target == "" {
			continue
		}
		out = append(out, b.events(e, target)...)
	}
	if !b.opt.Since.IsZero() {
		kept := out[:0]
		for _, ev := range out {
			if t, err := time.Parse(time.RFC3339, ev.When); err == nil && !t.Before(b.opt.Since) {
				kept = append(kept, ev)
			}
		}
		out = kept
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].When != out[j].When {
			return out[i].When < out[j].When
		}
		if out[i].MarkerID != out[j].MarkerID {
			return out[i].MarkerID < out[j].MarkerID
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// target resolves which of the entry's targets is one of the member IRIs,
// as a CURIE, or "". A marker still in the tree is matched by the edges it
// produced; a removed one by resolving its target text the way the
// extractor did, through the default namespace of the manifest over its path.
func (b *builder) target(e *model.LedgerEntry, member map[string]bool) string {
	if es, ok := b.byMarker[e.MarkerID]; ok {
		for _, edge := range es {
			if member[edge.O] {
				return b.g.Curie(edge.O)
			}
		}
		return ""
	}
	for _, t := range e.Targets {
		cur := rollup.StripNewCurie(t)
		if strings.HasPrefix(cur, ".") {
			m := b.ws.ManifestFor(e.Path)
			if m == nil || m.DefaultNamespace == "" {
				continue
			}
			cur = m.DefaultNamespace + ":" + cur[1:]
		}
		if n := b.g.NodeByCurie(cur); n != nil && member[n.ID] {
			return n.CURIE
		}
	}
	return ""
}

// events spells out one ledger entry as what happened and when.
func (b *builder) events(e *model.LedgerEntry, target string) []Event {
	base := Event{MarkerID: e.MarkerID, Relation: e.Relation, Target: target, Path: e.Path, Gate: e.Gate, Conf: e.Conf, URL: b.url(e.Path, 0)}
	var out []Event
	if e.Relation == "value" {
		ev := base
		ev.When, ev.Kind, ev.Actor = e.Created, "value-bound", e.Actor
		ev.Reason = fmt.Sprintf("value %q bound to %q", e.Value, e.Live)
		out = append(out, ev)
		if e.Status == model.StatusDrifted || e.Status == model.StatusStale {
			ev := base
			ev.When, ev.Kind = e.LastSeen, "value-drifted"
			ev.Reason = fmt.Sprintf("the bound code no longer reads %q; the published value %q is unconfirmed", e.Live, e.Value)
			out = append(out, ev)
		}
		return out
	}
	first := base
	first.When, first.Actor, first.Reason = e.Created, e.Actor, e.Rationale
	first.Kind = "linked"
	// A link that was reviewed later, or is still waiting, began as a proposal.
	if e.ReviewedAt != "" && e.ReviewedAt != e.Created || e.Status == model.StatusProposed || e.Status == model.StatusRejected {
		first.Kind = "proposed"
	}
	out = append(out, first)
	if e.ReviewedAt != "" && e.ReviewedAt != e.Created {
		ev := base
		ev.When, ev.Actor = e.ReviewedAt, e.ReviewedBy
		switch e.Status {
		case model.StatusRejected:
			ev.Kind, ev.Reason = "rejected", e.ReviewReason
		default:
			ev.Kind = "accepted"
		}
		out = append(out, ev)
	} else if e.Status == model.StatusRejected {
		ev := base
		ev.When, ev.Actor, ev.Kind, ev.Reason = orElse(e.ReviewedAt, e.LastSeen, e.Created), e.ReviewedBy, "rejected", e.ReviewReason
		out = append(out, ev)
	}
	switch e.Status {
	case model.StatusStale:
		ev := base
		ev.When, ev.Kind = orElse(e.LastSeen, e.Created), "stale"
		ev.Reason = "the anchored code changed since the link was accepted; awaiting re-confirmation"
		out = append(out, ev)
	case model.StatusRemoved:
		ev := base
		ev.When, ev.Kind = orElse(e.LastSeen, e.Created), "removed"
		ev.Reason = "the marker left the tree without a review decision"
		out = append(out, ev)
	}
	return out
}

func orElse(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

// findings lists the drift findings on the concept or its realizers, and the
// policy waivers naming them or any of those findings.
func (b *builder) findings(ids []string) ([]Finding, []Waiver) {
	member := map[string]bool{}
	for _, id := range ids {
		if n := b.g.Node(id); n != nil {
			member[n.CURIE] = true
		}
	}
	var out []Finding
	fids := map[string]bool{}
	for _, f := range b.opt.Drift.Findings {
		if f.Kind == rollup.KindConsistent {
			continue
		}
		hit := member[f.Concept]
		for _, v := range f.Via {
			hit = hit || member[v]
		}
		if !hit {
			continue
		}
		fids[f.ID] = true
		out = append(out, Finding{ID: f.ID, Kind: f.Kind, Severity: f.Severity, Path: f.Path, Line: f.Line, URL: b.url(f.Path, f.Line), Msg: f.Msg, Waived: f.Waived, Waiver: f.Waiver})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	var waivers []Waiver
	if b.opt.Policy != nil {
		for _, w := range b.opt.Policy.Waivers {
			if !member[w.Finding] && !fids[w.Finding] {
				continue
			}
			wv := Waiver{Finding: w.Finding, Owner: w.Owner, Expires: w.Expires, Reason: w.Reason}
			if exp, err := time.Parse(rollup.WaiverDate, w.Expires); err == nil {
				wv.Expired = rollup.WaiverExpired(exp, b.opt.Now)
			}
			waivers = append(waivers, wv)
		}
	}
	return out, waivers
}

// url renders the source-url template for a path, or "" without one.
func (b *builder) url(path string, line int) string {
	if b.opt.SourceURL == "" || path == "" {
		return ""
	}
	u := strings.ReplaceAll(b.opt.SourceURL, "{path}", path)
	if line > 0 {
		return strings.ReplaceAll(u, "{line}", fmt.Sprint(line))
	}
	// Without a line the fragment is dropped: "...#L" points nowhere.
	if i := strings.Index(u, "#"); i >= 0 && strings.Contains(u[i:], "{line}") {
		u = u[:i]
	}
	return strings.ReplaceAll(u, "{line}", "")
}

func splitLoc(loc string) (string, int) {
	path, l, ok := strings.Cut(loc, ":")
	if !ok {
		return loc, 0
	}
	n := 0
	fmt.Sscanf(l, "%d", &n)
	return path, n
}

func nameOf(cur string) string {
	if _, name, ok := strings.Cut(cur, ":"); ok {
		return name
	}
	return cur
}
