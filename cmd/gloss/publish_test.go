package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func publish(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTree(t, root, publishFixture)
	out := filepath.Join(root, "dist")
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
