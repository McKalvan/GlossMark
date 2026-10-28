// @gloss verifies gmi:Scaffolder scope=file

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/load"
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

// TestInitFirstRun: the plan says how to override guessed prefixes (issue
// #229), init writes the ledger it promises (#230) and ends with the
// draft-review-mark path (#227), which check repeats while the ontology is
// empty; outside a workspace the error names gloss init (#228).
func TestInitFirstRun(t *testing.T) {
	root := filepath.Join(t.TempDir(), "newrepo")
	writeTree(t, root, map[string]string{"api/main.py": "def send_invoice(): pass\n"})
	out, code := capture(t, "init", "--root", root, "--dry-run")
	if code != 0 || !strings.Contains(out, "core ne:, implementation nei:, boundaries neb:") || !strings.Contains(out, `prefixes guessed from the name "newrepo"`) || !strings.Contains(out, "--namespace <ns>") {
		t.Errorf("guessed plan: %d\n%s", code, out)
	}
	out, code = capture(t, "init", "--root", root, "--dry-run", "--namespace", "inv")
	if code != 0 || !strings.Contains(out, "core inv:, implementation invi:, boundaries invb:") || strings.Contains(out, "guessed") {
		t.Errorf("--namespace plan: %d\n%s", code, out)
	}
	if _, err := load.FindRoot(root); err == nil || !strings.Contains(err.Error(), "run gloss init to set up this repository (or pass --root)") {
		t.Errorf("no workspace: %v", err)
	}
	out, code = capture(t, "init", "--root", root, "--no-hook", "--namespace", "inv")
	if code != 0 || !strings.Contains(out, "1. draft concepts from the code: gloss suggest --concepts") || !strings.Contains(out, "3. gloss suggest proposes @gloss? markers") {
		t.Errorf("init epilogue: %d\n%s", code, out)
	}
	b, err := os.ReadFile(filepath.Join(root, ".glossmark", "ledger.jsonl"))
	if err != nil || !strings.HasPrefix(string(b), `{"type":"ledger","format":`) || strings.Count(string(b), "\n") != 1 {
		t.Errorf("header-only ledger: %v %q", err, b)
	}
	if gi, _ := os.ReadFile(filepath.Join(root, ".gitignore")); !strings.Contains(string(gi), ".claude/settings.local.json") {
		t.Errorf(".gitignore lacks settings.local.json:\n%s", gi)
	}
	if out, code := capture(t, "check", "--root", root); code != 0 || !strings.Contains(out, "0 concepts defined. Draft some with gloss suggest --concepts") {
		t.Errorf("check on an empty ontology: %d\n%s", code, out)
	}
}

// TestHookInstallPaths (issue #226): committed agent config runs gloss by
// name; a binary PATH does not find goes to .claude/settings.local.json, and
// a later install with gloss on PATH removes it.
func TestHookInstallPaths(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{".mcp.json": `{"mcpServers":{"glossmark":{"command":"/tmp/old/gloss","args":["mcp"]}}}`})
	if err := installHooks(root, "/opt/gm/gloss", "agent:x", true, true); err != nil {
		t.Fatal(err)
	}
	read := func(rel string) string {
		b, _ := os.ReadFile(filepath.Join(root, rel))
		return string(b)
	}
	if s := read(".claude/settings.json"); !strings.Contains(s, "agent:x gloss context") || strings.Contains(s, "/opt/gm") {
		t.Errorf("settings.json:\n%s", s)
	}
	if s := read(".mcp.json"); !strings.Contains(s, `"command": "gloss"`) {
		t.Errorf(".mcp.json:\n%s", s)
	}
	if s := read(".claude/settings.local.json"); !strings.Contains(s, "agent:x /opt/gm/gloss context") {
		t.Errorf("settings.local.json:\n%s", s)
	}
	if err := installHooks(root, "gloss", "agent:x", true, true); err != nil {
		t.Fatal(err)
	}
	if s := read(".claude/settings.local.json"); strings.Contains(s, "context") {
		t.Errorf("settings.local.json keeps the old path:\n%s", s)
	}
	if s := read(".claude/settings.json"); strings.Count(s, "gloss context") != 1 {
		t.Errorf("settings.json after reinstall:\n%s", s)
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
