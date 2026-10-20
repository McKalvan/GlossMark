// @gloss verifies gmi:WorkspaceLoader, gmi:Extractor, gmi:RollupEngine, gmi:DriftComparator, gmi:Linter, gmi:ViewProjector, gmi:CriticalityStamper scope=file

package e2e

import (
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/views"
)

// TestGraphQLScannedAndUnscannedMarkers: GraphQL SDL is in the built-in
// comment table, and a marker in a file type with no comment style is
// reported instead of silently skipped (#57).
func TestGraphQLScannedAndUnscannedMarkers(t *testing.T) {
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  hp: { iri: https://x/hp/, tier: core }\n")
	write("ontology/hp.gloss.yaml", "concepts:\n  Order: { kind: entity }\n")
	write("schema.graphql", "# @gloss represents hp:Order\ntype Order { id: ID! }\n")
	write("api.gql", "# @gloss represents hp:Order\ntype Q { order: Order }\n")
	write("model.pkl", "class X {}\n// @gloss represents hp:Order\n")
	write("data.json", "{\"note\": \"@gloss represents hp:Order\"}\n")
	write("annotated.json", "{\"note\": \"@gloss represents hp:Order\"}\n")
	write("annotated.json.gloss", "represents hp:Order\n")
	write("plain.txt", "nothing here\n")
	_, _, res, _ := build(t, root)
	paths := map[string]bool{}
	for _, m := range res.Markers {
		paths[m.Path] = true
	}
	if res.ScannedFiles != 3 || !paths["schema.graphql"] || !paths["api.gql"] {
		t.Errorf("scanned %d files, markers in %v; want both GraphQL files (and the sidecar) scanned", res.ScannedFiles, paths)
	}
	ws, err := load.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range ws.Diags {
		if d.Code == "GM117" {
			got = append(got, d.String())
		}
	}
	if len(got) != 2 || !strings.HasPrefix(got[0], "data.json:1:") || !strings.HasPrefix(got[1], "model.pkl:2:") || !strings.Contains(got[1], ".pkl") {
		t.Errorf("GM117 = %v, want data.json:1 and model.pkl:2", got)
	}
	// A comment style in glossmark.yaml scans the file and silences GM117.
	write("glossmark.yaml", "owns:\n  hp: { iri: https://x/hp/, tier: core }\ncomment-styles: { \".pkl\": [\"//\"] }\n")
	ws, g, _, _ := build(t, root)
	for _, d := range check.Run(ws, g) {
		if d.Code == "GM117" && strings.HasPrefix(d.Path, "model.pkl") {
			t.Errorf("GM117 with a comment style: %s", d)
		}
	}
}

// TestRepresentsAnyKindHolds: a table that stores a measure's values holds
// the measure, so a manifest declaring it under holds: with no allowed: is
// consistent: no GM503 and no gap (#63, ADR 0013).
func TestRepresentsAnyKindHolds(t *testing.T) {
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  wh: { iri: https://x/wh/, tier: core }\n")
	write("ontology/wh.gloss.yaml", "concepts:\n  Count: { kind: entity }\n  CountVariance: { kind: measure }\n  Recount: { kind: event }\n")
	write("sql/.gloss.yaml", "boundary: whb:db\nlevel: container\nholds: [wh:Count, wh:CountVariance]\n")
	write("sql/tables.sql", "-- @gloss represents wh:Count\nCREATE TABLE tbl_count (id int);\n-- @gloss represents wh:CountVariance\nCREATE TABLE tbl_cnt (id int);\n")
	ws, g, _, lg := build(t, root)
	for _, d := range check.Run(ws, g) {
		if d.Severity != "info" {
			t.Errorf("check: %s", d)
		}
	}
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	for _, k := range []string{rollup.KindGap, rollup.KindViolation} {
		if fs := findings(rep, k); len(fs) != 0 {
			t.Errorf("%s: %+v", k, fs)
		}
	}
	// An undeclared concept the code represents is a holds violation.
	write("sql/events.sql", "-- @gloss represents wh:Recount\nCREATE TABLE recounts (id int);\n")
	ws, g, _, lg = build(t, root)
	rep = rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	if v := findings(rep, rollup.KindViolation); len(v) != 1 || v[0].Concept != "wh:Recount" || v[0].Relation != "holds" {
		t.Errorf("violations = %+v, want holds wh:Recount", v)
	}
}

// harbor writes the shared core of the surfaces fixtures: an order service
// whose engine enforces a critical credit limit, inside one system.
func harbor(t *testing.T) (string, func(rel, content string)) {
	t.Helper()
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  hp: { iri: https://x/hp/, tier: core }\n  hpx: { iri: https://x/hpx/, tier: implementation }\n")
	write("ontology/hp.gloss.yaml", "concepts:\n"+
		"  OrderPlacement: { kind: process, criticality: high }\n"+
		"  CreditLimit: { kind: constraint, criticality: critical, constrains: OrderPlacement }\n"+
		"  Order: { kind: entity, criticality: high }\n")
	write("ontology/hpx.gloss.yaml", "concepts:\n"+
		"  OrderIntake: { kind: process, realizes: hp:OrderPlacement }\n"+
		"  CreditCheck: { kind: constraint, realizes: hp:CreditLimit }\n")
	write(".gloss.yaml", "boundary: hpb:harbor\nlevel: system\nrealizes: inherit\n")
	write("orders/.gloss.yaml", "boundary: hpb:orders\nlevel: container\npart-of: hpb:harbor\nrealizes: [hpx:OrderIntake, hpx:CreditCheck]\ndefault-namespace: hpx\n")
	write("orders/placement.py", "# @gloss implements .CreditCheck\ndef check_credit(): pass\n\n# @gloss implements .OrderIntake\ndef place_order(): pass\n")
	write("orders/tests/test_placement.py", "# @gloss verifies hp:OrderPlacement\ndef test_place(): pass\n\n# @gloss verifies hp:CreditLimit\ndef test_limit(): pass\n")
	return root, write
}

// placementOf returns a concept's boundary in the engineering view.
func placementOf(t *testing.T, root, concept string) string {
	t.Helper()
	ws, g, _, _ := build(t, root)
	for _, p := range views.ProjectAll(g, ws.Views) {
		if p.Name != "engineering" {
			continue
		}
		for _, n := range p.Nodes {
			if n.ID == concept {
				return n.Boundary
			}
		}
	}
	t.Fatalf("%s not in the engineering view", concept)
	return ""
}

// TestDeclaredEmptyList: realizes: [] says the boundary realizes nothing, so
// code that implements a concept under it is a violation, while an unwritten
// relation is still not compared; and a presents gap over code that
// implements the concept says so (#49, ADR 0012).
func TestDeclaredEmptyList(t *testing.T) {
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  hp: { iri: https://x/hp/, tier: core }\n")
	write("ontology/hp.gloss.yaml", "concepts:\n  Pricing: { kind: process }\n")
	write("storefront/quotes/views.py", "# @gloss implements hp:Pricing\ndef quote_view(): pass\n")
	run := func(manifest string) *rollup.Report {
		t.Helper()
		write("storefront/.gloss.yaml", "boundary: hpb:storefront\nlevel: container\n"+manifest)
		ws, g, _, lg := build(t, root)
		return rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	}
	rep := run("presents: [hp:Pricing]\nrealizes: []\n")
	if v := findings(rep, rollup.KindViolation); len(v) != 1 || v[0].Relation != "realizes" || !strings.HasPrefix(v[0].Msg, "hpb:storefront realizes hp:Pricing, declared realizes []") {
		t.Errorf("violations = %+v, want hpb:storefront realizes hp:Pricing, declared realizes []", v)
	}
	if gp := findings(rep, rollup.KindGap); len(gp) != 1 || !strings.Contains(gp[0].Msg, "nothing under it presents it; its code implements it instead") {
		t.Errorf("gaps = %+v, want a presents gap that names the implementation", gp)
	}
	// Without the empty list realizes is not compared.
	rep = run("presents: [hp:Pricing]\n")
	if v := findings(rep, rollup.KindViolation); len(v) != 0 {
		t.Errorf("violations without realizes: [] = %+v", v)
	}
	// An empty list with nothing derived is consistent.
	write("storefront/quotes/views.py", "# @gloss presents hp:Pricing\ndef quote_view(): pass\n")
	rep = run("presents: [hp:Pricing]\nrealizes: []\nholds: []\n")
	for _, k := range []string{rollup.KindGap, rollup.KindViolation} {
		if fs := findings(rep, k); len(fs) != 0 {
			t.Errorf("%s: %+v", k, fs)
		}
	}
}

// TestHostsHidesNoMechanism: hosts on a process warns in check (GM223) and
// is presented-only in drift when nothing implements it; hosts on a
// system-type or a page concept does neither; presents on a constraint
// suggests configures (GM224) (#50, ADR 0012).
func TestHostsHidesNoMechanism(t *testing.T) {
	root, write := workspace(t)
	write("glossmark.yaml", "owns:\n  hp: { iri: https://x/hp/, tier: core }\n  hpx: { iri: https://x/hpx/, tier: implementation }\n")
	write("ontology/hp.gloss.yaml", "concepts:\n"+
		"  PeriodClose: { kind: process, criticality: critical }\n"+
		"  Finance: { kind: system-type }\n"+
		"  CreditLimit: { kind: constraint }\n"+
		"  Pricing: { kind: process }\n")
	write("ontology/hpx.gloss.yaml", "concepts:\n  QuotePage: { kind: process, presents: hp:Pricing }\n")
	write("tools/hpctl/.gloss.yaml", "boundary: hpb:hpctl\nlevel: container\nhosts: [hp:PeriodClose]\n")
	write("tools/hpctl/cli.py", "# @gloss hosts hp:Finance\nimport click\n")
	write("tools/hpctl/shell.py", "# @gloss hosts hpx:QuotePage\nimport click\n")
	write("tools/hpctl/close_period.py", "import click\n\n\n# @gloss hosts hp:PeriodClose\ndef close_period(): pass\n")
	write("tools/hpctl/limits.py", "# @gloss presents hp:CreditLimit\ndef show(): pass\n")
	ws, g, _, lg := build(t, root)
	codes := map[string][]string{}
	for _, d := range check.Run(ws, g) {
		codes[d.Code] = append(codes[d.Code], d.String())
	}
	if w := codes["GM223"]; len(w) != 1 || !strings.HasPrefix(w[0], "tools/hpctl/close_period.py:4:") {
		t.Errorf("GM223 = %v, want one on close_period.py:4", w)
	}
	if w := codes["GM224"]; len(w) != 1 || !strings.Contains(w[0], "configures") {
		t.Errorf("GM224 = %v, want one suggesting configures", w)
	}
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	po := map[string]string{}
	for _, f := range findings(rep, rollup.KindPresentedOnly) {
		po[f.Concept] = f.Msg
	}
	if !strings.Contains(po["hp:PeriodClose"], "is hosted") {
		t.Errorf("presented-only = %v, want hp:PeriodClose hosted", po)
	}
	if _, ok := po["hp:Finance"]; ok {
		t.Errorf("a hosted system-type is not presented-only: %v", po)
	}
	if _, ok := po["hpx:QuotePage"]; ok {
		t.Errorf("a hosted page concept is not presented-only: %v", po)
	}
	for _, r := range rollup.ConceptStatus(g, rep) {
		if r.Concept == "hp:PeriodClose" && r.Status == rollup.StatusImplemented {
			t.Errorf("hp:PeriodClose status = %s", r.Status)
		}
	}
}

// TestConfiguresInManifests: a page that edits a constraint's value is
// declared with configures:, which the derived allowed set admits (its test
// too), lifts to the boundary as configures on any kind, is compared by
// drift, gives the page the constraint's level and leaves the constraint in
// the engine's box (#51, ADR 0012).
func TestConfiguresInManifests(t *testing.T) {
	root, write := harbor(t)
	write("admin/.gloss.yaml", "boundary: hpb:admin\nlevel: container\npart-of: hpb:harbor\ndefault-namespace: hpx\nconfigures: [hp:CreditLimit]\n")
	write("admin/CreditLimitPage.tsx", "// @gloss configures hp:CreditLimit\nexport function CreditLimitPage() {}\n")
	write("admin/tests/CreditLimitPage.test.tsx", "// @gloss verifies hp:CreditLimit\ntest('saves', () => {})\n")
	ws, g, _, lg := build(t, root)
	for _, d := range check.Run(ws, g) {
		if d.Severity != "info" {
			t.Errorf("check: %s", d)
		}
	}
	admin := g.NodeByCurie("hpb:admin")
	var derived bool
	for _, e := range g.Out(admin.ID, "configures") {
		derived = derived || e.Source == graph.SourceDerived && g.Curie(e.O) == "hp:CreditLimit"
	}
	if !derived {
		t.Errorf("no derived hpb:admin configures hp:CreditLimit edge")
	}
	if r := curies(g, g.Objects(admin.ID, "realizes")); has(r, "hp:CreditLimit") {
		t.Errorf("configures on a constraint must not lift to realizes: %v", r)
	}
	rep := rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	for _, k := range []string{rollup.KindGap, rollup.KindViolation} {
		if fs := findings(rep, k); len(fs) != 0 {
			t.Errorf("%s: %+v", k, fs)
		}
	}
	for _, a := range g.NodesOf(model.NodeArtifact) {
		if a.Path == "admin/CreditLimitPage.tsx" && a.Criticality != "critical" {
			t.Errorf("value editor criticality = %q, want critical", a.Criticality)
		}
	}
	if b := placementOf(t, root, "hp:CreditLimit"); b != "hpb:orders" {
		t.Errorf("hp:CreditLimit placed in %q, want hpb:orders", b)
	}
	// A declared configures the code does not do is a gap.
	write("admin/CreditLimitPage.tsx", "export function CreditLimitPage() {}\n")
	ws, g, _, lg = build(t, root)
	rep = rollup.Compare(g, ws, rollup.Options{Ledger: lg.Entries, Policy: ws.Config.EffectivePolicy()})
	if gp := findings(rep, rollup.KindGap); len(gp) != 1 || gp[0].Relation != "configures" || !strings.Contains(gp[0].Msg, "nothing under it configures it") {
		t.Errorf("gaps = %+v, want configures hp:CreditLimit", gp)
	}
}
