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

// TestRelationProfile (issue #261): relations: takes starter or full, unset
// reads as full, and InlineNames under starter is the starter list without
// the tests alias.
//
// @gloss verifies gmi:RelationProfile scope=symbol
func TestRelationProfile(t *testing.T) {
	dir := t.TempDir()
	for rel, want := range map[string]string{"": ProfileFull, "starter": ProfileStarter, "full": ProfileFull, "advanced": "error"} {
		cfg := "owns:\n  lib: { iri: https://x/lib/, tier: core }\n"
		if rel != "" {
			cfg += "relations: " + rel + "\n"
		}
		p := filepath.Join(dir, "glossmark.yaml")
		os.WriteFile(p, []byte(cfg), 0o644)
		c, err := LoadConfig(p)
		if want == "error" {
			if err == nil || !strings.Contains(err.Error(), "relations must be starter or full") {
				t.Errorf("relations: %s: err = %v", rel, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("relations: %q: %v", rel, err)
		}
		if got := c.RelationProfile(); got != want {
			t.Errorf("relations: %q: profile %q, want %q", rel, got, want)
		}
	}
	v := NewVocabulary()
	if got := strings.Join(v.InlineNames(ProfileStarter), " "); got != "consumes implements produces represents verifies" {
		t.Errorf("starter inline names: %s", got)
	}
	if full := v.InlineNames(ProfileFull); len(full) <= 5 || strings.Contains(strings.Join(full, " "), "tests") {
		t.Errorf("full inline names: %v", full)
	}
	if !InStarter("tests") || InStarter("presents") || !InStarter("realizes") {
		t.Error("InStarter: tests (alias) and realizes (seam) are in, presents is out")
	}
}
