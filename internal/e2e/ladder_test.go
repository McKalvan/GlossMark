// @gloss verifies gmi:AdoptionLadder, gmi:Doctor scope=file

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/doctor"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// driftFailOn is gloss drift's default --fail-on list.
const driftFailOn = "violation,value-drift,gap,dangling,overdue,planned-closed,plan-overdue"

// ladderRun is what the three commands would do on a workspace at a level:
// the errors gloss check reports and whether it fails, the drift findings
// that fail, and the gate gloss diff enforces against an empty base.
type ladderRun struct {
	errors     int
	checkFails bool
	failing    []rollup.Finding
	gate       gate.Class
	enforced   gate.Class
	doctorMsg  string
}

func runLadder(t *testing.T, root string) ladderRun {
	t.Helper()
	ws, g, res, lg := build(t, root)
	level := ws.Config.AdoptionLevel()
	var r ladderRun
	diags := append([]marker.Diagnostic{}, res.Diags...)
	diags = append(diags, check.Run(ws, g)...)
	for _, d := range append(ws.Diags, diags...) {
		if d.Severity == "error" {
			r.errors++
		}
	}
	r.checkFails = r.errors > 0 && model.AdoptionChecksFail(level)
	pol := ws.Config.EffectivePolicy()
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: pol, Now: time.Now()})
	kinds, err := rollup.ParseFailOn(driftFailOn)
	if err != nil {
		t.Fatal(err)
	}
	r.failing = rollup.FailingAtLevel(rep.Failing(kinds), g, level)
	im := gate.Classify(graph.New(ws.Prefixes), g, ws, gate.Options{Base: "main", Policy: pol, HeadLedger: lg.Entries, AfterReport: rep})
	gate.Enforce(im, level)
	r.gate, r.enforced = im.Gate, im.Enforced
	for _, c := range doctor.Run(ws, g, diags, doctor.Env{Actor: "human:test", LookPath: func(string) (string, error) { return "", os.ErrNotExist }}) {
		if c.Name == "adoption" {
			r.doctorMsg = c.Msg + " → " + c.Fix
		}
	}
	return r
}

