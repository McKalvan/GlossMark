// @gloss verifies gmi:AdoptionLadder scope=file

package model

import (
	"strings"
	"testing"
)

// TestAdoptionLevel: unset is full enforcement, each rung caps what fails,
// and a level off the ladder is refused when glossmark.yaml is read.
func TestAdoptionLevel(t *testing.T) {
	cases := []struct {
		body                    string
		level                   int
		set, check, drift, gate bool
		minCrit                 string
	}{
		{"", 3, false, true, true, true, ""},
		{"adoption: 0\n", 0, true, false, false, false, ""},
		{"adoption: 1\n", 1, true, true, false, false, ""},
		{"adoption: 2\n", 2, true, true, true, true, "high"},
		{"adoption: 3\n", 3, true, true, true, true, ""},
	}
	for _, c := range cases {
		cfg, err := loadCfg(t, c.body, nil)
		if err != nil {
			t.Fatalf("%q: %v", c.body, err)
		}
		lvl := cfg.AdoptionLevel()
		if lvl != c.level || cfg.AdoptionSet() != c.set {
			t.Errorf("%q: level %d set %v, want %d %v", c.body, lvl, cfg.AdoptionSet(), c.level, c.set)
		}
		if AdoptionChecksFail(lvl) != c.check || AdoptionDriftFails(lvl) != c.drift || AdoptionGateFails(lvl) != c.gate || AdoptionMinCriticality(lvl) != c.minCrit {
			t.Errorf("%q: check %v drift %v gate %v min %q", c.body, AdoptionChecksFail(lvl), AdoptionDriftFails(lvl), AdoptionGateFails(lvl), AdoptionMinCriticality(lvl))
		}
	}
	for _, bad := range []string{"adoption: 4\n", "adoption: -1\n"} {
		if _, err := loadCfg(t, bad, nil); err == nil || !strings.Contains(err.Error(), "adoption must be a level from 0 to 3") {
			t.Errorf("%q: err = %v", bad, err)
		}
	}
	if n := AdoptionNote(1, "drift reports only"); !strings.Contains(n, "level 1 (manifests)") || !strings.Contains(n, "set adoption: 2") {
		t.Errorf("note = %q", n)
	}
	if n := AdoptionNote(3, "x"); n != "" {
		t.Errorf("note at full = %q", n)
	}
	if len(AdoptionLevels) != AdoptionFull+1 {
		t.Errorf("%d levels described", len(AdoptionLevels))
	}
}
