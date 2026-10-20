// @gloss verifies gmi:RollupEngine scope=file

package e2e

import (
	"sort"
	"testing"

	"github.com/mckalvan/glossmark/internal/rollup"
)

// TestDanglingDeclaredReferences: drift reports every unresolved concept
// reference gloss check knows, not only markers: manifest relation keys,
// allowed: entries and the workspace's own ontology files (#110).
func TestDanglingDeclaredReferences(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\nprefixes:\n  o: https://x/o/\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Layer: { kind: process, subscribes: [z:GoneEvent] }\n  Map: { kind: entity }\n")
	write(".gloss.yaml", "boundary: o:map\nlevel: system\nsubscribes: [z:Outage]\nallowed: [z:Layer, z:Retired]\n")
	write("src/map.py", "# @gloss implements z:Layer\ndef layer(): pass\n")
	ws, g, _, _ := build(t, root)
	rep := rollup.Compare(g, ws, rollup.Options{Policy: ws.Config.EffectivePolicy()})
	var got []string
	for _, f := range findings(rep, rollup.KindDangling) {
		got = append(got, f.Concept+"@"+f.Path)
	}
	sort.Strings(got)
	want := []string{"z:GoneEvent@ontology/z.gloss.yaml", "z:Outage@.gloss.yaml", "z:Retired@.gloss.yaml"}
	if len(got) != len(want) {
		t.Fatalf("dangling findings %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dangling findings %v, want %v", got, want)
		}
	}
}
