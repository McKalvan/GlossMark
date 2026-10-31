// @gloss verifies gmi:AdoptionLadder scope=file

package rollup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
)

// ladderWorkspace writes a workspace that has reached the given rung and
// builds its graph: a glossary at 0, a manifest and file links at 1, a
// declared criticality at 2, a marker on a declaration at 3.
func ladderWorkspace(t *testing.T, level int) (*load.Workspace, *graph.Graph) {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("glossmark.yaml", "owns:\n  lib: { iri: https://x/lib/, tier: core }\n  circ: { iri: https://x/circ/, tier: implementation }\n")
	crit := ""
	if level >= 2 {
		crit = ", criticality: high"
	}
	write("ontology/lib.gloss.yaml", "concepts:\n  Loan: { kind: entity"+crit+" }\n  Checkout: { kind: process, produces: [Loan] }\n")
	write("ontology/circ.gloss.yaml", "concepts:\n  CheckoutHandler: { kind: process, realizes: lib:Checkout }\n  LoanStore: { kind: resource, internal: true }\n")
	if level >= 1 {
		write("services/circ/.gloss.yaml", "boundary: o:circ\nlevel: container\nrealizes: [lib:Checkout]\ndefault-namespace: circ\n")
		write("services/circ/checkout.py", "# @gloss implements .CheckoutHandler scope=file\ndef check_out(): ...\n")
	}
	if level >= 3 {
		write("services/circ/models.py", "# @gloss represents lib:Loan\nclass Loan: ...\n")
	}
	ws, err := load.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := extract.Build(ws)
	if err != nil {
		t.Fatal(err)
	}
	Derive(res.Graph, ws.Vocab)
	return ws, res.Graph
}

// TestObservedAdoption reads each rung back from what the tree holds.
func TestObservedAdoption(t *testing.T) {
	for level := 0; level <= model.AdoptionFull; level++ {
		ws, g := ladderWorkspace(t, level)
		o := ObservedAdoption(ws, g)
		if o.Level != level {
			t.Errorf("workspace at %d observed as %d: has %q missing %q", level, o.Level, o.Has, o.Missing)
		}
		if level < model.AdoptionFull && o.Missing[level+1] == "" {
			t.Errorf("workspace at %d: nothing missing for %d", level, level+1)
		}
	}
}

// TestFailingAtLevel: nothing fails below level 2, only high or critical
// findings at level 2, everything at level 3.
func TestFailingAtLevel(t *testing.T) {
	ws, g := ladderWorkspace(t, 3)
	failing := []Finding{
		{Kind: KindOrphan, Concept: "lib:Loan"},                          // high (declared)
		{Kind: KindOrphan, Concept: "circ:LoanStore"},                    // unclassified
		{Kind: KindGap, Boundary: "o:circ", Concept: "circ:LoanStore"},   // the concept decides, not the boundary (high)
		{Kind: KindOverdue, Boundary: "o:circ"},                          // a boundary alone: its own level (high)
		{Kind: KindViolation, Concept: "lib:Loan, circ:CheckoutHandler"}, // high via lib:Loan
	}
	_ = ws
	if got := FailingAtLevel(failing, g, 0); got != nil {
		t.Errorf("level 0 fails %d", len(got))
	}
	if got := FailingAtLevel(failing, g, 1); got != nil {
		t.Errorf("level 1 fails %d", len(got))
	}
	if got := FailingAtLevel(failing, g, 2); len(got) != 3 || got[0].Concept != "lib:Loan" || got[1].Kind != KindOverdue || got[2].Kind != KindViolation {
		t.Errorf("level 2 fails %+v", got)
	}
	if got := FailingAtLevel(failing, g, 3); len(got) != 5 {
		t.Errorf("level 3 fails %d", len(got))
	}
	if c := FindingCriticality(g, failing[1]); c != "" {
		t.Errorf("unclassified finding carries %q", c)
	}
}
