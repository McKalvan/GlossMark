// @gloss implements gmi:AgentBenchmark scope=file

package bench

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// Score is what the scorer says about one agent run: the gate's verdict,
// what the linter and the drift report found that the base did not, every
// concept and link the run added, and each assertion's result. Mistakes
// counts the failed assertions per category; it is the headline.
type Score struct {
	Task      string `json:"task"`
	Repo      string `json:"repo,omitempty"`
	Condition string `json:"condition"`
	Run       int    `json:"run,omitempty"`
	Agent     string `json:"agent,omitempty"`
	// Gate is gloss diff's class for the whole change; HardChanges lists
	// the HARD ones with their reasons.
	Gate        gate.Class `json:"gate"`
	HardChanges []string   `json:"hard_changes,omitempty"`
	// CheckErrors and CheckWarnings are the linter's diagnostics on the head.
	CheckErrors   []string `json:"check_errors,omitempty"`
	CheckWarnings []string `json:"check_warnings,omitempty"`
	// NewFindings are drift findings in the head the base did not have
	// (stale, untested and consistent left out); ClosedFindings the reverse.
	NewFindings    []string `json:"new_findings,omitempty"`
	ClosedFindings []string `json:"closed_findings,omitempty"`
	// NewConcepts are concepts the head defines or names that the base did
	// not, with how they arrived and the base concept each duplicates.
	NewConcepts []NewConcept `json:"new_concepts,omitempty"`
	// NewLinks are inline markers the head has that the base did not.
	NewLinks     []string `json:"new_links,omitempty"`
	ChangedFiles []string `json:"changed_files,omitempty"`
	Results      []Result `json:"results"`
	Mistakes     Mistakes `json:"mistakes"`
	Passed       int      `json:"passed"`
	Total        int      `json:"total"`
}

// NewConcept is a concept the run introduced. State is "added" (written
// into an ontology file without proposed: true), "proposed" (with it) or
// "rogue" (named in a ns:NEW: marker and not yet in the ontology).
type NewConcept struct {
	Curie       string `json:"curie"`
	State       string `json:"state"`
	DuplicateOf string `json:"duplicate_of,omitempty"`
}

// Result is one assertion's outcome.
type Result struct {
	Kind     string `json:"kind"`
	Category string `json:"category"`
	OK       bool   `json:"ok"`
	Msg      string `json:"msg"`
	Why      string `json:"why,omitempty"`
}

// Mistakes counts the failed assertions per category. Gate assertions are
// expectations about the gate, not domain mistakes, so they are counted
// apart and left out of Total.
type Mistakes struct {
	Rule    int `json:"rule"`
	Concept int `json:"concept"`
	Naming  int `json:"naming"`
	Gate    int `json:"gate"`
	Total   int `json:"total"`
}

// tree is one workspace, built the way gloss build does, with its
// diagnostics and drift report.
type tree struct {
	Root     string
	WS       *load.Workspace
	Graph    *graph.Graph
	Ledger   *ledger.Ledger
	Diags    []marker.Diagnostic
	Report   *rollup.Report
	Findings map[string]rollup.Finding
}

// buildTree runs load → extract → derive → ledger sync → check → compare on
// a root. The ledger is synced into a scratch file, never the workspace's
// own, so a marker the run edited is not reported stale against a review
// row the run never saw.
func buildTree(root string) (*tree, error) {
	ws, err := load.Load(root)
	if err != nil {
		return nil, fmt.Errorf("load %s: %v", root, err)
	}
	res, err := extract.Build(ws)
	if err != nil {
		return nil, fmt.Errorf("extract %s: %v", root, err)
	}
	g := res.Graph
	rollup.Derive(g, ws.Vocab)
	scratch, err := os.MkdirTemp("", "gloss-bench-ledger")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scratch)
	lg, err := ledger.Open(filepath.Join(scratch, ledger.FileName))
	if err != nil {
		return nil, err
	}
	lg.Sync(g, res.Markers, "agent:bench", time.Now().UTC())
	t := &tree{Root: root, WS: ws, Graph: g, Ledger: lg}
	t.Diags = append(append([]marker.Diagnostic{}, ws.Diags...), res.Diags...)
	t.Diags = append(t.Diags, check.Run(ws, g)...)
	pol := ws.Config.EffectivePolicy()
	t.Report = rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: pol, Now: time.Now()})
	rollup.AddDeprecatedUse(t.Report, g, pol, time.Now())
	t.Findings = map[string]rollup.Finding{}
	for _, f := range t.Report.Findings {
		t.Findings[f.ID] = f
	}
	return t, nil
}

