// @gloss verifies gmi:SuggestCalibration scope=file

package suggest

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// calibrationSet is one labelled workspace of the calibration harness: a
// reviewed workspace whose own markers are the labels. The harness strips
// them, runs suggest on the stripped copy and scores what comes back.
type calibrationSet struct {
	name string
	src  string   // workspace root, relative to this package
	skip []string // top-level entries left out of the copy
}

var calibrationSets = []calibrationSet{
	// The repository itself: GlossMark applied to GlossMark. Its markers
	// are mostly file-level; a proposal they do not state counts as wrong
	// even where a reviewer might keep it, so this set is conservative.
	{name: "self", src: filepath.Join("..", ".."), skip: []string{".git", ".glossmark", "examples", "corpus", "dist", ".claude"}},
	{name: "horse-factory", src: filepath.Join("..", "..", "examples", "horse-factory")},
	{name: "payments-api", src: filepath.Join("..", "..", "examples", "payments", "payments-api")},
	{name: "settlement-worker", src: filepath.Join("..", "..", "examples", "payments", "settlement-worker")},
	// Fixtures built from the adoption findings (#58-#69): concept-poor
	// legacy code, and a monorepo full of package clauses, fakes, DTOs,
	// Terraform, Kubernetes and docs.
	{name: "warehouse-legacy", src: filepath.Join("testdata", "calibration", "warehouse-legacy")},
	{name: "clinic-monorepo", src: filepath.Join("testdata", "calibration", "clinic-monorepo")},
}

type calibrated struct {
	set    string
	scored []Scored
	labels []Label
	thresh float64
}

// runCalibration strips each set, suggests on it with every conf kept,
// and scores the result.
func runCalibration(t *testing.T) []calibrated {
	t.Helper()
	var out []calibrated
	for _, cs := range calibrationSets {
		_, g0 := build(t, cs.src)
		ls := LabelsFromGraph(g0)
		root := t.TempDir()
		copyStripped(t, cs.src, root, cs.skip...)
		ws, g := build(t, root)
		ps, _, err := Run(ws, g, nil, Options{MinConf: 0.01})
		if err != nil {
			t.Fatal(err)
		}
		sc := Score(ws, ps, ls)
		for i := range sc {
			sc[i].Path = cs.name + "/" + sc[i].Path
		}
		for i := range ls {
			ls[i].Path = cs.name + "/" + ls[i].Path
		}
		out = append(out, calibrated{cs.name, sc, ls, ws.Config.EffectivePolicy().SoftConfThreshold})
	}
	return out
}

func pool(cs []calibrated, skip string) (sc []Scored, ls []Label) {
	for _, c := range cs {
		if c.set != skip {
			sc = append(sc, c.scored...)
			ls = append(ls, c.labels...)
		}
	}
	return sc, ls
}

// heldOut scores each set with the table fitted on the other sets
// (leave one set out), the honest estimate of how conf generalizes.
func heldOut(cs []calibrated) Report {
	var sc []Scored
	var ls []Label
	for _, c := range cs {
		train, _ := pool(cs, c.set)
		bb, be := Fit(train)
		sc = append(sc, Recalibrate(c.scored, bb, be)...)
		ls = append(ls, c.labels...)
	}
	return Evaluate("held out (each set scored with the table fitted on the others)", sc, ls, 0.7)
}

// TestCalibrationReport prints the calibration tables: go test -run
// TestCalibrationReport -v ./internal/suggest. With GLOSS_CALIBRATION_DUMP
// set it also writes every scored proposal to that file.
func TestCalibrationReport(t *testing.T) {
	cs := runCalibration(t)
	for _, c := range cs {
		t.Log("\n" + Evaluate(c.set, c.scored, c.labels, c.thresh).String())
	}
	all, labels := pool(cs, "")
	pooled := Evaluate("pooled (in sample)", all, labels, 0.7)
	t.Log("\n" + pooled.String() + pooled.BasisTable())
	t.Log("\n" + heldOut(cs).String())
	t.Log("\n" + sweep(all))
	if out := os.Getenv("GLOSS_CALIBRATION_DUMP"); out != "" {
		var sb strings.Builder
		for _, s := range all {
			sb.WriteString(strings.Join([]string{s.Verdict, s.Basis, strconv.FormatFloat(s.Conf, 'f', 2, 64), s.Path, s.Symbol, s.Relation, s.Full, s.Matched}, "\t") + "\n")
		}
		os.WriteFile(out, []byte(sb.String()), 0o644)
	}
}

