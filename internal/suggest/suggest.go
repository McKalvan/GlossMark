// @gloss implements gmi:MarkerSuggester scope=file
// @gloss produces gm:ProposedLink scope=file

// Package suggest proposes `@gloss?` markers for unmarked declarations by
// matching their names, docstrings, the tables they touch and the code
// their tests call against the ontology, so the first pass of an adoption
// is a reviewable patch instead of hand-written markers. Every proposal's
// conf is the precision its kind of evidence measured on the labelled
// calibration set (ADR 0021), not a string-match score.
package suggest

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/calib"
	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/infer"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/mcp"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
	surf "github.com/mckalvan/glossmark/internal/surface"
)

// Proposal is one marker to insert.
type Proposal struct {
	Path     string  `json:"path"`
	Line     int     `json:"line"` // 1-based line the marker is inserted above
	Symbol   string  `json:"symbol"`
	Relation string  `json:"relation"`
	Target   string  `json:"target"`
	Conf     float64 `json:"conf"`
	// Matched names the evidence: name-exact:Renewal, doc:replenishment,
	// calls:doIt, data:tbl_rma, ...
	Matched string `json:"matched"`
	// Scope is file or block when one marker stands for a run of
	// identical proposals; empty for a marker on one declaration.
	Scope string `json:"scope,omitempty"`
	// Basis is the calibration bucket conf was read from: evidence/role.
	Basis string `json:"basis"`
	// Source is where the evidence came from: code (names, docstrings,
	// tables touched, test calls), asyncapi, openapi, route, topic or
	// schema (#260). gloss suggest --eval reports precision per source.
	Source string `json:"source"`
	// Context is evidence that informs the review without choosing the
	// target, such as the tables the declaration touches.
	Context string `json:"context,omitempty"`
	// Deprecated: the target is a deprecated concept. Note then names its
	// replacement, and the marker carries the note, so the reviewer
	// chooses between the name the code still uses and its successor
	// (ADR 0030).
	Deprecated bool   `json:"deprecated,omitempty"`
	Note       string `json:"note,omitempty"`
	Marker     string `json:"marker"` // the full comment line, indented
	// AutoAccept: conf reaches the auto-accept threshold in force and the
	// target is not deprecated, so the gate classes the proposal SOFT
	// unless another rule raises it (set by gloss suggest, ADR 0024).
	AutoAccept bool `json:"auto_accept"`
	// channel is the address a flow proposal names; channelOnly when the
	// code named the channel and none of its messages (plumbing).
	channel     string
	channelOnly bool
}

// Sources a proposal's evidence can come from, named in Proposal.Source so
// gloss suggest --eval reports precision per source: the core ones, and
// the name of every infer.Source.
const (
	SourceCode   = "code"   // names, docstrings, tables touched, test calls
	SourceRoute  = "route"  // a framework route with no spec behind it
	SourceTopic  = "topic"  // a Kafka topic resource (Strimzi KafkaTopic)
	SourceSchema = "schema" // a table, view or model declaration, or an ORM mapping to one
)

// Sources lists every source, for gloss suggest --source.
var Sources = func() []string {
	out := []string{SourceCode, SourceRoute, SourceTopic, SourceSchema}
	for _, s := range infer.Builtin() {
		out = append(out, s.Name())
	}
	return out
}()

// Options tunes the matcher.
type Options struct {
	// MinConf is the lowest conf that becomes a proposal (0 means
	// DefaultMinConf).
	MinConf float64
	// Ledger, when set, holds review decisions: a link it records as
	// rejected (same marker id, or same anchor fingerprint, relation and
	// target) is not proposed again.
	Ledger map[string]*model.LedgerEntry
	// Proposed makes proposed: true concepts targets too. gloss onboard sets
	// it: its vocabulary is all proposals, and every link to one is HARD at
	// the gate, so nothing is accepted without a review (ADR 0030).
	Proposed bool
	// Sources keeps only the proposals of these sources (gloss suggest
	// --source); empty keeps every source.
	Sources []string
}

// DefaultMinConf drops the evidence that is wrong more often than right
// on the calibration set.
const DefaultMinConf = calib.DefaultMinConf

// codeExts are the source files suggest proposes markers in; YAML
// (Kubernetes workloads) and Markdown (headings) are read too, for the
// relations those files carry.
var codeExts = map[string]bool{".py": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".go": true, ".rs": true,
	".java": true, ".kt": true, ".rb": true, ".cs": true, ".php": true, ".swift": true, ".scala": true, ".tf": true, ".sql": true,
	".proto": true, ".prisma": true, ".feature": true, ".sh": true, ".bash": true, ".md": true, ".yaml": true, ".yml": true}

// Coverage says which relations and file types suggest proposes, for gloss
// suggest -h and MARKING.md; everything else is written by hand.
const Coverage = `relations: implements (SQL functions and procedures too), represents, verifies, provisions (Terraform resources, not the
roles, subnet groups, policies and other resources that only support one), documents (Markdown headings),
deploys (Kubernetes workloads), presents and hosts (page and layout components), subscribes, consumes and publishes
(listener annotations and producer or consumer calls); one scope=file or scope=block marker stands for a run.
specs: an AsyncAPI contract (documents per channel; publishes and subscribes on the code that names a channel or a
message), an OpenAPI spec (documents per operation, represents per schema, implements on the handler of a covered
route), Strimzi KafkaTopic resources (provisions), ORM table mappings (represents); --source picks sources.
targets: deprecated concepts too, with a note naming replaced-by and never auto; not proposed: true concepts (gloss onboard includes them: its vocabulary is all proposals).
files: .py .ts .tsx .js .jsx .mjs .go .rs .java .kt .rb .cs .php .swift .scala .tf .sql .proto .prisma .feature .sh .md .yaml
configures and produces only where a surface rule asks for them (an admin page editing a constraint, a document renderer)
never proposed: publishes-value, instructs, may-modify; sidecars, JSON, Dockerfiles, CI workflows`

