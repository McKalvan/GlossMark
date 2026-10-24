// @gloss verifies gm:PlanItem scope=file

package e2e

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/ontoedit"
	"github.com/mckalvan/glossmark/internal/review"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// driftDefaults are gloss drift's default --fail-on kinds.
const driftDefaults = "violation,value-drift,gap,dangling,overdue,planned-closed,plan-overdue"

// failingIDs returns the ids of the findings that fail gloss drift by default.
func failingIDs(t *testing.T, rep *rollup.Report) []string {
	t.Helper()
	kinds, err := rollup.ParseFailOn(driftDefaults)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, f := range rep.Failing(kinds) {
		ids = append(ids, f.ID)
	}
	sort.Strings(ids)
	return ids
}

func checkClean(t *testing.T, ws *load.Workspace, g *graph.Graph, step string) {
	t.Helper()
	for _, d := range check.Run(ws, g) {
		if d.Severity == "error" {
			t.Errorf("%s: gloss check: %s", step, d)
		}
	}
}

func compareNow(g *graph.Graph, ws *load.Workspace, now time.Time) *rollup.Report {
	return rollup.Compare(g, ws, rollup.Options{Policy: ws.Config.EffectivePolicy(), Now: now})
}

// TestPlannedIncrement is ADR 0034 P1's done-when, on the library example's
// circulation repo: an ontology-only change plans lib:Renewal (long form)
// and a planned realizes on the circulation manifest; it passes gloss check,
// adds no failing drift finding and opens plan items. A verifies-only
// marker keeps them open; an implements marker reports planned-closed;
// gloss accept --planned clears the concept's item and gloss diff classifies
// that edit AUTO, hard-paths included; a by in the past is plan-overdue.
func TestPlannedIncrement(t *testing.T) {
	src := example(t, "library/circulation")
	root := filepath.Join(t.TempDir(), "circulation")
	copyTree(t, src, root)
	libPath := filepath.Join(root, "ontology", "lib.gloss.yaml")
	manifest := filepath.Join(root, "services", "circulation", ".gloss.yaml")
	renew := filepath.Join(root, "services", "circulation", "loans", "renew.py")
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The base: the example before Renewal existed. The manifest's allowed
	// list extends the derived set, so a planned declaration can widen it.
	if err := ontoedit.RemoveConcept(libPath, "Renewal"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(renew); err != nil {
		t.Fatal(err)
	}
	mb, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	baseManifest := strings.Replace(string(mb), "allowed: [lib:Media, lib:Book, lib:DVD, lib:Item, lib:Patron, lib:Loan, lib:Renewal, lib:ItemReturned, circ:*]",
		"allowed: [derived, lib:Media, lib:Book, lib:DVD, lib:Item, lib:Patron, lib:Loan, lib:ItemReturned]", 1)
	if baseManifest == string(mb) {
		t.Fatal("manifest allowed list not found")
	}
	write(manifest, baseManifest)
	ws0, base, _, _ := build(t, root)
	checkClean(t, ws0, base, "base")
	if base.NodeByCurie("lib:Renewal") != nil {
		t.Fatal("base still has lib:Renewal")
	}
	baseFailing := failingIDs(t, compareNow(base, ws0, time.Now()))
	if a := rollup.AllowedFor(base, ws0, ws0.ManifestFor("services/circulation/loans/x.py")); a.Matches("lib:Renewal") {
		t.Fatalf("base allowed set admits lib:Renewal already: %v", a.Patterns)
	}

	// The ontology PR: plan the concept and the manifest's realizes.
	by := time.Now().AddDate(0, 0, 30).Format(model.PlanDate)
	const ticket = "https://github.com/acme/library/issues/412"
	if err := ontoedit.AddConcept(libPath, "Renewal", ontoedit.ConceptSpec{Kind: "process", Doc: "Extending a loan's due date before it falls due.", Planned: true, PlannedBy: by, PlannedTicket: ticket}); err != nil {
		t.Fatal(err)
	}
	plannedManifest := baseManifest + "planned:\n  realizes: [lib:Renewal]\n  by: " + by + "\n"
	write(manifest, plannedManifest)
	ws1, head1, _, _ := build(t, root)
	checkClean(t, ws1, head1, "ontology PR")
	n := head1.NodeByCurie("lib:Renewal")
	if n == nil || !n.Planned || n.PlanBy != by || n.PlanTicket != ticket {
		t.Fatalf("planned node: %+v", n)
	}
	rep1 := compareNow(head1, ws1, time.Now())
	if got := failingIDs(t, rep1); strings.Join(got, ",") != strings.Join(baseFailing, ",") {
		t.Errorf("ontology PR changed the failing findings:\n base %v\n head %v", baseFailing, got)
	}
	for _, f := range rep1.Findings {
		if f.Concept == "lib:Renewal" && (f.Kind == rollup.KindOrphan || f.Kind == rollup.KindGap) {
			t.Errorf("planned lib:Renewal reported as %s: %s", f.Kind, f.Msg)
		}
	}
	// One plan item per planned thing: the concept and the manifest's
	// declaration, both open, both naming the deadline.
	plans := findings(rep1, rollup.KindPlanned)
	if len(plans) != 2 || rep1.Metrics.PlanItems != 2 {
		t.Fatalf("plan items = %d (%+v), metrics %d", len(plans), plans, rep1.Metrics.PlanItems)
	}
	var concept, decl *rollup.Finding
	for i := range plans {
		switch {
		case plans[i].Boundary == "" && plans[i].Concept == "lib:Renewal":
			concept = &plans[i]
		case plans[i].Boundary == "acme:circulation-svc" && plans[i].Relation == "realizes" && plans[i].Concept == "lib:Renewal":
			decl = &plans[i]
		}
	}
	if concept == nil || decl == nil {
		t.Fatalf("plan items: %+v", plans)
	}
	if concept.By != by || concept.Ticket != ticket || !strings.Contains(concept.Msg, "by "+by) || !strings.Contains(concept.Msg, ticket) || !strings.Contains(concept.Msg, "nothing implements it yet") {
		t.Errorf("concept plan item: %+v", *concept)
	}
	if decl.By != by || !strings.Contains(decl.Msg, "plans to implement lib:Renewal") || !strings.Contains(decl.Msg, by) {
		t.Errorf("declaration plan item: %+v", *decl)
	}
	// The planned declaration widens the derived allowed set and counts as
	// declared: the root's realizes: [inherit] covers it, no gap anywhere.
	if a := rollup.AllowedFor(head1, ws1, ws1.ManifestFor("services/circulation/loans/x.py")); !a.Matches("lib:Renewal") {
		t.Errorf("planned realizes did not widen the allowed set: %v", a.Patterns)
	}
	for _, f := range findings(rep1, rollup.KindGap) {
		if f.Boundary == "acme:library-platform" && f.Relation == "realizes" {
			t.Errorf("root inherit did not cover the planned realizes: %s", f.Msg)
		}
	}
	// Planned concepts stay out of the coverage denominator.
	if rep1.Metrics.ConceptsInScope != compareNow(base, ws0, time.Now()).Metrics.ConceptsInScope {
		t.Errorf("planned concept entered the coverage denominator")
	}
	// A by in the past: both items are plan-overdue, and name the ticket.
	due, _ := time.Parse(model.PlanDate, by)
	late := compareNow(head1, ws1, due.AddDate(0, 0, 2))
	overdue := findings(late, rollup.KindPlanOverdue)
	if len(overdue) != 2 || len(findings(late, rollup.KindPlanned)) != 0 {
		t.Fatalf("overdue = %+v", overdue)
	}
	for _, f := range overdue {
		if !strings.Contains(f.Msg, by) || f.Severity != "error" || (f.Boundary == "" && !strings.Contains(f.Msg, ticket)) {
			t.Errorf("overdue item: %+v", f)
		}
	}
	if len(failingIDs(t, late)) != len(baseFailing)+2 {
		t.Errorf("plan-overdue should fail by default: %v", failingIDs(t, late))
	}
	// On the by day itself the plan still holds (inclusive, like a waiver).
	if n := len(findings(compareNow(head1, ws1, due.Add(12*time.Hour)), rollup.KindPlanOverdue)); n != 0 {
		t.Errorf("overdue on the by day itself: %d", n)
	}

	// A test written first: verifies alone keeps the plan open.
	testFile := filepath.Join(root, "services", "circulation", "tests", "test_renew.py")
	write(testFile, "# @gloss verifies lib:Renewal\ndef test_renew():\n    pass\n")
	ws2, head2, _, _ := build(t, root)
	checkClean(t, ws2, head2, "verifies only")
	rep2 := compareNow(head2, ws2, time.Now())
	if c := findings(rep2, rollup.KindPlannedClosed); len(c) != 0 {
		t.Errorf("verifies alone closed a plan item: %+v", c)
	}
	open := findings(rep2, rollup.KindPlanned)
	verified := false
	for _, f := range open {
		if f.Boundary == "" && strings.Contains(f.Msg, "verified, not implemented") {
			verified = true
		}
	}
	if len(open) != 2 || !verified {
		t.Errorf("after a verifies marker: %+v", open)
	}

	// The implementation PR: an implements marker closes both items.
	write(renew, "# @gloss implements lib:Renewal\ndef renew(loan_id: str) -> \"Loan\": ...\n")
	ws3, head3, _, _ := build(t, root)
	checkClean(t, ws3, head3, "implementation PR")
	rep3 := compareNow(head3, ws3, time.Now())
	closed := findings(rep3, rollup.KindPlannedClosed)
	if len(closed) != 2 || len(findings(rep3, rollup.KindPlanned)) != 0 || rep3.Metrics.PlanItems != 0 {
		t.Fatalf("planned-closed = %+v, open %+v", closed, findings(rep3, rollup.KindPlanned))
	}
	for _, f := range closed {
		switch f.Boundary {
		case "":
			if !strings.Contains(f.Msg, "gloss accept --planned lib:Renewal") {
				t.Errorf("concept closure: %s", f.Msg)
			}
		case "acme:circulation-svc":
			if !strings.Contains(f.Msg, "move it from planned to realizes") {
				t.Errorf("declaration closure: %s", f.Msg)
			}
		default:
			t.Errorf("unexpected closure: %+v", f)
		}
	}
	for _, f := range findings(rep3, rollup.KindViolation) {
		if f.Concept == "lib:Renewal" {
			t.Errorf("planned realizes did not count as declared: %s", f.Msg)
		}
	}
	if got := failingIDs(t, rep3); len(got) != len(baseFailing)+2 {
		t.Errorf("planned-closed should fail by default: %v", got)
	}
	// gloss review offers accept (AUTO) on the concept's closed item.
	eng := &review.Engine{Root: root, Actor: "human:test", Load: func() (*review.State, error) {
		ws, g, res, lg := build(t, root)
		return &review.State{WS: ws, Graph: g, Ledger: lg, Markers: res.Markers, Report: compareNow(g, ws, time.Now())}, nil
	}}
	items, err := eng.Queue()
	if err != nil {
		t.Fatal(err)
	}
	offered := false
	for _, it := range items {
		if it.Finding == rollup.KindPlannedClosed && it.Concept == "lib:Renewal" && it.Boundary == "" {
			if a := it.Action(review.ActAccept); a == nil || a.Gate != string(gate.AUTO) || !a.AgentMay {
				t.Errorf("review action on the closed item: %+v", it.Actions)
			}
			offered = true
		}
	}
	if !offered {
		t.Errorf("gloss review does not list the closed plan item: %+v", items)
	}

	// gloss accept --planned lib:Renewal drops the key; the edit is AUTO in
	// gloss diff even with the file under hard-paths.
	if planned, err := ontoedit.IsPlanned(libPath, "Renewal"); err != nil || !planned {
		t.Fatalf("IsPlanned before: %v %v", planned, err)
	}
	if err := ontoedit.RemovePlanned(libPath, "Renewal"); err != nil {
		t.Fatal(err)
	}
	if planned, _ := ontoedit.IsPlanned(libPath, "Renewal"); planned {
		t.Fatal("planned still present after RemovePlanned")
	}
	lb, _ := os.ReadFile(libPath)
	if strings.Contains(string(lb), "planned") || !strings.Contains(string(lb), "Extending a loan's due date") || !strings.Contains(string(lb), "PickupWindow") {
		t.Errorf("ontology file after accept --planned:\n%s", lb)
	}
	ws4, head4, _, _ := build(t, root)
	checkClean(t, ws4, head4, "accept --planned")
	if n := head4.NodeByCurie("lib:Renewal"); n == nil || n.Planned {
		t.Fatalf("after accept: %+v", n)
	}
	im := gate.Classify(head3, head4, ws4, gate.Options{Base: "impl", Policy: ws4.Config.EffectivePolicy(), ChangedFiles: []string{"ontology/lib.gloss.yaml"}})
	dropped := false
	for _, ch := range im.Changes {
		switch ch.Kind {
		case "planned-dropped":
			dropped = ch.Subject == "lib:Renewal" && ch.Gate == gate.AUTO && ch.Bump == ""
		case "hard-path", "version-bump":
			t.Errorf("dropping planned is not bookkeeping to the gate: %+v", ch)
		}
	}
	if !dropped || im.Gate != gate.AUTO {
		t.Errorf("gloss diff after accept --planned: gate %s, changes %+v", im.Gate, im.Changes)
	}
	rep4 := compareNow(head4, ws4, time.Now())
	for _, f := range rep4.Findings {
		if f.Boundary == "" && f.Concept == "lib:Renewal" && strings.HasPrefix(f.Kind, "plan") {
			t.Errorf("concept plan item survives accept --planned: %+v", f)
		}
	}
	// The manifest's item closes when the entry moves to the plain list.
	write(manifest, strings.Replace(baseManifest, "lib:Return, lib:LoanPeriod]", "lib:Return, lib:LoanPeriod, lib:Renewal]", 1))
	ws5, head5, _, _ := build(t, root)
	checkClean(t, ws5, head5, "manifest moved")
	rep5 := compareNow(head5, ws5, time.Now())
	for _, kind := range []string{rollup.KindPlanned, rollup.KindPlannedClosed, rollup.KindPlanOverdue} {
		if fs := findings(rep5, kind); len(fs) != 0 {
			t.Errorf("%s after the increment closed: %+v", kind, fs)
		}
	}
	if got := failingIDs(t, rep5); strings.Join(got, ",") != strings.Join(baseFailing, ",") {
		t.Errorf("the closed increment changed the failing findings:\n base %v\n head %v", baseFailing, got)
	}
}

// TestPlannedForms: planned: true, planned: false and the mapping on block
// and flow entries, RemovePlanned on each, GM307 for a malformed by, GM200
// not firing (planned is no relation), and GM110 for a misspelt key inside
// a manifest's planned: block.
func TestPlannedForms(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  zi: { iri: https://x/zi/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  A: { kind: process, planned: true, doc: \"a\" }\n  B: { kind: process, planned: { by: 2030-01-31, ticket: https://t/1 }, doc: \"b\" }\n  C:\n    kind: process\n    planned:\n      by: 2030-02-28\n      ticket: https://t/2\n    doc: \"c\"\n  D: { kind: process, planned: false, doc: \"d\" }\n  E: { kind: process, planned: { by: tomorrow }, doc: \"e\" }\n")
	write("ontology/zi.gloss.yaml", "concepts:\n  AImpl: { kind: process, realizes: z:A, planned: true, doc: \"the mechanism\" }\n")
	write(".gloss.yaml", "boundary: o:root\nlevel: system\nplanned:\n  realizes: [z:C]\n  by: 2030-01-01\n")
	ws, g, _, _ := build(t, root)
	for curie, want := range map[string]struct {
		planned    bool
		by, ticket string
	}{
		"z:A": {true, "", ""}, "z:B": {true, "2030-01-31", "https://t/1"}, "z:C": {true, "2030-02-28", "https://t/2"}, "z:D": {false, "", ""}, "zi:AImpl": {true, "", ""},
	} {
		n := g.NodeByCurie(curie)
		if n == nil || n.Planned != want.planned || n.PlanBy != want.by || n.PlanTicket != want.ticket {
			t.Errorf("%s: %+v, want %+v", curie, n, want)
		}
	}
	codes := map[string]int{}
	for _, d := range append(ws.Diags, check.Run(ws, g)...) {
		codes[d.Code]++
	}
	if codes["GM307"] != 1 || codes["GM200"] != 0 || codes["GM110"] != 0 {
		t.Errorf("diagnostics: %v", codes)
	}
	// A misspelt key inside planned: is GM110 at its own line; the rest of
	// the block still applies.
	bad, writeBad := tempRepo(t)
	writeBad("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n")
	writeBad("ontology/z.gloss.yaml", "concepts:\n  C: { kind: process, planned: true, doc: \"c\" }\n")
	writeBad(".gloss.yaml", "boundary: o:root\nlevel: system\nplanned:\n  realizes: [z:C]\n  bye: 2030-01-01\n")
	bws, err := load.Load(bad)
	if err != nil {
		t.Fatal(err)
	}
	gm110 := 0
	for _, d := range bws.Diags {
		if d.Code == "GM110" {
			gm110++
			if !strings.Contains(d.Msg, "planned.bye") || d.Line != 5 {
				t.Errorf("GM110: %s", d)
			}
		}
	}
	if gm110 != 1 || !bws.Manifests[0].Plans("realizes", "z:C") {
		t.Errorf("misspelt planned key: %d GM110, plans %v", gm110, bws.Manifests[0].PlannedSets())
	}
	// A planned implementation concept is out of orphans and coverage; the
	// planned core concepts are plan items, D is a plain orphan.
	rep := rollup.Compare(g, ws, rollup.Options{Policy: ws.Config.EffectivePolicy()})
	if o := concepts(findings(rep, rollup.KindOrphan)); len(o) != 1 || o[0] != "z:D" {
		t.Errorf("orphans = %v", o)
	}
	var conceptItems, declItems []string
	for _, f := range findings(rep, rollup.KindPlanned) {
		if f.Boundary == "" {
			conceptItems = append(conceptItems, f.Concept)
		} else {
			declItems = append(declItems, f.Boundary+" "+f.Relation+" "+f.Concept)
		}
	}
	if len(conceptItems) != 5 || !has(conceptItems, "zi:AImpl") || !has(conceptItems, "z:C") || !has(conceptItems, "z:E") {
		t.Errorf("concept plan items = %v", conceptItems)
	}
	if len(declItems) != 1 || declItems[0] != "o:root realizes z:C" {
		t.Errorf("declaration plan items = %v", declItems)
	}
	if rep.Metrics.ConceptsInScope != 0 {
		t.Errorf("planned impl concept in coverage: %d", rep.Metrics.ConceptsInScope)
	}
	// RemovePlanned handles every layout and leaves the rest alone.
	z := filepath.Join(root, "ontology", "z.gloss.yaml")
	for _, name := range []string{"A", "B", "C", "D"} {
		if err := ontoedit.RemovePlanned(z, name); err != nil {
			t.Fatal(err)
		}
	}
	if err := ontoedit.RemovePlanned(z, "D"); err == nil || !strings.Contains(err.Error(), "not planned") {
		t.Errorf("RemovePlanned twice: %v", err)
	}
	b, _ := os.ReadFile(z)
	want := "concepts:\n  A: { kind: process, doc: \"a\" }\n  B: { kind: process, doc: \"b\" }\n  C:\n    kind: process\n    doc: \"c\"\n  D: { kind: process, doc: \"d\" }\n  E: { kind: process, planned: { by: tomorrow }, doc: \"e\" }\n"
	if string(b) != want {
		t.Errorf("after RemovePlanned:\n%s\nwant:\n%s", b, want)
	}
	// Dropping a plan while a planned-dropped change also carries a kind
	// change is still HARD for the kind change.
	_, head, _, _ := build(t, root)
	im := gate.Classify(g, head, ws, gate.Options{Base: "base", Policy: ws.Config.EffectivePolicy(), ChangedFiles: []string{"ontology/z.gloss.yaml"}})
	n := 0
	for _, ch := range im.Changes {
		if ch.Kind == "planned-dropped" && ch.Gate == gate.AUTO {
			n++
		}
		if ch.Kind == "hard-path" {
			t.Errorf("hard-path fallback fired on a bookkeeping-only edit: %+v", ch)
		}
	}
	if n != 3 || im.Gate != gate.AUTO {
		t.Errorf("gate after dropping three plans: %s %+v", im.Gate, im.Changes)
	}
}
