// @gloss verifies gmi:MappingSets scope=file
// @gloss verifies gm:MappingCuration scope=file

package mapping_test

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/calib"
	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/mapping"
)

// calibrationSet runs Suggest on each labelled pair: the first namespace of
// each workspace is the owned side, the second is matched against.
func calibrationSet(t *testing.T) (map[string][]mapping.Candidate, map[string][]mapping.Label) {
	t.Helper()
	cands, labels := map[string][]mapping.Candidate{}, map[string][]mapping.Label{}
	for _, name := range []string{"payments", "library", "fleet"} {
		root := filepath.Join("testdata", "calibration", name)
		ws, err := load.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		res, err := extract.Build(ws)
		if err != nil {
			t.Fatal(err)
		}
		var prefixes []string
		for p := range ws.Owned {
			prefixes = append(prefixes, p)
		}
		sort.Strings(prefixes)
		gold, err := os.ReadFile(filepath.Join(root, "gold.tsv"))
		if err != nil {
			t.Fatal(err)
		}
		labels[name] = mapping.ReadLabels(string(gold))
		owned := labels[name][0].Subject[:strings.Index(labels[name][0].Subject, ":")]
		other := labels[name][0].Object[:strings.Index(labels[name][0].Object, ":")]
		cands[name] = mapping.Suggest(res.Graph, mapping.SuggestOptions{Owned: map[string]bool{owned: true}, Against: map[string]bool{other: true}})
	}
	return cands, labels
}

func TestMappingCalibrationIsFitted(t *testing.T) {
	cands, labels := calibrationSet(t)
	fitted := calib.Table{}
	for name := range cands {
		for b, c := range mapping.Score(cands[name], labels[name]) {
			f := fitted[b]
			f.N += c.N
			f.Correct += c.Correct
			fitted[b] = f
		}
	}
	shipped, refit := mapping.ShippedCalibration.Confs(), fitted.Confs()
	var bad []string
	for b := range fitted {
		if math.Abs(shipped(b)-refit(b)) > 0.005 {
			bad = append(bad, fmt.Sprintf("%s: shipped %.2f, measured %.2f", b, shipped(b), refit(b)))
		}
	}
	if len(bad) > 0 {
		var keys []string
		for b := range fitted {
			keys = append(keys, b)
		}
		sort.Strings(keys)
		var sb strings.Builder
		for _, b := range keys {
			fmt.Fprintf(&sb, "\t%q: {N: %d, Correct: %d},\n", b, fitted[b].N, fitted[b].Correct)
		}
		sort.Strings(bad)
		t.Errorf("ShippedCalibration drifted from the labelled set:\n  %s\nrefitted table:\nvar ShippedCalibration = calib.Table{\n%s}", strings.Join(bad, "\n  "), sb.String())
	}
}

func TestMappingSuggestions(t *testing.T) {
	cands, labels := calibrationSet(t)
	var total, correct int
	for name := range cands {
		sc := mapping.Score(cands[name], labels[name])
		for _, c := range sc {
			total += c.N
			correct += c.Correct
		}
	}
	if total == 0 {
		t.Fatal("no suggestions on the calibration set")
	}
	t.Logf("calibration set: %d suggestions, %d correct (%.0f%%)", total, correct, 100*float64(correct)/float64(total))
	// The known traps are suggested as close matches at most, or not at all:
	// lib:Hold and sch:Hold share a name and differ in kind.
	for _, c := range cands["library"] {
		if c.Subject == "lib:Hold" && c.Object == "sch:Hold" && c.Predicate == "skos:exactMatch" {
			t.Errorf("same name, different kind suggested as exact: %+v", c)
		}
	}
	// Aliases find another team's name.
	found := false
	for _, c := range cands["payments"] {
		if c.Subject == "pay:Merchant" && c.Object == "led:Vendor" && c.Predicate == "skos:exactMatch" {
			found = true
		}
	}
	if !found {
		t.Errorf("alias match pay:Merchant = led:Vendor not suggested: %+v", cands["payments"])
	}
}
