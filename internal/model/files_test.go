// @gloss verifies gmi:WorkspaceLoader scope=file

package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadPolicy(t *testing.T, policy string) (*Config, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "glossmark.yaml")
	if err := os.WriteFile(p, []byte("owns:\n  z: { iri: https://x/z/, tier: core }\npolicy: "+policy+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return LoadConfig(p)
}

// TestCoverageFailUnder: policy.coverage-fail-under takes a percentage or
// current, as the --fail-under flag does (#128).
func TestCoverageFailUnder(t *testing.T) {
	for _, c := range []struct {
		policy, want, err string
	}{
		{"{ coverage-fail-under: current }", "current", ""},
		{`{ coverage-fail-under: "current" }`, "current", ""},
		{"{ coverage-fail-under: Current }", "current", ""},
		{"{ coverage-fail-under: 70 }", "70", ""},
		{"{ coverage-fail-under: 62.5 }", "62.5", ""},
		{"{}", "", ""},
		{"{ coverage-fail-under: lots }", "", "neither a percentage"},
		{"{ coverage-fail-under: 120 }", "", "neither a percentage"},
	} {
		cfg, err := loadPolicy(t, c.policy)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%s: err = %v, want %q", c.policy, err, c.err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.policy, err)
			continue
		}
		if got := string(cfg.EffectivePolicy().CoverageFailUnder); got != c.want {
			t.Errorf("%s: coverage-fail-under = %q, want %q", c.policy, got, c.want)
		}
	}
}
