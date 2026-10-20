// @gloss verifies gmi:WorkspaceLoader, gmi:Extractor, gmi:RollupEngine, gmi:DriftComparator, gmi:Linter, gmi:ViewProjector, gmi:CriticalityStamper scope=file

package e2e

import (
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/rollup"
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
