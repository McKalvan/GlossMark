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
}

// Options tunes the matcher.
type Options struct {
	// MinConf is the lowest conf that becomes a proposal (0 means
	// DefaultMinConf).
	MinConf float64
	// Ledger, when set, holds review decisions: a link it records as
	// rejected (same marker id, or same anchor fingerprint, relation and
	// target) is not proposed again.
	Ledger map[string]*model.LedgerEntry
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
targets: deprecated concepts too, with a note naming replaced-by and never auto; not proposed: true concepts.
files: .py .ts .tsx .js .jsx .mjs .go .rs .java .kt .rb .cs .php .swift .scala .tf .sql .proto .prisma .feature .sh .md .yaml
configures and produces only where a surface rule asks for them (an admin page editing a constraint, a document renderer)
never proposed: publishes-value, instructs, may-modify; sidecars, JSON, Dockerfiles, CI workflows`

type runner struct {
	ws      *load.Workspace
	g       *graph.Graph
	m       *matcher
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
	// renderer), whose rule sets the relation (ADR 0041).
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
	r := &runner{ws: ws, g: g, m: newMatcher(ws, g, stop), allowed: map[*model.Manifest]rollup.Allowed{}, linked: map[string][]callee{}, tables: map[string]string{}}
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
		ext := strings.ToLower(filepath.Ext(f.rel))
		if ext == ".yaml" || ext == ".yml" {
			decls = workloads(f.lines)
		} else {
			decls = marker.Declarations(f.rel, f.lines, f.style)
		}
		for _, d := range decls {
			if marked[fmt.Sprintf("%s#%d", f.rel, d.Line)] {
				continue
			}
			role := roleWorkload
			if d.Kind != "workload" {
				var ok bool
				if role, ok = surface(d, f.lines, f.rel, f.test); !ok {
					continue
				}
			}
			p, ok := r.propose(f, d, role)
			if !ok || p.Conf < opt.MinConf {
				continue
			}
			if le := opt.Ledger[extract.MarkerID(f.rel, p.Relation, []string{p.Target}, d.Symbol, "symbol")]; (le != nil && le.Status == model.StatusRejected) ||
				rejected[link{d.Fingerprint, p.Relation, p.Target}] {
				skipped++
				continue
			}
			ps = append(ps, p)
		}
		ps = collapse(f, dedupeMembers(ps, decls))
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
	if out == nil {
		out = []Proposal{}
	}
	return out, skipped, nil
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

// propose chooses the target, relation and conf for one declaration.
func (r *runner) propose(f *file, d marker.Decl, role string) (Proposal, bool) {
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
	body := bodyOf(f.lines, d)
	consumer, producer := flow(d, body)
	var hits []hit
	if role != roleHeading && role != roleWorkload {
		hits = append(hits, r.m.namedInDoc(d.Symbol, d.Parent)...)
	}
	hits = append(hits, r.m.nameHits(sym)...)
	if role == rolePage || role == roleShell {
		// A component named Page takes its meaning from its route.
		hits = append(hits, r.m.nameHits(filepath.Base(filepath.Dir(f.rel)))...)
	}
	hits = append(hits, r.m.docHits(d.Doc)...)
	context := ""
	switch {
	case role == roleTest:
		hits = append(hits, r.callHits(body, lastPart(d.Symbol), strings.Join(f.lines, "\n"))...)
	case role != roleHeading && role != roleWorkload && role != roleTable:
		dh, ctx := r.dataHits(body)
		hits = append(hits, dh...)
		context = ctx
	}
	if consumer || producer {
		hits = append(hits, r.topicHits(d, body)...)
	}
	h, rel, tie, ok := r.choose(f, d, role, hits, consumer, producer)
	if !ok {
		return Proposal{}, false
	}
	target := h.c.curie
	if f.m != nil && f.m.DefaultNamespace != "" && strings.HasPrefix(target, f.m.DefaultNamespace+":") {
		target = "." + strings.TrimPrefix(target, f.m.DefaultNamespace+":")
	}
	basis := h.kind + "/" + role
	if tie {
		basis += "/tie"
	}
	p := Proposal{Path: f.rel, Line: d.Insert, Symbol: d.Symbol, Relation: rel, Target: target, Matched: h.matched(), Basis: basis, Context: context, Conf: confFor(basis)}
	if h.c.n.Deprecated {
		p.Deprecated, p.Note = true, deprecatedNote(h.c.n)
	}
	p.Marker = markerLine(f.style, d.Indent, p)
	return p, true
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
func (r *runner) choose(f *file, d marker.Decl, role string, hits []hit, consumer, producer bool) (hit, string, bool, bool) {
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
		// shows, not the engine's concept (ADR 0041).
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
		if role != roleTest && !f.test {
			// The surface rule overrides the role's relation: produces on
			// a document renderer, configures on an admin page editing a
			// constraint, presents on a surface for a process an engine
			// elsewhere implements (ADR 0041).
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
			best = hit{strong[0].c, strong[0].kind, strong[0].terms + "+core:" + best.c.curie}
		case len(impls) == 1:
			best = hit{impls[0].c, best.kind, best.terms + "+impl"}
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
				out = append(out, hit{c, evTopic, lit})
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
		return []hit{{c, evCalls, via[best]}}
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
		return []hit{{c, evData, e}}, ctx
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
		switch kind {
		case "Deployment", "StatefulSet", "DaemonSet", "CronJob", "Job", "Rollout", "Service", "KnativeService":
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
		out = append(out, marker.Decl{Symbol: kind + "/" + name, Kind: "workload", Line: first + 1, End: end, Insert: first + 1})
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