type runner struct {
	ws      *load.Workspace
	g       *graph.Graph
	m       *matcher
	sources []infer.Source
	allowed map[*model.Manifest]rollup.Allowed
	// linked maps a symbol's last part to the targets implements links
	// give it, for tests that call it; tables maps a table name to the
	// entity a represents link gives it.
	linked map[string][]callee
	tables map[string]string
}

type file struct {
	rel   string
	lines []string
	style marker.CommentStyle
	m     *model.Manifest
	test  bool
	phase int
	// kind is the surface the file is (a page, a CLI, a BFF, a document
	// renderer), whose rule sets the relation (ADR 0046).
	kind surf.Kind
}

// Run proposes markers for the files under paths (all workspace files when
// empty). skipped counts the proposals left out because the ledger records
// them as rejected.
func Run(ws *load.Workspace, g *graph.Graph, paths []string, opt Options) (out []Proposal, skipped int, err error) {
	if opt.MinConf <= 0 {
		opt.MinConf = DefaultMinConf
	}
	var stop []string
	if ws.Config.Suggest != nil {
		stop = ws.Config.Suggest.Stopwords
	}
	r := &runner{ws: ws, g: g, m: newMatcher(ws, g, stop, opt.Proposed), allowed: map[*model.Manifest]rollup.Allowed{}, linked: map[string][]callee{}, tables: map[string]string{}}
	for _, s := range infer.Builtin() {
		if err := s.Index(ws, vocab{r.m, g}); err != nil {
			return nil, 0, fmt.Errorf("%s: %w", s.Name(), err)
		}
		r.sources = append(r.sources, s)
	}
	type link struct{ fp, rel, target string }
	rejected := map[link]bool{}
	for _, e := range opt.Ledger {
		if e.Status == model.StatusRejected && len(e.Targets) == 1 && e.AnchorFingerprint != "" {
			rejected[link{e.AnchorFingerprint, e.Relation, e.Targets[0]}] = true
		}
	}
	marked := map[string]bool{} // path#line of anchored declarations
	for _, a := range g.NodesOf(model.NodeArtifact) {
		if a.Anchor == nil {
			continue
		}
		if a.Anchor.Scope == "file" {
			marked[a.Path+"#file"] = true
		}
		marked[fmt.Sprintf("%s#%d", a.Path, a.Anchor.StartLine)] = true
		for _, e := range g.Out(a.ID, "") {
			on := g.Node(e.O)
			if on == nil || on.CURIE == "" || a.Anchor.Symbol == "" {
				continue
			}
			switch {
			case e.P == "implements":
				r.link(a.Anchor.Symbol, a.Path, on.CURIE)
			case e.P == "represents" && isTableSym(a.Anchor.Symbol):
				r.tables[strings.ToLower(lastPart(a.Anchor.Symbol))] = on.CURIE
			}
		}
	}
	// Tables first, so code that touches them knows what they represent;
	// tests last, so they see what the code they call was linked to.
	var files []*file
	for _, rel := range ws.Files {
		ext := strings.ToLower(filepath.Ext(rel))
		if !Under(rel, paths) || !codeExts[ext] || marked[rel+"#file"] || skipDoc(rel) {
			continue
		}
		style, ok := marker.StyleFor(rel, ws.Config.CommentStyles)
		if !ok {
			continue
		}
		lines, err := readLines(ws.Abs(rel))
		if err != nil {
			return nil, 0, err
		}
		f := &file{rel: rel, lines: lines, style: style, m: ws.ManifestFor(rel), test: ws.IsTestPath(rel), phase: 1, kind: surf.Detect(rel, lines)}
		switch {
		case ext == ".sql" || ext == ".proto" || ext == ".prisma":
			f.phase = 0
		case f.test:
			f.phase = 2
		}
		files = append(files, f)
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].phase < files[j].phase })
	for _, f := range files {
		var ps []Proposal
		var decls []marker.Decl
		items := map[int]specItem{} // decl index → the spec item it stands for
		ext := strings.ToLower(filepath.Ext(f.rel))
		switch {
		case ext == ".yaml" || ext == ".yml":
			// A spec file's items come from the source that reads it; any
			// other YAML may hold Kubernetes workloads and topics.
			for _, src := range r.sources {
				for _, it := range src.Items(f.rel) {
					items[len(decls)] = specItem{it, src.Name()}
					decls = append(decls, marker.Decl{Symbol: it.Symbol, Kind: "item", Line: it.Line, End: it.End, Insert: it.Line, Indent: it.Indent, Doc: it.Doc})
				}
			}
			if len(decls) == 0 {
				decls = workloads(f.lines)
			}
		default:
			decls = marker.Declarations(f.rel, f.lines, f.style)
		}
		for i, d := range decls {
			if marked[fmt.Sprintf("%s#%d", f.rel, d.Line)] {
				continue
			}
			var found []Proposal
			var role string
			switch d.Kind {
			case "item":
				it := items[i]
				role = it.Role
				if p, ok := r.proposeItem(f, d, it); ok {
					found = append(found, p)
				}
			case "workload", "topic":
				role = roleWorkload
				if d.Kind == "topic" {
					role = roleResource
				}
				if p, ok := r.propose(f, d, role, nil); ok {
					found = append(found, p)
				}
			default:
				var ok bool
				if role, ok = surface(d, f.lines, f.rel, f.test); !ok {
					continue
				}
				hints := r.hints(f, d, role)
				if p, ok := r.propose(f, d, role, hints); ok {
					found = append(found, p)
				}
				if role == roleFunc || role == roleMember {
					for _, src := range r.sources {
						flows, _ := flowHints(hints[src.Name()])
						found = append(found, r.proposeFlows(f, d, role, src.Name(), flows)...)
					}
				}
			}
			for _, p := range found {
				if p.Conf < opt.MinConf {
					continue
				}
				if le := opt.Ledger[extract.MarkerID(f.rel, p.Relation, []string{p.Target}, d.Symbol, "symbol")]; (le != nil && le.Status == model.StatusRejected) ||
					rejected[link{d.Fingerprint, p.Relation, p.Target}] {
					skipped++
					continue
				}
				ps = append(ps, p)
			}
		}
		ps = collapse(f, dedupeMembers(plumbing(dedupeLinks(ps)), decls))
		parentAt := map[int]string{}
		for _, d := range decls {
			parentAt[d.Insert] = d.Parent
		}
		for _, p := range ps {
			full := fullTarget(ws, f.rel, p.Target)
			switch {
			case p.Relation == "implements" && p.Scope == "" && p.Conf >= callableConf:
				sym := p.Symbol
				if par := parentAt[p.Line]; par != "" {
					sym = par + "." + sym
				}
				r.link(sym, f.rel, full)
			case p.Relation == "represents" && isTableSym(p.Symbol):
				r.tables[strings.ToLower(lastPart(p.Symbol))] = full
			}
		}
		out = append(out, ps...)
	}
	if len(opt.Sources) > 0 {
		var kept []Proposal
		for _, p := range out {
			if contains(opt.Sources, p.Source) {
				kept = append(kept, p)
			}
		}
		out = kept
	}
	if out == nil {
		out = []Proposal{}
	}
	return out, skipped, nil
}

