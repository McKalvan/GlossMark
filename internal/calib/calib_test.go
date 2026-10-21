// @gloss verifies gmi:SuggestCalibration scope=file

package calib

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

// conf for the test tables: each basis names its conf after the slash.
func testConf(basis string) float64 {
	switch basis {
	case "a":
		return 0.9
	case "b":
		return 0.7
	case "c":
		return 0.5
	case "d":
		return 0.2
	}
	return Unmeasured
}

func TestDerive(t *testing.T) {
	// a: 10 of 10, b: 8 of 10, c: 2 of 10, d (under the floor): 0 of 10.
	tab := Table{"a": {10, 10, 0, 0}, "b": {10, 8, 0, 0}, "c": {10, 2, 0, 0}, "d": {10, 0, 0, 0}}
	cases := []struct {
		target, minConf float64
		met             bool
		threshold       float64
		auto, n         int
		precision       float64
	}{
		{0.95, 0.3, true, 0.9, 10, 30, 1},
		{0.9, 0.3, true, 0.7, 20, 30, 0.9},
		{0.6, 0.3, true, 0.5, 30, 30, 2.0 / 3},
		// The floor bounds the search: d is never auto-accepted at 0.3,
		// and is at 0.1 when the target allows it.
		{0.5, 0.1, true, 0.2, 40, 40, 0.5},
		{1.0, 0.3, true, 0.9, 10, 30, 1},
	}
	for _, c := range cases {
		a := Derive(testConf, tab, c.target, c.minConf)
		if a.Met != c.met || math.Abs(a.Threshold-c.threshold) > 1e-9 || a.Auto != c.auto || a.N != c.n || math.Abs(a.Precision-c.precision) > 1e-9 {
			t.Errorf("target %.2f floor %.2f: %+v", c.target, c.minConf, a)
		}
		if want := float64(c.auto) / float64(c.n); math.Abs(a.Share-want) > 1e-9 {
			t.Errorf("target %.2f: share %.3f, want %.3f", c.target, a.Share, want)
		}
	}
}

// TestDeriveNothingMeetsTarget: when no conf reaches the target, nothing
// is auto-accepted, and the summary says so with the best precision seen.
func TestDeriveNothingMeetsTarget(t *testing.T) {
	a := Derive(testConf, Table{"a": {10, 6, 0, 0}, "b": {10, 5, 0, 0}}, 0.9, 0.3)
	if a.Met || a.Threshold != Never || a.Auto != 0 || a.Share != 0 || math.Abs(a.Best-0.6) > 1e-9 {
		t.Fatalf("%+v", a)
	}
	if s := a.String(); !strings.Contains(s, "auto-accept: nothing") || !strings.Contains(s, "60.0%") || !strings.Contains(s, "every proposal needs review") {
		t.Errorf("summary: %q", s)
	}
	if a := Derive(testConf, Table{}, 0.9, 0.3); a.Met || a.Threshold != Never || !strings.Contains(a.String(), "no proposals") {
		t.Errorf("empty table: %+v", a)
	}
}

// TestDeriveLeavesHeldOut: a proposal with a deprecated target is never
// auto-accepted (ADR 0030), so the threshold's precision and count leave
// it out, and the share still counts it among the proposals (ADR 0032).
func TestDeriveLeavesHeldOut(t *testing.T) {
	// a: 10 of 10, two of them held; b: 8 of 10, its two wrong ones held.
	// Counting the held ones, b would bring the precision to 0.9; without
	// them, every auto-accepted proposal is correct.
	tab := Table{"a": {10, 10, 2, 2}, "b": {10, 8, 2, 0}}
	a := Derive(testConf, tab, 0.95, 0.3)
	if !a.Met || a.Threshold != 0.7 || a.Auto != 16 || a.N != 20 || a.Precision != 1 || math.Abs(a.Share-0.8) > 1e-9 {
		t.Errorf("%+v", a)
	}
	if f := At(testConf, tab, 0.9, 0.3); f.Auto != 8 || f.N != 20 || f.Precision != 1 {
		t.Errorf("fixed: %+v", f)
	}
}

