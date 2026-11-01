// @gloss implements gmi:BackstageExport scope=file
// @gloss consumes gm:Graph, gm:Ledger, gm:Drift scope=file
// @gloss produces gm:View scope=file

// Package backstage writes the graph as the documents a developer portal
// reads: one self-contained JSON document per boundary (its concepts with
// their criticality, the code and tests behind them, its coverage, its open
// drift findings and the last review decisions under it) and one landscape
// index listing every boundary across repositories with its drift summary.
// The Backstage plugin under integrations/backstage is one consumer; a docs
// site plugin is another. The format is versioned (Format) and described by
// schemas/backstage-export.schema.json.
package backstage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/viewer"
)

// Format is the document format version. It changes only when a reader
// written against the previous one would misread a document.
const Format = 1

// Dir is the default output directory, relative to the repository root.
const Dir = "glossmark"

// LandscapeFile is the index document's file name inside Dir.
const LandscapeFile = "landscape.json"

// Document is one boundary: what a component's or system's tab shows.
type Document struct {
	Format    int    `json:"format"`
	Kind      string `json:"kind"` // "boundary"
	Generated string `json:"generated"`
	Generator string `json:"generator,omitempty"`
	// Repo is the repository the boundary's manifest lives in.
	Repo string `json:"repo"`
	// SourceURL is the repository's link template ({path}, {line}); every
	// Link and Finding carries its resolved url too, so a reader needs it
	// only for paths of its own.
	SourceURL string    `json:"sourceUrl,omitempty"`
	Boundary  Boundary  `json:"boundary"`
	Concepts  []Concept `json:"concepts"`
	Coverage  Coverage  `json:"coverage"`
	Findings  []Finding `json:"findings"`
	// Decisions are the last review decisions on links under the boundary,
	// newest first (at most MaxDecisions), with the gate class each had.
	Decisions []Decision `json:"decisions"`
}

// Boundary is the manifest's identity and governance.
type Boundary struct {
	CURIE string `json:"curie"`
	IRI   string `json:"iri"`
	// Slug is the file name stem (claimb:assessment → claimb-assessment).
	Slug  string `json:"slug"`
	Label string `json:"label,omitempty"`
	Level string `json:"level,omitempty"`
	Doc   string `json:"doc,omitempty"`
	// Path is the manifest's directory, "" at the repository root; in a
	// landscape it starts with the repository name.
	Path            string   `json:"path"`
	Owner           string   `json:"owner,omitempty"`
	Criticality     string   `json:"criticality,omitempty"`
	CriticalityFrom string   `json:"criticalityFrom,omitempty"`
	PartOf          []string `json:"partOf,omitempty"`
	Deprecated      bool     `json:"deprecated,omitempty"`
	ReplacedBy      string   `json:"replacedBy,omitempty"`
	RetireBy        string   `json:"retireBy,omitempty"`
}

// Concept is one concept the boundary relates to, with the code under the
// boundary that links to it.
type Concept struct {
	CURIE string `json:"curie"`
	IRI   string `json:"iri"`
	Label string `json:"label,omitempty"`
	Kind  string `json:"kind,omitempty"`
	Tier  string `json:"tier,omitempty"`
	Doc   string `json:"doc,omitempty"`
	// Relations are how the boundary relates to it (realizes, holds,
	// presents, hosts, configures, produces, consumes, publishes,
	// subscribes), sorted; Declared says the manifest states at least one
	// of them, Derived that the code under the boundary does.
	Relations   []string `json:"relations"`
	Declared    bool     `json:"declared"`
	Derived     bool     `json:"derived"`
	Criticality string   `json:"criticality,omitempty"`
	// Status is gloss export --status's word for the concept across the
	// repository (implemented, untested, unimplemented, orphan, planned,
	// proposed); "" for an imported concept.
	Status     string `json:"status,omitempty"`
	Deprecated bool   `json:"deprecated,omitempty"`
	ReplacedBy string `json:"replacedBy,omitempty"`
	Planned    bool   `json:"planned,omitempty"`
	PlanBy     string `json:"planBy,omitempty"`
	// Artifacts are the code under the boundary linked to the concept or
	// to a concept that realizes it; Verifiers the tests, anywhere.
	Artifacts []Link `json:"artifacts"`
	Verifiers []Link `json:"verifiers"`
	// Findings are the ids of the document's findings about this concept.
	Findings []string `json:"findings,omitempty"`
}

