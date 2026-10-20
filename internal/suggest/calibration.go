// @gloss implements gmi:SuggestCalibration scope=file

package suggest

import (
	"math"
	"strings"
)

// A proposal's conf is the precision its basis (evidence/role, plus /tie
// when an unrelated concept matched as strongly) measured on the
// calibration set: the labelled workspaces in calibrate_test.go, stripped
// of their markers and re-suggested. Each value is (correct+1)/(n+2), so a
// basis seen a few times stays near one half until the data says
// otherwise. TestCalibrationTableIsFitted fails when the table and the
// measurement disagree, and prints the table to paste here (ADR 0021).
//
// A basis seen fewer than minBasisN times falls back to its evidence row;
// evidence never measured falls back to unmeasuredConf.
var calibration = map[string]float64{
	"calls/test":               0.43,
	"doc/func":                 0.41,
	"doc/test":                 0.09,
	"doc/test/tie":             0.22,
	"doc/type":                 0.26,
	"doc/type/tie":             0.14,
	"name-contains/func":       0.36,
	"name-contains/heading":    0.14,
	"name-contains/test":       0.11,
	"name-contains/type":       0.25,
	"name-exact/heading":       0.27,
	"name-exact/table":         0.86,
	"name-exact/type":          0.44,
	"name-overlap/func":        0.18,
	"name-overlap/func/tie":    0.33,
	"name-overlap/heading":     0.06,
	"name-overlap/heading/tie": 0.15,
	"name-overlap/test":        0.30,
	"name-overlap/test/tie":    0.25,
	"name-partial/func":        0.44,
	"name-partial/heading":     0.17,
	"name-partial/type":        0.57,
	"named-in-doc/func":        0.67,
	"named-in-doc/member":      0.83,
}

var calibrationByEvidence = map[string]float64{
	"alias-exact":   0.67,
	"calls":         0.43,
	"data":          0.67,
	"doc":           0.26,
	"name-contains": 0.15,
	"name-exact":    0.56,
	"name-overlap":  0.13,
	"name-partial":  0.44,
	"named-in-doc":  0.80,
}

// minBasisN is how often a basis must occur in the calibration set to get
// its own row.
const minBasisN = 4

// unmeasuredConf is the conf of evidence the calibration set never saw.
const unmeasuredConf = 0.3

func confFor(basis string) float64 {
	if c, ok := calibration[basis]; ok {
		return c
	}
	if c, ok := calibrationByEvidence[strings.SplitN(basis, "/", 2)[0]]; ok {
		return c
	}
	return unmeasuredConf
}

// smoothed is the Laplace estimate of a precision, to two decimals.
func smoothed(correct, n int) float64 {
	return math.Round(float64(correct+1)/float64(n+2)*100) / 100
}

// Fit computes the calibration tables from a scored batch: a row per
// basis seen at least minBasisN times, and a row per evidence kind.
func Fit(scored []Scored) (byBasis, byEvidence map[string]float64) {
	type count struct{ n, correct int }
	bb, be := map[string]*count{}, map[string]*count{}
	tally := func(m map[string]*count, k string, ok bool) {
		c := m[k]
		if c == nil {
			c = &count{}
			m[k] = c
		}
		c.n++
		if ok {
			c.correct++
		}
	}
	for _, s := range scored {
		tally(bb, s.Basis, s.Verdict == Correct)
		tally(be, strings.SplitN(s.Basis, "/", 2)[0], s.Verdict == Correct)
	}
	byBasis, byEvidence = map[string]float64{}, map[string]float64{}
	for k, c := range bb {
		if c.n >= minBasisN {
			byBasis[k] = smoothed(c.correct, c.n)
		}
	}
	for k, c := range be {
		byEvidence[k] = smoothed(c.correct, c.n)
	}
	return byBasis, byEvidence
}

// Recalibrate sets each proposal's conf from the given tables, as confFor
// would with them shipped: for held-out evaluation.
func Recalibrate(scored []Scored, byBasis, byEvidence map[string]float64) []Scored {
	out := make([]Scored, len(scored))
	for i, s := range scored {
		c, ok := byBasis[s.Basis]
		if !ok {
			if c, ok = byEvidence[strings.SplitN(s.Basis, "/", 2)[0]]; !ok {
				c = unmeasuredConf
			}
		}
		s.Conf = c
		out[i] = s
	}
	return out
}
