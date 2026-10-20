// @gloss verifies gmi:RollupEngine scope=file

package e2e

import (
	"sort"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// TestOverlapAndOverdue pins ADR 0019 (#95): two unrelated boundaries
// realizing or holding one concept is an overlap; a declared migration
// (deprecated + replaced-by) silences it; past retire-by the retiring
// boundary is overdue, which fails.
func TestOverlapAndOverdue(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\nprefixes:\n  o: https://x/o/\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Invoicing: { kind: process }\n  Invoice: { kind: entity }\n")
	write(".gloss.yaml", "boundary: o:root\nlevel: system\n")
	write("new/.gloss.yaml", "boundary: o:new\nlevel: container\n")
	write("old/gen.py", "# @gloss implements z:Invoicing\ndef gen(): pass\n")
	write("old/models.py", "# @gloss represents z:Invoice\nclass Invoice: pass\n")
	write("new/run.py", "# @gloss implements z:Invoicing\ndef run(): pass\n")
	write("new/table.sql", "-- @gloss represents z:Invoice\nCREATE TABLE invoices (id int);\n")
	drift := func(oldManifest string) *rollup.Report {
		write("old/.gloss.yaml", "boundary: o:old\nlevel: container\n"+oldManifest)
		ws, g, _, _ := build(t, root)
		return rollup.Compare(g, ws, rollup.Options{Policy: ws.Config.EffectivePolicy()})
	}
	rep := drift("")
	var msgs []string
	for _, f := range findings(rep, rollup.KindOverlap) {
		msgs = append(msgs, f.Msg)
	}
	all := strings.Join(msgs, "\n")
	if len(msgs) != 2 || !strings.Contains(all, "z:Invoice is held by o:new and o:old") || !strings.Contains(all, "z:Invoicing is realized by o:new and o:old") {
		t.Errorf("overlaps %v, want one for z:Invoicing and one for z:Invoice", msgs)
	}
	rep = drift("deprecated: true\nreplaced-by: o:new\nretire-by: 2999-12-31\n")
	if n := len(findings(rep, rollup.KindOverlap)) + len(findings(rep, rollup.KindOverdue)); n != 0 {
		t.Errorf("a declared migration: %d overlap/overdue findings, want 0", n)
	}
	rep = drift("deprecated: true\nreplaced-by: o:new\nretire-by: 2020-01-31\n")
	od := findings(rep, rollup.KindOverdue)
	if len(od) != 1 || od[0].Boundary != "o:old" || !strings.Contains(od[0].Msg, "due to retire by 2020-01-31") || !strings.Contains(od[0].Msg, "z:Invoicing") {
		t.Errorf("overdue = %+v", od)
	}
	kinds, _ := rollup.ParseFailOn("overdue")
	if len(rep.Failing(kinds)) != 1 {
		t.Errorf("--fail-on overdue should fail")
	}
}

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
