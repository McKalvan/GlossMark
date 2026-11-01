// @gloss verifies gmi:SuggestCalibration scope=file

package suggest

import (
	"fmt"
	"sync"

	"github.com/mckalvan/glossmark/internal/calib"
	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/rollup"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// calibrationSet is one labelled workspace of the calibration harness: a
// reviewed workspace whose own accepted markers are the labels. SelfEval
// strips them, runs suggest on the stripped copy and scores what comes
// back.
type calibrationSet struct {
	name string
	src  string // workspace root, relative to this package
}

// examples is where the sets copied from github.com/McKalvan/glossmark-examples
// (at ed53dfe) live: its showcase workspaces, the concept-poor lab
// workspace and three more lab workspaces under lab/, whose markers were
// written and reviewed by hand when each was adopted. Their READMEs, demo
// scripts and expected outputs are left out. lab/confidence-calibration/meterworks is not a set: it carries no
// markers, and its labels.csv only covers what suggest v0.1.0 proposed.
var examples = filepath.Join("testdata", "calibration", "glossmark-examples")

var calibrationSets = []calibrationSet{
	// The repository itself: GlossMark applied to GlossMark. Its markers
	// are mostly file-level; a proposal they do not state counts as wrong
	// even where a reviewer might keep it, so this set is conservative.
	{name: "self", src: filepath.Join("..", "..")},
	{name: "circulation", src: filepath.Join("..", "..", "examples", "library", "circulation")},
	{name: "holds-worker", src: filepath.Join("..", "..", "examples", "library", "holds-worker")},
	// Fixtures built from the adoption findings (#58-#69): concept-poor
	// legacy code, and a monorepo full of package clauses, fakes, DTOs,
	// Terraform, Kubernetes and docs.
	{name: "warehouse-legacy", src: filepath.Join("testdata", "calibration", "warehouse-legacy")},
	{name: "clinic-monorepo", src: filepath.Join("testdata", "calibration", "clinic-monorepo")},
	// Hand-marked workspaces from glossmark-examples.
	{name: "ex/clinic-booking", src: filepath.Join(examples, "clinic-booking")},
	{name: "ex/claims-events", src: filepath.Join(examples, "claims-events")},
	{name: "ex/kitchen-agents", src: filepath.Join(examples, "kitchen-agents")},
	{name: "ex/library-legacy", src: filepath.Join(examples, "library-legacy")},
	{name: "ex/shipment-core", src: filepath.Join(examples, "freight-federation", "shipment-core")},
	{name: "ex/dispatch", src: filepath.Join(examples, "freight-federation", "dispatch")},
	{name: "ex/tracking", src: filepath.Join(examples, "freight-federation", "tracking")},
	{name: "ex/opstool", src: filepath.Join(examples, "opstool")},
	// Lab workspaces whose markers state one reviewed answer (ADR 0032):
	// the variant each lab's verdict recommends, and one stage of the
	// monolith split (the during stage holds both sides). The lab
	// variants that differ only in which relation they chose are left
	// out: their labels disagree by design.
	{name: "lab/cross-cutting", src: filepath.Join(examples, "lab", "cross-cutting-concerns", "variant-c-constraints")},
	{name: "lab/deep-hierarchies", src: filepath.Join(examples, "lab", "deep-hierarchies", "catalogue")},
	{name: "lab/monolith-split", src: filepath.Join(examples, "lab", "monolith-decomposition", "during")},
}

type calibrated struct {
	set    string
	scored []Scored
	labels []Label
}

var (
	calibrationOnce sync.Once
	calibrationRun  []calibrated
	calibrationErr  error
)

// runCalibration strips each set, suggests on it with every conf kept,
// and scores the result. The run is shared by the tests of this package.
func runCalibration(t *testing.T) []calibrated {
	t.Helper()
	calibrationOnce.Do(func() {
		for _, cs := range calibrationSets {
			ws, err := load.Load(cs.src)
			if err != nil {
				calibrationErr = fmt.Errorf("%s: %w", cs.name, err)
				return
			}
			res, err := extract.Build(ws)
			if err != nil {
				calibrationErr = fmt.Errorf("%s: %w", cs.name, err)
				return
			}
			rollup.Derive(res.Graph, ws.Vocab)
			m, err := SelfEval(ws, res.Graph, nil, nil)
			if err != nil {
				calibrationErr = fmt.Errorf("%s: %w", cs.name, err)
				return
			}
			sc, ls := m.Scored, m.Labels
			for i := range sc {
				sc[i].Path = cs.name + "/" + sc[i].Path
			}
			ls = append([]Label(nil), ls...)
			for i := range ls {
				ls[i].Path = cs.name + "/" + ls[i].Path
			}
			calibrationRun = append(calibrationRun, calibrated{cs.name, sc, ls})
		}
	})
	if calibrationErr != nil {
		t.Fatal(calibrationErr)
	}
	return calibrationRun
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

// shippedRule is the default auto-accept rule: the default target on the
// shipped calibration.
func shippedRule() calib.AutoAccept {
	r := calib.Derive(confFor, calib.Shipped, calib.DefaultPrecision, DefaultMinConf)
	r.Source = "shipped"
	return r
}

// heldOut scores each set with the table fitted on the other sets (leave
// one set out) and the auto-accept threshold that table derives for the
// default target: the honest estimate of how conf and the threshold carry
// over to a repository the calibration never saw.
func heldOut(cs []calibrated) (all Report, perSet []Report, sc []Scored) {
	var ls []Label
	all = Report{Name: "held out (each set scored, and its threshold derived, from the other sets)", ByBasis: map[string]*BandRow{}}
	for _, c := range cs {
		train, _ := pool(cs, c.set)
		tab := Fit(train)
		rule := calib.Derive(tab.Confs(), tab, calib.DefaultPrecision, DefaultMinConf)
		rule.Source = "the other sets"
		rc := Recalibrate(c.scored, tab)
		rep := Evaluate(c.set, rc, c.labels, rule)
		perSet = append(perSet, rep)
		sc = append(sc, rc...)
		ls = append(ls, c.labels...)
		for _, s := range rc {
			if s.AutoAccepts(rule.Threshold) {
				all.Auto.add(s)
			}
			if s.Conf >= DefaultMinConf-1e-9 {
				all.Kept.add(s)
			}
		}
	}
	bands := Evaluate("", sc, ls, shippedRule())
	all.Rows, all.All, all.Labels, all.Recalled = bands.Rows, bands.All, bands.Labels, bands.Recalled
	return all, perSet, sc
}

// heldOutSummary renders the held-out auto-accept numbers, per set and
// pooled.
func heldOutSummary(all Report, perSet []Report) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s: %d proposals, %d labels\n", all.Name, all.All.N, all.Labels)
	for _, r := range perSet {
		th := "nothing"
		if r.Rule.Threshold <= 1 {
			th = fmt.Sprintf("conf >= %.2f", r.Rule.Threshold)
		}
		fmt.Fprintf(&sb, "  %-20s %-13s auto %3d of %3d (%6s), precision %6s\n", r.Name, th, r.Auto.N, r.Kept.N, pct(r.Auto.N, r.Kept.N), pct(r.Auto.Correct, r.Auto.N))
	}
	fmt.Fprintf(&sb, "  %-20s %-13s auto %3d of %3d (%6s), precision %6s\n", "pooled", "", all.Auto.N, all.Kept.N, pct(all.Auto.N, all.Kept.N), pct(all.Auto.Correct, all.Auto.N))
	fmt.Fprintf(&sb, "  kept (conf >= %.2f): %d, precision %s\n", DefaultMinConf, all.Kept.N, pct(all.Kept.Correct, all.Kept.N))
	return sb.String()
}

// TestCalibrationReport prints the calibration tables: go test -run
// TestCalibrationReport -v ./internal/suggest. With GLOSS_CALIBRATION_DUMP
// set it also writes every scored proposal to that file.
func TestCalibrationReport(t *testing.T) {
	cs := runCalibration(t)
	rule := shippedRule()
	for _, c := range cs {
		t.Log("\n" + Evaluate(c.set, c.scored, c.labels, rule).String())
	}
	all, labels := pool(cs, "")
	pooled := Evaluate("pooled (in sample)", all, labels, rule)
	t.Log("\n" + pooled.String() + pooled.SourceTable() + pooled.BasisTable())
	t.Log("\n" + sweep("pooled, in sample", all))
	hall, hsets, hsc := heldOut(cs)
	t.Log("\n" + heldOutSummary(hall, hsets) + hall.BandTable())
	t.Log("\n" + sweep("held out", hsc))
	for _, target := range []float64{0.5, 0.6, 0.7, 0.8, 0.9, 0.95} {
		t.Logf("target %.2f: %s", target, calib.Derive(confFor, calib.Shipped, target, DefaultMinConf))
	}
	if out := os.Getenv("GLOSS_CALIBRATION_DUMP"); out != "" {
		var sb strings.Builder
		for _, s := range all {
			sb.WriteString(strings.Join([]string{s.Verdict, s.Basis, strconv.FormatFloat(s.Conf, 'f', 2, 64), s.Path, s.Symbol, s.Relation, s.Full, s.Matched, strconv.FormatBool(s.Deprecated)}, "\t") + "\n")
		}
		os.WriteFile(out, []byte(sb.String()), 0o644)
	}
}

// sweep renders the share kept and the precision at each conf threshold.
func sweep(name string, sc []Scored) string {
	var sb strings.Builder
	sb.WriteString("threshold sweep (" + name + ")\n")
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
	tab := Fit(all)
	stale := os.Getenv("GLOSS_CALIBRATION_REFIT") != ""
	// A basis seen MinBasisN times has its own row; the rarer ones are
	// read from their role or evidence row (calib.Table.Row), and the rare
	// bases read from one row are checked together.
	rows := map[string]calib.Count{}
	some := map[string]string{} // a basis read from the row
	for basis, c := range tab {
		k := tab.Row(basis)
		if k != basis {
			k += " (rare bases)"
		}
		r := rows[k]
		r.N += c.N
		r.Correct += c.Correct
		rows[k] = r
		some[k] = basis
	}
	for k, c := range rows {
		basis := some[k]
		p := float64(c.Correct+1) / float64(c.N+2)
		if d := math.Abs(confFor(basis) - p); d > 2*math.Sqrt(p*(1-p)/float64(c.N+2))+0.05 {
			t.Errorf("basis %s: conf %.2f, measured %.2f (%d of %d correct)", k, confFor(basis), p, c.Correct, c.N)
			stale = true
		}
	}
	// The shipped threshold must be what the measurement derives, within
	// one conf step of the table.
	want := calib.Derive(tab.Confs(), tab, calib.DefaultPrecision, DefaultMinConf)
	if got := shippedRule(); got.Met != want.Met || math.Abs(got.Threshold-want.Threshold) > 0.1 {
		t.Errorf("shipped %s; the calibration set measures %s", got, want)
		stale = true
	}
	if stale {
		t.Errorf("refit internal/calib/shipped.go with:\n\nvar Shipped = %s\n", goLiteral(tab))
	}
}

// TestConfIsCalibrated: in every band with enough proposals, the share
// that is correct is within sampling error (two standard errors, plus
// 0.05) of the mean conf, and precision does not fall as conf rises (#69).
func TestConfIsCalibrated(t *testing.T) {
	all, labels := pool(runCalibration(t), "")
	rep := Evaluate("pooled", all, labels, shippedRule())
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

func goLiteral(m calib.Table) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString("Table{\n")
	for _, k := range keys {
		c := m[k]
		fmt.Fprintf(&sb, "\t%q: {%d, %d, %d, %d},\n", k, c.N, c.Correct, c.Held, c.HeldCorrect)
	}
	sb.WriteString("}")
	return sb.String()
}