// dedupeLinks drops a proposal that repeats another's line, relation and
// target: the flow a spec infers and the topic the body names are one
// link. The stronger-sourced one, read first, stays.
func dedupeLinks(ps []Proposal) []Proposal {
	seen := map[string]bool{}
	var out []Proposal
	for _, p := range ps {
		k := fmt.Sprintf("%d\x00%s\x00%s", p.Line, p.Relation, p.Target)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, p)
	}
	return out
}

// callableConf is the conf a link suggest proposes itself must reach
// before the tests that call its symbol are proposed to verify its
// target: a link right more often than wrong. At the default floor, tests
// followed every weak name match the table let through, and calls
// evidence fed on itself each time the table was refitted (ADR 0032).
const callableConf = 0.5

// callee is a linked declaration a test may call: its target and the names
// that own it (its class or receiver, its file's stem), which a test that
// calls it usually names too.
type callee struct {
	target string
	owners []string
}

func (r *runner) link(sym, path, target string) {
	name := lastPart(sym)
	if len(name) < 4 {
		return
	}
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	owners := []string{stem}
	if i := strings.LastIndex(sym, "."); i > 0 {
		owners = append(owners, lastPart(sym[:i]))
	}
	r.linked[name] = append(r.linked[name], callee{target, owners})
}

func isTableSym(sym string) bool {
	return strings.HasPrefix(sym, "table.") || strings.HasPrefix(sym, "view.")
}

// skipDoc leaves out Markdown that is not documentation of the domain:
// agent instruction files, changelogs, decision records.
func skipDoc(rel string) bool {
	if !strings.HasSuffix(strings.ToLower(rel), ".md") {
		return false
	}
	base := filepath.Base(rel)
	switch strings.ToUpper(strings.TrimSuffix(base, filepath.Ext(base))) {
	case "AGENTS", "CLAUDE", "CHANGELOG", "CONTRIBUTING", "LICENSE", "SKILL", "SECURITY", "CODE_OF_CONDUCT":
		return true
	}
	return strings.Contains("/"+rel, "/adr/")
}

// specItem is a spec file's item with the source that declared it.
type specItem struct {
	infer.Item
	source string
}

// hints asks every source what a code declaration names through its
// spec, keyed by source name.
func (r *runner) hints(f *file, d marker.Decl, role string) map[string][]infer.Hint {
	if len(r.sources) == 0 || role == roleTest || role == roleHeading || role == roleWorkload || role == roleTable || role == roleResource {
		return nil
	}
	dir := ""
	if f.m != nil {
		dir = f.m.Dir
	}
	decl := infer.Decl{Path: f.rel, Dir: dir, Symbol: d.Symbol, Role: role, Line: d.Line, Decorators: d.Decorators, Body: bodyOf(f.lines, d)}
	out := map[string][]infer.Hint{}
	for _, src := range r.sources {
		if hs := src.Hints(decl); len(hs) > 0 {
			out[src.Name()] = hs
		}
	}
	return out
}

