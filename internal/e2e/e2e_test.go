// @gloss verifies gmi:Extractor, gmi:Linter, gmi:RollupEngine, gmi:DriftComparator, gmi:Gate, gmi:ReviewLedger, gmi:ViewProjector, gmi:WorkspaceLoader scope=file

// Package e2e runs the whole pipeline on the example repositories and asserts
// the findings the design document's worked examples call for.
package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/views"
)

func example(t *testing.T, rel string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "examples", rel))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// build runs load → extract → derive → ledger sync (in memory) for a root.
func build(t *testing.T, root string) (*load.Workspace, *graph.Graph, *extract.Result, *ledger.Ledger) {
	t.Helper()
	ws, err := load.Load(root)
	if err != nil {
		t.Fatalf("load %s: %v", root, err)
	}
	res, err := extract.Build(ws)
	if err != nil {
		t.Fatal(err)
	}
	rollup.Derive(res.Graph, ws.Vocab)
	lg, _ := ledger.Open(filepath.Join(t.TempDir(), "ledger.jsonl"))
	lg.Sync(res.Graph, res.Markers, "human:test", time.Now())
	for _, d := range res.Diags {
		if d.Severity == "error" {
			t.Errorf("extract diagnostic: %s", d)
		}
	}
	return ws, res.Graph, res, lg
}

func findings(r *rollup.Report, kind string) []rollup.Finding {
	var out []rollup.Finding
	for _, f := range r.Findings {
		if f.Kind == kind {
			out = append(out, f)
		}
	}
	return out
}

func concepts(fs []rollup.Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Concept)
	}
	return out
}

func has(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func TestHorseFactory(t *testing.T) {
	root := example(t, "horse-factory")
	start := time.Now()
	ws, g, res, lg := build(t, root)
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("extractor took %s, gate is 5s", took)
	}
	if res.ScannedFiles < 10 || len(res.Markers) < 15 {
		t.Fatalf("scanned %d files, %d markers", res.ScannedFiles, len(res.Markers))
	}
	// Generated files are excluded by the manifest glob.
	for _, m := range res.Markers {
		if strings.Contains(m.Path, "/gen/") {
			t.Errorf("generated file scanned: %s", m.Path)
		}
	}
	// check: no errors; the Donkey link is outside allowed (warning because proposed).
	diags := check.Run(ws, g)
	for _, d := range diags {
		if d.Severity == "error" {
			t.Errorf("unexpected check error: %s", d)
		}
	}
	gm502 := false
	for _, d := range diags {
		if d.Code == "GM502" {
			gm502 = true
		}
	}
	if !gm502 {
		t.Errorf("expected GM502 outside-allowed warning, got %v", diags)
	}
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	viol := findings(rep, rollup.KindViolation)
	if len(viol) != 1 || viol[0].Concept != "zoo:Donkey" || !viol[0].Proposed || viol[0].Relation != "produces" {
		t.Errorf("violations = %+v, want one proposed produces zoo:Donkey", viol)
	}
	if n := len(findings(rep, rollup.KindGap)); n != 0 {
		t.Errorf("gaps = %d, want 0", n)
	}
	rogues := findings(rep, rollup.KindRogue)
	if len(rogues) != 1 || rogues[0].Concept != "impl:NEW:Gelding" {
		t.Errorf("rogues = %+v", rogues)
	}
	if len(rep.Decisions) == 0 || rep.Decisions[0].Concept != "impl:Gelding" || rep.Decisions[0].Owners[0] != "horse-platform" {
		t.Errorf("decisions = %+v", rep.Decisions)
	}
	orph := concepts(findings(rep, rollup.KindOrphan))
	for _, want := range []string{"impl:BreedingPlanner", "impl:TailQueue", "zoo:HorseOrder"} {
		if !has(orph, want) {
			t.Errorf("orphans %v lack %s", orph, want)
		}
	}
	if has(orph, "zoo:FoalBorn") || has(orph, "zoo:ProductionCapacity") {
		t.Errorf("orphans %v include a reached concept", orph)
	}
	// Taxonomic lift: the system boundary's produces {Horse, Stallion, Donkey?} summarizes to Animal and is consistent.
	cons := findings(rep, rollup.KindConsistent)
	ok := false
	for _, f := range cons {
		if f.Boundary == "acme:animal-production" && f.Relation == "produces" && strings.Contains(f.Msg, "zoo:Animal") {
			ok = true
		}
	}
	if !ok {
		t.Errorf("expected animal-production produces consistent via LCS, got %+v", cons)
	}
	m := rep.Metrics
	if m.InScopeFiles == 0 || m.CoveragePct < 50 || m.Proposed != 3 {
		t.Errorf("metrics %+v", m)
	}
	// Views: business folds impl into core and shows the published measure under its core name.
	ps := views.ProjectAll(g, ws.Views)
	var biz, eng *views.Projection
	for _, p := range ps {
		switch p.Name {
		case "business":
			biz = p
		case "engineering":
			eng = p
		}
	}
	if biz == nil || eng == nil {
		t.Fatal("missing preset views")
	}
	for _, n := range biz.Nodes {
		if strings.HasPrefix(n.ID, "impl:") {
			t.Errorf("business view shows impl node %s", n.ID)
		}
		if n.ID == "zoo:ProductionCapacity" && (!n.Published || n.PublishedAs != "impl:FoalingPipelineThroughput") {
			t.Errorf("published seam not surfaced: %+v", n)
		}
		if n.ID == "zoo:Breeding" && !has(n.Folded, "impl:StallionProduction") {
			t.Errorf("Breeding should fold StallionProduction: %+v", n)
		}
	}
	lifted := false
	for _, e := range biz.Edges {
		if e.S == "acme:animal-production" && e.P == "produces" && e.O == "zoo:Animal" && e.Lifted {
			lifted = true
		}
	}
	if !lifted {
		t.Error("business view lacks lifted produces → zoo:Animal")
	}
	if len(eng.Nodes) <= len(biz.Nodes) {
		t.Errorf("engineering view (%d nodes) should be larger than business (%d)", len(eng.Nodes), len(biz.Nodes))
	}
	// Graph round-trips deterministically.
	var sb1, sb2 strings.Builder
	g.WriteJSONL(&sb1)
	g2, err := graph.ReadJSONL(strings.NewReader(sb1.String()))
	if err != nil {
		t.Fatal(err)
	}
	g2.WriteJSONL(&sb2)
	if sb1.String() != sb2.String() {
		t.Error("graph.jsonl is not stable across a round trip")
	}
}