// Link is one artifact linked to a concept.
type Link struct {
	Relation string `json:"relation"`
	Path     string `json:"path"`
	Line     int    `json:"line,omitempty"`
	Symbol   string `json:"symbol,omitempty"`
	URL      string `json:"url,omitempty"`
	// Source is inline, manifest or inferred; Via names the realizing
	// concept the link came through when it is not this one.
	Source   string `json:"source,omitempty"`
	Via      string `json:"via,omitempty"`
	Proposed bool   `json:"proposed,omitempty"`
}

// Coverage is the boundary's share of the repository's coverage: the
// concepts it relates to that are in the coverage denominator (ADR 0013),
// how many have code and a test, and the files under it carrying an
// accepted link.
type Coverage struct {
	ConceptsInScope    int     `json:"conceptsInScope"`
	ConceptsCovered    int     `json:"conceptsCovered"`
	ConceptCoveragePct float64 `json:"conceptCoveragePct"`
	InScopeFiles       int     `json:"inScopeFiles"`
	CoveredFiles       int     `json:"coveredFiles"`
	CoveragePct        float64 `json:"coveragePct"`
}

// Finding is one open drift finding on the boundary, on code under it or
// on a concept it relates to.
type Finding struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Severity string   `json:"severity"`
	Boundary string   `json:"boundary,omitempty"`
	Relation string   `json:"relation,omitempty"`
	Concept  string   `json:"concept,omitempty"`
	Declared []string `json:"declared,omitempty"`
	Derived  []string `json:"derived,omitempty"`
	Msg      string   `json:"msg"`
	Path     string   `json:"path,omitempty"`
	Line     int      `json:"line,omitempty"`
	URL      string   `json:"url,omitempty"`
	By       string   `json:"by,omitempty"`
	Ticket   string   `json:"ticket,omitempty"`
	Waived   bool     `json:"waived,omitempty"`
	Waiver   string   `json:"waiver,omitempty"`
}

// Decision is one ledger row with a review decision on it.
type Decision struct {
	MarkerID   string   `json:"markerId"`
	Path       string   `json:"path"`
	URL        string   `json:"url,omitempty"`
	Relation   string   `json:"relation"`
	Targets    []string `json:"targets"`
	Status     string   `json:"status"`
	Actor      string   `json:"actor,omitempty"`
	Created    string   `json:"created,omitempty"`
	ReviewedBy string   `json:"reviewedBy,omitempty"`
	ReviewedAt string   `json:"reviewedAt,omitempty"`
	// Gate is the class the link had when it was accepted (AUTO, SOFT,
	// HARD), when the ledger recorded one.
	Gate         string `json:"gate,omitempty"`
	Rationale    string `json:"rationale,omitempty"`
	ReviewReason string `json:"reviewReason,omitempty"`
}

// MaxDecisions caps Document.Decisions.
const MaxDecisions = 20

// Landscape is the index: every boundary with where its document is and
// how it stands.
type Landscape struct {
	Format     int           `json:"format"`
	Kind       string        `json:"kind"` // "landscape"
	Generated  string        `json:"generated"`
	Generator  string        `json:"generator,omitempty"`
	Repos      []Repo        `json:"repos"`
	Boundaries []Summary     `json:"boundaries"`
	ContextMap []ContextEdge `json:"contextMap"`
	// Metrics are the repository's, or the union's, drift metrics.
	Metrics rollup.Metrics `json:"metrics"`
}

// Repo is one repository in the landscape.
type Repo struct {
	Name      string `json:"name"`
	SourceURL string `json:"sourceUrl,omitempty"`
	// Boundaries counts the repository's documents.
	Boundaries int `json:"boundaries"`
}