// propose chooses the target, relation and conf for one declaration.
// hints are what the sources said about it (nil for a declaration no
// source reads).
func (r *runner) propose(f *file, d marker.Decl, role string, hints map[string][]infer.Hint) (Proposal, bool) {
	sym := lastPart(d.Symbol)
	switch role {
	case roleHeading, roleWorkload:
		sym = d.Symbol
		if i := strings.Index(sym, "/"); i >= 0 && role == roleWorkload {
			sym = sym[i+1:]
		}
	case roleTable:
		// Legacy schemas prefix every table: tbl_rma is rma.
		low := strings.ToLower(sym)
		for _, p := range []string{"tbl_", "tb_", "t_"} {
			if strings.HasPrefix(low, p) && len(sym) > len(p) {
				sym = sym[len(p):]
				break
			}
		}
	case roleTest:
		if strings.Contains(d.Symbol, ":") {
			sym = d.Symbol[strings.Index(d.Symbol, ":")+1:]
		}
		sym = testSubject(sym)
	}
	if d.Kind == "topic" {
		// A Kafka topic resource is named after its topic: claims.filed.v1.
		sym = d.Symbol[strings.Index(d.Symbol, "/")+1:]
	}
	body := bodyOf(f.lines, d)
	consumer, producer := flow(d, body)
	var hits []hit
	source := SourceCode
	switch {
	case d.Kind == "topic":
		source = SourceTopic
		hits = r.topicResourceHits(sym)
	default:
		if role == roleTable {
			source = SourceSchema
		}
		if role != roleHeading && role != roleWorkload {
			hits = append(hits, r.m.namedInDoc(d.Symbol, d.Parent)...)
		}
		hits = append(hits, r.m.nameHits(sym)...)
	}
	if role == rolePage || role == roleShell {
		// A component named Page takes its meaning from its route.
		hits = append(hits, r.m.nameHits(filepath.Base(filepath.Dir(f.rel)))...)
	}
	context := ""
	sourced, flows := 0, 0
	switch {
	case d.Kind == "topic":
	case role == roleTest:
		hits = append(hits, r.m.docHits(d.Doc)...)
		hits = append(hits, r.callHits(body, lastPart(d.Symbol), strings.Join(f.lines, "\n"))...)
	default:
		hits = append(hits, r.m.docHits(d.Doc)...)
		if role != roleHeading && role != roleWorkload && role != roleTable {
			dh, ctx := r.dataHits(body)
			hits = append(hits, dh...)
			context = ctx
		}
		if role == roleType {
			hits = append(hits, r.schemaHits(body)...)
		}
		// What the specs say the declaration names: a route the OpenAPI
		// document maps to a process, ... Flows are proposed apart.
		for _, src := range r.sources {
			fl, plain := flowHints(hints[src.Name()])
			flows += len(fl)
			for _, h := range plain {
				hs := r.resolve(src.Name(), h.Target, "", h.Relation)
				sourced += len(hs)
				hits = append(hits, hs...)
			}
		}
		// A route no spec covers is name evidence on its path.
		if (role == roleFunc || role == roleMember) && sourced == 0 {
			hits = append(hits, r.routeHits(d, body)...)
		}
	}
	// A listener's or producer's topic literal is weaker evidence than a
	// contract that names the topic's events: when a source covers the
	// declaration (proposeFlows), the literal adds nothing.
	if (consumer || producer) && flows == 0 {
		hits = append(hits, r.topicHits(d, body)...)
	}
	h, rel, tie, ok := r.choose(f, d, role, hits, consumer, producer, "")
	if !ok {
		return Proposal{}, false
	}
	switch {
	case h.source != "":
		source = h.source
	case h.kind == evSchema:
		source = SourceSchema
	case h.kind == evRoute:
		source = SourceRoute
	}
	basis := h.basis(role, tie)
	p := Proposal{Path: f.rel, Line: d.Insert, Symbol: d.Symbol, Relation: rel, Target: r.shorten(f, h.c.curie), Matched: h.matched(), Basis: basis, Source: source, Context: context, Conf: confFor(basis)}
	if h.c.n.Deprecated {
		p.Deprecated, p.Note = true, deprecatedNote(h.c.n)
	}
	p.Marker = markerLine(f.style, d.Indent, p)
	return p, true
}

// proposeItem proposes the link a spec item stands for: documents on an
// AsyncAPI channel or an OpenAPI operation, represents on a schema,
// anchored as a YAML block.
func (r *runner) proposeItem(f *file, d marker.Decl, it specItem) (Proposal, bool) {
	hits := r.resolve(it.source, it.Target, it.Doc, "")
	h, rel, tie, ok := r.choose(f, d, it.Role, hits, false, false, it.Relation)
	if !ok {
		return Proposal{}, false
	}
	basis := h.basis(it.Role, tie)
	p := Proposal{Path: f.rel, Line: d.Insert, Symbol: d.Symbol, Relation: rel, Target: r.shorten(f, h.c.curie), Matched: h.matched(), Basis: basis, Source: it.source, Scope: "block", Conf: confFor(basis)}
	if h.c.n.Deprecated {
		p.Deprecated, p.Note = true, deprecatedNote(h.c.n)
	}
	p.Marker = markerLine(f.style, d.Indent, p)
	return p, true
}

// resolve turns a source's target into hits: the concept it named, or
// what the matcher finds for its names and doc, of the kinds it allows.
// A basis the source set replaces the matcher's evidence kind, so the
// calibration bucket is the source's (openapi/operation/func); none
// keeps the matcher's (openapi/name-exact/operation).
func (r *runner) resolve(source string, t infer.Target, doc, rel string) []hit {
	if t.Concept != "" {
		c := r.m.byCurie[t.Concept]
		if c == nil {
			return nil
		}
		return []hit{{c: c, kind: t.Basis, terms: t.Terms, source: source, rel: rel}}
	}
	var hs []hit
	for _, n := range t.Names {
		hs = append(hs, r.m.nameHits(n)...)
	}
	hs = append(hs, r.m.docHits(doc)...)
	var out []hit
	for _, h := range hs {
		if len(t.Kinds) > 0 && !r.kindOK(h.c, t.Kinds) {
			continue
		}
		h.source, h.rel = source, rel
		switch {
		case t.Basis != "":
			h.kind, h.terms = t.Basis, t.Terms
		case t.Terms != "":
			h.terms += "<" + t.Terms
		}
		out = append(out, h)
	}
	return out
}

// kindOK reports a concept of one of the kinds, or an implementation
// concept that realizes one.
func (r *runner) kindOK(c *concept, kinds []string) bool {
	if contains(kinds, c.n.Kind) {
		return true
	}
	for cur := range c.realizes {
		if o := r.m.byCurie[cur]; o != nil && contains(kinds, o.n.Kind) {
			return true
		}
	}
	return false
}

// processKinds are what a route can implement.
var processKinds = []string{"process", "capability"}

// topicWords are the words a topic resource's name shares with any topic:
// they never tell one from another.
var topicWords = map[string]bool{"topic": true, "queue": true, "stream": true, "channel": true, "event": true, "kafka": true, "subject": true}

// topicResourceHits finds the resource concepts a Kafka topic's name
// stands up: every distinguishing word of the concept's name is in the
// topic's (claims.filed.v1: FiledTopic). With several, the ones that cover
// the most of the topic's name are kept.
func (r *runner) topicResourceHits(name string) []hit {
	toks := mcp.Tokens(infer.StripVersion(name))
	if len(toks) == 0 {
		return nil
	}
	have := map[string]bool{}
	for _, t := range toks {
		have[t] = true
	}
	var out []hit
	best := 0
	for _, c := range r.m.concepts {
		if c.n.Kind != "resource" {
			continue
		}
		var own []string
		for _, t := range c.toks {
			if !topicWords[t] {
				own = append(own, t)
			}
		}
		if len(own) == 0 || !r.m.nonStop(own) {
			continue
		}
		all := true
		for _, t := range own {
			all = all && have[t]
		}
		if !all {
			continue
		}
		if len(own) > best {
			best, out = len(own), nil
		}
		if len(own) == best {
			out = append(out, hit{c: c, kind: evTopic, terms: name})
		}
	}
	return out
}

