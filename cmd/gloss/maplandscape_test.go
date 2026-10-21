// @gloss verifies gmi:LandscapeAssembler scope=file

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLandscapeMappings: an accepted exact match makes two repositories'
// concepts one in the landscape (an equivalence class, and a context-map edge
// between a publisher of one and a subscriber of the other); a proposed match
// means nothing yet; with mapping-lift a narrow match lifts as is-a.
func TestLandscapeMappings(t *testing.T) {
	t.Setenv("GLOSS_ACTOR", "human:bob")
	farm := zooFarm(t)
	zoo := filepath.Join(filepath.Dir(farm), "zoo")
	writeTree(t, filepath.Dir(farm), map[string]string{
		"zoo/births/.gloss.yaml":   "boundary: o:births\nlevel: component\npublishes: [zoo:FoalBorn]\n",
		"zoo/births/announce.py":   "# @gloss publishes zoo:FoalBorn\ndef announce(foal): ...\n",
		"farm/stables/.gloss.yaml": "boundary: o:stables\nlevel: component\nsubscribes: [farm:FoalBirth]\n",
		"farm/stables/on_birth.py": "# @gloss subscribes farm:FoalBirth\ndef on_birth(evt): ...\n",
	})
	gloss(t, "map", "init", "zoo-farm", "--owner", "farm-ops", "--root", farm)
	gloss(t, "map", "propose", "farm:FoalBirth", "exact", "zoo:FoalBorn", "--root", farm)
	gloss(t, "map", "propose", "farm:Colt", "narrow", "zoo:Foal", "--root", farm)

	landscape := func() (string, map[string]any) {
		t.Helper()
		out, _ := capture(t, "landscape", "--fail-on", "none", zoo, farm)
		js, _ := capture(t, "landscape", "--format", "json", "--fail-on", "none", zoo, farm)
		var m map[string]any
		if err := json.Unmarshal([]byte(js), &m); err != nil {
			t.Fatalf("landscape json: %v\n%s", err, js)
		}
		return out, m
	}
	out, _ := landscape()
	if strings.Contains(out, "o:stables → o:births") || strings.Contains(out, "same concept") {
		t.Errorf("a proposed mapping changed the landscape:\n%s", out)
	}
	if !strings.Contains(out, "2 proposed") {
		t.Errorf("landscape does not count the proposals:\n%s", out)
	}

	gloss(t, "map", "accept", "farm:FoalBirth", "exact", "zoo:FoalBorn", "--root", farm)
	gloss(t, "map", "accept", "farm:Colt", "narrow", "zoo:Foal", "--root", farm)
	out, m := landscape()
	for _, want := range []string{"o:stables → o:births via farm:FoalBirth = zoo:FoalBorn", "same concept: farm:FoalBirth = zoo:FoalBorn"} {
		if !strings.Contains(out, want) {
			t.Errorf("landscape lacks %q:\n%s", want, out)
		}
	}
	if ms, _ := m["mappings"].(map[string]any); ms["Lifted"] != float64(0) {
		t.Errorf("lifted without mapping-lift: %v", ms)
	}

	// Lift is opt-in: any member repository may turn it on.
	cfg := filepath.Join(farm, "glossmark.yaml")
	b, _ := os.ReadFile(cfg)
	os.WriteFile(cfg, append(b, []byte("mapping-lift: true\n")...), 0o644)
	out, m = landscape()
	if ms, _ := m["mappings"].(map[string]any); ms["Lifted"] != float64(1) {
		t.Errorf("mapping-lift: %v, want 1 lifted", ms)
	}
	if !strings.Contains(out, "1 is-a from broad/narrow matches") {
		t.Errorf("landscape text does not report the lift:\n%s", out)
	}
}