// Summary is one boundary's row in the landscape.
type Summary struct {
	CURIE string `json:"curie"`
	Slug  string `json:"slug"`
	// Document is the boundary document's file name, relative to the index.
	Document    string   `json:"document"`
	Repo        string   `json:"repo"`
	Label       string   `json:"label,omitempty"`
	Level       string   `json:"level,omitempty"`
	Path        string   `json:"path"`
	Owner       string   `json:"owner,omitempty"`
	Criticality string   `json:"criticality,omitempty"`
	PartOf      []string `json:"partOf,omitempty"`
	Deprecated  bool     `json:"deprecated,omitempty"`
	Concepts    int      `json:"concepts"`
	// Findings counts the document's open findings by kind.
	Findings map[string]int `json:"findings"`
	// Severity is the worst open finding: error, warning, info or ""
	// (clean).
	Severity string   `json:"severity,omitempty"`
	Coverage Coverage `json:"coverage"`
}

// ContextEdge is one implied depends-on between boundaries (consumer →
// producer), as gloss landscape prints it.
type ContextEdge struct {
	From string   `json:"from"`
	To   string   `json:"to"`
	Via  []string `json:"via,omitempty"`
}

// Options tunes Build.
type Options struct {
	// Repo names the repository; in a landscape it is empty and each
	// boundary's repository is the first segment of its manifest path.
	Repo string
	// Landscape says paths carry a repository prefix (gloss landscape).
	Landscape bool
	// SourceURLs maps a repository name to its link template.
	SourceURLs map[string]string
	// Ledger holds the review decisions; in a landscape the paths carry
	// the repository prefix.
	Ledger    map[string]*model.LedgerEntry
	Now       time.Time
	Generator string
	// ContextMap is the landscape's context-map edges, when known.
	ContextMap []ContextEdge
}

// Build makes one document per manifest whose boundary is in the graph,
// sorted by CURIE, and the landscape that indexes them. rep is the drift
// report of the same graph.
func Build(g *graph.Graph, ws *load.Workspace, rep *rollup.Report, opt Options) ([]*Document, *Landscape) {
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	b := &builder{g: g, ws: ws, rep: rep, opt: opt, now: opt.Now.UTC().Format(time.RFC3339)}
	b.details = viewer.Details(g, rep)
	b.status = map[string]rollup.ConceptStatusRow{}
	for _, r := range rollup.ConceptStatus(g, rep) {
		b.status[r.Concept] = r
	}
	b.accepted = b.acceptedFiles()
	// By CURIE from the nodes, not Expand: in a landscape a boundary prefix
	// of another repository is not among the merged workspace's prefixes.
	byCurie := map[string]*graph.Node{}
	for _, n := range g.NodesOf(model.NodeBoundary) {
		byCurie[n.CURIE] = n
	}
	slugs := map[string]int{}
	var docs []*Document
	manifests := append([]*model.Manifest{}, ws.Manifests...)
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].Boundary < manifests[j].Boundary })
	for _, m := range manifests {
		n := byCurie[m.Boundary]
		if n == nil {
			continue
		}
		d := b.document(m, n)
		// Two boundaries whose CURIEs slug alike (a:b and a-b) get numbered.
		slugs[d.Boundary.Slug]++
		if k := slugs[d.Boundary.Slug]; k > 1 {
			d.Boundary.Slug = fmt.Sprintf("%s-%d", d.Boundary.Slug, k)
		}
		docs = append(docs, d)
	}
	return docs, b.landscape(docs)
}

type builder struct {
	g        *graph.Graph
	ws       *load.Workspace
	rep      *rollup.Report
	opt      Options
	now      string
	details  map[string]*viewer.ConceptDetail
	status   map[string]rollup.ConceptStatusRow
	accepted map[string]bool // files carrying an accepted link
}

// Slug is the file name stem of a boundary CURIE: lower case, every run of
// characters outside [a-z0-9] one hyphen.
func Slug(curie string) string {
	var sb strings.Builder
	dash := false
	for _, r := range strings.ToLower(curie) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			sb.WriteRune(r)
			dash = false
		} else if !dash && sb.Len() > 0 {
			sb.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(sb.String(), "-")
}