// sweep renders the share kept and the precision at each conf threshold.
func sweep(sc []Scored) string {
	var sb strings.Builder
	sb.WriteString("threshold sweep (pooled)\n")
	for th := 0.3; th < 0.96; th += 0.05 {
		n, ok := 0, 0
		for _, s := range sc {
			if s.Conf >= th-1e-9 {
				n++
				if s.Verdict == Correct {
					ok++
				}
			}
		}
		fmt.Fprintf(&sb, "  conf >= %.2f: %4d (%s), precision %s\n", th, n, pct(n, len(sc)), pct(ok, n))
	}
	return sb.String()
}

// TestCalibrationTableIsFitted: the conf every basis gets is what the
// calibration set measures for it, within sampling error, so conf means
// estimated precision. The repository is one of the sets, so ordinary
// edits move the numbers a little; a matcher change that moves them more
// fails here, with the refitted table to paste (GLOSS_CALIBRATION_REFIT=1
// prints it regardless).
func TestCalibrationTableIsFitted(t *testing.T) {
	all, _ := pool(runCalibration(t), "")
	bb, be := Fit(all)
	n, ok := map[string]int{}, map[string]int{}
	for _, s := range all {
		n[s.Basis]++
		if s.Verdict == Correct {
			ok[s.Basis]++
		}
	}
	stale := os.Getenv("GLOSS_CALIBRATION_REFIT") != ""
	for basis, c := range n {
		p := float64(ok[basis]+1) / float64(c+2)
		if d := math.Abs(confFor(basis) - p); d > 2*math.Sqrt(p*(1-p)/float64(c+2))+0.05 {
			t.Errorf("basis %s: conf %.2f, measured %.2f (%d of %d correct)", basis, confFor(basis), p, ok[basis], c)
			stale = true
		}
	}
	if stale {
		t.Errorf("refit calibration.go with:\n\nvar calibration = %s\n\nvar calibrationByEvidence = %s\n", goLiteral(bb), goLiteral(be))
	}
}

// TestConfIsCalibrated: in every band with enough proposals, the share
// that is correct is within sampling error (two standard errors, plus
// 0.05) of the mean conf, and precision does not fall as conf rises (#69).
func TestConfIsCalibrated(t *testing.T) {
	all, labels := pool(runCalibration(t), "")
	rep := Evaluate("pooled", all, labels, 0.7)
	last, lastSE := -1.0, 0.0
	for _, b := range rep.Rows {
		if b.N < 10 {
			continue
		}
		c := b.MeanConf()
		se := math.Sqrt(c * (1 - c) / float64(b.N))
		if d := math.Abs(b.Precision() - c); d > 2*se+0.05 {
			t.Errorf("band %.1f-%.1f: precision %.2f, mean conf %.2f (n=%d)", b.Lo, b.Hi, b.Precision(), c, b.N)
		}
		if b.Precision() < last-2*math.Max(se, lastSE) {
			t.Errorf("band %.1f-%.1f: precision %.2f falls below the band under it (%.2f)", b.Lo, b.Hi, b.Precision(), last)
		}
		last, lastSE = b.Precision(), se
	}
}

func sameTable(a, b map[string]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || math.Abs(v-w) > 0.005 {
			return false
		}
	}
	return true
}

func goLiteral(m map[string]float64) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString("map[string]float64{\n")
	for _, k := range keys {
		fmt.Fprintf(&sb, "\t%q: %.2f,\n", k, m[k])
	}
	sb.WriteString("}")
	return sb.String()
}
