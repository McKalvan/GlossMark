// @gloss verifies gmi:Extractor, gmi:Linter, gmi:RollupEngine, gmi:DriftComparator, gmi:Gate, gmi:ReviewLedger, gmi:ViewProjector, gmi:WorkspaceLoader, gmi:LandscapeAssembler, gmi:CriticalityStamper, gmi:McpServer, gmi:OntologyEditor, gmi:ImportLock, gmi:AgentsSnippet, gmi:ValueBinder scope=file

// Package e2e runs the whole pipeline on the example repositories and asserts
// the findings the design document's worked examples call for.
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mckalvan/glossmark/internal/agents"
	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/mcp"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/ontoedit"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/version"
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
	// Boundaries house what their code realizes: at system zoom the container folds
	// into the system; at container zoom Breeding sits in the service, a part follows
	// its mechanism, Foal (represented only by the root migration) stays in the system,
	// and HorseOrder, which nothing implements, is unplaced.
	wantIn := map[string]map[string]string{
		"business":    {"zoo:Breeding": "acme:animal-production", "zoo:HorseOrder": "", "acme:animal-production": ""},
		"engineering": {"zoo:Breeding": "acme:horse-factory-svc", "impl:HoofStep": "acme:horse-factory-svc", "zoo:Foal": "acme:animal-production", "zoo:HorseOrder": "", "acme:horse-factory-svc": ""},
	}
	for _, p := range []*views.Projection{biz, eng} {
		for _, n := range p.Nodes {
			if want, ok := wantIn[p.Name][n.ID]; ok && n.Boundary != want {
				t.Errorf("%s view: %s in boundary %q, want %q", p.Name, n.ID, n.Boundary, want)
			}
			if n.ID == "acme:horse-factory-svc" && n.Parent != "acme:animal-production" {
				t.Errorf("container boundary should nest in the system: %+v", n)
			}
		}
	}
	// Criticality: declared on zoo:Breeding, inherited by what realizes it and by the boundary.
	wantCrit := map[string][2]string{
		"zoo:Breeding":                   {"medium", "declared"},
		"impl:StallionProduction":        {"medium", "zoo:Breeding"},
		"impl:HoofStep":                  {"medium", "impl:HoofAssembler"},
		"zoo:HorseOrder":                 {"high", "declared"},
		"impl:FoalingPipelineThroughput": {"low", "declared"},
		"acme:horse-factory-svc":         {"medium", "zoo:Breeding"},
		"acme:animal-production":         {"medium", "zoo:Breeding"},
		"zoo:Donkey":                     {"", ""},
	}
	for cu, want := range wantCrit {
		n := g.NodeByCurie(cu)
		if n == nil {
			t.Fatalf("missing %s", cu)
		}
		if n.Criticality != want[0] || n.CriticalityFrom != want[1] {
			t.Errorf("%s criticality = %q from %q, want %q from %q", cu, n.Criticality, n.CriticalityFrom, want[0], want[1])
		}
	}
	for _, a := range g.NodesOf(model.NodeArtifact) {
		// An artifact takes the level of what it implements, not its boundary's
		// (no criticality-floor here): the low-criticality metrics module.
		if a.Path == "services/horse-factory/breeding/metrics.py" && (a.Criticality != "low" || a.CriticalityFrom != "impl:FoalingPipelineThroughput") {
			t.Errorf("metrics.py takes its concept's level: %+v", a)
		}
	}
	for _, n := range eng.Nodes {
		if n.ID == "acme:horse-factory-svc" && (n.Criticality != "medium" || n.Owner != "horse-platform") {
			t.Errorf("projection should carry boundary criticality and owner: %+v", n)
		}
	}
	// A manifest that understates its risk is a governance warning.
	svc := g.NodeByCurie("acme:horse-factory-svc")
	svc.Criticality, svc.CriticalityFrom = "low", "declared"
	us := rollup.Understated(g)
	seenU := map[string]int{}
	for _, u := range us {
		seenU[u.Concept]++
		if u.Boundary != "acme:horse-factory-svc" || u.Declared != "low" || u.Level != "medium" {
			t.Errorf("understated = %+v", u)
		}
	}
	if seenU["zoo:Breeding"] != 1 {
		t.Errorf("understated should list zoo:Breeding once: %+v", us)
	}
	gm600 := false
	for _, d := range check.Run(ws, g) {
		if d.Code == "GM600" && strings.Contains(d.Msg, "zoo:Breeding") {
			gm600 = true
		}
	}
	if !gm600 {
		t.Error("expected GM600 for an understated boundary")
	}
	svc.Criticality, svc.CriticalityFrom = "medium", "zoo:Breeding"
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
	if b := g.NodeByCurie("acme:payments-api"); b == nil || b.Criticality != "critical" || b.CriticalityFrom != "pay:Authorization" {
		t.Errorf("payments-api should inherit critical from pay:Authorization: %+v", b)
	}
	if b := g.NodeByCurie("acme:payments-platform"); b == nil || b.Criticality != "critical" || b.CriticalityFrom != "declared" {
		t.Errorf("payments-platform declares critical: %+v", b)
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
	// Governance: the Payment proposal touches a critical concept; the cutoff change a medium one.
	if im.Criticality != "critical" {
		t.Errorf("impact criticality = %q, want critical", im.Criticality)
	}
	for _, ch := range im.Changes {
		if ch.Kind == "link-added" && ch.Object == "pay:Payment" && (ch.Criticality != "critical" || ch.CriticalityFrom != "pay:Payment" || !strings.Contains(strings.Join(ch.Reasons, ";"), "criticality critical")) {
			t.Errorf("Payment link should carry critical and the policy reason: %+v", ch)
		}
		if ch.Kind == "published-changed" && ch.Subject == "sw:CutoffClock" && ch.Criticality != "medium" {
			t.Errorf("CutoffClock change should carry medium (via pay:SettlementCutoff): %+v", ch)
		}
	}
	txt := gate.RenderText(im)
	if !strings.Contains(txt, "published sw:CutoffClock value 16:00Z → 17:00Z") || !strings.Contains(txt, "Views changed:") {
		t.Errorf("render:\n%s", txt)
	}
	if !strings.Contains(txt, "criticality critical") || !strings.Contains(txt, "Criticality touched: ") || !strings.Contains(txt, "pay:Payment critical (declared)") || !strings.Contains(txt, "[CRITICAL, agent") {
		t.Errorf("render lacks criticality:\n%s", txt)
	}
	md := gate.RenderMarkdown(im)
	if !strings.Contains(md, "<!-- glossmark-impact -->") {
		t.Error("markdown lacks the comment marker used to update PR comments")
	}
	if !strings.Contains(md, "criticality **critical**") || !strings.Contains(md, "**Criticality touched**") {
		t.Errorf("markdown lacks the criticality summary:\n%s", md)
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

// TestProposedConcepts covers the ontology-side ?: propose, link, gate, accept.
func TestProposedConcepts(t *testing.T) {
	src := example(t, "horse-factory")
	root := filepath.Join(t.TempDir(), "hf")
	copyTree(t, src, root)
	_, base, _, _ := build(t, root)
	// An agent proposes impl:Gelding and links to it.
	implPath := filepath.Join(root, "ontology", "impl.gloss.yaml")
	if err := ontoedit.AddConcept(implPath, "Gelding", ontoedit.ConceptSpec{Kind: "process", Realizes: []string{"zoo:Breeding"}, Doc: "Castration step.", Proposed: true, ProposedBy: "agent:bot@m"}); err != nil {
		t.Fatal(err)
	}
	files := []string{filepath.Join(root, "services", "horse-factory", "breeding", "engine.py")}
	if n, err := ontoedit.RewriteNewMarkers(files, "impl:Gelding"); err != nil || n != 1 {
		t.Fatalf("rewrite: n=%d err=%v", n, err)
	}
	ws, head, _, lg := build(t, root)
	n := head.NodeByCurie("impl:Gelding")
	if n == nil || !n.Proposed || n.ProposedBy != "agent:bot@m" {
		t.Fatalf("proposed concept missing or unflagged: %+v", n)
	}
	// The link now resolves: no rogue, a proposed finding, a decision request.
	rep := rollup.Compare(head, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	if len(findings(rep, rollup.KindRogue)) != 0 {
		t.Errorf("rogue should be gone after proposal: %+v", findings(rep, rollup.KindRogue))
	}
	if p := findings(rep, rollup.KindProposed); len(p) != 1 || p[0].Concept != "impl:Gelding" {
		t.Errorf("proposed findings = %+v", p)
	}
	// Lint: proposed impl concept without realizes would be a warning, with realizes nothing.
	for _, d := range check.Run(ws, head) {
		if d.Severity == "error" {
			t.Errorf("check error: %s", d)
		}
	}
	// Ledger tracks the concept proposal.
	if e := lg.Entry("concept:impl:Gelding"); e == nil || e.Status != model.StatusProposed || e.Actor != "agent:bot@m" {
		t.Errorf("ledger concept entry = %+v", lg.Entry("concept:impl:Gelding"))
	}
	// Gate: the proposal is HARD but in one PR; the link to it is HARD because the target is proposed.
	im := gate.Classify(base, head, ws, gate.Options{Base: "base", Policy: ws.Config.EffectivePolicy(), HeadLedger: lg.Entries})
	var proposed, linkHard bool
	for _, ch := range im.Changes {
		if ch.Kind == "concept-proposed" && ch.Subject == "impl:Gelding" && ch.Gate == gate.HARD {
			proposed = true
		}
		if ch.Kind == "link-added" && ch.Object == "impl:Gelding" && ch.Gate == gate.HARD {
			linkHard = true
		}
	}
	if !proposed || !linkHard {
		t.Errorf("gate changes: %+v", im.Changes)
	}
	// Accept: drop proposed:, the graph node is plain, the ledger entry accepted, the diff shows concept-accepted.
	if err := ontoedit.SetProposed(implPath, "Gelding", false, ""); err != nil {
		t.Fatal(err)
	}
	ws2, head2, res2, _ := build(t, root)
	lg.Sync(head2, res2.Markers, "human:alice", time.Now())
	if n := head2.NodeByCurie("impl:Gelding"); n == nil || n.Proposed {
		t.Fatalf("after accept: %+v", n)
	}
	if e := lg.Entry("concept:impl:Gelding"); e.Status != model.StatusAccepted || e.ReviewedBy != "human:alice" {
		t.Errorf("ledger after accept = %+v", e)
	}
	im2 := gate.Classify(head, head2, ws2, gate.Options{Base: "proposal", Policy: ws2.Config.EffectivePolicy()})
	ok := false
	for _, ch := range im2.Changes {
		if ch.Kind == "concept-accepted" && ch.Gate == gate.AUTO {
			ok = true
		}
	}
	if !ok {
		t.Errorf("expected AUTO concept-accepted, got %+v", im2.Changes)
	}
	// The YAML edit preserved the other entries and comments.
	b, _ := os.ReadFile(implPath)
	if !strings.Contains(string(b), "HoofAssembler") || !strings.Contains(string(b), "# unpublished") {
		t.Errorf("ontology file lost content:\n%s", b)
	}
}

// TestSearchByMeaning: aliases, CamelCase splitting, stems, typos, one hop.
func TestSearchByMeaning(t *testing.T) {
	_, g, _, _ := build(t, example(t, "payments/payments-api"))
	first := func(q string) string {
		hits := mcp.Search(g, q, "", "", "", 5)
		if len(hits) == 0 {
			return ""
		}
		return hits[0].Curie
	}
	cases := map[string]string{
		"chargeback":        "pay:Refund",           // alias
		"settlement cutoff": "pay:SettlementCutoff", // CamelCase split
		"refunding":         "pay:Refunding",
		"refunds":           "pay:Refund",     // stem
		"settlment":         "pay:Settlement", // typo
		"captured":          "pay:Capture",    // stem
		"capture confirmed": "pay:CaptureConfirmed",
	}
	for q, want := range cases {
		if got := first(q); got != want {
			t.Errorf("search %q = %q, want %q (hits %+v)", q, got, want, mcp.Search(g, q, "", "", "", 3))
		}
	}
	// Ranking v2 on horse-factory: every query word matched beats one word,
	// "birth" reaches FoalBorn through the synonym table, a kind word boosts
	// that kind, and the hit carries the doc and what matched.
	_, hf, _, _ := build(t, example(t, "horse-factory"))
	hits2 := mcp.Search(hf, "foal birth", "", "", "", 5)
	if len(hits2) == 0 || hits2[0].Curie != "zoo:FoalBorn" || hits2[0].Doc == "" || !strings.HasPrefix(hits2[0].Matched, "name:") {
		t.Errorf("foal birth = %+v", hits2)
	}
	if len(hits2) > 1 && hits2[1].Score == hits2[0].Score {
		t.Errorf("foal birth: FoalBorn should rank strictly first, got %+v", hits2[:2])
	}
	if ev := mcp.Search(hf, "foal event", "", "", "", 3); len(ev) == 0 || ev[0].Curie != "zoo:FoalBorn" || !strings.HasSuffix(ev[0].Matched, "+kind") {
		t.Errorf("foal event = %+v", ev)
	}
	// One hop: the impl handler is found through the core concept it realizes.
	hits := mcp.Search(g, "authorization", "pa", "", "", 5)
	if len(hits) == 0 || hits[0].Curie != "pa:AuthorizeHandler" || !strings.HasPrefix(hits[0].Matched, "via") && !strings.HasPrefix(hits[0].Matched, "name") {
		t.Errorf("one-hop search = %+v", hits)
	}
}

// TestTouched: what a change to a file touches.
func TestTouched(t *testing.T) {
	ws, g, _, _ := build(t, example(t, "payments/settlement-worker"))
	ts := gate.Touched(g, ws, []string{"internal/batch/cutoff.go", "internal/batch/settler.go", "ontology/sw.gloss.yaml"})
	if len(ts) != 3 {
		t.Fatalf("touched %d files", len(ts))
	}
	cut := ts[0]
	if cut.Gate != gate.HARD {
		t.Errorf("cutoff.go should be HARD (published seam): %+v", cut)
	}
	found := false
	for _, c := range cut.Concepts {
		if c.Curie == "sw:CutoffClock" && c.Published == "pay:SettlementCutoff" {
			found = true
		}
		if c.Curie == "pay:SettlementCutoff" && (len(c.Constrains) != 1 || c.Constrains[0] != "pay:Settlement") {
			t.Errorf("SettlementCutoff should constrain Settlement: %+v", c)
		}
	}
	if !found {
		t.Errorf("cutoff.go concepts: %+v", cut.Concepts)
	}
	// settler.go implements sw:BatchSettler, which inherits critical from pay:Settlement;
	// the default policy makes anything critical HARD (docs/GOVERNANCE.md).
	if ts[1].Gate != gate.HARD || ts[1].Criticality != "critical" || ts[1].CriticalityFrom != "sw:BatchSettler" {
		t.Errorf("settler.go should be HARD with criticality critical via the concept it implements: %+v", ts[1])
	}
	for _, c := range ts[1].Concepts {
		if c.Curie == "sw:BatchSettler" && (c.Criticality != "critical" || c.CriticalityFrom != "pay:Settlement") {
			t.Errorf("BatchSettler should inherit critical from pay:Settlement: %+v", c)
		}
	}
	if ts[2].Gate != gate.HARD || len(ts[2].Concepts) < 3 {
		t.Errorf("ontology file should be HARD with its concepts: %+v", ts[2])
	}
}

// TestImportRangesAndLock: a range outside the resolved version fails check;
// the lockfile notices an upstream change under the same version number.
func TestImportRangesAndLock(t *testing.T) {
	tmp := t.TempDir()
	copyTree(t, example(t, "payments/payments-api"), filepath.Join(tmp, "payments-api"))
	copyTree(t, example(t, "payments/settlement-worker"), filepath.Join(tmp, "settlement-worker"))
	root := filepath.Join(tmp, "settlement-worker")
	// The example ships a lockfile; start from the unlocked state.
	os.Remove(filepath.Join(root, load.LockName))
	codes := func() []string {
		ws, err := load.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, d := range ws.Diags {
			out = append(out, d.Code)
		}
		return out
	}
	// No lock yet → GM108; range ^1.2 satisfied by 1.2.1 → no GM106.
	c := codes()
	if !has(c, "GM108") || has(c, "GM106") {
		t.Fatalf("initial diags = %v", c)
	}
	// Lock it.
	ws, _ := load.Load(root)
	if err := load.WriteLock(root, ws.Resolved); err != nil {
		t.Fatal(err)
	}
	if c := codes(); has(c, "GM108") || has(c, "GM107") {
		t.Fatalf("after lock diags = %v", c)
	}
	// Upstream edits the ontology without bumping → GM107 (hash).
	up := filepath.Join(tmp, "payments-api", "ontology", "pay.gloss.yaml")
	b, _ := os.ReadFile(up)
	os.WriteFile(up, append(b, []byte("  Chargeback: { kind: entity }\n")...), 0o644)
	if c := codes(); !has(c, "GM107") {
		t.Fatalf("content change not noticed: %v", c)
	}
	// Upstream bumps to 2.0.0 → outside ^1.2 → GM106 and GM107 (version).
	os.WriteFile(up, []byte(strings.Replace(string(b), "version: 1.2.1", "version: 2.0.0", 1)), 0o644)
	if c := codes(); !has(c, "GM106") || !has(c, "GM107") {
		t.Fatalf("major bump not caught: %v", c)
	}
	// An unsupported grammar directive is refused outright.
	cfg := filepath.Join(root, "glossmark.yaml")
	cb, _ := os.ReadFile(cfg)
	os.WriteFile(cfg, append([]byte("grammar: 99\n"), cb...), 0o644)
	if _, err := load.Load(root); err == nil || !strings.Contains(err.Error(), "grammar 99") {
		t.Fatalf("grammar 99 should be refused, got %v", err)
	}
}

// TestGraphFormatGuard: a graph written by a newer format is refused.
func TestGraphFormatGuard(t *testing.T) {
	newer := `{"type":"context","format":99,"generator":"gloss 9.0.0","prefixes":{}}` + "\n"
	if _, err := graph.ReadJSONL(strings.NewReader(newer)); err == nil || !strings.Contains(err.Error(), "format 99") {
		t.Fatalf("expected refusal, got %v", err)
	}
	// Format 0 (no header field) still reads.
	old := `{"type":"context","generator":"gloss 0.1.0","prefixes":{}}` + "\n"
	if _, err := graph.ReadJSONL(strings.NewReader(old)); err != nil {
		t.Fatal(err)
	}
	// Written graphs carry the current format.
	var sb strings.Builder
	graph.New(nil).WriteJSONL(&sb)
	if !strings.Contains(sb.String(), `"format":`+fmt.Sprint(version.GraphFormat)) {
		t.Fatalf("header lacks format: %s", sb.String())
	}
}

// TestOntologyFileLint: an unquoted doc with a comma inside { } is cut at the
// comma and the rest becomes null keys; check must say so (GM305), and report
// any other null key (GM304). Quoting the doc clears both.
func TestOntologyFileLint(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("glossmark.yaml", "owns:\n  pf: { iri: https://x/pf/, tier: core }\n")
	write("ontology/pf.gloss.yaml", "concepts:\n"+
		"  Account: { kind: entity, doc: One account at an institution (checking, savings, loan) }\n"+
		"  Institution: { kind: entity, doc: \"A bank, broker or lender.\" }\n"+
		"  Category:\n    kind: entity\n    doc: A label on a transaction.\n    constrains:\n")
	codes := func() map[string][]string {
		ws, err := load.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		res, _ := extract.Build(ws)
		rollup.Derive(res.Graph, ws.Vocab)
		out := map[string][]string{}
		for _, d := range check.Run(ws, res.Graph) {
			out[d.Code] = append(out[d.Code], d.Msg)
		}
		return out
	}
	got := codes()
	if len(got["GM305"]) != 1 || !strings.Contains(got["GM305"][0], "pf:Account") || !strings.Contains(got["GM305"][0], "savings") {
		t.Errorf("GM305 = %v", got["GM305"])
	}
	if len(got["GM304"]) != 1 || !strings.Contains(got["GM304"][0], "pf:Category") || !strings.Contains(got["GM304"][0], `"constrains"`) {
		t.Errorf("GM304 = %v", got["GM304"])
	}
	// Quoting the doc and dropping the empty key clears both.
	write("ontology/pf.gloss.yaml", "concepts:\n"+
		"  Account: { kind: entity, doc: \"One account at an institution (checking, savings, loan)\" }\n"+
		"  Institution: { kind: entity, doc: \"A bank, broker or lender.\" }\n"+
		"  Category:\n    kind: entity\n    doc: A label on a transaction.\n")
	got = codes()
	if len(got["GM304"])+len(got["GM305"]) != 0 {
		t.Errorf("after quoting: GM304=%v GM305=%v", got["GM304"], got["GM305"])
	}
	// gloss propose writes a quoted doc so the entry survives a later reflow into flow style.
	path := filepath.Join(root, "ontology", "pf.gloss.yaml")
	if err := ontoedit.AddConcept(path, "Holding", ontoedit.ConceptSpec{Kind: "entity", Doc: "A position: shares, bonds, cash.", Proposed: true, ProposedBy: "agent:x"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `doc: "A position: shares, bonds, cash."`) {
		t.Errorf("propose did not quote the doc:\n%s", b)
	}
	if got = codes(); len(got["GM304"])+len(got["GM305"]) != 0 {
		t.Errorf("after propose: GM304=%v GM305=%v", got["GM304"], got["GM305"])
	}
}

// TestActorDurability: the actor belongs to a marker's first appearance. A
// marker the ledger does not know takes the last built graph's actor, then
// GLOSS_ACTOR, then the git blame author, then the fallback; an existing
// entry keeps its actor whatever the session says.
func TestActorDurability(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Alice Example", "GIT_AUTHOR_EMAIL=a@x", "GIT_COMMITTER_NAME=Alice Example", "GIT_COMMITTER_EMAIL=a@x")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Doing: { kind: process }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  Doer: { kind: process, realizes: z:Doing }\n  Other: { kind: process, realizes: z:Doing }\n")
	write(".gloss.yaml", "boundary: o:b\nlevel: container\ndefault-namespace: i\n")
	write("a.py", "# @gloss implements .Doer\ndef go(x): pass\n")
	git("init", "-q")
	git("add", ".")
	git("commit", "-q", "-m", "committed marker")
	// b.py is not committed.
	write("b.py", "# @gloss? implements .Other conf=0.8\ndef other(): pass\n")

	build := func(actors *ledger.Actors, session string) (*ledger.Ledger, map[string]string) {
		ws, err := load.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		res, _ := extract.Build(ws)
		rollup.Derive(res.Graph, ws.Vocab)
		lg, _ := ledger.Open(filepath.Join(t.TempDir(), "ledger.jsonl"))
		lg.Actors = actors
		lg.Sync(res.Graph, res.Markers, session, time.Now())
		by := map[string]string{}
		for _, fm := range res.Markers {
			by[fm.Path] = lg.Entry(fm.ID).Actor
		}
		return lg, by
	}
	// No previous graph, no GLOSS_ACTOR: blame for the committed line, the fallback for the rest.
	_, by := build(&ledger.Actors{Root: root, Fallback: "human:runner"}, "human:runner")
	if by["a.py"] != "human:alice-example" || by["b.py"] != "human:runner" {
		t.Errorf("blame/fallback: %v", by)
	}
	// GLOSS_ACTOR set: it names the session's new markers, blame is not consulted.
	lg, by := build(&ledger.Actors{Root: root, Session: "agent:bot@m", Fallback: "human:runner"}, "agent:bot@m")
	if by["a.py"] != "agent:bot@m" || by["b.py"] != "agent:bot@m" {
		t.Errorf("session: %v", by)
	}
	// The built graph records those actors; a later build by someone else, with
	// no ledger, keeps them.
	ws, _ := load.Load(root)
	res, _ := extract.Build(ws)
	rollup.Derive(res.Graph, ws.Vocab)
	lg.Sync(res.Graph, res.Markers, "agent:bot@m", time.Now())
	previous := ledger.PreviousActors(res.Graph)
	if len(previous) != 2 {
		t.Fatalf("previous actors = %v", previous)
	}
	lg2, by := build(&ledger.Actors{Previous: previous, Root: root, Fallback: "human:bob"}, "human:bob")
	if by["a.py"] != "agent:bot@m" || by["b.py"] != "agent:bot@m" {
		t.Errorf("previous graph: %v", by)
	}
	if ch := lg2.ActorChanges(previous); len(ch) != 0 {
		t.Errorf("no actor should change: %+v", ch)
	}
	// An existing entry keeps its actor; a disagreement with the last graph is reported.
	for _, e := range lg2.Entries {
		if e.Path == "a.py" {
			e.Actor = "human:carol"
		}
	}
	ch := lg2.ActorChanges(previous)
	if len(ch) != 1 || ch[0].Path != "a.py" || ch[0].From != "agent:bot@m" || ch[0].To != "human:carol" {
		t.Errorf("actor changes = %+v", ch)
	}
}

// TestRedundantMarkersAndConceptCoverage: a symbol marker shadowed by a file
// marker (GM130) and the same marker on consecutive declarations (GM131) are
// reported; the coverage headline counts concepts with an artifact and a
// verifier, and the counters are per marker, not per target.
func TestRedundantMarkersAndConceptCoverage(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Doing: { kind: process }\n  Thing: { kind: entity }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  Doer: { kind: process, realizes: z:Doing }\n  Helper: { kind: process, realizes: z:Doing }\n  Store: { kind: resource, realizes: z:Thing }\n  Q: { kind: resource, internal: true }\n")
	write(".gloss.yaml", "boundary: o:b\nlevel: container\ndefault-namespace: i\n")
	write("app.py", "# @gloss implements .Doer scope=file\n\n# @gloss implements .Doer\ndef go(): pass\n\n# @gloss implements .Helper, .Store\ndef a(): pass\n\n# @gloss implements .Store, .Helper\ndef b(): pass\n\n# @gloss implements .Helper, .Store\ndef c(): pass\n\n# @gloss represents .Store\nclass S: pass\n")
	// Two in a row is honest and stays silent.
	write("pair.py", "# @gloss implements .Helper\ndef settle_batch(): pass\n\n# @gloss implements .Helper\ndef settle_single(): pass\n")
	write("tests/test_app.py", "# @gloss verifies .Doer\ndef test_go(): pass\n")
	ws, g, _, lg := build(t, root)
	codes := map[string][]int{}
	for _, d := range check.Run(ws, g) {
		codes[d.Code] = append(codes[d.Code], d.Line)
	}
	if len(codes["GM130"]) != 1 || codes["GM130"][0] != 3 {
		t.Errorf("GM130 lines = %v, want [3]", codes["GM130"])
	}
	if len(codes["GM131"]) != 1 || codes["GM131"][0] != 6 {
		t.Errorf("GM131 lines = %v, want [6]: one finding at the start of the three-marker run (targets in any order), none for the pair", codes["GM131"])
	}
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	m := rep.Metrics
	// Doer has an artifact and a verifier; Helper and Store have artifacts only; Q is internal and out of scope.
	if m.ConceptsInScope != 3 || m.ConceptsCovered != 1 || m.ConceptCoveragePct < 33 || m.ConceptCoveragePct > 34 {
		t.Errorf("concept coverage = %d/%d (%.1f%%)", m.ConceptsCovered, m.ConceptsInScope, m.ConceptCoveragePct)
	}
	// Nine marker lines state twelve targets; every one is accepted.
	if m.Markers != 9 || m.Targets != 12 || m.Accepted != 9 || m.Proposed != 0 {
		t.Errorf("markers=%d targets=%d accepted=%d proposed=%d", m.Markers, m.Targets, m.Accepted, m.Proposed)
	}
}

// TestRepeatedMarkerStacked: GM131 groups markers by the declaration they
// anchor to, so a repeated marker is found under whatever else is stacked
// on each declaration, and every repeated target is named (issue #75).
func TestRepeatedMarkerStacked(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Doing: { kind: process }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  Auth: { kind: process, realizes: z:Doing }\n  Log: { kind: process, realizes: z:Doing }\n  A: { kind: process, realizes: z:Doing }\n  B: { kind: process, realizes: z:Doing }\n  C: { kind: process, realizes: z:Doing }\n")
	write(".gloss.yaml", "boundary: o:b\nlevel: container\ndefault-namespace: i\n")
	// Two identical markers stacked on each of three handlers, each also
	// carrying its own domain concept.
	var b strings.Builder
	for _, h := range []string{"A", "B", "C"} {
		fmt.Fprintf(&b, "# @gloss implements .%s\n# @gloss implements .Auth\n# @gloss implements .Log\ndef handle_%s(): pass\n\n", h, strings.ToLower(h))
	}
	write("handlers.py", b.String())
	ws, g, _, _ := build(t, root)
	var got []string
	for _, d := range check.Run(ws, g) {
		if d.Code == "GM131" {
			got = append(got, fmt.Sprintf("%d %s", d.Line, d.Msg))
		}
	}
	if len(got) != 1 || !strings.HasPrefix(got[0], "2 implements i:Auth, i:Log is repeated on 3 consecutive declarations (lines 2-13)") {
		t.Errorf("GM131 = %v, want one finding at line 2 naming i:Auth and i:Log", got)
	}
}

// TestVerifiesCanonical: the test-path inferrer emits verifies, and an inline
// tests marker is accepted with a prefer-verifies warning (GM240) and stored
// as verifies, with the written form in prov.alias.
func TestVerifiesCanonical(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Doing: { kind: process }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  Doer: { kind: process, realizes: z:Doing }\n  Other: { kind: process, realizes: z:Doing }\n")
	write(".gloss.yaml", "boundary: o:b\nlevel: container\ndefault-namespace: i\n")
	write("app.py", "# @gloss implements .Doer\ndef go(): pass\n\n# @gloss implements .Other\ndef other(): pass\n")
	write("tests/test_app.py", "# @gloss implements .Doer\ndef test_go(): pass\n\n# @gloss tests .Other\ndef test_other(): pass\n")
	ws, g, _, _ := build(t, root)
	doer := g.NodeByCurie("i:Doer")
	if len(g.In(doer.ID, "verifies")) == 0 || len(g.In(doer.ID, "tests")) != 0 {
		t.Errorf("inferrer should emit verifies, not tests: verifies=%d tests=%d", len(g.In(doer.ID, "verifies")), len(g.In(doer.ID, "tests")))
	}
	other := g.NodeByCurie("i:Other")
	var vs []*graph.Edge
	for _, e := range g.In(other.ID, "verifies") {
		if e.Source == graph.SourceInline {
			vs = append(vs, e)
		}
	}
	if len(vs) != 1 || vs[0].Prov.Alias != "tests" {
		t.Errorf("tests marker should be stored as one inline verifies with alias tests: %+v", vs)
	}
	for _, e := range g.Edges() {
		if e.P == "tests" {
			t.Errorf("no edge should carry the alias as predicate: %s %s %s (%s)", e.S, e.P, e.O, e.Source)
		}
	}
	var gm240 []int
	for _, d := range check.Run(ws, g) {
		if d.Severity == "error" {
			t.Errorf("unexpected error: %s", d)
		}
		if d.Code == "GM240" {
			gm240 = append(gm240, d.Line)
		}
	}
	if len(gm240) != 1 || gm240[0] != 4 {
		t.Errorf("GM240 lines = %v, want [4]", gm240)
	}
	// tests still counts: Other is not untested.
	rep := rollup.Compare(g, ws, rollup.Options{Policy: ws.Config.EffectivePolicy()})
	for _, f := range findings(rep, rollup.KindUntested) {
		if f.Concept == "i:Other" || f.Concept == "i:Doer" {
			t.Errorf("%s should count as tested: %s", f.Concept, f.Msg)
		}
	}
}

// TestOneDiagnosticPerKey: an undeclared relation with several targets is
// one GM200 per ontology key, and a tests marker with several targets is one
// GM240 (issue #82).
func TestOneDiagnosticPerKey(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  A: { kind: process }\n  B: { kind: process }\n  C: { kind: process }\n  Auth:\n    kind: process\n    guards: [A, B, C]\n  Log:\n    kind: process\n    guards: [A, B]\n")
	write("ontology/i.gloss.yaml", "concepts:\n  Doer: { kind: process, realizes: z:A }\n")
	write(".gloss.yaml", "boundary: o:b\nlevel: container\ndefault-namespace: i\n")
	write("app.py", "# @gloss implements .Doer\ndef go(): pass\n")
	write("tests/test_app.py", "# @gloss tests z:A, z:B, z:C\ndef test_go(): pass\n")
	ws, err := load.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := extract.Build(ws)
	if err != nil {
		t.Fatal(err)
	}
	rollup.Derive(res.Graph, ws.Vocab)
	codes := map[string][]int{}
	for _, d := range check.Run(ws, res.Graph) {
		codes[d.Code] = append(codes[d.Code], d.Line)
	}
	if got := codes["GM200"]; len(got) != 2 {
		t.Errorf("GM200 lines = %v, want one per guards: key", got)
	}
	if got := codes["GM240"]; len(got) != 1 || got[0] != 1 {
		t.Errorf("GM240 lines = %v, want [1]", got)
	}
}

// TestDerivedAllowed: a manifest without allowed: gets a derived set (default
// namespace, declared concepts, descendants, realized cores, nested
// boundaries); a link outside it is GM503 in check and HARD in the gate. A
// written list is a deliberate narrowing (GM502).
func TestDerivedAllowed(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Doing: { kind: process }\n  Thing: { kind: entity }\n  Part: { kind: entity, part-of: Thing }\n  Sub: { kind: entity, is-a: Thing }\n  Elsewhere: { kind: entity }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  Doer: { kind: process, realizes: z:Doing }\n")
	write(".gloss.yaml", "boundary: o:root\nlevel: system\n")
	write("svc/.gloss.yaml", "boundary: o:svc\nlevel: container\npart-of: o:root\ndefault-namespace: i\nrealizes: [i:Doer]\nproduces: [z:Thing]\n")
	write("svc/a.py", "# @gloss implements .Doer\ndef go(): pass\n\n# @gloss represents z:Part\nclass P: pass\n\n# @gloss represents z:Sub\nclass S: pass\n\n# @gloss produces z:Doing\ndef d(): pass\n\n# @gloss represents z:Elsewhere\nclass E: pass\n")
	write("top.sql", "-- @gloss represents z:Sub\nCREATE TABLE sub (id int);\n")
	ws, g, _, _ := build(t, root)
	svc := ws.ManifestFor("svc/a.py")
	a := rollup.AllowedFor(g, ws, svc)
	if !a.Derived || !has(a.Patterns, "i:*") || !has(a.Patterns, "z:Thing") || !has(a.Patterns, "z:Part") || !has(a.Patterns, "z:Sub") || !has(a.Patterns, "z:Doing") || has(a.Patterns, "z:Elsewhere") {
		t.Errorf("derived allowed = %+v", a)
	}
	// The root inherits what its nested boundary allows.
	if ra := rollup.AllowedFor(g, ws, ws.ManifestFor("top.sql")); !ra.Derived || !has(ra.Patterns, "z:Sub") || !has(ra.Patterns, "i:*") {
		t.Errorf("root allowed = %+v", ra)
	}
	codes := map[string][]string{}
	for _, d := range check.Run(ws, g) {
		codes[d.Code] = append(codes[d.Code], d.Msg)
	}
	if len(codes["GM503"]) != 1 || !strings.Contains(codes["GM503"][0], "z:Elsewhere") || len(codes["GM502"]) != 0 {
		t.Errorf("GM503=%v GM502=%v", codes["GM503"], codes["GM502"])
	}
	if ex := check.ExplainAllowed(ws, g); !strings.Contains(ex, "o:svc (svc/.gloss.yaml): allowed = i:*, i:Doer, z:Doing, z:Part, z:Sub, z:Thing") || !strings.Contains(ex, "[derived") {
		t.Errorf("explain:\n%s", ex)
	}
	// Gate: the Elsewhere link is HARD with the derived-set reason; Part is SOFT.
	for _, e := range g.Edges() {
		if e.Source != graph.SourceInline || e.P != "represents" {
			continue
		}
		cls, why := gate.ClassifyLink(g, ws, e, ws.Config.EffectivePolicy())
		switch g.Curie(e.O) {
		case "z:Elsewhere":
			if cls != gate.HARD || !strings.Contains(strings.Join(why, ";"), "derived") {
				t.Errorf("Elsewhere: %s %v", cls, why)
			}
		case "z:Part":
			if cls != gate.SOFT {
				t.Errorf("Part: %s %v", cls, why)
			}
		}
	}
	// allowed: [derived, z:Elsewhere] keeps the derived set and adds one concept.
	write("svc/.gloss.yaml", "boundary: o:svc\nlevel: container\npart-of: o:root\ndefault-namespace: i\nrealizes: [i:Doer]\nproduces: [z:Thing]\nallowed: [derived, z:Elsewhere]\n")
	ws, g, _, _ = build(t, root)
	if a := rollup.AllowedFor(g, ws, ws.ManifestFor("svc/a.py")); !a.Derived || !has(a.Patterns, "z:Elsewhere") || !has(a.Patterns, "z:Part") {
		t.Errorf("derived plus extras = %+v", a)
	}
	for _, d := range check.Run(ws, g) {
		if d.Code == "GM503" || d.Code == "GM502" || d.Code == "GM501" {
			t.Errorf("allowed: [derived, ...]: %s", d)
		}
	}
	// A written list narrows: only i:* now, so z:Part becomes GM502.
	write("svc/.gloss.yaml", "boundary: o:svc\nlevel: container\npart-of: o:root\ndefault-namespace: i\nrealizes: [i:Doer]\nproduces: [z:Thing]\nallowed: [i:*]\n")
	ws, g, _, _ = build(t, root)
	codes = map[string][]string{}
	for _, d := range check.Run(ws, g) {
		codes[d.Code] = append(codes[d.Code], d.Msg)
	}
	if len(codes["GM502"]) != 4 || len(codes["GM503"]) != 0 {
		t.Errorf("after narrowing: GM502=%v GM503=%v", codes["GM502"], codes["GM503"])
	}
}

// TestAdoptionDiff: a diff whose base has no graph is reported as counts per
// boundary and per view with only the decision requests listed; the Markdown
// comment groups changes by boundary; a HARD link never carries the SOFT
// eligibility text; an owned namespace is "added", not a cross-repo import.
func TestAdoptionDiff(t *testing.T) {
	ws, head, _, lg := build(t, example(t, "horse-factory"))
	pol := ws.Config.EffectivePolicy()
	after := rollup.Compare(head, ws, rollup.Options{Ledger: lg.Entries, Policy: pol})
	empty := graph.New(ws.Prefixes)
	im := gate.Classify(empty, head, ws, gate.Options{Base: "main (no graph)", Policy: pol, HeadLedger: lg.Entries, AfterReport: after, Adoption: true})
	a := im.Adoption
	if a == nil || a.Concepts == 0 || a.Markers == 0 || a.Rogues != 1 || a.Proposed != 3 || len(a.PerBoundary) < 2 || len(a.PerView) != len(ws.Views.Views) || a.Hidden == 0 {
		t.Fatalf("adoption summary = %+v", a)
	}
	for _, ch := range im.Changes {
		if ch.Kind != "new-concept" && ch.Kind != "concept-proposed" {
			t.Errorf("adoption keeps only decision requests, got %s %s", ch.Kind, ch.Msg)
		}
	}
	md := gate.RenderMarkdown(im)
	if n := strings.Count(md, "\n"); n > 60 {
		t.Errorf("adoption comment is %d lines; want under 60", n)
	}
	if !strings.Contains(md, "Decisions requested") || !strings.Contains(md, "acme:horse-factory-svc") || strings.Contains(md, "cross-repo") {
		t.Errorf("markdown:\n%s", md)
	}
	// Without adoption mode the same diff lists every link, grouped by boundary, and names the owned namespaces as added.
	full := gate.Classify(empty, head, ws, gate.Options{Base: "main", Policy: pol, HeadLedger: lg.Entries, AfterReport: after})
	if full.Adoption != nil || len(full.Changes) <= len(im.Changes) {
		t.Fatalf("full diff: adoption=%v changes=%d", full.Adoption != nil, len(full.Changes))
	}
	var nsAdded bool
	for _, ch := range full.Changes {
		if ch.Gate == gate.HARD && strings.Contains(strings.Join(ch.Reasons, ";"), "inside allowed") {
			t.Errorf("HARD change carries the SOFT reason: %+v", ch)
		}
		if ch.Kind == "namespace-added" {
			nsAdded = true
			if strings.Contains(strings.Join(ch.Reasons, ";"), "cross-repo") {
				t.Errorf("owned namespace reported as cross-repo import: %+v", ch)
			}
		}
	}
	if !nsAdded {
		t.Error("expected namespace-added changes")
	}
	md = gate.RenderMarkdown(full)
	if !strings.Contains(md, "<details open><summary><b>acme:horse-factory-svc</b>") || !strings.Contains(md, "<b>Ontology, manifests and glossmark.yaml</b>") {
		t.Errorf("grouped markdown:\n%s", md)
	}
}

// TestInfraAndDataSatisfyDeclarations: provisions, configures and deploys on
// a resource or system-type lift to realizes; represents on an entity lifts
// to holds; a manifest can declare both and gloss drift compares them.
func TestInfraAndDataSatisfyDeclarations(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Host: { kind: resource }\n  Shop: { kind: system-type }\n  Order: { kind: entity }\n  Other: { kind: entity }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  AwsHost: { kind: resource, realizes: z:Host }\n  OrdersTable: { kind: entity, realizes: z:Order }\n")
	write("deploy/.gloss.yaml", "boundary: o:deploy\nlevel: container\nrealizes: [z:Host, z:Shop]\n")
	write("deploy/main.tf", "# @gloss provisions i:AwsHost\nresource \"aws_instance\" \"host\" {}\n")
	write("deploy/ci.yml", "# @gloss deploys z:Shop scope=file\nname: deploy\n")
	write("db/.gloss.yaml", "boundary: o:db\nlevel: container\nholds: [z:Order]\n")
	write("db/0001.sql", "-- @gloss represents i:OrdersTable\nCREATE TABLE orders (id int);\n-- @gloss represents z:Other\nCREATE TABLE other (id int);\n")
	ws, g, _, lg := build(t, root)
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	if gaps := findings(rep, rollup.KindGap); len(gaps) != 0 {
		t.Errorf("infra and data links should satisfy the declarations; gaps: %+v", gaps)
	}
	viol := findings(rep, rollup.KindViolation)
	if len(viol) != 1 || viol[0].Boundary != "o:db" || viol[0].Relation != "holds" || viol[0].Concept != "z:Other" {
		t.Errorf("want one holds violation for z:Other, got %+v", viol)
	}
	deploy := g.NodeByCurie("o:deploy")
	if r := g.Objects(deploy.ID, "realizes"); !has(curies(g, r), "i:AwsHost") || !has(curies(g, r), "z:Host") || !has(curies(g, r), "z:Shop") {
		t.Errorf("deploy realizes = %v", curies(g, r))
	}
	for _, d := range check.Run(ws, g) {
		if d.Severity == "error" {
			t.Errorf("check error: %s", d)
		}
	}
}

func curies(g *graph.Graph, ids []string) []string {
	var out []string
	for _, id := range ids {
		out = append(out, g.Curie(id))
	}
	return out
}

// TestRealizesInheritance: a concept inherits realizes through is-a, a part
// realizes its whole, a container can write realizes: inherit, and the
// criticality understatement check (GM600) sees the closure.
func TestRealizesInheritance(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("glossmark.yaml", "owns:\n  pf: { iri: https://x/pf/, tier: core }\n  pfi: { iri: https://x/pfi/, tier: implementation }\n")
	write("ontology/pf.gloss.yaml", "concepts:\n  Importing: { kind: process, criticality: high }\n  Analysis: { kind: process }\n")
	write("ontology/pfi.gloss.yaml", "concepts:\n"+
		"  Importer: { kind: process, realizes: pf:Importing }\n"+
		"  CsvImporter: { kind: process, is-a: Importer }\n"+
		"  Pipeline: { kind: process, realizes: pf:Analysis }\n"+
		"  Step: { kind: process, part-of: Pipeline }\n")
	write(".gloss.yaml", "boundary: o:lib\nlevel: system\nrealizes: inherit\ncriticality: low\n")
	write("importers/.gloss.yaml", "boundary: o:importers\nlevel: container\npart-of: o:lib\ndefault-namespace: pfi\nrealizes: [pfi:Importer]\n")
	write("importers/csv.py", "# @gloss implements .CsvImporter\ndef run(): pass\n")
	write("analytics/.gloss.yaml", "boundary: o:analytics\nlevel: container\npart-of: o:lib\ndefault-namespace: pfi\nrealizes: [pfi:Pipeline]\n")
	write("analytics/step.py", "# @gloss implements .Step\ndef step(): pass\n")
	ws, g, _, lg := build(t, root)
	// GM320 does not fire for CsvImporter (is-a carries realizes); GM600 sees o:lib understating pf:Importing through the closure.
	codes := map[string][]string{}
	for _, d := range check.Run(ws, g) {
		codes[d.Code] = append(codes[d.Code], d.Msg)
	}
	if len(codes["GM320"]) != 0 {
		t.Errorf("GM320 = %v", codes["GM320"])
	}
	if len(codes["GM600"]) == 0 || !strings.Contains(strings.Join(codes["GM600"], ";"), "pf:Importing") {
		t.Errorf("GM600 should report o:lib (low) housing pf:Importing (high) through CsvImporter is-a Importer: %v", codes["GM600"])
	}
	// The closure: CsvImporter stands for Importer's core; Step for Pipeline and its core.
	if r := rollup.RealizesStar(g, g.NodeByCurie("pfi:CsvImporter").ID); !has(curies(g, r), "pf:Importing") {
		t.Errorf("CsvImporter realizes* = %v", curies(g, r))
	}
	if r := rollup.RealizesStar(g, g.NodeByCurie("pfi:Step").ID); !has(curies(g, r), "pfi:Pipeline") || !has(curies(g, r), "pf:Analysis") {
		t.Errorf("Step realizes* = %v", curies(g, r))
	}
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	if v := findings(rep, rollup.KindViolation); len(v) != 0 {
		t.Errorf("no violation expected (CsvImporter covers Importer, Step covers Pipeline, lib inherits): %+v", v)
	}
	if gp := findings(rep, rollup.KindGap); len(gp) != 0 {
		t.Errorf("no gap expected: %+v", gp)
	}
	// The inherited parent is compared: a child declaring something nothing implements is the child's gap, not the parent's.
	write("analytics/.gloss.yaml", "boundary: o:analytics\nlevel: container\npart-of: o:lib\ndefault-namespace: pfi\nrealizes: [pfi:Pipeline, pf:Importing]\n")
	ws, g, _, lg = build(t, root)
	rep = rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	gaps := findings(rep, rollup.KindGap)
	if len(gaps) != 1 || gaps[0].Boundary != "o:analytics" {
		t.Errorf("gaps = %+v, want one on o:analytics only", gaps)
	}
	// And an unlisted realization in the parent is still a violation there.
	write("extra.py", "# @gloss implements pfi:Pipeline\ndef x(): pass\n")
	write(".gloss.yaml", "boundary: o:lib\nlevel: system\nrealizes: [inherit]\ncriticality: low\n")
	write("importers/.gloss.yaml", "boundary: o:importers\nlevel: container\npart-of: o:lib\ndefault-namespace: pfi\nrealizes: [pfi:Importer]\n")
	write("analytics/.gloss.yaml", "boundary: o:analytics\nlevel: container\npart-of: o:lib\ndefault-namespace: pfi\nrealizes: [pfi:Importer]\n")
	ws, g, _, lg = build(t, root)
	rep = rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	var libViol []string
	for _, v := range findings(rep, rollup.KindViolation) {
		if v.Boundary == "o:lib" {
			libViol = append(libViol, v.Concept)
		}
	}
	if !has(libViol, "pfi:Pipeline") {
		t.Errorf("o:lib should report pfi:Pipeline as a violation (nobody declares it): %v", libViol)
	}
}

// TestPresentsAndHosts: a page presents a concept and a shell hosts one;
// neither lifts to realizes or carries criticality. untested tells the
// tested engine from the untested page, and a surface with no mechanism
// behind it is reported (presented-only).
func TestPresentsAndHosts(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("glossmark.yaml", "owns:\n  pf: { iri: https://x/pf/, tier: core }\n  pfi: { iri: https://x/pfi/, tier: implementation }\n")
	write("ontology/pf.gloss.yaml", "concepts:\n  Finance: { kind: system-type }\n  FirePlanning: { kind: process, criticality: critical }\n  Budgeting: { kind: process }\n")
	write("ontology/pfi.gloss.yaml", "concepts:\n"+
		"  FireEngine: { kind: process, realizes: pf:FirePlanning }\n"+
		"  FirePage: { kind: process, presents: pf:FirePlanning }\n"+
		"  BudgetPage: { kind: process, presents: pf:Budgeting }\n"+
		"  WebApp: { kind: system-type, hosts: pf:Finance }\n")
	write("engine/.gloss.yaml", "boundary: o:engine\nlevel: container\ndefault-namespace: pfi\nrealizes: [pfi:FireEngine]\n")
	write("engine/fire.py", "# @gloss implements .FireEngine\ndef fire(): pass\n")
	write("engine/tests/test_fire.py", "# @gloss verifies .FireEngine\ndef test_fire(): pass\n")
	write("web/.gloss.yaml", "boundary: o:web\nlevel: container\ndefault-namespace: pfi\npresents: [pf:FirePlanning, pf:Budgeting]\nhosts: [pf:Finance]\n")
	write("web/FirePage.tsx", "// @gloss implements .FirePage\nexport function FirePage() {}\n")
	write("web/BudgetPage.tsx", "// @gloss implements .BudgetPage\nexport function BudgetPage() {}\n")
	write("web/main.tsx", "// @gloss implements .WebApp scope=file\nrender(App)\n")
	ws, g, _, lg := build(t, root)
	for _, d := range check.Run(ws, g) {
		if d.Severity == "error" || d.Code == "GM320" {
			t.Errorf("check: %s", d)
		}
	}
	web := g.NodeByCurie("o:web")
	if r := curies(g, g.Objects(web.ID, "realizes")); has(r, "pf:FirePlanning") || has(r, "pf:Finance") {
		t.Errorf("web must not realize what it presents or hosts: %v", r)
	}
	if p := curies(g, g.Objects(web.ID, "presents")); !has(p, "pf:FirePlanning") || !has(p, "pf:Budgeting") {
		t.Errorf("web presents = %v", p)
	}
	if h := curies(g, g.Objects(web.ID, "hosts")); !has(h, "pf:Finance") {
		t.Errorf("web hosts = %v", h)
	}
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	if v := findings(rep, rollup.KindViolation); len(v) != 0 {
		t.Errorf("violations: %+v", v)
	}
	if gp := findings(rep, rollup.KindGap); len(gp) != 0 {
		t.Errorf("gaps: %+v", gp)
	}
	untested := concepts(findings(rep, rollup.KindUntested))
	if !has(untested, "pfi:FirePage") || has(untested, "pfi:FireEngine") || has(untested, "pf:FirePlanning") {
		t.Errorf("untested = %v: the engine is tested, the page is not", untested)
	}
	if po := concepts(findings(rep, rollup.KindPresentedOnly)); len(po) != 1 || po[0] != "pf:Budgeting" {
		t.Errorf("presented-only = %v, want [pf:Budgeting]", po)
	}
	// Criticality follows the mechanism, not the page.
	rollup.Criticality(g)
	for _, a := range g.NodesOf(model.NodeArtifact) {
		switch a.Path {
		case "engine/fire.py":
			if a.Criticality != "critical" {
				t.Errorf("engine artifact criticality = %q", a.Criticality)
			}
		case "web/FirePage.tsx":
			if a.Criticality == "critical" {
				t.Errorf("page artifact inherited critical from what it presents (via %s)", a.CriticalityFrom)
			}
		}
	}
}

// TestBriefingV2: the briefing ranks concepts by relevance to the directory,
// respects a token budget, shows only the acting agent's remit, and states
// the one proposal sentence that AGENTS.md and the MCP server also use.
func TestBriefingV2(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	var core strings.Builder
	core.WriteString("concepts:\n  Grading: { kind: process }\n  Gate: { kind: constraint }\n  Admin: { kind: entity }\n  Agent: { kind: entity }\n  AgentArena: { kind: system-type }\n")
	for i := 0; i < 150; i++ {
		fmt.Fprintf(&core, "  Filler%03d: { kind: entity, doc: \"A filler concept that pads the alphabetical list well past any budget.\" }\n", i)
	}
	write("glossmark.yaml", "owns:\n  arena: { iri: https://x/arena/, tier: core }\n  aa: { iri: https://x/aa/, tier: implementation }\n")
	write("ontology/arena.gloss.yaml", core.String())
	write("ontology/aa.gloss.yaml", "concepts:\n  GradingPipeline: { kind: process, realizes: arena:Grading }\n  Console: { kind: process, realizes: arena:Admin }\n")
	write(".gloss.yaml", "boundary: o:arena\nlevel: system\n")
	write("grading/.gloss.yaml", "boundary: o:grading\nlevel: container\npart-of: o:arena\ndefault-namespace: aa\nrealizes: [aa:GradingPipeline]\nallowed: [arena:*, aa:*]\n")
	write("grading/pipeline.py", "# @gloss implements .GradingPipeline\ndef grade(): pass\n\n# @gloss implements arena:Gate\ndef gate(): pass\n")
	write("admin/console.py", "# @gloss implements aa:Console\ndef console(): pass\n")
	write(".claude/agents/grader.md", "<!-- @gloss may-modify aa:Grading*, arena:Grading scope=file -->\n# Grader\n")
	write(".claude/agents/admin-bot.md", "<!-- @gloss may-modify arena:Admin scope=file -->\n# Admin bot\n")
	ws, g, _, lg := build(t, root)
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	m := ws.ManifestFor("grading/x")
	out := agents.Context(ws, g, m, rep, agents.ContextOptions{Dir: "grading", Budget: 2000, Actor: "agent:grader@m"})
	idx := func(c string) int {
		i := strings.Index(out, "- "+c+" (")
		if i < 0 {
			return 1 << 30
		}
		return i
	}
	if !(idx("arena:Grading") < idx("arena:Admin") && idx("arena:Gate") < idx("arena:Admin") && idx("aa:GradingPipeline") < idx("arena:Admin")) {
		t.Errorf("grading concepts should come before arena:Admin:\n%s", out)
	}
	if len(out) > 2000*4 {
		t.Errorf("briefing is %d chars, over the 2000-token budget", len(out))
	}
	if !strings.Contains(out, "use search_concepts for the rest") {
		t.Errorf("the budget should cut the 150 filler concepts")
	}
	if !strings.Contains(out, "Your remit as agent:grader@m") || strings.Contains(out, "admin-bot") {
		t.Errorf("remit should be the grader's only:\n%s", out)
	}
	if !strings.Contains(out, agents.ProposalSentence) || !strings.Contains(agents.Snippet(ws, g, m), agents.ProposalSentence) {
		t.Error("briefing and AGENTS.md snippet must state the same proposal sentence")
	}
	// Without a named agent every remit is listed.
	if all := agents.Context(ws, g, m, rep, agents.ContextOptions{Dir: "grading", Budget: 2000}); !strings.Contains(all, "admin-bot") || !strings.Contains(all, "grader") {
		t.Errorf("all remits expected without GLOSS_ACTOR")
	}
}

// TestViewContainment: a concept goes into the boundary whose own code links
// to it, not the common parent of every boundary that reaches it by lifting;
// a curated view (one that names concepts) is flagged for the viewer to open
// expanded and keeps only the artifacts and edges around what it names.
func TestViewContainment(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Thing: { kind: process }\n  Other: { kind: entity }\n  Elsewhere: { kind: entity, part-of: Other }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  Engine: { kind: process, realizes: z:Thing }\n  Step: { kind: process, part-of: Engine }\n")
	write("ontology/views.yaml", "views:\n  all:\n    include: [z:*, i:*]\n    zoom: code\n  thing:\n    include: [z:Thing, i:Engine]\n    zoom: code\n")
	write(".gloss.yaml", "boundary: o:root\nlevel: system\n")
	write("a/.gloss.yaml", "boundary: o:a\nlevel: container\npart-of: o:root\n")
	write("a/engine.py", "# @gloss implements i:Engine\ndef run(): pass\n")
	write("b/.gloss.yaml", "boundary: o:b\nlevel: container\npart-of: o:root\n")
	write("b/step.py", "# @gloss implements i:Step\ndef step(): pass\n\n# @gloss represents z:Other\nclass O: pass\n\n# @gloss represents z:Elsewhere\nclass E: pass\n")
	ws, g, _, _ := build(t, root)
	ps := views.ProjectAll(g, ws.Views)
	byName := map[string]*views.Projection{}
	for _, p := range ps {
		byName[p.Name] = p
	}
	node := func(p *views.Projection, id string) *views.PNode {
		for i := range p.Nodes {
			if p.Nodes[i].ID == id {
				return &p.Nodes[i]
			}
		}
		return nil
	}
	all := byName["all"]
	if all.Curated {
		t.Error("a glob-only view is not curated")
	}
	// b reaches i:Engine only by lifting through i:Step; a implements it directly.
	if n := node(all, "i:Engine"); n == nil || n.Boundary != "o:a" {
		t.Errorf("i:Engine boundary = %+v, want o:a", n)
	}
	th := byName["thing"]
	if !th.Curated {
		t.Error("a view naming concepts is curated")
	}
	for _, n := range th.Nodes {
		if n.Type == "artifact" && n.Path != "a/engine.py" {
			t.Errorf("curated view kept an unrelated artifact: %s", n.ID)
		}
	}
	for _, e := range th.Edges {
		if e.S == "z:Elsewhere" || e.O == "z:Other" {
			t.Errorf("curated view kept an edge between concepts it does not name: %+v", e)
		}
	}
	if n := node(th, "a/engine.py#run"); n == nil || n.Line != 2 {
		t.Errorf("artifact line = %+v", n)
	}
}

// TestValueFrom: a published value bound to code with value-from (or a
// publishes-value marker) is baselined on first build; changing the code
// line alone is a value-drift finding and a HARD change in the diff;
// changing both together moves the baseline; accept --value confirms.
func TestValueFrom(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("glossmark.yaml", "owns:\n  pf: { iri: https://x/pf/, tier: core }\n  pfi: { iri: https://x/pfi/, tier: implementation }\n")
	write("ontology/pf.gloss.yaml", "concepts:\n  Withdrawal: { kind: constraint }\n  Ceiling: { kind: constraint }\n")
	ont := "concepts:\n  DefaultSafeWithdrawalRate:\n    kind: measure\n    realizes: pf:Withdrawal\n    published: { as: pf:Withdrawal }\n    value: \"4% (25x annual expenses)\"\n    value-from: { path: pfinance/fire.py, symbol: swr }\n" +
		"  SpendingCeiling: { kind: measure, realizes: pf:Ceiling, value: \"10000\" }\n"
	write("ontology/pfi.gloss.yaml", ont)
	write(".gloss.yaml", "boundary: o:pf\nlevel: system\ndefault-namespace: pfi\n")
	code := "def plan(years: int, swr: float = 0.04):\n    return years\n\n# @gloss publishes-value .SpendingCeiling scope=line\nCEILING = 10000\n"
	write("pfinance/fire.py", code)
	lgPath := filepath.Join(root, ".glossmark", "ledger.jsonl")
	run := func() (*load.Workspace, *graph.Graph, *ledger.Ledger) {
		t.Helper()
		ws, err := load.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		res, _ := extract.Build(ws)
		for _, d := range res.Diags {
			if d.Severity == "error" {
				t.Errorf("extract: %s", d)
			}
		}
		rollup.Derive(res.Graph, ws.Vocab)
		lg, _ := ledger.Open(lgPath)
		lg.Sync(res.Graph, res.Markers, "human:alice", time.Now())
		lg.Save()
		return ws, res.Graph, lg
	}
	drift := func(ws *load.Workspace, g *graph.Graph, lg *ledger.Ledger) []string {
		return concepts(findings(rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries}), rollup.KindValueDrift))
	}
	ws, base, lg := run()
	n := base.NodeByCurie("pfi:DefaultSafeWithdrawalRate")
	if n.ValueFrom != "pfinance/fire.py:1" || n.LiveValue != "def plan(years: int, swr: float = 0.04):" {
		t.Fatalf("binding = %q %q", n.ValueFrom, n.LiveValue)
	}
	if c := base.NodeByCurie("pfi:SpendingCeiling"); c.ValueFrom != "pfinance/fire.py:5" || c.LiveValue != "CEILING = 10000" {
		t.Fatalf("marker binding = %q %q", c.ValueFrom, c.LiveValue)
	}
	if d := drift(ws, base, lg); len(d) != 0 {
		t.Errorf("baseline drift = %v", d)
	}
	// Code changes, ontology does not: drift, and HARD in the diff.
	write("pfinance/fire.py", strings.Replace(code, "0.04", "0.035", 1))
	ws, head, lg := run()
	if d := drift(ws, head, lg); len(d) != 1 || d[0] != "pfi:DefaultSafeWithdrawalRate" {
		t.Errorf("drift = %v", d)
	}
	im := gate.Classify(base, head, ws, gate.Options{Base: "base", Policy: ws.Config.EffectivePolicy()})
	hard := false
	for _, ch := range im.Changes {
		if ch.Kind == "published-value-drifted" && ch.Gate == gate.HARD && ch.Subject == "pfi:DefaultSafeWithdrawalRate" {
			hard = true
		}
	}
	if !hard || im.Gate != gate.HARD {
		t.Errorf("diff should be HARD published-value-drifted: %+v", im.Changes)
	}
	// Confirming the code as is clears the drift.
	lg.Accept("value:pfi:DefaultSafeWithdrawalRate", "human:bob", head.NodeByCurie("pfi:DefaultSafeWithdrawalRate").ValueFP, time.Now())
	lg.Save()
	ws, head, lg = run()
	if d := drift(ws, head, lg); len(d) != 0 {
		t.Errorf("after accept drift = %v", d)
	}
	// Code and value move together: no drift, a new baseline.
	write("pfinance/fire.py", strings.Replace(strings.Replace(code, "0.04", "0.035", 1), "10000", "12000", 1))
	write("ontology/pfi.gloss.yaml", strings.Replace(ont, `value: "10000"`, `value: "12000"`, 1))
	ws, head, lg = run()
	if d := drift(ws, head, lg); len(d) != 0 {
		t.Errorf("moving together should not drift: %v", d)
	}
	// A symbol that is never assigned is an error.
	write("ontology/pfi.gloss.yaml", strings.Replace(ont, "symbol: swr", "symbol: nope", 1))
	ws2, _ := load.Load(root)
	res, _ := extract.Build(ws2)
	gm360 := false
	for _, d := range res.Diags {
		if d.Code == "GM360" {
			gm360 = true
		}
	}
	if !gm360 {
		t.Error("expected GM360 for an unassigned symbol")
	}
}

// TestCriticalityFloorAndSuggest: an artifact takes the level of the concept
// it implements; a boundary's level reaches its artifacts only with
// criticality-floor: true; suggest proposes high for constraints, published
// concepts and concepts verified only by security tests.
func TestCriticalityFloorAndSuggest(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("glossmark.yaml", "owns:\n  pay: { iri: https://x/pay/, tier: core }\n  pa: { iri: https://x/pa/, tier: implementation }\n")
	write("ontology/pay.gloss.yaml", "concepts:\n  Payment: { kind: entity }\n  Metrics: { kind: measure, criticality: low }\n  Cutoff: { kind: constraint }\n  Login: { kind: process }\n")
	write("ontology/pa.gloss.yaml", "concepts:\n  Exporter: { kind: process, realizes: pay:Metrics }\n  Clock: { kind: measure, realizes: pay:Cutoff, published: { as: pay:Cutoff } }\n  Session: { kind: process, realizes: pay:Login }\n")
	write("platform/.gloss.yaml", "boundary: o:platform\nlevel: system\ncriticality: critical\n")
	write("platform/metrics/.gloss.yaml", "boundary: o:metrics\nlevel: container\npart-of: o:platform\n")
	write("platform/metrics/export.py", "# @gloss implements pa:Exporter\ndef export(): pass\n")
	write("platform/auth/session.py", "# @gloss implements pa:Session\ndef login(): pass\n")
	write("platform/auth/tests/test_auth_security.py", "# @gloss verifies pa:Session\ndef test_csrf_token(): pass\n")
	level := func(g *graph.Graph, path string) (string, string) {
		for _, a := range g.NodesOf(model.NodeArtifact) {
			if a.Path == path {
				return a.Criticality, a.CriticalityFrom
			}
		}
		return "", ""
	}
	_, g, _, _ := build(t, root)
	if l, from := level(g, "platform/metrics/export.py"); l != "low" || from != "pa:Exporter" {
		t.Errorf("without a floor the metrics exporter is low via pa:Exporter (which inherits it from pay:Metrics), got %q via %q", l, from)
	}
	write("platform/.gloss.yaml", "boundary: o:platform\nlevel: system\ncriticality: critical\ncriticality-floor: true\n")
	_, g, _, _ = build(t, root)
	if l, from := level(g, "platform/metrics/export.py"); l != "critical" || from != "o:platform" {
		t.Errorf("with the floor the exporter is critical via o:platform, got %q via %q", l, from)
	}
	sugg := map[string]string{}
	for _, s := range rollup.SuggestCriticality(g) {
		sugg[s.Concept] = s.Reason
	}
	for _, c := range []string{"pay:Cutoff", "pa:Clock", "pa:Session"} {
		if sugg[c] == "" {
			t.Errorf("expected a high suggestion for %s, got %v", c, sugg)
		}
	}
	if _, ok := sugg["pay:Payment"]; ok {
		t.Error("an unremarkable entity stays unclassified")
	}
	if _, ok := sugg["pay:Metrics"]; ok {
		t.Error("a declared level is never overridden")
	}
}

// TestPlacement: every concept in a projection says why it sits where it
// does. housed, flow and unlinked come from the examples; shared, bound and
// surface do not occur there, so a small fixture covers them.
func TestPlacement(t *testing.T) {
	type want struct{ view, concept, placement, reason string }
	expect := func(t *testing.T, root string, wants []want) {
		t.Helper()
		ws, g, _, _ := build(t, root)
		byName := map[string]*views.Projection{}
		for _, p := range views.ProjectAll(g, ws.Views) {
			byName[p.Name] = p
			for _, n := range p.Nodes {
				if (n.Type == "concept") != (n.Placement != "") {
					t.Errorf("%s: %s %s has placement %q", p.Name, n.Type, n.ID, n.Placement)
				}
			}
		}
		for _, w := range wants {
			var got *views.PNode
			for i, n := range byName[w.view].Nodes {
				if n.ID == w.concept {
					got = &byName[w.view].Nodes[i]
				}
			}
			if got == nil || got.Placement != w.placement || !strings.Contains(got.PlacementReason, w.reason) {
				t.Errorf("%s %s = %+v, want placement %s with reason containing %q", w.view, w.concept, got, w.placement, w.reason)
			}
		}
	}
	expect(t, example(t, "payments/payments-api"), []want{
		{"engineering", "pay:Authorization", views.PlacementHoused, "inside acme:payments-api"},
		{"engineering", "pay:Refund", views.PlacementFlow, "produced by acme:payments-api"},
		{"engineering", "pay:Merchant", views.PlacementUnlinked, "no boundary realizes"},
	})
	expect(t, example(t, "horse-factory"), []want{
		{"engineering", "zoo:Foal", views.PlacementHoused, "inside "},
		{"engineering", "zoo:FoalBorn", views.PlacementFlow, "published by acme:horse-factory-svc"},
		{"engineering", "impl:TailQueue", views.PlacementUnlinked, "no boundary realizes"},
	})

	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Thing: { kind: process }\n  Budgeting: { kind: process }\n"+
		"  Cutoff: { kind: constraint, value: \"3\", value-from: { path: cfg.py, symbol: CUTOFF } }\n")
	write("ontology/views.yaml", "views:\n  all:\n    include: [z:*]\n    zoom: container\n")
	write("cfg.py", "CUTOFF = 3\n")
	write("a/.gloss.yaml", "boundary: o:a\nlevel: container\nrealizes: [z:Thing]\n")
	write("a/x.py", "# @gloss implements z:Thing\ndef x(): pass\n")
	write("b/.gloss.yaml", "boundary: o:b\nlevel: container\nrealizes: [z:Thing]\n")
	write("b/y.py", "# @gloss implements z:Thing\ndef y(): pass\n")
	write("web/.gloss.yaml", "boundary: o:web\nlevel: container\npresents: [z:Budgeting]\n")
	write("web/page.tsx", "// @gloss presents z:Budgeting\nexport function Page() {}\n")
	expect(t, root, []want{
		{"all", "z:Thing", views.PlacementShared, "realized or represented by o:a, o:b"},
		{"all", "z:Cutoff", views.PlacementBound, "value bound to code at cfg.py:1"},
		{"all", "z:Budgeting", views.PlacementSurface, "presented by o:web"},
	})
}