// repoOf names the repository a manifest belongs to.
func (b *builder) repoOf(m *model.Manifest) string {
	if b.opt.Landscape {
		if i := strings.Index(m.Path, "/"); i >= 0 {
			return m.Path[:i]
		}
		return m.Path
	}
	return b.opt.Repo
}

// url renders the source-url template for a path; in a landscape the
// path's first segment picks the repository and is stripped.
func (b *builder) url(path string, line int) string {
	if path == "" {
		return ""
	}
	repo := b.opt.Repo
	if b.opt.Landscape {
		i := strings.Index(path, "/")
		if i < 0 {
			return ""
		}
		repo, path = path[:i], path[i+1:]
	}
	tpl := b.opt.SourceURLs[repo]
	if tpl == "" || !strings.HasPrefix(tpl, "http") {
		return ""
	}
	u := strings.ReplaceAll(tpl, "{path}", path)
	if line > 0 {
		return strings.ReplaceAll(u, "{line}", fmt.Sprint(line))
	}
	if i := strings.Index(u, "#"); i >= 0 && strings.Contains(u[i:], "{line}") {
		u = u[:i]
	}
	return strings.ReplaceAll(u, "{line}", "")
}

// under reports whether a path's nearest manifest is m.
func (b *builder) under(path string, m *model.Manifest) bool {
	return path != "" && b.ws.ManifestFor(path) == m
}

func (b *builder) document(m *model.Manifest, n *graph.Node) *Document {
	g := b.g
	repo := b.repoOf(m)
	d := &Document{Format: Format, Kind: "boundary", Generated: b.now, Generator: b.opt.Generator, Repo: repo, SourceURL: b.opt.SourceURLs[repo]}
	d.Boundary = Boundary{CURIE: n.CURIE, IRI: n.ID, Slug: Slug(n.CURIE), Label: n.Label, Level: m.Level, Doc: m.Doc, Path: m.Dir, Owner: n.Owner,
		Criticality: n.Criticality, CriticalityFrom: n.CriticalityFrom, Deprecated: m.Deprecated, ReplacedBy: m.ReplacedBy, RetireBy: m.RetireBy}
	if d.Boundary.Label == "" {
		d.Boundary.Label = m.Label
	}
	if d.Boundary.Owner == "" {
		d.Boundary.Owner = m.Owner
	}
	for _, o := range g.Objects(n.ID, "part-of") {
		d.Boundary.PartOf = append(d.Boundary.PartOf, g.Curie(o))
	}
	sort.Strings(d.Boundary.PartOf)
	// The concepts: every compared relation from the boundary node, declared
	// by the manifest or derived from the code beneath it.
	type rel struct {
		rels              map[string]bool
		declared, derived bool
	}
	seen := map[string]*rel{}
	for _, r := range rollup.ComparedRelations {
		for _, e := range g.Out(n.ID, r) {
			c := g.Node(e.O)
			if c == nil || c.NodeType != model.NodeConcept {
				continue
			}
			x := seen[e.O]
			if x == nil {
				x = &rel{rels: map[string]bool{}}
				seen[e.O] = x
			}
			x.rels[r] = true
			if e.Source == graph.SourceDeclared {
				x.declared = true
			} else {
				x.derived = true
			}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return g.Curie(ids[i]) < g.Curie(ids[j]) })
	findings := b.findings(m, n, ids)
	d.Findings = findings
	byConcept := map[string][]string{}
	for _, f := range findings {
		if f.Concept != "" {
			byConcept[f.Concept] = append(byConcept[f.Concept], f.ID)
		}
	}
	d.Concepts = []Concept{}
	for _, id := range ids {
		cn := g.Node(id)
		x := seen[id]
		c := Concept{CURIE: cn.CURIE, IRI: cn.ID, Label: cn.Label, Kind: cn.Kind, Tier: cn.Tier, Doc: cn.Doc, Declared: x.declared, Derived: x.derived,
			Criticality: cn.Criticality, Deprecated: cn.Deprecated, ReplacedBy: cn.ReplacedBy, Planned: cn.Planned, PlanBy: cn.PlanBy,
			Artifacts: []Link{}, Verifiers: []Link{}, Findings: byConcept[cn.CURIE]}
		for r := range x.rels {
			c.Relations = append(c.Relations, r)
		}
		sort.Strings(c.Relations)
		if st, ok := b.status[cn.CURIE]; ok {
			c.Status = st.Status
		}
		if det := b.details[cn.CURIE]; det != nil {
			for _, l := range det.Artifacts {
				if b.under(l.Path, m) {
					c.Artifacts = append(c.Artifacts, b.link(l))
				}
			}
			for _, l := range det.Verifiers {
				c.Verifiers = append(c.Verifiers, b.link(l))
			}
		}
		d.Concepts = append(d.Concepts, c)
	}
	d.Coverage = b.coverage(m, ids)
	d.Decisions = b.decisions(m)
	return d
}