func TestPayments(t *testing.T) {
	api := example(t, "payments/payments-api")
	ws, g, _, lg := build(t, api)
	if errs := errorsOf(check.Run(ws, g)); len(errs) > 0 {
		t.Errorf("payments-api check errors: %v", errs)
	}
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	viol := findings(rep, rollup.KindViolation)
	if len(viol) != 1 || viol[0].Boundary != "acme:payments-api" || viol[0].Concept != "pay:Refunding" || !viol[0].Proposed {
		t.Errorf("payments-api violations = %+v, want Refunding handled but not declared", viol)
	}
	gaps := concepts(findings(rep, rollup.KindGap))
	if len(gaps) != 1 || gaps[0] != "pay:Payout" {
		t.Errorf("gaps = %v, want pay:Payout (produced by the other repo)", gaps)
	}
	unt := concepts(findings(rep, rollup.KindUntested))
	if !has(unt, "pay:Capture") {
		t.Errorf("untested %v lacks pay:Capture", unt)
	}
	if has(unt, "pay:Authorization") || has(unt, "pa:AuthorizeHandler") {
		t.Errorf("Authorization is verified by a feature file; untested = %v", unt)
	}
	if has(unt, "pa:CaptureHandler") {
		t.Errorf("impl concept reported alongside its core: %v", unt)
	}
	if !has(concepts(findings(rep, rollup.KindOrphan)), "pay:Merchant") {
		t.Error("pay:Merchant should be an orphan")
	}

	// settlement-worker imports pay: and extends it.
	sw := example(t, "payments/settlement-worker")
	ws2, g2, _, lg2 := build(t, sw)
	if errs := errorsOf(check.Run(ws2, g2)); len(errs) > 0 {
		t.Errorf("settlement-worker check errors: %v", errs)
	}
	if n := g2.NodeByCurie("pay:Settlement"); n == nil || !n.Imported {
		t.Fatal("imported core concept missing")
	}
	rep2 := rollup.Compare(g2, ws2, rollup.Options{Ledger: lg2.Entries, Policy: ws2.Config.EffectivePolicy()})
	if n := len(findings(rep2, rollup.KindViolation)); n != 0 {
		t.Errorf("settlement-worker violations = %d", n)
	}
	ext := false
	for _, d := range rep2.Decisions {
		if d.Kind == "extension-upstream" && d.Concept == "sw:PartialSettlement" {
			ext = true
		}
	}
	if !ext {
		t.Errorf("expected extension-upstream decision for sw:PartialSettlement, got %+v", rep2.Decisions)
	}

	// Landscape: union by IRI → context-map edge and the Payout gap closes.
	merged, err := load.Merge([]*load.Workspace{ws, ws2})
	if err != nil {
		t.Fatal(err)
	}
	u := graph.New(merged.Prefixes)
	for _, gg := range []*graph.Graph{g, g2} {
		for _, n := range gg.Nodes() {
			nn := *n
			if nn.NodeType == model.NodeConcept || nn.NodeType == model.NodeNamespace {
				nn.Imported = !merged.Owned[nn.NS]
			}
			if ex := u.Node(nn.ID); ex != nil && ex.Imported && !nn.Imported {
				*ex = nn
				continue
			}
			u.AddNode(&nn)
		}
		for _, e := range gg.Edges() {
			if e.Source != graph.SourceDerived {
				ee := *e
				u.AddEdge(&ee)
			}
		}
	}
	rollup.Derive(u, merged.Vocab)
	dep := false
	for _, e := range u.Out(mustIRI(t, u, "acme:settlement-worker"), "depends-on") {
		if u.Curie(e.O) == "acme:payments-api" && has(e.Prov.Via, "pay:CaptureConfirmed") {
			dep = true
		}
	}
	if !dep {
		t.Error("landscape lacks settlement-worker depends-on payments-api via pay:CaptureConfirmed")
	}
	rep3 := rollup.Compare(u, merged, rollup.Options{Policy: merged.Config.EffectivePolicy()})
	if n := len(findings(rep3, rollup.KindGap)); n != 0 {
		t.Errorf("landscape gaps = %+v, want none (Payout produced by settlement-worker)", findings(rep3, rollup.KindGap))
	}
	if v := findings(rep3, rollup.KindViolation); len(v) != 1 || v[0].Concept != "pay:Refunding" {
		t.Errorf("landscape violations = %+v", v)
	}
}

