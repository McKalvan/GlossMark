// @gloss verifies gmi:Scaffolder scope=file

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInitCommand (issue #135): the dry-run plan lists the hook files and
// the build, init builds the first graph, and --import pins the sibling.
func TestInitCommand(t *testing.T) {
	tmp := t.TempDir()
	writeTree(t, tmp, map[string]string{
		"core/glossmark.yaml":          "owns:\n  frt: { iri: https://x.test/frt/, tier: core, version: 1.0.0 }\n",
		"core/ontology/frt.gloss.yaml": "namespace: frt\nversion: 1.0.0\nconcepts:\n  Shipment: { kind: entity, doc: \"A consignment.\" }\n",
		"dispatch/src/a.go":            "package a\n",
	})
	root := filepath.Join(tmp, "dispatch")
	out, code := capture(t, "init", "--root", root, "--dry-run", "--import", "../core")
	if code != 0 {
		t.Fatalf("dry run: %d\n%s", code, out)
	}
	for _, want := range []string{"then   gloss lock (pins frt:", "then   gloss hook install (.claude/settings.json, .mcp.json", "then   gloss build (.glossmark/graph.jsonl"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run lacks %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "glossmark.yaml")); err == nil {
		t.Fatal("dry run wrote files")
	}
	out, code = capture(t, "init", "--root", root, "--no-hook", "--import", "../core")
	if code != 0 {
		t.Fatalf("init: %d\n%s", code, out)
	}
	for _, f := range []string{".glossmark/graph.jsonl", "glossmark.lock"} {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Errorf("init did not write %s:\n%s", f, out)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".mcp.json")); err == nil {
		t.Error("--no-hook installed the hook")
	}
}

// TestLandscapeSharedIRI (issue #135, FF3): two prefixes resolving to one
// namespace IRI are refused instead of merged.
func TestLandscapeSharedIRI(t *testing.T) {
	tmp := t.TempDir()
	writeTree(t, tmp, map[string]string{
		"alpha/glossmark.yaml":          "owns:\n  frt: { iri: https://x.test/core/, tier: core }\n",
		"alpha/ontology/frt.gloss.yaml": "concepts:\n  A: { kind: entity }\n",
		"beta/glossmark.yaml":           "owns:\n  bc: { iri: https://x.test/core/, tier: core }\n",
		"beta/ontology/bc.gloss.yaml":   "concepts:\n  B: { kind: entity }\n",
	})
	out, code := capture(t, "landscape", filepath.Join(tmp, "alpha"), filepath.Join(tmp, "beta"))
	if code == 0 || !strings.Contains(out, "frt: and bc: both resolve to https://x.test/core/") {
		t.Errorf("landscape = %d:\n%s", code, out)
	}
}