func (b *builder) link(l viewer.Link) Link {
	return Link{Relation: l.Relation, Path: l.Path, Line: l.Line, Symbol: l.Symbol, URL: b.url(l.Path, l.Line), Source: l.Source, Via: l.Via, Proposed: l.Proposed}
}

// findings selects the report's open findings for a boundary: those on the
// boundary itself, on a path under it, or (when they name no boundary) on
// a concept it relates to.
func (b *builder) findings(m *model.Manifest, n *graph.Node, ids []string) []Finding {
	concepts := map[string]bool{}
	for _, id := range ids {
		concepts[b.g.Curie(id)] = true
	}
	out := []Finding{}
	if b.rep == nil {
		return out
	}
	for _, f := range b.rep.Findings {
		if f.Kind == rollup.KindConsistent {
			continue
		}
		switch {
		case f.Boundary == n.CURIE:
		case f.Boundary == "" && b.under(f.Path, m):
		case f.Boundary == "" && f.Concept != "" && concepts[f.Concept]:
		default:
			continue
		}
		out = append(out, Finding{ID: f.ID, Kind: f.Kind, Severity: f.Severity, Boundary: f.Boundary, Relation: f.Relation, Concept: f.Concept,
			Declared: f.Declared, Derived: f.Derived, Msg: f.Msg, Path: f.Path, Line: f.Line, URL: b.url(f.Path, f.Line), By: f.By, Ticket: f.Ticket, Waived: f.Waived, Waiver: f.Waiver})
	}
	return out
}

// acceptedFiles lists the paths carrying a link that is accepted: not
// proposed, and not stale or rejected in the ledger.
func (b *builder) acceptedFiles() map[string]bool {
	out := map[string]bool{}
	for _, e := range b.g.Edges() {
		if !graph.LinkSource(e.Source) || e.Prov == nil || e.Prov.Proposed {
			continue
		}
		if le, ok := b.opt.Ledger[e.Prov.ID]; ok && le.Status != "" && le.Status != model.StatusAccepted && le.Status != model.StatusRemoved {
			continue
		}
		out[e.Prov.Path] = true
	}
	return out
}

// coverage scores the boundary: concepts in the coverage denominator that
// it relates to, covered when code reaches them and a test verifies them,
// and the files under it with an accepted link.
func (b *builder) coverage(m *model.Manifest, ids []string) Coverage {
	var c Coverage
	for _, id := range ids {
		n := b.g.Node(id)
		if n.Imported || n.Deprecated || n.Proposed || n.Planned || rollup.InternalExempt(n) || n.Tier != model.TierImpl && !b.implementedDirectly(id) {
			continue
		}
		c.ConceptsInScope++
		if b.status[n.CURIE].Status == rollup.StatusImplemented {
			c.ConceptsCovered++
		}
	}
	if c.ConceptsInScope > 0 {
		c.ConceptCoveragePct = 100 * float64(c.ConceptsCovered) / float64(c.ConceptsInScope)
	}
	for _, f := range b.ws.Files {
		if !b.under(f, m) {
			continue
		}
		c.InScopeFiles++
		if b.accepted[f] {
			c.CoveredFiles++
		}
	}
	if c.InScopeFiles > 0 {
		c.CoveragePct = 100 * float64(c.CoveredFiles) / float64(c.InScopeFiles)
	}
	return c
}