var tableMapping = regexp.MustCompile(`__tablename__\s*=\s*["'](\w+)["']|@Table\s*\(\s*(?:name\s*=\s*)?["'](\w+)["']|\b(?:table_name|tableName|db_table|__table__)\s*[:=]\s*["'](\w+)["']|@@map\s*\(\s*["'](\w+)["']\s*\)`)

// schemaHits finds the entity a type maps to a table of (__tablename__,
// @Table(name), tableName, db_table): the entity a represents link already
// binds the table to, else the one the table's name is.
func (r *runner) schemaHits(body string) []hit {
	var out []hit
	for _, m := range tableMapping.FindAllStringSubmatch(body, -1) {
		name := m[1] + m[2] + m[3] + m[4]
		if cur, ok := r.tables[strings.ToLower(name)]; ok {
			if c := r.m.byCurie[cur]; c != nil {
				out = append(out, hit{c: c, kind: evSchema, terms: name})
				continue
			}
		}
		bare := name
		for _, p := range []string{"tbl_", "tb_", "t_"} {
			if strings.HasPrefix(strings.ToLower(bare), p) && len(bare) > len(p) {
				bare = bare[len(p):]
				break
			}
		}
		for _, h := range r.m.nameHits(bare) {
			if (h.kind == evNameExact || h.kind == evAliasExact) && (h.c.n.Kind == "entity" || h.c.n.Kind == "event") {
				out = append(out, hit{c: h.c, kind: evSchema, terms: name})
			}
		}
	}
	return out
}

// routeHits is the name evidence of a route no spec covers: the process
// its path's own words name.
func (r *runner) routeHits(d marker.Decl, body string) []hit {
	var out []hit
	for _, rt := range infer.Routes(d.Decorators, body) {
		for _, h := range r.m.nameHits(infer.PathWords(rt.Path)) {
			if (h.kind == evNameExact || h.kind == evAliasExact || h.kind == evNameContains || h.kind == evNamePartial) && r.kindOK(h.c, processKinds) {
				out = append(out, hit{c: h.c, kind: evRoute, terms: strings.TrimSpace(rt.Method + " " + rt.Path)})
			}
		}
	}
	return out
}

// deprecatedNote tells the reviewer the target is deprecated and what
// replaced it. A marker note is quoted, so it holds no double quote.
func deprecatedNote(n *graph.Node) string {
	if n.ReplacedBy == "" {
		return n.CURIE + " is deprecated"
	}
	return n.CURIE + " is deprecated, replaced-by " + n.ReplacedBy + "; keep it, or link the replacement"
}

// AutoAccepts reports whether p is an auto-accept candidate under a
// threshold: conf reaches it and the target is not deprecated, which the
// gate always classes HARD.
func (p Proposal) AutoAccepts(threshold float64) bool {
	return !p.Deprecated && p.Conf >= threshold-1e-9
}

// choose returns the strongest hit this boundary may use, the relation it
// gets, and whether an unrelated concept matched as strongly (a tie the
// reviewer has to break).
// fixed, when set, is the relation every hit gets (a spec item's).
func (r *runner) choose(f *file, d marker.Decl, role string, hits []hit, consumer, producer bool, fixed string) (hit, string, bool, bool) {
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.strength() != b.strength() {
			return a.strength() > b.strength()
		}
		// A live concept beats a deprecated one the code names as well.
		if a.c.n.Deprecated != b.c.n.Deprecated {
			return !a.c.n.Deprecated
		}
		// Infrastructure stands up resources, a deployment runs a system.
		if pa, pb := kindPreferred(role, a.c.n.Kind), kindPreferred(role, b.c.n.Kind); pa != pb {
			return pa
		}
		// Code implements the implementation tier; a tie goes to it. A
		// surface (a page, a route, a renderer) names the core concept it
		// shows, not the engine's concept (ADR 0046).
		if a.c.impl != b.c.impl {
			return a.c.impl != surf.PrefersCore(f.kind)
		}
		if len(a.c.toks) != len(b.c.toks) {
			return len(a.c.toks) < len(b.c.toks)
		}
		return a.c.curie < b.c.curie
	})
	var valid []hit
	rels := map[string]string{}
	seen := map[string]bool{}
	for _, h := range hits {
		if seen[h.c.curie] {
			continue
		}
		seen[h.c.curie] = true
		rel := relationFor(role, d.Kind, h.c.n.Kind, h.c.impl, consumer, producer)
		switch {
		case fixed != "":
			rel = fixed
		case h.rel != "":
			rel = h.rel
		case role != roleTest && !f.test:
			// The surface rule overrides the role's relation: produces on
			// a document renderer, configures on an admin page editing a
			// constraint, presents on a surface for a process an engine
			// elsewhere implements (ADR 0046). A relation a spec fixes
			// (fixed, h.rel) is more specific and wins over it.
			if sr := surf.Default(r.g, r.ws, f.kind, f.rel, h.c.n); sr != "" && (rel == "implements" || rel == "presents" || rel == "represents") {
				rel = sr
			}
		}
		if !r.usable(f, h.c, rel) {
			continue
		}
		valid = append(valid, h)
		rels[h.c.curie] = rel
	}
	if len(valid) == 0 {
		return hit{}, "", false, false
	}
	best := valid[0]
	// A data shape a process's doc names is what the process handles, not
	// its mechanism ("announces ShipmentDelivered"): a type whose own name
	// is an entity or event the type can represent links to that instead
	// (ADR 0032). The displaced concept is no tie.
	var displaced *concept
	if role == roleType && best.kind == evNamedInDoc && rels[best.c.curie] == "implements" {
		for _, h := range valid[1:] {
			if (h.kind == evNameExact || h.kind == evAliasExact) && rels[h.c.curie] == "represents" {
				displaced, best = best.c, h
				break
			}
		}
	}
	// Code names the business concept but implements its mechanism: an
	// implementation concept that realizes the core concept the name
	// matched, and that the declaration also resembles, is the better
	// target (CancellationWindow: the policy that applies the window). With
	// several such mechanisms, only one the code calls or that names the
	// declaration wins. Documentation describes the business concept and
	// keeps it.
	if !best.c.impl && role != roleHeading && displaced == nil {
		var impls []hit
		for _, h := range valid[1:] {
			if h.c.impl && h.c.realizes[best.c.curie] {
				impls = append(impls, h)
			}
		}
		var strong []hit
		for _, h := range impls {
			if h.strength() >= strength[evCalls] {
				strong = append(strong, h)
			}
		}
		switch {
		case len(strong) == 1:
			best = hit{c: strong[0].c, kind: strong[0].kind, terms: strong[0].terms + "+core:" + best.c.curie, source: strong[0].source, rel: strong[0].rel}
		case len(impls) == 1:
			best = hit{c: impls[0].c, kind: best.kind, terms: best.terms + "+impl"}
		}
	}
	tie := false
	for _, h := range valid {
		if h.c == best.c || h.c == displaced || h.strength() < best.strength() {
			continue
		}
		if h.c.realizes[best.c.curie] || best.c.realizes[h.c.curie] {
			continue
		}
		// A deprecated concept and its replacement are one choice, which
		// the note puts to the reviewer.
		if h.c.n.ReplacedBy == best.c.curie || best.c.n.ReplacedBy == h.c.curie {
			continue
		}
		tie = true
	}
	return best, rels[best.c.curie], tie, true
}

