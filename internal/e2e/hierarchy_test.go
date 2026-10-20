// @gloss verifies gmi:Extractor, gmi:RollupEngine, gmi:DriftComparator, gmi:Linter scope=file

package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/views"
)

// workspace returns a temp root and a writer for files under it.
func workspace(t *testing.T) (string, func(rel, content string)) {
	t.Helper()
	root := t.TempDir()
	return root, func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
}

// TestPartOfCoverageDirection: part-of coverage points one way, as is-a
// does. A declared whole covers code that implements a part; a declared
// part does not cover code that implements the whole, and stays a gap. The
// wholes lifted from a declared part's own code are not violations.
func TestPartOfCoverageDirection(t *testing.T) {
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n")
	write("ontology/z.gloss.yaml", "concepts:\n"+
		"  OrderFulfilment: { kind: process }\n"+
		"  Picking: { kind: process, part-of: OrderFulfilment }\n"+
		"  ScanConfirm: { kind: process, part-of: Picking }\n"+
		"  Component: { kind: entity }\n"+
		"  Cassette: { kind: entity, is-a: Component }\n")
	run := func(manifest, code string) (*rollup.Report, []string) {
		t.Helper()
		write("scan/.gloss.yaml", "boundary: o:scan\nlevel: container\n"+manifest)
		write("scan/verifier.py", code)
		ws, g, _, lg := build(t, root)
		var gm503 []string
		for _, d := range check.Run(ws, g) {
			if d.Code == "GM503" {
				gm503 = append(gm503, d.Msg)
			}
		}
		return rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()}), gm503
	}
	// The repro: a leaf declaration, code implementing the whole process.
	rep, gm503 := run("realizes: [z:ScanConfirm]\n", "# @gloss implements z:OrderFulfilment\ndef verify(): pass\n")
	if v := findings(rep, rollup.KindViolation); len(v) != 1 || v[0].Concept != "z:OrderFulfilment" {
		t.Errorf("violations = %+v, want one for z:OrderFulfilment", v)
	}
	if gp := findings(rep, rollup.KindGap); len(gp) != 1 || gp[0].Concept != "z:ScanConfirm" {
		t.Errorf("gaps = %+v, want one for z:ScanConfirm", gp)
	}
	if len(gm503) != 1 {
		t.Errorf("declaring a part should not admit links to the whole; GM503 = %v", gm503)
	}
	// The is-a control behaves the same way.
	rep, _ = run("realizes: [z:Cassette]\n", "# @gloss implements z:Component\ndef verify(): pass\n")
	if len(findings(rep, rollup.KindViolation)) != 1 || len(findings(rep, rollup.KindGap)) != 1 {
		t.Errorf("is-a control: want one violation and one gap, got %+v", rep.Findings)
	}
	// Code for the declared part: the wholes it lifts are the part's, consistent.
	rep, _ = run("realizes: [z:ScanConfirm]\n", "# @gloss implements z:ScanConfirm\ndef verify(): pass\n")
	if v, gp := findings(rep, rollup.KindViolation), findings(rep, rollup.KindGap); len(v)+len(gp) != 0 {
		t.Errorf("part declared and implemented: violations %+v, gaps %+v", v, gp)
	}
	// A declared whole covers a derived part.
	rep, _ = run("realizes: [z:OrderFulfilment]\n", "# @gloss implements z:ScanConfirm\ndef verify(): pass\n")
	if v, gp := findings(rep, rollup.KindViolation), findings(rep, rollup.KindGap); len(v)+len(gp) != 0 {
		t.Errorf("whole declared, part implemented: violations %+v, gaps %+v", v, gp)
	}
	// Code naming a whole next to code for the part is still a violation,
	// and so is the whole above it, which that code lifts.
	write("scan/extra.py", "# @gloss implements z:Picking\ndef pick(): pass\n")
	rep, _ = run("realizes: [z:ScanConfirm]\n", "# @gloss implements z:ScanConfirm\ndef verify(): pass\n")
	if v := concepts(findings(rep, rollup.KindViolation)); len(v) != 2 || !has(v, "z:Picking") || !has(v, "z:OrderFulfilment") {
		t.Errorf("violations = %v, want z:Picking and z:OrderFulfilment", v)
	}
}

// TestSiblingParentBoundary: a parent boundary whose manifest lives beside
// its child (checkout/ part-of platform/) keeps its own file, dir and level
// whatever order the manifests load in, and a parent named only by
// reference is a stub with no level that views still place above its child.
func TestSiblingParentBoundary(t *testing.T) {
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Payment: { kind: process }\n  Refund: { kind: process }\n  Checkout: { kind: process }\n")
	write("ontology/views.yaml", "views:\n  all:\n    include: [z:*]\n    zoom: system\n")
	write("platform/.gloss.yaml", "boundary: o:platform\nlevel: domain\nlabel: Commerce platform\nrealizes: [z:Payment, z:Refund]\n")
	write("checkout/.gloss.yaml", "boundary: o:checkout\nlevel: container\npart-of: o:platform\nrealizes: [z:Checkout]\n")
	write("checkout/flow.py", "# @gloss implements z:Checkout\ndef flow(): pass\n")
	write("orphan/.gloss.yaml", "boundary: o:orphan\nlevel: container\npart-of: o:group\n")
	ws, g, _, lg := build(t, root)
	p := g.NodeByCurie("o:platform")
	if p == nil || p.Kind != "domain" || p.Dir != "platform" || p.File != "platform/.gloss.yaml" || p.Label != "Commerce platform" {
		t.Fatalf("o:platform = %+v, want kind domain, dir platform, file platform/.gloss.yaml", p)
	}
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	found := false
	for _, f := range findings(rep, rollup.KindGap) {
		if f.Boundary == "o:platform" && f.Concept == "z:Refund" {
			found = true
			if f.Path != "platform/.gloss.yaml" {
				t.Errorf("Refund gap points at %s, want platform/.gloss.yaml", f.Path)
			}
		}
	}
	if !found {
		t.Errorf("want a gap for z:Refund on o:platform: %+v", rep.Findings)
	}
	stub := g.NodeByCurie("o:group")
	if stub == nil || stub.Kind != "" || stub.File != "orphan/.gloss.yaml" {
		t.Errorf("o:group stub = %+v, want no kind and the referring manifest's file", stub)
	}
	// At zoom system the stub (one level above a container) is drawn.
	shown := false
	for _, pr := range views.ProjectAll(g, ws.Views) {
		for _, n := range pr.Nodes {
			if n.ID == "o:group" {
				shown = true
			}
		}
	}
	if !shown {
		t.Errorf("the level-less stub o:group should sit one level above its container child")
	}
}
