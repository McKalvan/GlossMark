// @gloss verifies gmi:RollupEngine scope=file

package e2e

import (
	"sort"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/views"
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

// TestReplacedByEdgeAndViews pins ADR 0020 (#100, #101): replaced-by is a
// declared edge views can select and draw; view nodes carry deprecated and
// replaced-by; a successor in a namespace known only under prefixes: needs
// no import and is no error, and GM222 says it is unresolved here; an unknown
// views.yaml key is a diagnostic, not an abort.
func TestReplacedByEdgeAndViews(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  hire: { iri: https://x/hire/, tier: core }\n  hq: { iri: https://x/hq/, tier: implementation }\nprefixes:\n  o: https://x/o/\n  inv: https://x/inv/\n")
	write("ontology/hire.gloss.yaml", "concepts:\n  Invoicing: { kind: process }\n")
	write("ontology/hq.gloss.yaml", "concepts:\n  InvoiceGenerator: { kind: process, realizes: hire:Invoicing, deprecated: true, replaced-by: inv:InvoiceRun }\n  Router: { kind: process, realizes: hire:Invoicing }\n")
	write("ontology/views.yaml", "views:\n  migration:\n    include: [hire:Invoicing, hq:InvoiceGenerator, hq:Router]\n    relations: [realizes, replaced-by]\n    zoom: code\n")
	write(".gloss.yaml", "boundary: o:hirehq-invoicing\nlevel: container\ndeprecated: true\nreplaced-by: o:invoicing-service\n")
	write("gen.py", "# @gloss implements hq:InvoiceGenerator\ndef gen(): pass\n")
	ws, g, res, _ := build(t, root)
	var gm []string
	for _, d := range append(append(ws.Diags, res.Diags...), check.Run(ws, g)...) {
		switch d.Code {
		case "GM341", "GM215", "GM230", "GM222", "GM117":
			gm = append(gm, d.String())
		}
	}
	all := strings.Join(gm, "\n")
	if strings.Contains(all, "GM341") || strings.Contains(all, "GM215") {
		t.Errorf("a successor under prefixes: needs no import:\n%s", all)
	}
	if !strings.Contains(all, "hq:InvoiceGenerator is deprecated; its replacement inv:InvoiceRun is unresolved here") {
		t.Errorf("GM222 should say the replacement is unresolved:\n%s", all)
	}
	n := 0
	for _, e := range g.Edges() {
		if e.P == "replaced-by" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("%d replaced-by edges, want one for the concept and one for the boundary", n)
	}
	rep := rollup.Compare(g, ws, rollup.Options{Policy: ws.Config.EffectivePolicy()})
	if d := findings(rep, rollup.KindDangling); len(d) != 0 {
		t.Errorf("an unloaded successor is not dangling in one workspace: %+v", d)
	}

	// With the successor in the workspace the view draws the edge.
	write("glossmark.yaml", "owns:\n  hire: { iri: https://x/hire/, tier: core }\n  hq: { iri: https://x/hq/, tier: implementation }\n  inv: { iri: https://x/inv/, tier: implementation }\nprefixes:\n  o: https://x/o/\n")
	write("ontology/inv.gloss.yaml", "concepts:\n  InvoiceRun: { kind: process, realizes: hire:Invoicing }\n")
	write("ontology/views.yaml", "views:\n  migration:\n    include: [hire:Invoicing, hq:InvoiceGenerator, inv:InvoiceRun]\n    relations: [realizes, replaced-by]\n    zoom: code\n")
	ws, g, _, _ = build(t, root)
	p := views.Project(g, "migration", ws.Views.Views["migration"])
	var dep *views.PNode
	for i := range p.Nodes {
		if p.Nodes[i].ID == "hq:InvoiceGenerator" {
			dep = &p.Nodes[i]
		}
	}
	if dep == nil || !dep.Deprecated || dep.ReplacedBy != "inv:InvoiceRun" {
		t.Errorf("view node = %+v", dep)
	}
	found := false
	for _, e := range p.Edges {
		found = found || (e.S == "hq:InvoiceGenerator" && e.P == "replaced-by" && e.O == "inv:InvoiceRun")
	}
	if !found {
		t.Errorf("view edges %+v lack the replaced-by edge", p.Edges)
	}

	// An unknown key in a view is a diagnostic at its line; the view still loads.
	write("ontology/views.yaml", "views:\n  migration:\n    include: [hire:Invoicing]\n    where: { deprecated: true }\n")
	ws, err := load.Load(root)
	if err != nil {
		t.Fatalf("an unknown view key aborted the load: %v", err)
	}
	var diags []string
	for _, d := range ws.Diags {
		diags = append(diags, d.String())
	}
	if !strings.Contains(strings.Join(diags, "\n"), `ontology/views.yaml:4: error: unknown key "views.migration.where"`) || ws.Views.Views["migration"] == nil {
		t.Errorf("diags %v, views %v", diags, ws.Views.Views)
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