// kindPreferred reports a target kind the role's relation is made for.
func kindPreferred(role, kind string) bool {
	switch role {
	case roleResource:
		return kind == "resource"
	case roleWorkload:
		return kind == "system-type"
	}
	return false
}

// usable reports whether the boundary may link to c with rel: inside the
// allowed set gloss check uses (written or derived, the derived keyword
// expanded), and of a kind rel accepts.
func (r *runner) usable(f *file, c *concept, rel string) bool {
	if f.m != nil {
		a, ok := r.allowed[f.m]
		if !ok {
			a = rollup.AllowedFor(r.g, r.ws, f.m)
			r.allowed[f.m] = a
		}
		if len(a.Patterns) > 0 && !a.Matches(c.curie) {
			return false
		}
	}
	kind := c.n.Kind
	if v := r.ws.Vocab.Get(rel); v != nil && len(v.RangeKinds) > 0 && kind != "" && !contains(v.RangeKinds, kind) {
		return false
	}
	switch rel {
	case "provisions", "deploys":
		// Infrastructure stands up resources and systems, not domain
		// entities that share the resource's name (aws_db_subnet_group.booking).
		return kind == "resource" || kind == "system-type"
	}
	return true
}

// relationFor chooses the relation for a declaration of the given role
// and a target of the given kind.
func relationFor(role, declKind, conceptKind string, impl, consumer, producer bool) string {
	switch role {
	case roleTest:
		return "verifies"
	case roleTable:
		return "represents"
	case roleResource:
		return "provisions"
	case roleHeading:
		return "documents"
	case roleWorkload:
		return "deploys"
	case rolePage:
		return "presents"
	case roleShell:
		return "hosts"
	}
	switch {
	case consumer && conceptKind == "event":
		return "subscribes"
	case producer && conceptKind == "event":
		return "publishes"
	case consumer && conceptKind == "entity":
		return "consumes"
	}
	if declKind == "proto" && role == roleType {
		return "represents"
	}
	// An implementation-tier measure is the code that computes it
	// (OnTimeRateCalculator, NoShowReport), so a type named for it
	// implements it; a core measure's type holds its values (ADR 0032).
	if role == roleType && (conceptKind == "entity" || conceptKind == "event" || conceptKind == "measure" && !impl || conceptKind == "resource") {
		return "represents"
	}
	return "implements"
}

var (
	listener   = regexp.MustCompile(`^@(KafkaListener|RabbitListener|SqsListener|JmsListener|StreamListener|EventListener|Subscribe|EventPattern|MessagePattern|KafkaHandler|Incoming|Consumer|consumer|subscriber|on_event|app\.agent)\b`)
	consumeAPI = regexp.MustCompile(`\.(subscribe|Subscribe|SubscribeTopics|consume|Consume|poll|receive_messages|ReceiveMessage)\(|\bfor\s+\w+\s+(?:in|:=\s*range)\s+\w*consumer`)
	produceAPI = regexp.MustCompile(`\.(publish|Publish|produce|Produce|send_message|SendMessage|emit|Emit)\(|(?i)(producer|kafkaTemplate|publisher)\.send\(`)
	literal    = regexp.MustCompile(`"([A-Za-z][\w.\-/]{2,})"|'([A-Za-z][\w.\-/]{2,})'`)
	ident      = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
)

// flow reports a declaration that consumes messages (a listener
// annotation, a subscribe or poll call) or produces them.
func flow(d marker.Decl, body string) (consumer, producer bool) {
	for _, dec := range d.Decorators {
		if listener.MatchString(dec) {
			consumer = true
		}
	}
	if consumeAPI.MatchString(body) {
		consumer = true
	}
	if produceAPI.MatchString(body) {
		producer = true
	}
	return consumer, producer
}

// topicHits finds event concepts a listener's or producer's topic names.
func (r *runner) topicHits(d marker.Decl, body string) []hit {
	text := strings.Join(d.Decorators, "\n") + "\n" + body
	var out []hit
	for _, m := range literal.FindAllStringSubmatch(text, -1) {
		lit := m[1] + m[2]
		toks := mcp.Tokens(lit)
		for _, c := range r.m.concepts {
			if c.n.Kind == "event" && containsSeq(toks, c.toks) {
				out = append(out, hit{c: c, kind: evTopic, terms: lit})
			}
		}
	}
	return out
}

