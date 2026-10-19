// @gloss verifies gm:GlossMark scope=file

package e2e

import (
	"path/filepath"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// TestSelfHost runs GlossMark on its own repository: the root glossmark.yaml,
// the gm: and gmi: ontologies and the markers in this source tree must lint
// clean and declare what the code derives. Orphans and untested concepts are
// reported by `gloss drift` but do not fail here; violations, gaps and
// dangling references do.
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
	for _, curie := range []string{"gm:GlossMark", "gm:Extraction", "gmi:MarkerGrammar", "gmi:GlossCli"} {
		if g.Node(mustIRI(t, g, curie)) == nil {
			t.Errorf("%s missing from the self-hosted graph", curie)
		}
	}
}
