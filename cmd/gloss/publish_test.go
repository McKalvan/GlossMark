package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// publishFixture is an owner with a core namespace and an implementation
// namespace in which only Window is published.
var publishFixture = map[string]string{
	"glossmark.yaml": "owns:\n" +
		"  frt: { iri: https://x.test/frt/, file: ontology/frt.gloss.yaml, tier: core }\n" +
		"  shc: { iri: https://x.test/shc/, file: ontology/shc.gloss.yaml, tier: implementation }\n",
	"ontology/frt.gloss.yaml": "version: 1.0.0\nconcepts:\n  DeliveryWindow: { kind: measure }\n",
	"ontology/shc.gloss.yaml": "# shipment-core implementation\nversion: 1.1.0\nconcepts:\n" +
		"  Window:  { kind: measure, realizes: frt:DeliveryWindow, published: { as: frt:DeliveryWindow } }\n" +
		"  Handler: { kind: process, realizes: frt:DeliveryWindow }\n" +
		"  Outbox:  { kind: resource, internal: true }\n",
}

// publish runs gloss publish on the fixture and returns the output directory.
func publish(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "owner")
	writeTree(t, root, publishFixture)
	out := filepath.Join(filepath.Dir(root), "dist")
	if s, code := capture(t, "publish", "--root", root, "-o", out); code != 0 {
		t.Fatalf("publish exit %d: %s", code, s)
	}
	return out
}

// TestPublishPublishedAsOnce: published: { as: X } yields one published-as
// entry, not the key plus the declared edge (issue #130).
func TestPublishPublishedAsOnce(t *testing.T) {
	out := publish(t)
	b, err := os.ReadFile(filepath.Join(out, "shc", "ontology.jsonld"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Graph []map[string]any `json:"@graph"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Graph) != 1 {
		t.Fatalf("want only the published concept, got %v", doc.Graph)
	}
	if got := doc.Graph[0]["published-as"]; got != "frt:DeliveryWindow" {
		t.Fatalf("published-as = %#v, want one entry", got)
	}
}

// TestPublishFiltersYAMLAndStatesTier: the published <ns>.gloss.yaml holds
// the same concepts as ontology.jsonld (issue #129) and states its tier, so
// an importer pointed at the directory enforces the published seam without
// tier: on the import (issue #96).
func TestPublishFiltersYAMLAndStatesTier(t *testing.T) {
	out := publish(t)
	ws := filepath.Dir(out)
	b, err := os.ReadFile(filepath.Join(out, "shc", "shc.gloss.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	y := string(b)
	if !strings.Contains(y, "Window:") || strings.Contains(y, "Handler") || strings.Contains(y, "Outbox") {
		t.Fatalf("published YAML should hold Window only:\n%s", y)
	}
	if !strings.Contains(y, "\ntier: implementation\n") {
		t.Fatalf("published YAML states no tier:\n%s", y)
	}
	// The core namespace is published whole, with its tier.
	if b, _ := os.ReadFile(filepath.Join(out, "frt", "frt.gloss.yaml")); !strings.Contains(string(b), "tier: core\n") || !strings.Contains(string(b), "DeliveryWindow") {
		t.Fatalf("core YAML:\n%s", b)
	}

	// An importer of the published directory, with no tier: on the import.
	imp := filepath.Join(ws, "dispatch")
	importer := func(shcSource, marker string) {
		writeTree(t, imp, map[string]string{
			"glossmark.yaml": "owns:\n  dsp: { iri: https://x.test/dsp/, file: ontology/dsp.gloss.yaml, tier: implementation }\n" +
				"imports:\n  shc: { iri: https://x.test/shc/, source: " + shcSource + " }\n  frt: { iri: https://x.test/frt/, source: ../dist/frt/ }\n",
			"ontology/dsp.gloss.yaml": "version: 0.1.0\nconcepts:\n  Planner: { kind: process, realizes: frt:DeliveryWindow }\n",
			"plan.go":                 "package plan\n\n// @gloss consumes " + marker + "\nfunc Plan() {}\n",
		})
	}
	importer("../dist/shc/", "shc:Window")
	if s, _ := capture(t, "check", "--root", imp); strings.Contains(s, "error:") || strings.Contains(s, "GM113") {
		t.Fatalf("a published concept should be referenceable without findings:\n%s", s)
	}

	// A source file with a top-level tier: (as publish writes it) enforces
	// GM230 on an unpublished concept; with no tier anywhere the import warns.
	importer("../shc.gloss.yaml", "shc:Handler")
	writeTree(t, ws, map[string]string{"shc.gloss.yaml": "tier: implementation\n" + publishFixture["ontology/shc.gloss.yaml"]})
	if s, _ := capture(t, "check", "--root", imp); !strings.Contains(s, "GM230") {
		t.Fatalf("expected GM230 for shc:Handler:\n%s", s)
	}
	writeTree(t, ws, map[string]string{"shc.gloss.yaml": publishFixture["ontology/shc.gloss.yaml"]})
	if s, _ := capture(t, "check", "--root", imp); !strings.Contains(s, "GM113") {
		t.Fatalf("expected GM113 for an import that states no tier:\n%s", s)
	}
}