// callHits proposes verifies for a test that calls a symbol an implements
// link already binds, by the most-called target. A callee whose class or
// file the test file names (Manager().doIt, from core.Manager import) wins
// over a namesake elsewhere.
func (r *runner) callHits(body, self string, fileText string) []hit {
	count := map[string]int{}
	via := map[string]string{}
	for _, id := range ident.FindAllString(body, -1) {
		if id == self {
			continue
		}
		for _, c := range r.linked[id] {
			n := 1
			for _, o := range c.owners {
				if len(o) >= 3 && wordIn(fileText, o) {
					n = 10
					break
				}
			}
			count[c.target] += n
			if via[c.target] == "" {
				via[c.target] = id
			}
		}
	}
	best, bestN := "", 0
	for t, n := range count {
		if n > bestN || n == bestN && t < best {
			best, bestN = t, n
		}
	}
	if c := r.m.byCurie[best]; c != nil {
		return []hit{{c: c, kind: evCalls, terms: via[best]}}
	}
	return nil
}

// dataHits reads the tables a declaration's body names. When a represents
// link binds one to an entity and exactly one process or capability in the
// ontology works on that entity, it is a candidate target; either way the
// tables are reported as context.
func (r *runner) dataHits(body string) ([]hit, string) {
	if len(r.tables) == 0 {
		return nil, ""
	}
	var touched []string
	entities := map[string]bool{}
	seen := map[string]bool{}
	for _, id := range ident.FindAllString(body, -1) {
		l := strings.ToLower(id)
		e, ok := r.tables[l]
		if !ok || seen[l] {
			continue
		}
		seen[l] = true
		touched = append(touched, e+" ("+l+")")
		entities[e] = true
	}
	if len(touched) == 0 {
		return nil, ""
	}
	sort.Strings(touched)
	procs := map[*concept]string{}
	for e := range entities {
		en := r.g.NodeByCurie(e)
		if en == nil {
			continue
		}
		for _, c := range r.m.concepts {
			if c.n.Kind != "process" && c.n.Kind != "capability" {
				continue
			}
			for _, ed := range r.g.Out(c.n.ID, "") {
				if ed.O == en.ID && ed.P != "is-a" && ed.Source != graph.SourceDerived {
					procs[c] = e
				}
			}
		}
	}
	ctx := "touches " + strings.Join(touched, ", ")
	if len(procs) != 1 {
		return nil, ctx
	}
	for c, e := range procs {
		return []hit{{c: c, kind: evData, terms: e}}, ctx
	}
	return nil, ctx
}

// bodyOf is the declaration's block as one string, decorators included.
func bodyOf(lines []string, d marker.Decl) string {
	from, to := d.Line, d.End
	if to < from {
		to = from
	}
	if to > len(lines) {
		to = len(lines)
	}
	return strings.Join(lines[from-1:to], "\n")
}

// dedupeMembers drops a member's proposal when its class already carries
// the same link: the class marker says it once.
func dedupeMembers(ps []Proposal, decls []marker.Decl) []Proposal {
	parent := map[string]string{}
	for _, d := range decls {
		if d.Parent != "" {
			parent[fmt.Sprintf("%d", d.Insert)] = d.Parent
		}
	}
	onClass := map[string]bool{}
	for _, p := range ps {
		if parent[fmt.Sprintf("%d", p.Line)] == "" {
			onClass[p.Symbol+"\x00"+p.Relation+"\x00"+p.Target] = true
		}
	}
	var out []Proposal
	for _, p := range ps {
		if par := parent[fmt.Sprintf("%d", p.Line)]; par != "" && onClass[par+"\x00"+p.Relation+"\x00"+p.Target] {
			continue
		}
		out = append(out, p)
	}
	return out
}

// collapse turns a run of identical proposals into one marker, as GM131
// asks (#66): when every proposal in the file is the same link (three or
// more, or two or more tests) it becomes one scope=file marker at the top;
// otherwise a run of three or more consecutive ones becomes one
// scope=block marker above the first.
func collapse(f *file, ps []Proposal) []Proposal {
	if len(ps) < 2 {
		return ps
	}
	same := func(a, b Proposal) bool { return a.Relation == b.Relation && a.Target == b.Target }
	all := true
	for _, p := range ps[1:] {
		all = all && same(p, ps[0])
	}
	if all && (len(ps) >= minRun || ps[0].Relation == "verifies") {
		p := best(ps)
		p.Line, p.Scope, p.Symbol = fileInsert(f.lines), "file", ""
		p.Marker = markerLine(f.style, "", p)
		return []Proposal{p}
	}
	var out []Proposal
	for i := 0; i < len(ps); {
		j := i + 1
		for j < len(ps) && same(ps[j], ps[i]) {
			j++
		}
		if j-i >= minRun {
			p := best(ps[i:j])
			p.Line, p.Symbol, p.Scope = ps[i].Line, ps[i].Symbol, "block"
			p.Marker = markerLine(f.style, indentOf(f.lines, ps[i].Line), p)
			out = append(out, p)
		} else {
			out = append(out, ps[i:j]...)
		}
		i = j
	}
	return out
}

// minRun is the run length GM131 reports.
const minRun = 3

func best(ps []Proposal) Proposal {
	b := ps[0]
	for _, p := range ps[1:] {
		if p.Conf > b.Conf {
			b = p
		}
	}
	return b
}

func indentOf(lines []string, line int) string {
	if line < 1 || line > len(lines) {
		return ""
	}
	l := lines[line-1]
	return l[:len(l)-len(strings.TrimLeft(l, " \t"))]
}

// fileInsert is the line a file-level marker goes above: the first line,
// after a shebang, a PHP opening tag or an XML declaration.
func fileInsert(lines []string) int {
	if len(lines) > 0 {
		t := strings.TrimSpace(lines[0])
		if strings.HasPrefix(t, "#!") || strings.HasPrefix(t, "<?php") || strings.HasPrefix(t, "<?xml") {
			return 2
		}
	}
	return 1
}