func mustIRI(t *testing.T, g *graph.Graph, curie string) string {
	n := g.NodeByCurie(curie)
	if n == nil {
		t.Fatalf("no node %s", curie)
	}
	return n.ID
}

func errorsOf[D interface{ String() string }](ds []D) []string {
	var out []string
	for _, d := range ds {
		s := d.String()
		if strings.Contains(s, ": error: ") {
			out = append(out, s)
		}
	}
	return out
}

// TestGateCutoffChange reproduces the design's PR: moving the published
// cutoff value is HARD because the business view changes; a new in-remit
// proposal is SOFT; a proposal outside the agent's remit is HARD.
func TestGateCutoffChange(t *testing.T) {
	src := example(t, "payments/settlement-worker")
	api := example(t, "payments/payments-api")
	tmp := t.TempDir()
	// Copy both repos so the relative import (../payments-api) keeps working.
	copyTree(t, api, filepath.Join(tmp, "payments-api"))
	copyTree(t, src, filepath.Join(tmp, "settlement-worker"))
	root := filepath.Join(tmp, "settlement-worker")
	wsBase, base, _, _ := build(t, root)
	// Head: change the cutoff value and add an agent proposal outside its remit.
	ont := filepath.Join(root, "ontology", "sw.gloss.yaml")
	b, _ := os.ReadFile(ont)
	os.WriteFile(ont, []byte(strings.Replace(string(b), `value: "16:00Z"`, `value: "17:00Z"`, 1)), 0o644)
	os.WriteFile(filepath.Join(root, "internal", "batch", "merchant.go"), []byte("package batch\n\n// @gloss? represents pay:Payment conf=0.9\ntype Row struct{}\n\n// @gloss? implements pay:Settlement conf=0.9\nfunc Settle() {}\n"), 0o644)
	os.WriteFile(filepath.Join(root, ".claude-agent.md"), []byte("<!-- @gloss may-modify sw:* scope=file -->\n<!-- @gloss instructs sw:BatchSettler scope=file -->\n"), 0o644)
	ws, head, res, _ := build(t, root)
	lg, _ := ledger.Open(filepath.Join(t.TempDir(), "ledger.jsonl"))
	lg.Sync(head, res.Markers, "agent:claude-agent@claude-fable-5-1", time.Now())
	_ = wsBase
	im := gate.Classify(base, head, ws, gate.Options{Base: "base", Policy: ws.Config.EffectivePolicy(), HeadLedger: lg.Entries})
	if im.Gate != gate.HARD {
		t.Fatalf("gate = %s, want HARD", im.Gate)
	}
	var pubChanged, softLink, remitHard bool
	for _, ch := range im.Changes {
		switch {
		case ch.Kind == "published-changed" && ch.Subject == "sw:CutoffClock" && ch.Gate == gate.HARD:
			pubChanged = true
		case ch.Kind == "link-added" && ch.Object == "pay:Settlement" && ch.Gate == gate.HARD && strings.Contains(strings.Join(ch.Reasons, ";"), "remit"):
			remitHard = true
		case ch.Kind == "link-added" && ch.Object == "pay:Payment" && ch.Gate == gate.HARD && strings.Contains(strings.Join(ch.Reasons, ";"), "remit"):
			softLink = true // outside sw:* remit too; both proposals land HARD
		}
	}
	if !pubChanged {
		t.Errorf("expected HARD published-changed for sw:CutoffClock; changes: %+v", im.Changes)
	}
	if !remitHard || !softLink {
		t.Errorf("expected remit-based HARD gates; changes: %+v", im.Changes)
	}
	if !has(im.ViewsChanged, "business") {
		t.Errorf("business view should change when a published value moves; got %v", im.ViewsChanged)
	}
	txt := gate.RenderText(im)
	if !strings.Contains(txt, "published sw:CutoffClock value 16:00Z → 17:00Z") || !strings.Contains(txt, "Views changed:") {
		t.Errorf("render:\n%s", txt)
	}
	md := gate.RenderMarkdown(im)
	if !strings.Contains(md, "<!-- glossmark-impact -->") {
		t.Error("markdown lacks the comment marker used to update PR comments")
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			if info.Name() == ".glossmark" {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, info.Mode())
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestLedgerTransitions covers proposed → accepted → stale.
func TestLedgerTransitions(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Thing: { kind: entity }\n  Doing: { kind: process }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  Doer: { kind: process, realizes: z:Doing }\n")
	write(".gloss.yaml", "boundary: o:b\nlevel: container\ndefault-namespace: i\n")
	write("a.py", "# @gloss? implements .Doer conf=0.9\ndef go(x): pass\n")
	lgPath := filepath.Join(root, ".glossmark", "ledger.jsonl")
	run := func(actor string) (*ledger.Ledger, *extract.Result) {
		ws, err := load.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		res, _ := extract.Build(ws)
		rollup.Derive(res.Graph, ws.Vocab)
		lg, _ := ledger.Open(lgPath)
		lg.Sync(res.Graph, res.Markers, actor, time.Now())
		if err := lg.Save(); err != nil {
			t.Fatal(err)
		}
		return lg, res
	}
	lg, res := run("agent:bot@m")
	id := res.Markers[0].ID
	if e := lg.Entry(id); e == nil || e.Status != model.StatusProposed || e.Actor != "agent:bot@m" || e.Model != "m" {
		t.Fatalf("after proposal: %+v", lg.Entry(id))
	}
	// Human flips the ? in the editor.
	write("a.py", "# @gloss implements .Doer conf=0.9\ndef go(x): pass\n")
	lg, _ = run("human:alice")
	if e := lg.Entry(id); e.Status != model.StatusAccepted || e.ReviewedBy != "human:alice" {
		t.Fatalf("after accept: %+v", e)
	}
	// Signature changes → stale.
	write("a.py", "# @gloss implements .Doer conf=0.9\ndef go(x, y): pass\n")
	lg, _ = run("human:alice")
	if e := lg.Entry(id); e.Status != model.StatusStale {
		t.Fatalf("after signature change: %+v", e)
	}
	// Marker removed → removed.
	write("a.py", "def go(x, y): pass\n")
	lg, _ = run("human:alice")
	if e := lg.Entry(id); e.Status != model.StatusRemoved {
		t.Fatalf("after removal: %+v", e)
	}
}

// TestOwnSubjectRule: an importer may extend but never mutate a foreign namespace.
func TestOwnSubjectRule(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("upstream/zoo.gloss.yaml", "concepts:\n  Horse: { kind: entity }\n  Mare: { kind: entity, is-a: Horse }\n")
	write("glossmark.yaml", "owns:\n  mine: { iri: https://x/mine/, tier: core }\nimports:\n  zoo: { iri: https://x/zoo/, source: upstream/zoo.gloss.yaml }\n")
	write("ontology/mine.gloss.yaml", "concepts:\n  Pony: { kind: entity, is-a: zoo:Horse }\n  Saddle: { kind: entity, attaches-to: zoo:Horse }\n  Breeder: { kind: entity, has-part: zoo:Mare }\n")
	ws, err := load.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	res, _ := extract.Build(ws)
	rollup.Derive(res.Graph, ws.Vocab)
	var codes []string
	for _, d := range check.Run(ws, res.Graph) {
		codes = append(codes, d.Code)
	}
	if !has(codes, "GM400") {
		t.Errorf("has-part on an imported concept must be rejected (GM400); got %v", codes)
	}
	for _, c := range codes {
		if c != "GM400" && c != "GM302" {
			t.Errorf("unexpected diagnostic %s (Pony is-a zoo:Horse and Saddle attaches-to zoo:Horse are allowed)", c)
		}
	}
}
