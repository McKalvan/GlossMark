// @gloss implements gmi:SuggestCalibration scope=file

package suggest

import "github.com/mckalvan/glossmark/internal/calib"

// A proposal's conf is the precision its basis (evidence/role, plus /tie
// when an unrelated concept matched as strongly) measured on the
// calibration set: calib.Shipped, the counts of the labelled workspaces in
// calibrate_test.go, stripped of their markers and re-suggested. Each
// value is (correct+1)/(n+2), so a basis seen a few times stays near one
// half until the data says otherwise (ADR 0021).
var confFor = calib.Shipped.Confs()

// Fit counts a scored batch per basis: the table calib.Shipped is, and
// what gloss suggest --calibrate writes for a repository.
func Fit(scored []Scored) calib.Table {
	t := calib.Table{}
	for _, s := range scored {
		c := t[s.Basis]
		c.N++
		if s.Verdict == Correct {
			c.Correct++
		}
		t[s.Basis] = c
	}
	return t
}

// Recalibrate sets each proposal's conf from the given table, as confFor
// would with it shipped: for held-out evaluation.
func Recalibrate(scored []Scored, t calib.Table) []Scored {
	conf := t.Confs()
	out := make([]Scored, len(scored))
	for i, s := range scored {
		s.Conf = conf(s.Basis)
		out[i] = s
	}
	return out
}
