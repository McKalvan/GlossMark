// @gloss verifies gmi:Gate scope=file

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/model"
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