func (b *builder) implementedDirectly(id string) bool {
	for _, e := range b.g.In(id, "implements") {
		if a := b.g.Node(e.S); a != nil && a.NodeType == model.NodeArtifact && graph.LinkSource(e.Source) {
			return true
		}
	}
	return false
}

// decisions lists the reviewed ledger rows under the boundary, newest first.
func (b *builder) decisions(m *model.Manifest) []Decision {
	out := []Decision{}
	for _, le := range b.opt.Ledger {
		if le.ReviewedAt == "" || !b.under(le.Path, m) {
			continue
		}
		switch le.Status {
		case model.StatusAccepted, model.StatusRejected, model.StatusStale:
		default:
			continue
		}
		out = append(out, Decision{MarkerID: le.MarkerID, Path: le.Path, URL: b.url(le.Path, 0), Relation: le.Relation, Targets: le.Targets, Status: le.Status,
			Actor: le.Actor, Created: le.Created, ReviewedBy: le.ReviewedBy, ReviewedAt: le.ReviewedAt, Gate: le.Gate, Rationale: le.Rationale, ReviewReason: le.ReviewReason})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ReviewedAt != out[j].ReviewedAt {
			return out[i].ReviewedAt > out[j].ReviewedAt
		}
		return out[i].MarkerID < out[j].MarkerID
	})
	if len(out) > MaxDecisions {
		out = out[:MaxDecisions]
	}
	return out
}

func (b *builder) landscape(docs []*Document) *Landscape {
	l := &Landscape{Format: Format, Kind: "landscape", Generated: b.now, Generator: b.opt.Generator, Repos: []Repo{}, Boundaries: []Summary{}, ContextMap: b.opt.ContextMap}
	if l.ContextMap == nil {
		l.ContextMap = []ContextEdge{}
	}
	if b.rep != nil {
		l.Metrics = b.rep.Metrics
	}
	repos := map[string]*Repo{}
	for _, d := range docs {
		r := repos[d.Repo]
		if r == nil {
			r = &Repo{Name: d.Repo, SourceURL: b.opt.SourceURLs[d.Repo]}
			repos[d.Repo] = r
		}
		r.Boundaries++
		s := Summary{CURIE: d.Boundary.CURIE, Slug: d.Boundary.Slug, Document: d.Boundary.Slug + ".json", Repo: d.Repo, Label: d.Boundary.Label, Level: d.Boundary.Level,
			Path: d.Boundary.Path, Owner: d.Boundary.Owner, Criticality: d.Boundary.Criticality, PartOf: d.Boundary.PartOf, Deprecated: d.Boundary.Deprecated,
			Concepts: len(d.Concepts), Findings: map[string]int{}, Coverage: d.Coverage}
		for _, f := range d.Findings {
			if f.Waived {
				continue
			}
			s.Findings[f.Kind]++
			if severityRank[f.Severity] > severityRank[s.Severity] {
				s.Severity = f.Severity
			}
		}
		l.Boundaries = append(l.Boundaries, s)
	}
	for _, name := range sortedKeys(repos) {
		l.Repos = append(l.Repos, *repos[name])
	}
	return l
}

var severityRank = map[string]int{"info": 1, "warning": 2, "error": 3}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Write writes every document as <dir>/<slug>.json and the landscape as
// <dir>/landscape.json, creating dir. Documents of boundaries that no
// longer exist are not removed: the directory is the caller's.
func Write(dir string, docs []*Document, l *Landscape) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, d := range docs {
		if err := writeJSON(filepath.Join(dir, d.Boundary.Slug+".json"), d); err != nil {
			return err
		}
	}
	return writeJSON(filepath.Join(dir, LandscapeFile), l)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
