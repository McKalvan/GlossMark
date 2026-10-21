// @gloss verifies gmi:Gate scope=file

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// tempRepo returns a fresh workspace root and a helper that writes files into it.
func tempRepo(t *testing.T) (string, func(rel, content string)) {
	t.Helper()
	root := t.TempDir()
	return root, func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestOneGateClassifier: gloss diff, impact, status, accept --policy and MCP
// all gate a link through ClassifyLink, so a link to a critical concept is
// HARD everywhere (#117), a file's impact label is HARD when one of its links
// is outside the agent's remit (#123), and a page that presents a critical
// concept is rated as the graph rates it, not at the concept's level (#47).
func TestOneGateClassifier(t *testing.T) {
	root, write := tempRepo(t)
	// An implementation namespace keeps the engineering view from being core-only.
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  zi: { iri: https://x/zi/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Rule: { kind: process, criticality: critical }\n  Order: { kind: process }\n  Menu: { kind: process }\n  Limit: { kind: process, criticality: critical }\n")
	write("ontology/zi.gloss.yaml", "concepts:\n  Helper: { kind: process, internal: true }\n")
	write(".gloss.yaml", "boundary: o:root\nlevel: system\n")
	write(".curator.md", "<!-- @gloss may-modify z:Menu, z:Rule, z:Limit scope=file -->\n")
	_, base, _, _ := build(t, root)
	write("src/rule.py", "# @gloss? implements z:Rule conf=0.95\ndef rule(): pass\n")
	write("src/order.py", "# @gloss? implements z:Order conf=0.9\ndef order(): pass\n")
	write("web/LimitPage.tsx", "// @gloss? presents z:Limit conf=0.9\nexport function LimitPage() {}\n")
	ws, head, res, _ := build(t, root)
	lg, _ := ledger.Open(filepath.Join(t.TempDir(), "ledger.jsonl"))
	lg.Sync(head, res.Markers, "agent:curator", time.Now())
	pol := ws.Config.EffectivePolicy()
	im := gate.Classify(base, head, ws, gate.Options{Base: "base", Policy: pol, HeadLedger: lg.Entries})
	diff := map[string]gate.Change{}
	for _, ch := range im.Changes {
		if ch.Kind == "link-added" {
			diff[ch.Object] = ch
		}
	}
	n := 0
	for _, e := range head.Edges() {
		if e.Source != graph.SourceInline || e.P == "may-modify" {
			continue
		}
		n++
		cls, why := gate.ClassifyLink(head, ws, e, pol)
		ch := diff[head.Curie(e.O)]
		if cls != ch.Gate || strings.Join(why, ";") != strings.Join(ch.Reasons, ";") {
			t.Errorf("%s: ClassifyLink %s %v, diff %s %v", head.Curie(e.O), cls, why, ch.Gate, ch.Reasons)
		}
	}
	if n != 3 {
		t.Fatalf("%d inline links, want 3", n)
	}
	if ch := diff["z:Rule"]; ch.Gate != gate.HARD || strings.Join(ch.Reasons, ";") != "criticality critical: z:Rule" {
		t.Errorf("link to a critical concept: %s %v", ch.Gate, ch.Reasons)
	}
	if ch := diff["z:Limit"]; ch.Gate != gate.SOFT || ch.Criticality != "" {
		t.Errorf("presents a critical concept: %s %q %v", ch.Gate, ch.Criticality, ch.Reasons)
	}
	ts := gate.Touched(head, ws, []string{"src/rule.py", "src/order.py", "web/LimitPage.tsx"})
	if ts[0].Gate != gate.HARD || ts[0].Criticality != "critical" {
		t.Errorf("rule.py: %+v", ts[0])
	}
	if ts[1].Gate != gate.HARD {
		t.Errorf("order.py is outside the curator's remit, want HARD: %+v", ts[1])
	}
	for _, a := range head.NodesOf(model.NodeArtifact) {
		if a.Path == "web/LimitPage.tsx" && a.Criticality != ts[2].Criticality {
			t.Errorf("page: graph criticality %q, impact %q", a.Criticality, ts[2].Criticality)
		}
	}
	if ts[2].Gate != gate.SOFT || !strings.Contains(strings.Join(ts[2].Hints, ";"), "it is not rated by it") {
		t.Errorf("LimitPage.tsx: %+v", ts[2])
	}
}

// TestWaiverValidation: a waiver without an ISO expires: date is a check
// error and waives nothing; one that names no finding is a warning (#126).
func TestWaiverValidation(t *testing.T) {
	root, write := tempRepo(t)
	write("ontology/z.gloss.yaml", "concepts:\n  Doing: { kind: process }\n  Lonely: { kind: process }\n")
	write(".gloss.yaml", "boundary: o:b\nlevel: container\n")
	write("a.py", "# @gloss implements z:Doing\ndef a(): pass\n")
	for _, c := range []struct {
		waiver, code string
		waived       bool
	}{
		{"{ finding: z:Lonely, owner: alice, expires: 2999-12-31 }", "", true},
		{`{ finding: z:Lonely, owner: alice, expires: "2999-12-31" }`, "", true},
		{"{ finding: z:Lonely, owner: alice, expires: 30 June 2020 }", "GM610", false},
		{"{ finding: z:Lonely, owner: alice }", "GM610", false},
		{"{ finding: orphan:z:Lonley, owner: alice, expires: 2999-12-31 }", "GM611", false},
		{"{ finding: bogus:Nothing, owner: alice, expires: 2999-12-31 }", "GM611", false},
		// An expired waiver is reported as expired, not also as matching nothing (#102).
		{"{ finding: z:Lonely, owner: alice, expires: 2026-01-31 }", "GM612", false},
		{"{ finding: z:Gone, owner: alice, expires: 2026-01-31 }", "GM612", false},
	} {
		write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\npolicy:\n  waivers:\n    - "+c.waiver+"\n")
		ws, g, _, _ := build(t, root)
		var codes []string
		for _, d := range check.Run(ws, g) {
			if strings.HasPrefix(d.Code, "GM61") {
				codes = append(codes, d.Code+" "+d.Severity)
			}
		}
		want := ""
		switch c.code {
		case "GM610":
			want = "GM610 error"
		case "GM611":
			want = "GM611 warning"
		case "GM612":
			want = "GM612 warning"
		}
		if strings.Join(codes, ";") != want {
			t.Errorf("%s: check %v, want %q", c.waiver, codes, want)
		}
		rep := rollup.Compare(g, ws, rollup.Options{Policy: ws.Config.EffectivePolicy()})
		for _, f := range findings(rep, rollup.KindOrphan) {
			if f.Concept == "z:Lonely" && f.Waived != c.waived {
				t.Errorf("%s: orphan waived = %v, want %v (%s)", c.waiver, f.Waived, c.waived, f.Msg)
			}
		}
	}
}

// TestCoreTestCredit: a test of a core concept covers an implementation
// concept realizing it only when that concept is the sole realizer or the
// test sits in a boundary holding one of its artifacts (#52).
func TestCoreTestCredit(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Invoicing: { kind: process }\n  Paying: { kind: process }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  BillingEngine: { kind: process, realizes: z:Invoicing }\n  InvoiceDocument: { kind: process, realizes: z:Invoicing }\n  Payer: { kind: process, realizes: z:Paying }\n")
	write(".gloss.yaml", "boundary: o:root\nlevel: system\n")
	write("billing/.gloss.yaml", "boundary: o:billing\nlevel: container\npart-of: o:root\ndefault-namespace: i\nrealizes: [i:BillingEngine, i:Payer]\n")
	write("documents/.gloss.yaml", "boundary: o:documents\nlevel: container\npart-of: o:root\ndefault-namespace: i\nrealizes: [i:InvoiceDocument]\n")
	write("billing/engine.py", "# @gloss implements .BillingEngine\ndef bill(): pass\n\n# @gloss implements .Payer\ndef pay(): pass\n")
	write("billing/test_engine.py", "# @gloss verifies z:Invoicing\ndef test_bill(): pass\n")
	// The sole realizer of z:Paying is covered by a test anywhere.
	write("tests/test_pay.py", "# @gloss verifies z:Paying\ndef test_pay(): pass\n")
	write("documents/pdf.py", "# @gloss implements .InvoiceDocument\ndef render(): pass\n")
	report := func() *rollup.Report {
		ws, g, _, lg := build(t, root)
		return rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	}
	rep := report()
	if u := concepts(findings(rep, rollup.KindUntested)); !has(u, "i:InvoiceDocument") || has(u, "i:BillingEngine") || has(u, "i:Payer") {
		t.Errorf("untested = %v, want i:InvoiceDocument only", u)
	}
	if m := rep.Metrics; m.ConceptsCovered != 2 || m.ConceptsInScope != 3 {
		t.Errorf("coverage %d of %d, want 2 of 3", m.ConceptsCovered, m.ConceptsInScope)
	}
	// A test of the core concept beside the renderer covers it.
	write("documents/test_pdf.py", "# @gloss verifies z:Invoicing\ndef test_render(): pass\n")
	if m := report().Metrics; m.ConceptsCovered != 3 {
		t.Errorf("with a test in o:documents: coverage %d of %d, want 3", m.ConceptsCovered, m.ConceptsInScope)
	}
}

// TestCoreTestCreditContainment: a test of a core concept in a boundary
// that encloses the realizer's boundary covers it (a system-level
// acceptance test), and so does a test beside the files that configure or
// deploy a system-type. A test in a sibling boundary still does not, and
// the untested line says where the core concept's tests are instead.
func TestCoreTestCreditContainment(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Invoicing: { kind: process }\n  Checkout: { kind: process }\n")
	write("ontology/i.gloss.yaml", "concepts:\n"+
		"  BillingEngine: { kind: process, realizes: z:Invoicing }\n"+
		"  InvoiceDocument: { kind: process, realizes: z:Invoicing }\n"+
		"  DeskCheckout: { kind: process, realizes: z:Checkout }\n"+
		"  KioskCheckout: { kind: process, realizes: z:Checkout }\n"+
		"  BillingService: { kind: system-type, realizes: z:Invoicing }\n")
	write(".gloss.yaml", "boundary: o:root\nlevel: system\n")
	write("billing/.gloss.yaml", "boundary: o:billing\nlevel: container\npart-of: o:root\ndefault-namespace: i\nrealizes: [i:BillingEngine, i:BillingService]\n")
	write("documents/.gloss.yaml", "boundary: o:documents\nlevel: container\npart-of: o:root\ndefault-namespace: i\nrealizes: [i:InvoiceDocument]\n")
	// desk/ names no part-of: nesting makes it part of o:root (ADR 0026).
	write("desk/.gloss.yaml", "boundary: o:desk\nlevel: container\ndefault-namespace: i\nrealizes: [i:DeskCheckout]\n")
	write("kiosk/.gloss.yaml", "boundary: o:kiosk\nlevel: container\npart-of: o:root\ndefault-namespace: i\nrealizes: [i:KioskCheckout]\n")
	write("billing/engine.py", "# @gloss implements .BillingEngine\ndef bill(): pass\n")
	write("billing/deploy.yaml", "# @gloss configures .BillingService scope=file\nreplicas: 2\n")
	write("billing/test_engine.py", "# @gloss verifies z:Invoicing\ndef test_bill(): pass\n")
	write("documents/pdf.py", "# @gloss implements .InvoiceDocument\ndef render(): pass\n")
	write("desk/checkout.py", "# @gloss implements .DeskCheckout\ndef checkout(): pass\n")
	write("kiosk/checkout.py", "# @gloss implements .KioskCheckout\ndef checkout(): pass\n")
	// A system-level acceptance test at the root.
	write("features/checkout.feature", "# @gloss verifies z:Checkout\nFeature: Checkout\n")
	ws, g, _, lg := build(t, root)
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	u := findings(rep, rollup.KindUntested)
	if c := concepts(u); len(c) != 1 || !has(c, "i:InvoiceDocument") {
		t.Errorf("untested = %v, want i:InvoiceDocument only", c)
	}
	for _, f := range u {
		if f.Concept == "i:InvoiceDocument" && !strings.Contains(f.Msg, "the tests of z:Invoicing, which it realizes, sit in boundaries that hold none of its code") {
			t.Errorf("untested message %q should say where z:Invoicing's tests are", f.Msg)
		}
	}
	if m := rep.Metrics; m.ConceptsCovered != 4 || m.ConceptsInScope != 5 {
		t.Errorf("coverage %d of %d, want 4 of 5", m.ConceptsCovered, m.ConceptsInScope)
	}
}

// TestUntestedNamesWhyCoreTestMisses: when a core concept counts as tested
// only because another realizer is verified, the untested line says so
// instead of pointing at tests the core concept does not have.
func TestUntestedNamesWhyCoreTestMisses(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Availability: { kind: capability }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  ApiService: { kind: system-type, realizes: z:Availability }\n  SlotSearch: { kind: capability, realizes: z:Availability }\n")
	write(".gloss.yaml", "boundary: o:api\nlevel: container\ndefault-namespace: i\nrealizes: [i:ApiService, i:SlotSearch]\n")
	write("slots.go", "package api\n\n// @gloss implements .SlotSearch\nfunc FindSlots() {}\n")
	write("deploy.yaml", "# @gloss deploys .ApiService scope=file\nreplicas: 1\n")
	write("api_test.go", "// @gloss verifies .ApiService scope=file\npackage api\n")
	ws, g, _, lg := build(t, root)
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	u := findings(rep, rollup.KindUntested)
	if len(u) != 1 || u[0].Concept != "i:SlotSearch" || !strings.Contains(u[0].Msg, "z:Availability, which it realizes, counts as tested only because i:ApiService is tested") {
		t.Errorf("untested = %+v, want i:SlotSearch naming i:ApiService", u)
	}
}

// TestUntestedNorUntestedEvent: an impl concept realizing an untested
// event (events are never reported themselves) keeps the "nor" suffix.
func TestUntestedNorUntestedEvent(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Booked: { kind: event }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  Emitter: { kind: process, realizes: z:Booked }\n")
	write(".gloss.yaml", "boundary: o:svc\nlevel: container\ndefault-namespace: i\nrealizes: [i:Emitter]\n")
	write("emit.go", "package svc\n\n// @gloss implements .Emitter\nfunc Emit() {}\n")
	ws, g, _, lg := build(t, root)
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	u := findings(rep, rollup.KindUntested)
	if len(u) != 1 || !strings.HasSuffix(u[0].Msg, "nothing tests or verifies it (nor z:Booked, which it realizes)") {
		t.Errorf("untested = %+v, want the nor suffix", u)
	}
}
