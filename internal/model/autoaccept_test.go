// @gloss verifies gmi:SuggestCalibration scope=file

package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/calib"
)

func loadCfg(t *testing.T, body string, files map[string]string) (*Config, error) {
	t.Helper()
	dir := t.TempDir()
	for k, v := range files {
		os.WriteFile(filepath.Join(dir, k), []byte(v), 0o644)
	}
	p := filepath.Join(dir, "glossmark.yaml")
	os.WriteFile(p, []byte("owns:\n  z: { iri: https://x/z/, tier: core }\n"+body), 0o644)
	return LoadConfig(p)
}

// TestAutoAcceptPrecedence (ADR 0024): suggest.auto-accept-precision
// derives the threshold and beats policy.soft-conf-threshold; the latter
// alone fixes it; neither means the default target; suggest.min-conf is
// the floor; suggest.calibration replaces the shipped counts.
func TestAutoAcceptPrecedence(t *testing.T) {
	shipped := func(target, floor float64) calib.AutoAccept {
		return calib.Derive(calib.Shipped.Confs(), calib.Shipped, target, floor)
	}
	cases := []struct {
		name, body string
		files      map[string]string
		fixed      bool
		threshold  float64
		target     float64
		source     string
		notes      int
	}{
		{"neither", "", nil, false, shipped(0.9, 0.3).Threshold, 0.9, "shipped", 0},
		{"fixed", "policy:\n  soft-conf-threshold: 0.65\n", nil, true, 0.65, 0, "shipped", 0},
		{"target", "suggest:\n  auto-accept-precision: 0.8\n", nil, false, shipped(0.8, 0.3).Threshold, 0.8, "shipped", 0},
		{"both", "suggest:\n  auto-accept-precision: 0.8\npolicy:\n  soft-conf-threshold: 0.65\n", nil, false, shipped(0.8, 0.3).Threshold, 0.8, "shipped", 1},
		{"floor", "suggest:\n  auto-accept-precision: 0.5\n  min-conf: 0.5\n", nil, false, shipped(0.5, 0.5).Threshold, 0.5, "shipped", 0},
		{"repo calibration", "suggest:\n  calibration: cal.tsv\n", map[string]string{"cal.tsv": "# measured\nname-exact/type\t10\t10\ndoc/func\t10\t0\n"}, false, calib.Shipped.Conf("name-exact/type"), 0.9, "cal.tsv", 0},
		{"missing calibration", "suggest:\n  calibration: cal.tsv\n", nil, false, shipped(0.9, 0.3).Threshold, 0.9, "shipped", 1},
	}
	for _, c := range cases {
		cfg, err := loadCfg(t, c.body, c.files)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		a := cfg.AutoAccept()
		if a.Fixed != c.fixed || a.Threshold != c.threshold || a.Target != c.target || a.Source != c.source {
			t.Errorf("%s: %+v, want fixed=%v threshold=%.2f target=%.2f source=%s", c.name, a, c.fixed, c.threshold, c.target, c.source)
		}
		if got := cfg.EffectivePolicy().SoftConfThreshold; got != a.Threshold {
			t.Errorf("%s: policy threshold %.2f, rule %.2f", c.name, got, a.Threshold)
		}
		if n := len(cfg.AutoAcceptNotes()); n != c.notes {
			t.Errorf("%s: notes %v", c.name, cfg.AutoAcceptNotes())
		}
	}
	// A nil config (no glossmark.yaml read) gets the default too.
	var nilCfg *Config
	if got := nilCfg.EffectivePolicy().SoftConfThreshold; got != shipped(0.9, 0.3).Threshold {
		t.Errorf("nil config: %.2f", got)
	}
}

func TestSuggestConfigErrors(t *testing.T) {
	for body, want := range map[string]string{
		"suggest:\n  auto-accept-precision: 2\n": "auto-accept-precision must be between 0 and 1",
		"suggest:\n  min-conf: 1.5\n":            "min-conf must be between 0 and 1",
		"suggest:\n  calibration: c.tsv\n":       "suggest.calibration c.tsv: line 1",
		"suggest:\n  precision: 0.9\n":           "field precision not found",
	} {
		_, err := loadCfg(t, body, map[string]string{"c.tsv": "a\tb\n"})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want %q", body, err, want)
		}
	}
}
