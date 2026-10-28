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

// TestManifestLinks: a links: entry decodes its path or symbol, the review
// keys and every other key as a relation, and records the lines it spans in
// the file, in a manifest and in glossmark.yaml alike (ADR 0041).
func TestManifestLinks(t *testing.T) {
	dir := t.TempDir()
	mp := filepath.Join(dir, ".gloss.yaml")
	os.WriteFile(mp, []byte("boundary: o:b\nlinks:\n  - { path: \"api/**\", implements: i:H, verifies: [z:A, z:B], conf: 0.5 }\n  - symbol: Svc.Run\n    represents: z:S\n    proposed: true\n    note: why\n"), 0o644)
	m, unknown, err := LoadManifest(mp)
	if err != nil || len(unknown) != 0 || len(m.Links) != 2 {
		t.Fatalf("manifest: %v %v %+v", err, unknown, m)
	}
	l := m.Links[0]
	if l.Path != "api/**" || l.Conf == nil || *l.Conf != 0.5 || l.Line != 3 || l.EndLine != 3 || strings.Join(l.RelationNames(), ",") != "implements,verifies" || len(l.Relations["verifies"]) != 2 {
		t.Errorf("flow entry: %+v", l)
	}
	l = m.Links[1]
	if l.Symbol != "Svc.Run" || !l.Proposed || l.Note != "why" || l.Line != 4 || l.EndLine != 7 || l.Where() != `symbol "Svc.Run"` {
		t.Errorf("block entry: %+v", l)
	}
	cp := filepath.Join(dir, "glossmark.yaml")
	os.WriteFile(cp, []byte("owns:\n  z: { iri: https://x/z/, tier: core }\nlinks:\n  - { path: \"**/*_test.go\", verifies: z:A }\n"), 0o644)
	c, err := LoadConfig(cp)
	if err != nil || len(c.Links) != 1 || c.Links[0].Line != 4 || c.Links[0].Path != "**/*_test.go" {
		t.Errorf("config links: %v %+v", err, c.Links)
	}
}