func markerLine(style marker.CommentStyle, indent string, p Proposal) string {
	tok := commentLine(style)
	line := fmt.Sprintf("%s%s @gloss? %s %s", indent, tok[0], p.Relation, p.Target)
	if p.Scope != "" {
		line += " scope=" + p.Scope
	}
	line += fmt.Sprintf(" conf=%.2f", p.Conf)
	if p.Note != "" {
		line += fmt.Sprintf(" note=%q", strings.ReplaceAll(p.Note, `"`, "'"))
	}
	if tok[1] != "" {
		line += " " + tok[1]
	}
	return line
}

// workloads finds the Kubernetes workloads in a YAML file, one per
// document, for deploys proposals.
func workloads(lines []string) []marker.Decl {
	var out []marker.Decl
	start := 0
	flush := func(end int) {
		kind, name := "", ""
		for k := start; k < end; k++ {
			l := lines[k]
			t := strings.TrimSpace(l)
			if strings.HasPrefix(l, "kind:") {
				kind = strings.TrimSpace(strings.TrimPrefix(t, "kind:"))
			}
			if strings.HasPrefix(t, "name:") && strings.HasPrefix(l, "  ") && !strings.HasPrefix(l, "   ") && name == "" {
				name = strings.Trim(strings.TrimSpace(strings.TrimPrefix(t, "name:")), `"'`)
			}
		}
		declKind := "workload"
		switch kind {
		case "Deployment", "StatefulSet", "DaemonSet", "CronJob", "Job", "Rollout", "Service", "KnativeService":
		case "KafkaTopic":
			// A Strimzi topic stands up the topic it names (provisions).
			declKind = "topic"
			if t := topicName(lines[start:end]); t != "" {
				name = t
			}
		default:
			return
		}
		if kind == "Service" || name == "" {
			return
		}
		first := start
		for first < end && (strings.TrimSpace(lines[first]) == "" || strings.HasPrefix(strings.TrimSpace(lines[first]), "#")) {
			first++
		}
		if first >= end {
			return
		}
		out = append(out, marker.Decl{Symbol: kind + "/" + name, Kind: declKind, Line: first + 1, End: end, Insert: first + 1})
	}
	for k, l := range lines {
		if strings.TrimSpace(l) == "---" {
			flush(k)
			start = k + 1
		}
	}
	flush(len(lines))
	return out
}

// topicName is spec.topicName of a KafkaTopic document, when set.
func topicName(lines []string) string {
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "topicName:") && strings.HasPrefix(l, "  ") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(t, "topicName:")), `"'`)
		}
	}
	return ""
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func commentLine(style marker.CommentStyle) [2]string {
	for _, t := range style.Line {
		if t != "" {
			return [2]string{t, ""}
		}
	}
	if len(style.Block) > 0 {
		return style.Block[0]
	}
	return [2]string{"#", ""}
}

func lastPart(sym string) string {
	if i := strings.LastIndexAny(sym, ".:/"); i >= 0 {
		return sym[i+1:]
	}
	return sym
}

// Under reports whether rel (slash-separated, relative to the root) is one of
// paths or inside one of them; no paths, "" or "." match everything.
func Under(rel string, paths []string) bool {
	if len(paths) == 0 {
		return true
	}
	for _, p := range paths {
		p = strings.Trim(strings.TrimPrefix(filepath.ToSlash(p), "./"), "/")
		if p == "" || p == "." || rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	return false
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out, sc.Err()
}

// Patch renders proposals as a unified diff against the files under root,
// applicable with git apply or patch -p1.
func Patch(root string, ps []Proposal) (string, error) {
	byFile := map[string][]Proposal{}
	for _, p := range ps {
		byFile[p.Path] = append(byFile[p.Path], p)
	}
	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)
	var sb strings.Builder
	for _, f := range files {
		lines, err := readLines(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			return "", err
		}
		ins := byFile[f]
		sort.SliceStable(ins, func(i, j int) bool { return ins[i].Line < ins[j].Line })
		fmt.Fprintf(&sb, "--- a/%s\n+++ b/%s\n", f, f)
		const ctx = 3
		offset := 0
		for i := 0; i < len(ins); {
			// Merge insertions whose context windows overlap into one hunk.
			start := max(0, ins[i].Line-1-ctx)
			j := i
			end := min(len(lines), ins[j].Line-1+ctx)
			for j+1 < len(ins) && ins[j+1].Line-1-ctx <= end {
				j++
				end = min(len(lines), ins[j].Line-1+ctx)
			}
			n := j - i + 1
			fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", start+1, end-start, start+1+offset, end-start+n)
			k := i
			for l := start; l < end; l++ {
				for k <= j && ins[k].Line-1 == l {
					sb.WriteString("+" + ins[k].Marker + "\n")
					k++
				}
				sb.WriteString(" " + lines[l] + "\n")
			}
			for ; k <= j; k++ {
				sb.WriteString("+" + ins[k].Marker + "\n")
			}
			offset += n
			i = j + 1
		}
	}
	return sb.String(), nil
}

// Write inserts the proposals into the files.
func Write(root string, ps []Proposal) error {
	byFile := map[string][]Proposal{}
	for _, p := range ps {
		byFile[p.Path] = append(byFile[p.Path], p)
	}
	for f, ins := range byFile {
		path := filepath.Join(root, filepath.FromSlash(f))
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.SplitAfter(string(b), "\n")
		sort.SliceStable(ins, func(i, j int) bool { return ins[i].Line > ins[j].Line })
		for _, p := range ins {
			at := p.Line - 1
			if at > len(lines) {
				at = len(lines)
			}
			lines = append(lines[:at], append([]string{p.Marker + "\n"}, lines[at:]...)...)
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "")), 0o644); err != nil {
			return err
		}
	}
	return nil
}
