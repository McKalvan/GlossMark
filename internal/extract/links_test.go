// @gloss verifies gmi:LinkMapper scope=file

package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
)

// linksTree writes a workspace whose booking manifest maps paths and
// symbols to concepts (ADR 0045) and returns the extractor's result.
func linksTree(t *testing.T, files map[string]string) *Result {
	t.Helper()
	root := t.TempDir()
	base := map[string]string{
		"glossmark.yaml":        "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\nlinks:\n  - { path: \"**/*_test.go\", verifies: z:Booking }\n",
		"ontology/z.gloss.yaml": "namespace: z\nconcepts:\n  Booking: { kind: process }\n  Slot: { kind: entity }\n",
		"ontology/i.gloss.yaml": "namespace: i\nconcepts:\n  Handler: { kind: process, realizes: z:Booking }\n  Policy: { kind: process, realizes: z:Booking }\n",
		"svc/.gloss.yaml": "boundary: o:svc\nlevel: container\ndefault-namespace: i\nlinks:\n" +
			"  - { path: \"api/**\", implements: .Handler, conf: 0.8 }\n" +
			"  - { symbol: Service.Cancel, implements: .Policy, note: \"the window\" }\n" +
			"  - symbol: Slot\n    path: \"models/*.go\"\n    represents: z:Slot\n    proposed: true\n" +
			"  - { path: \"nowhere/**\", implements: .Handler }\n" +
			"  - { symbol: Missing, implements: .Handler }\n" +
			"  - { path: \"api/**\" }\n",
		"svc/api/main.go":       "package main\n\nfunc main() {}\n",
		"svc/api/marked.go":     "// @gloss implements .Policy scope=file\npackage main\n",
		"svc/booking/svc.go":    "package booking\n\ntype Service struct{}\n\n// Cancel releases the slot.\nfunc (s *Service) Cancel() {}\n",
		"svc/models/slot.go":    "package models\n\ntype Slot struct{}\n",
		"svc/booking/x_test.go": "package booking\n\nfunc TestX() {}\n",
	}
	for k, v := range files {
		base[k] = v
	}
	for rel, content := range base {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ws, err := load.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Build(ws)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestManifestLinks: a links: entry maps every scanned file a path glob
// matches at file scope, or the named declaration in each, with manifest
// provenance; an inline marker on the artifact overrides a mapping of the
// same relation; glossmark.yaml maps across the tree; and an entry that
// names nothing, or matches nothing, is reported at its line.
func TestManifestLinks(t *testing.T) {
	res := linksTree(t, nil)
	g := res.Graph
	type want struct {
		s, p, o, source, scope, symbol, from string
	}
	wants := []want{
		{"svc/api/main.go", "implements", "i:Handler", graph.SourceManifest, "file", "", "svc/.gloss.yaml:5"},
		{"svc/api/marked.go", "implements", "i:Policy", graph.SourceInline, "file", "", ""},
		{"svc/booking/svc.go#Service.Cancel", "implements", "i:Policy", graph.SourceManifest, "symbol", "Service.Cancel", "svc/.gloss.yaml:6"},
		{"svc/models/slot.go#Slot", "represents", "z:Slot", graph.SourceManifest, "symbol", "Slot", "svc/.gloss.yaml:7"},
		{"svc/booking/x_test.go", "verifies", "z:Booking", graph.SourceManifest, "file", "", "glossmark.yaml:5"},
	}
	for _, w := range wants {
		found := false
		for _, e := range g.Out(graph.ArtifactPrefix+w.s, w.p) {
			if g.Curie(e.O) != w.o || e.Source != w.source {
				continue
			}
			found = true
			if e.Prov == nil || e.Prov.Scope != w.scope || e.Prov.Symbol != w.symbol || e.Prov.From != w.from || e.Prov.Fingerprint == "" || e.Prov.ID == "" {
				t.Errorf("%s %s %s: prov %+v, want scope %q symbol %q from %q", w.s, w.p, w.o, e.Prov, w.scope, w.symbol, w.from)
			}
		}
		if !found {
			t.Errorf("no %s edge %s %s %s", w.source, w.s, w.p, w.o)
		}
	}
	// marked.go says what it implements itself, so the api/** mapping of
	// the same relation is left to the marker, whatever its target.
	for _, e := range g.Out(graph.ArtifactPrefix+"svc/api/marked.go", "implements") {
		if e.Source == graph.SourceManifest {
			t.Errorf("mapped implements beside the inline marker on marked.go: %+v", e.Prov)
		}
	}
	var mapped, proposed int
	for _, fm := range res.Markers {
		if !fm.Mapped() {
			continue
		}
		mapped++
		if fm.Marker.Proposed {
			proposed++
		}
		if file, line := fm.Stated(); file != fm.From || line != fm.FromLine || fm.ID == "" {
			t.Errorf("mapped marker %+v states %s:%d", fm, file, line)
		}
		if fm.Path == "svc/booking/svc.go" && (fm.Marker.Keys["note"] != "the window" || fm.Marker.Line != 6) {
			t.Errorf("symbol link: %+v", fm.Marker)
		}
		if fm.Path == "svc/api/main.go" && fm.Marker.Keys["conf"] != "0.8" {
			t.Errorf("conf not carried: %+v", fm.Marker)
		}
	}
	if mapped != 4 || proposed != 1 {
		t.Errorf("%d mapped links (%d proposed), want 4 (1)", mapped, proposed)
	}
	// The mapped file-scope link has the id an inline scope=file marker
	// would, so a marker rewritten as a mapping keeps its ledger row.
	if id := MarkerID("svc/api/main.go", "implements", []string{".Handler"}, "", "file"); !strings.Contains(strings.Join(ids(res), " "), id) {
		t.Errorf("mapped link ids %v lack the marker id %s", ids(res), id)
	}
	var codes []string
	for _, d := range res.Diags {
		codes = append(codes, d.Code+"@"+d.Path+":"+itoa(d.Line))
	}
	for _, w := range []string{"GM126@svc/.gloss.yaml:11", "GM126@svc/.gloss.yaml:12", "GM125@svc/.gloss.yaml:13"} {
		if !strings.Contains(strings.Join(codes, " "), w) {
			t.Errorf("diagnostics %v lack %s", codes, w)
		}
	}
}

func ids(res *Result) []string {
	var out []string
	for _, fm := range res.Markers {
		out = append(out, fm.ID)
	}
	return out
}

func itoa(n int) string { return fmt.Sprint(n) }
