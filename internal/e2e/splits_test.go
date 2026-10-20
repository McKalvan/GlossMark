// @gloss verifies gmi:RollupEngine scope=file

package e2e

import (
	"sort"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// TestRetypeExplained: a manifest holds a concept that is no longer an
// entity; check says so (GM223) and the drift gap names the link that did
// not lift instead of claiming nothing represents it (#111).
func TestRetypeExplained(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\nprefixes:\n  o: https://x/o/\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Tariff: { kind: constraint }\n")
	write(".gloss.yaml", "boundary: o:analytics\nlevel: system\nholds: [z:Tariff]\n")
	write("models/tariff_bands.sql", "-- @gloss represents z:Tariff\nCREATE TABLE tariff_bands (id int);\n")
	ws, g, _, _ := build(t, root)
	var gm223 []string
	for _, d := range check.Run(ws, g) {
		if d.Code == "GM223" {
			gm223 = append(gm223, d.String())
		}
	}
	if len(gm223) != 1 || !strings.Contains(gm223[0], ".gloss.yaml:1") || !strings.Contains(gm223[0], "z:Tariff is a constraint; holds takes entities") {
		t.Errorf("GM223 = %v", gm223)
	}
	rep := rollup.Compare(g, ws, rollup.Options{Policy: ws.Config.EffectivePolicy()})
	gaps := findings(rep, rollup.KindGap)
	if len(gaps) != 1 || !strings.Contains(gaps[0].Msg, "models/tariff_bands.sql:1 represents") || strings.Contains(gaps[0].Msg, "nothing under it") {
		t.Errorf("gap = %+v", gaps)
	}
}

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
