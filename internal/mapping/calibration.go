// @gloss implements gmi:MappingSets scope=file

package mapping

import "github.com/mckalvan/glossmark/internal/calib"

// ShippedCalibration is what each evidence basis of Suggest measured on the
// labelled mapping set in testdata/calibration (three namespace pairs, gold
// mappings written by a reviewer). A suggestion's conf is its basis's
// smoothed precision, (correct+1)/(n+2), as for gloss suggest (ADR 0021).
// TestMappingCalibrationIsFitted fails, and prints the refitted table, when
// this drifts from what the set measures. The set is small: a repository
// measures its own precision with gloss map suggest --eval.
var ShippedCalibration = calib.Table{
	"alias-exact":                {N: 15, Correct: 11},
	"name-exact":                 {N: 5, Correct: 5},
	"name-exact/kind-mismatch":   {N: 2, Correct: 1},
	"name-fuzzy":                 {N: 1, Correct: 1},
	"name-overlap":               {N: 3, Correct: 2},
	"name-overlap/kind-mismatch": {N: 1, Correct: 0},
}