// TestAdoptionLadder (issue #258): a workspace grows one rung at a time and
// at each rung only what the rung promises can fail. The fixture carries one
// defect of every kind from the start, so what each level lets through is
// visible: a dangling reference (a check error and a drift finding), a gap on
// an unclassified concept, and a HARD gate on a critical concept.
func TestAdoptionLadder(t *testing.T) {
	write := func(root, rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	at := func(level int) string {
		root := t.TempDir()
		write(root, "glossmark.yaml", "adoption: "+string(rune('0'+level))+"\nowns:\n  lib: { iri: https://x/lib/, tier: core }\n  circ: { iri: https://x/circ/, tier: implementation }\n")
		crit := ""
		if level >= 2 {
			crit = ", criticality: critical"
		}
		write(root, "ontology/lib.gloss.yaml", "concepts:\n  Patron: { kind: entity"+crit+" }\n  Loan: { kind: entity }\n  Checkout: { kind: process, produces: [Loan] }\n  Hold: { kind: process }\n")
		write(root, "ontology/circ.gloss.yaml", "concepts:\n  CheckoutHandler: { kind: process, realizes: lib:Checkout }\n  PatronStore: { kind: resource, realizes: lib:Patron }\n")
		// A dangling reference in the glossary's own docs: an error at every level.
		write(root, "docs/patrons.md", "<!-- @gloss documents lib:Borrower scope=file -->\n# Patrons\n")
		if level >= 1 {
			write(root, "services/circ/.gloss.yaml", "boundary: o:circ\nlevel: container\nrealizes: [lib:Checkout, lib:Hold]\ndefault-namespace: circ\n")
			write(root, "services/circ/checkout.py", "# @gloss implements .CheckoutHandler scope=file\ndef check_out(): ...\n")
			write(root, "services/circ/patrons.py", "# @gloss implements .PatronStore scope=file\nclass PatronStore: ...\n")
		}
		if level >= 3 {
			write(root, "services/circ/models.py", "# @gloss represents lib:Loan\nclass Loan: ...\n# @gloss represents lib:Patron\nclass Patron: ...\n")
		}
		return root
	}

	// Level 0: the dangling reference is reported, nothing fails, the gate is advisory.
	r := runLadder(t, at(0))
	if r.errors == 0 || r.checkFails || len(r.failing) != 0 || r.enforced != gate.AUTO {
		t.Errorf("level 0: %+v", r)
	}
	if !strings.Contains(r.doctorMsg, "level 0 (vocabulary) fails on nothing") || !strings.Contains(r.doctorMsg, "Next, level 1 (manifests)") {
		t.Errorf("level 0 doctor: %s", r.doctorMsg)
	}
	// Level 1: check fails on the dangling reference; drift (the gap on lib:Hold) warns only; the gate is advisory.
	r = runLadder(t, at(1))
	if !r.checkFails || len(r.failing) != 0 || r.enforced != gate.AUTO || r.gate == gate.AUTO {
		t.Errorf("level 1: %+v", r)
	}
	if !strings.Contains(r.doctorMsg, "Written: level 1 (manifests)") || !strings.Contains(r.doctorMsg, "Next, level 2 (criticality)") {
		t.Errorf("level 1 doctor: %s", r.doctorMsg)
	}
	// Level 2: only findings and changes on critical or high concepts fail: the gap on
	// lib:Hold (unclassified) stays advisory, while the critical lib:Patron is gated HARD.
	r = runLadder(t, at(2))
	for _, f := range r.failing {
		if f.Concept == "lib:Hold" {
			t.Errorf("level 2 fails on the unclassified gap: %+v", f)
		}
	}
	if r.enforced != gate.HARD || !strings.Contains(r.doctorMsg, "Written: level 2 (criticality)") || !strings.Contains(r.doctorMsg, "Next, level 3 (full)") {
		t.Errorf("level 2: %+v", r)
	}
	// Level 3: everything --fail-on names fails, and the doctor has nothing to add.
	r = runLadder(t, at(3))
	var gaps []string
	for _, f := range r.failing {
		if f.Kind == rollup.KindGap {
			gaps = append(gaps, f.Concept)
		}
	}
	if !r.checkFails || !has(gaps, "lib:Hold") || r.enforced != r.gate || r.gate != gate.HARD {
		t.Errorf("level 3: %+v gaps %v", r, gaps)
	}
	if !strings.Contains(r.doctorMsg, "level 3 (full) fails on everything") || !strings.Contains(r.doctorMsg, "Written: level 3 (full)") || strings.Contains(r.doctorMsg, "Next,") {
		t.Errorf("level 3 doctor: %s", r.doctorMsg)
	}
}

// TestAdoptionLadderShowcase runs the ladder over the showcase workspaces of
// glossmark-examples when GLOSS_EXAMPLES names a checkout (they are not in
// this repository). Each workspace is copied and pinned to every level in
// turn: below 2 nothing fails, at 2 only findings on critical or high
// concepts, at 3 the workspace's documented drift.
func TestAdoptionLadderShowcase(t *testing.T) {
	examples := os.Getenv("GLOSS_EXAMPLES")
	if examples == "" {
		t.Skip("GLOSS_EXAMPLES not set: no glossmark-examples checkout to run against")
	}
	// dispatch imports its siblings by relative path, so the whole federation is copied.
	for _, name := range []string{"clinic-booking", "claims-events", "library-legacy", "kitchen-agents", "freight-federation/dispatch"} {
		tree, sub := name, ""
		if dir, base, ok := strings.Cut(name, "/"); ok {
			tree, sub = dir, base
		}
		src := filepath.Join(examples, "showcase", tree)
		if _, err := os.Stat(filepath.Join(src, sub, "glossmark.yaml")); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		var full ladderRun
		for level := model.AdoptionFull; level >= 0; level-- {
			dst := t.TempDir()
			copyTree(t, src, dst)
			dst = filepath.Join(dst, sub)
			cfg := filepath.Join(dst, "glossmark.yaml")
			b, _ := os.ReadFile(cfg)
			os.WriteFile(cfg, append([]byte("adoption: "+string(rune('0'+level))+"\n"), b...), 0o644)
			r := runLadder(t, dst)
			switch level {
			case model.AdoptionFull:
				full = r
				if !strings.Contains(r.doctorMsg, "Written: level 3 (full)") {
					t.Errorf("%s at 3: doctor %s", name, r.doctorMsg)
				}
			case model.AdoptionCriticality:
				if len(r.failing) > len(full.failing) || r.enforced > full.enforced {
					t.Errorf("%s at 2 fails more than at 3: %d vs %d, %s vs %s", name, len(r.failing), len(full.failing), r.enforced, full.enforced)
				}
				for _, f := range r.failing {
					if model.CriticalityRank(f.Severity) < 0 && f.Concept == "" && f.Boundary == "" {
						t.Errorf("%s at 2 fails on a finding with no concept: %+v", name, f)
					}
				}
			default:
				if (r.checkFails && level == 0) || len(r.failing) != 0 || r.enforced != gate.AUTO {
					t.Errorf("%s at %d: check fails %v, %d drift findings fail, gate %s", name, level, r.checkFails, len(r.failing), r.enforced)
				}
			}
		}
	}
}