// Options for Score.
type Options struct {
	Condition string
	Run       int
	Agent     string
}

// ScoreDirs scores head, the tree an agent left, against base, the tree it
// started from, for one task.
func ScoreDirs(task *Task, base, head string, opt Options) (*Score, error) {
	b, err := buildTree(base)
	if err != nil {
		return nil, fmt.Errorf("base: %v", err)
	}
	h, err := buildTree(head)
	if err != nil {
		return nil, fmt.Errorf("head: %v", err)
	}
	changed, err := changedFiles(base, head)
	if err != nil {
		return nil, err
	}
	return score(task, b, h, changed, opt), nil
}

// score is ScoreDirs after both trees are built.
func score(task *Task, b, h *tree, changed []string, opt Options) *Score {
	s := &Score{Task: task.ID, Repo: task.Repo, Condition: opt.Condition, Run: opt.Run, Agent: opt.Agent, ChangedFiles: changed}
	pol := h.WS.Config.EffectivePolicy()
	im := gate.Classify(b.Graph, h.Graph, h.WS, gate.Options{
		Base: "base", Policy: pol, HeadLedger: h.Ledger.Entries, ChangedFiles: changed,
		BeforeReport: b.Report, AfterReport: h.Report, BaseViews: b.WS.Views,
		BaseHardPaths: b.WS.Config.EffectivePolicy().HardPaths,
	})
	s.Gate = im.Gate
	for _, c := range im.Changes {
		if c.Gate == gate.HARD {
			s.HardChanges = append(s.HardChanges, fmt.Sprintf("%s %s (%s)", c.Kind, c.Msg, strings.Join(c.Reasons, "; ")))
		}
		// The drift report baselines a published value in the ledger, which
		// a scratch ledger does not have; the gate compares the two graphs
		// and sees the drift, so it is scored as the finding it would be.
		if c.Kind == "published-value-drifted" {
			id := rollup.KindValueDrift + ":" + c.Subject
			if _, ok := h.Findings[id]; !ok {
				h.Findings[id] = rollup.Finding{ID: id, Kind: rollup.KindValueDrift, Severity: "error", Concept: c.Subject, Path: c.Path, Msg: c.Msg}
			}
		}
	}
	for _, d := range h.Diags {
		switch d.Severity {
		case "error":
			s.CheckErrors = append(s.CheckErrors, d.String())
		case "warning":
			s.CheckWarnings = append(s.CheckWarnings, d.String())
		}
	}
	s.NewFindings, s.ClosedFindings = findingDelta(b, h)
	s.NewConcepts = conceptDelta(b, h)
	s.NewLinks = linkDelta(b, h)

	ev := &evaluator{task: task, base: b, head: h, score: s, changed: changed}
	for _, a := range task.Effective() {
		r := ev.eval(a)
		r.Kind, r.Category, r.Why = a.Kind, Category(a.Kind), a.Why
		s.Results = append(s.Results, r)
		s.Total++
		if r.OK {
			s.Passed++
			continue
		}
		switch r.Category {
		case CategoryRule:
			s.Mistakes.Rule++
		case CategoryConcept:
			s.Mistakes.Concept++
		case CategoryNaming:
			s.Mistakes.Naming++
		case CategoryGate:
			s.Mistakes.Gate++
		}
	}
	s.Mistakes.Total = s.Mistakes.Rule + s.Mistakes.Concept + s.Mistakes.Naming
	return s
}

// scoredKinds are the drift kinds a run is held to: what the base did not
// have and the head does is a mistake. Stale and untested are left out
// (any edit stales a scope=file link; a new mechanism without a test is a
// different benchmark), consistent and the plan kinds are not findings.
var scoredKinds = map[string]bool{
	rollup.KindViolation: true, rollup.KindValueDrift: true, rollup.KindDangling: true,
	rollup.KindUnguarded: true, rollup.KindOverlap: true, rollup.KindDeprecatedUse: true,
}