func TestAtFixedThreshold(t *testing.T) {
	a := At(testConf, Table{"a": {10, 10, 0, 0}, "b": {10, 8, 0, 0}, "c": {10, 2, 0, 0}}, 0.7, 0.3)
	if !a.Fixed || a.Auto != 20 || a.N != 30 || math.Abs(a.Precision-0.9) > 1e-9 || !strings.Contains(a.String(), "policy.soft-conf-threshold") {
		t.Errorf("%+v %s", a, a)
	}
}

// TestConfFallsBackToEvidence: a basis seen under MinBasisN times gets its
// evidence row, and evidence never seen gets Unmeasured.
func TestConfFallsBackToEvidence(t *testing.T) {
	tab := Table{"name-exact/type": {8, 6, 0, 0}, "name-exact/page": {2, 0, 0, 0}, "doc/func": {4, 1, 0, 0}}
	conf := tab.Confs()
	for basis, want := range map[string]float64{
		"name-exact/type":  Smoothed(6, 8),
		"name-exact/page":  Smoothed(6, 10),
		"name-exact/table": Smoothed(6, 10),
		"doc/func":         Smoothed(1, 4),
		"calls/test":       Unmeasured,
	} {
		if got := conf(basis); got != want || tab.Conf(basis) != want {
			t.Errorf("%s: conf %.2f (Conf %.2f), want %.2f", basis, got, tab.Conf(basis), want)
		}
	}
}

// TestConfBacksOffToRole: a rare basis that is no tie reads its role row
// (its evidence and role, ties included) when that is seen MinBasisN
// times; a rare tie, and a basis whose role row is rare too, read the
// evidence row (ADR 0032).
func TestConfBacksOffToRole(t *testing.T) {
	tab := Table{"name-contains/table": {3, 3, 0, 0}, "name-contains/table/tie": {2, 1, 0, 0}, "name-contains/type": {20, 2, 0, 0}, "name-contains/func/tie": {2, 0, 0, 0}}
	conf := tab.Confs()
	for basis, want := range map[string]float64{
		"name-contains/table":     Smoothed(4, 5),
		"name-contains/table/tie": Smoothed(6, 27),
		"name-contains/type":      Smoothed(2, 20),
		"name-contains/func/tie":  Smoothed(6, 27),
		"name-contains/func":      Smoothed(6, 27),
	} {
		if got := conf(basis); got != want || tab.Conf(basis) != want {
			t.Errorf("%s: conf %.2f (Conf %.2f), want %.2f", basis, got, tab.Conf(basis), want)
		}
	}
	if r := tab.Row("name-contains/table"); r != "name-contains/table/*" {
		t.Errorf("row %s", r)
	}
}

func TestReadWriteRoundTrip(t *testing.T) {
	tab := Table{"name-exact/type": {8, 6, 1, 1}, "doc/func": {4, 1, 0, 0}}
	var b bytes.Buffer
	if err := Write(&b, tab, "measured on x\nsecond line"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(b.String(), "# measured on x\n# second line\n") {
		t.Errorf("header: %q", b.String())
	}
	got, err := Read(&b)
	if err != nil || len(got) != 2 || got["name-exact/type"] != (Count{8, 6, 1, 1}) || got["doc/func"] != (Count{4, 1, 0, 0}) {
		t.Errorf("round trip: %v %v", got, err)
	}
	// A file written before held counts has three fields.
	if got, err := Read(strings.NewReader("a\t4\t3\n")); err != nil || got["a"] != (Count{4, 3, 0, 0}) {
		t.Errorf("three fields: %v %v", got, err)
	}
	for _, bad := range []string{"a\t1\n", "a\tx\t1\n", "a\t1\t2\n", "a\t4\t3\t1\n", "a\t4\t3\t2\t3\n", "a\t4\t0\t1\t1\n"} {
		if _, err := Read(strings.NewReader(bad)); err == nil {
			t.Errorf("%q: accepted", bad)
		}
	}
}

// TestShippedDefaultIsMet: the shipped calibration meets the default
// target at some conf, so the default auto-accepts something.
func TestShippedDefaultIsMet(t *testing.T) {
	a := Derive(Shipped.Confs(), Shipped, DefaultPrecision, DefaultMinConf)
	if !a.Met || a.Precision < DefaultPrecision || a.Auto == 0 {
		t.Errorf("%s", a)
	}
}
