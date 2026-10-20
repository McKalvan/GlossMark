// @gloss verifies gmi:Extractor, gmi:RollupEngine, gmi:DriftComparator, gmi:Linter scope=file

package e2e

import (
	"os"
	"path/filepath"
	"testing"

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