func findingDelta(b, h *tree) (added, closed []string) {
	for id, f := range h.Findings {
		if _, ok := b.Findings[id]; !ok && scoredKinds[f.Kind] && !f.Waived {
			added = append(added, f.Msg)
		}
	}
	for id, f := range b.Findings {
		if _, ok := h.Findings[id]; !ok && f.Kind != rollup.KindConsistent && f.Kind != rollup.KindStale {
			closed = append(closed, id)
		}
	}
	sort.Strings(added)
	sort.Strings(closed)
	return added, closed
}

// normName folds a concept's local name or alias so "Police Report",
// "police_report" and "PoliceReport" are one name.
func normName(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func localName(curie string) string {
	if i := strings.LastIndex(curie, ":"); i >= 0 {
		return curie[i+1:]
	}
	return curie
}

// names maps every normalized name and alias of the concepts in g to the
// CURIE that owns it.
func names(g *graph.Graph) map[string]string {
	m := map[string]string{}
	for _, n := range g.NodesOf(model.NodeConcept) {
		for _, s := range append([]string{localName(n.CURIE), n.Label}, n.Aliases...) {
			if k := normName(s); k != "" {
				if _, ok := m[k]; !ok {
					m[k] = n.CURIE
				}
			}
		}
	}
	return m
}

func conceptDelta(b, h *tree) []NewConcept {
	base := map[string]bool{}
	for _, n := range b.Graph.NodesOf(model.NodeConcept) {
		base[n.CURIE] = true
	}
	// A rogue the base already had is the repository's open question, not
	// the run's invention.
	for _, f := range b.Report.Findings {
		if f.Kind == rollup.KindRogue {
			base[rollup.StripNewCurie(f.Concept)] = true
		}
	}
	known := names(b.Graph)
	var out []NewConcept
	seen := map[string]bool{}
	add := func(curie, state string, aliases []string) {
		if seen[curie] {
			return
		}
		seen[curie] = true
		nc := NewConcept{Curie: curie, State: state}
		for _, s := range append([]string{localName(curie)}, aliases...) {
			if dup, ok := known[normName(s)]; ok && dup != curie {
				nc.DuplicateOf = dup
				break
			}
		}
		out = append(out, nc)
	}
	for _, n := range h.Graph.NodesOf(model.NodeConcept) {
		if base[n.CURIE] || n.Imported {
			continue
		}
		state := "added"
		if n.Proposed {
			state = "proposed"
		}
		add(n.CURIE, state, n.Aliases)
	}
	for _, f := range h.Report.Findings {
		if cur := rollup.StripNewCurie(f.Concept); f.Kind == rollup.KindRogue && !base[cur] {
			add(cur, "rogue", nil)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Curie < out[j].Curie })
	return out
}

// inlineLinks lists a tree's inline markers as "path relation target".
func inlineLinks(t *tree) map[string]*graph.Edge {
	m := map[string]*graph.Edge{}
	for _, e := range t.Graph.Edges() {
		if e.Source != graph.SourceInline || e.Prov == nil {
			continue
		}
		m[fmt.Sprintf("%s %s %s", e.Prov.Path, e.P, t.Graph.Curie(e.O))] = e
	}
	return m
}

func linkDelta(b, h *tree) []string {
	base := inlineLinks(b)
	var out []string
	for k := range inlineLinks(h) {
		if _, ok := base[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// changedFiles lists the repo-relative paths whose content differs between
// the two roots, added and removed ones included. Git state, the built
// graph and the directories the loader skips are left out.
func changedFiles(base, head string) ([]string, error) {
	hb, err := hashTree(base)
	if err != nil {
		return nil, err
	}
	hh, err := hashTree(head)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for p, d := range hh {
		if !bytes.Equal(hb[p], d) {
			set[p] = true
		}
	}
	for p := range hb {
		if _, ok := hh[p]; !ok {
			set[p] = true
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

func hashTree(root string) (map[string][]byte, error) {
	m := map[string][]byte{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if p != root && (load.SkipDir(d.Name()) || d.Name() == ".glossmark") {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		m[rel] = b
		return nil
	})
	return m, err
}

// evaluator checks one assertion at a time against the scored trees.
type evaluator struct {
	task       *Task
	base, head *tree
	score      *Score
	changed    []string
}

func (ev *evaluator) eval(a Assertion) Result {
	switch a.Kind {
	case "check-clean":
		return ev.checkClean(a)
	case "no-new-finding":
		return ev.noNewFinding(a)
	case "finding-closed":
		if _, was := ev.base.Findings[a.ID]; !was {
			return fail("%s is not a finding of the base", a.ID)
		}
		if f, still := ev.head.Findings[a.ID]; still {
			return fail("still open: %s", f.Msg)
		}
		return ok("%s closed", a.ID)
	case "file-matches":
		b, err := os.ReadFile(filepath.Join(ev.head.Root, filepath.FromSlash(a.Path)))
		if err != nil {
			return fail("%s: %v", a.Path, err)
		}
		if !regexp.MustCompile(a.Pattern).Match(b) {
			return fail("%s does not match %q", a.Path, a.Pattern)
		}
		return ok("%s matches %q", a.Path, a.Pattern)
	case "file-unchanged":
		if ev.isChanged(a.Path) {
			return fail("%s was changed", a.Path)
		}
		return ok("%s unchanged", a.Path)
	case "file-changed":
		if !ev.isChanged(a.Path) {
			return fail("%s was not changed", a.Path)
		}
		return ok("%s changed", a.Path)
	case "concept-value":
		n := ev.head.Graph.NodeByCurie(a.Concept)
		if n == nil {
			return fail("%s is not in the head ontology", a.Concept)
		}
		v := fmt.Sprint(n.Value)
		if !regexp.MustCompile(a.Pattern).MatchString(v) {
			return fail("%s value %q does not match %q", a.Concept, v, a.Pattern)
		}
		return ok("%s value %q", a.Concept, v)
	case "no-unproposed-concept":
		return ev.newConcepts(a, func(c NewConcept) bool { return c.State == "added" }, "added to an ontology file without proposed: true")
	case "no-duplicate-concept":
		return ev.newConcepts(a, func(c NewConcept) bool { return c.DuplicateOf != "" }, "duplicates an existing concept")
	case "no-new-concept":
		return ev.newConcepts(a, func(NewConcept) bool { return true }, "new, and the ticket needs no new concept")
	case "link":
		if hits := ev.links(a, ev.head); len(hits) > 0 {
			return ok("%s", strings.Join(hits, "; "))
		}
		return fail("no inline marker %s %s%s", orAny(a.Relation), a.Concept, underMsg(a.Under))
	case "no-link":
		base := map[string]bool{}
		for _, hit := range ev.links(a, ev.base) {
			base[hit] = true
		}
		var added []string
		for _, hit := range ev.links(a, ev.head) {
			if !base[hit] {
				added = append(added, hit)
			}
		}
		if len(added) > 0 {
			return fail("links %s: %s", a.Concept, strings.Join(added, "; "))
		}
		return ok("nothing new links %s%s", a.Concept, underMsg(a.Under))
	case "forbidden-terms":
		return ev.forbiddenTerms(a)
	case "gate":
		if a.Is != "" && string(ev.score.Gate) != a.Is {
			return fail("gate %s, expected %s", ev.score.Gate, a.Is)
		}
		if a.Max != "" && gate.Max(ev.score.Gate, gate.Class(a.Max)) != gate.Class(a.Max) {
			return fail("gate %s, at most %s expected: %s", ev.score.Gate, a.Max, strings.Join(ev.score.HardChanges, "; "))
		}
		return ok("gate %s", ev.score.Gate)
	}
	return fail("unknown assertion kind %q", a.Kind)
}

func ok(format string, a ...any) Result   { return Result{OK: true, Msg: fmt.Sprintf(format, a...)} }
func fail(format string, a ...any) Result { return Result{OK: false, Msg: fmt.Sprintf(format, a...)} }

func orAny(rel string) string {
	if rel == "" {
		return "<any relation>"
	}
	return rel
}

func underMsg(under string) string {
	if under == "" {
		return ""
	}
	return " under " + under
}

func (ev *evaluator) isChanged(path string) bool {
	for _, c := range ev.changed {
		if c == path {
			return true
		}
	}
	return false
}

func (ev *evaluator) checkClean(a Assertion) Result {
	if n := len(ev.score.CheckErrors); n > 0 {
		return fail("%d check error(s): %s", n, strings.Join(ev.score.CheckErrors, "; "))
	}
	if a.Warnings && len(ev.score.CheckWarnings) > 0 {
		return fail("%d check warning(s): %s", len(ev.score.CheckWarnings), strings.Join(ev.score.CheckWarnings, "; "))
	}
	return ok("gloss check clean")
}

func (ev *evaluator) noNewFinding(a Assertion) Result {
	kinds := map[string]bool{}
	for _, k := range a.Kinds {
		kinds[k] = true
	}
	var bad []string
	for id, f := range ev.head.Findings {
		if _, was := ev.base.Findings[id]; was || f.Waived {
			continue
		}
		if len(kinds) > 0 && !kinds[f.Kind] || len(kinds) == 0 && !scoredKinds[f.Kind] {
			continue
		}
		if a.Boundary != "" && f.Boundary != a.Boundary || a.Concept != "" && f.Concept != a.Concept {
			continue
		}
		bad = append(bad, f.Kind+": "+f.Msg)
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return fail("%s", strings.Join(bad, "; "))
	}
	return ok("no new drift finding")
}

func (ev *evaluator) newConcepts(a Assertion, bad func(NewConcept) bool, what string) Result {
	allow := map[string]bool{}
	for _, c := range a.Allow {
		allow[c] = true
	}
	var hits []string
	for _, c := range ev.score.NewConcepts {
		if allow[c.Curie] || !bad(c) {
			continue
		}
		s := c.Curie + " (" + c.State
		if c.DuplicateOf != "" {
			s += ", duplicates " + c.DuplicateOf
		}
		hits = append(hits, s+")")
	}
	if len(hits) > 0 {
		return fail("%s: %s", what, strings.Join(hits, "; "))
	}
	return ok("no concept %s", what)
}

// links lists the inline markers in t that satisfy a link assertion.
func (ev *evaluator) links(a Assertion, t *tree) []string {
	var hits []string
	for k, e := range inlineLinks(t) {
		if a.Relation != "" && e.P != a.Relation || t.Graph.Curie(e.O) != a.Concept {
			continue
		}
		if a.Under != "" && !strings.HasPrefix(e.Prov.Path, strings.TrimSuffix(a.Under, "/")+"/") {
			continue
		}
		hits = append(hits, k)
	}
	sort.Strings(hits)
	return hits
}

// forbiddenTerms scans what the run named (new concepts, their aliases, the
// targets of new markers) and, when Paths is set, the changed files that
// match, for the terms the task forbids.
func (ev *evaluator) forbiddenTerms(a Assertion) Result {
	var surfaces []string
	for _, c := range ev.score.NewConcepts {
		surfaces = append(surfaces, "concept "+c.Curie)
		if n := ev.head.Graph.NodeByCurie(c.Curie); n != nil {
			for _, al := range n.Aliases {
				surfaces = append(surfaces, "alias "+al+" of "+c.Curie)
			}
		}
	}
	for _, l := range ev.score.NewLinks {
		surfaces = append(surfaces, "marker "+l)
	}
	if len(a.Paths) > 0 {
		for _, p := range ev.changed {
			if !model.MatchAnyGlob(a.Paths, p) {
				continue
			}
			if b, err := os.ReadFile(filepath.Join(ev.head.Root, filepath.FromSlash(p))); err == nil {
				surfaces = append(surfaces, "file "+p+": "+string(b))
			}
		}
	}
	var hits []string
	for _, term := range a.Terms {
		re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(term))
		for _, s := range surfaces {
			if re.MatchString(s) {
				where := s
				if i := strings.Index(s, ": "); i > 0 && strings.HasPrefix(s, "file ") {
					where = s[:i]
				}
				hits = append(hits, fmt.Sprintf("%q in %s", term, where))
				break
			}
		}
	}
	if len(hits) > 0 {
		return fail("%s", strings.Join(hits, "; "))
	}
	return ok("none of %s names anything new", strings.Join(a.Terms, ", "))
}
