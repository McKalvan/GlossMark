// @gloss verifies gm:Confidence scope=file

package e2e

import (
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/graph"
)

// TestSoftConfThreshold: with policy.soft-conf-threshold set and no
// precision target (the rule before ADR 0024, which gloss init wrote), a
// proposed link below the threshold (0.7, gmi:DefaultSoftThreshold) is HARD
// with the threshold as its reason; one at or above it stays SOFT.
func TestSoftConfThreshold(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\npolicy:\n  soft-conf-threshold: 0.7\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Low: { kind: process }\n  High: { kind: process }\n")
	write(".gloss.yaml", "boundary: o:root\nlevel: system\n")
	write("src/low.py", "# @gloss? implements z:Low conf=0.5\ndef low(): pass\n")
	write("src/high.py", "# @gloss? implements z:High conf=0.7\ndef high(): pass\n")
	ws, g, _, _ := build(t, root)
	pol := ws.Config.EffectivePolicy()
	if pol.SoftConfThreshold != 0.7 {
		t.Fatalf("soft threshold = %v, want 0.7", pol.SoftConfThreshold)
	}
	got := map[string]string{}
	for _, e := range g.Edges() {
		if e.Source != graph.SourceInline {
			continue
		}
		cls, why := gate.ClassifyLink(g, ws, e, pol)
		got[g.Curie(e.O)] = string(cls) + " " + strings.Join(why, ";")
	}
	if !strings.HasPrefix(got["z:Low"], "HARD") || !strings.Contains(got["z:Low"], "conf 0.50 below threshold 0.70") {
		t.Errorf("conf 0.5: %q, want HARD below threshold", got["z:Low"])
	}
	if !strings.HasPrefix(got["z:High"], "SOFT") {
		t.Errorf("conf 0.7: %q, want SOFT", got["z:High"])
	}
}
