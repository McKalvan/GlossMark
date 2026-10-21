// @gloss verifies gm:GlossMark scope=file

package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// TestSelfHost runs GlossMark on its own repository: the root glossmark.yaml,
// the gm: and gmi: ontologies and the markers in this source tree must lint
// clean and declare what the code derives. Violations, gaps, dangling
// references, orphans, untested mechanisms and proposed concepts fail here.
// Every concept the repository owns carries a doc: an undefined concept is a
// name, not a vocabulary. Stale links do not fail here: a scope=file link goes
// stale on any edit to its file (ADR 0006), so a local test run would fail on
// every edit. CI fails on them instead (`gloss drift --fail-on stale` in
// .github/workflows/ci.yml), so nothing reaches main unreviewed.
func TestSelfHost(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ws, g, _, lg := build(t, root)
	for _, d := range check.Run(ws, g) {
		if d.Severity == "error" || d.Severity == "warning" {
			t.Errorf("gloss check: %s", d)
		}
	}
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	for _, kind := range []string{rollup.KindViolation, rollup.KindGap, rollup.KindDangling} {
		for _, f := range findings(rep, kind) {
			t.Errorf("gloss drift: %s: %s", kind, f.Msg)
		}
	}
	for _, curie := range []string{"gm:GlossMark", "gm:Extraction", "gmi:MarkerGrammar", "gmi:GlossCli", "gm:Onboarding", "gmi:Scaffolder"} {
		if g.Node(mustIRI(t, g, curie)) == nil {
			t.Errorf("%s missing from the self-hosted graph", curie)
		}
	}
	// Orphans and presented-only pages would mean the self-description has a
	// hole; an untested mechanism or a concept still awaiting review means it
	// is unfinished (issue #32).
	for _, kind := range []string{rollup.KindOrphan, rollup.KindPresentedOnly, rollup.KindUntested, rollup.KindProposed} {
		for _, f := range findings(rep, kind) {
			t.Errorf("gloss drift: %s: %s", kind, f.Msg)
		}
	}
	// A `@gloss?` line is not a drift finding, so the count is checked here:
	// nothing in this tree waits on review.
	if n := rep.Metrics.Proposed; n > 0 {
		t.Errorf("gloss drift: %d proposed links; accept or reject them (gloss status)", n)
	}
	for _, n := range g.NodesOf(model.NodeConcept) {
		if !n.Imported && strings.TrimSpace(n.Doc) == "" {
			t.Errorf("%s has no doc in its ontology file", n.CURIE)
		}
	}
	// The two published-value bindings resolve: value-from in the ontology and
	// a publishes-value marker in the code.
	for curie, want := range map[string]string{
		"gmi:GraphFormatVersion":   "const GraphFormat = 1",
		"gmi:DefaultSoftThreshold": "SoftConfThreshold: 0.7,",
	} {
		if n := g.NodeByCurie(curie); n == nil || n.LiveValue != want {
			t.Errorf("%s should be bound to %q, got %+v", curie, want, n)
		}
	}
	// The page presents views and does not realize them.
	if r := rollup.RealizesStar(g, mustIRI(t, g, "gmi:HtmlViewer")); has(curies(g, r), "gm:View") {
		t.Errorf("HtmlViewer presents gm:View; it must not realize it: %v", curies(g, r))
	}
}
