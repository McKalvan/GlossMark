package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mckalvan/glossmark/internal/review"
)

// Regression tests for repository content that tries to reach outside the
// repository: a checked-out PR must not read or write files beyond its root.

// TestValueFromStaysInRepo: a value-from path that climbs out of the root,
// directly or through a symlink, is an error and its target never reaches
// the graph.
func TestValueFromStaysInRepo(t *testing.T) {
	tmp := t.TempDir()
	secret := "SECRET_TOKEN=hunter2"
	writeTree(t, tmp, map[string]string{
		"outside.txt":         secret + "\n",
		"repo/glossmark.yaml": "owns:\n  z: { iri: https://x/z/, file: ontology/z.gloss.yaml, tier: core }\n",
		"repo/ontology/z.gloss.yaml": "concepts:\n" +
			"  Up: { kind: constraint, value-from: { path: ../outside.txt, line: 1 } }\n" +
			"  Link: { kind: constraint, value-from: { path: inside.txt, line: 1 } }\n",
	})
	root := filepath.Join(tmp, "repo")
	if err := os.Symlink(filepath.Join(tmp, "outside.txt"), filepath.Join(root, "inside.txt")); err != nil {
		t.Skip("no symlinks:", err)
	}
	out, _ := capture(t, "build", "--root", root)
	if !strings.Contains(out, "../outside.txt is outside the repository") || !strings.Contains(out, "inside.txt links outside the repository") {
		t.Fatalf("expected both value-from paths refused:\n%s", out)
	}
	g, _ := os.ReadFile(filepath.Join(root, ".glossmark", "graph.jsonl"))
	if strings.Contains(string(g)+out, "hunter2") {
		t.Fatal("a file outside the repository leaked into the build")
	}
}

// TestLockVersionCannotEscape: an imported ontology's version names the
// vendored copy; a version holding ../ must not place it outside
// .glossmark/imports.
func TestLockVersionCannotEscape(t *testing.T) {
	tmp := t.TempDir()
	writeTree(t, tmp, map[string]string{
		"app/glossmark.yaml":        "owns:\n  a: { iri: https://x.test/a/, file: ontology/a.gloss.yaml, tier: core }\nimports:\n  up: { iri: https://x.test/up/, source: ../up/up.gloss.yaml }\n",
		"app/ontology/a.gloss.yaml": "version: 0.1.0\nconcepts:\n  A: { kind: entity }\n",
		"up/up.gloss.yaml":          "version: ../../../../PWNED\nconcepts:\n  U: { kind: entity }\n",
	})
	root := filepath.Join(tmp, "app")
	capture(t, "lock", "--root", root)
	filepath.Walk(tmp, func(p string, info os.FileInfo, err error) error {
		if err == nil && strings.Contains(filepath.Base(p), "PWNED") && !strings.HasPrefix(p, filepath.Join(root, ".glossmark", "imports")+string(filepath.Separator)) {
			t.Errorf("gloss lock wrote outside .glossmark/imports: %s", p)
		}
		return nil
	})
}

// TestWriteLinkStaysInScannedFiles: write-link goes only into a file the
// build scans, named as the walk names it, and only as one marker line.
func TestWriteLinkStaysInScannedFiles(t *testing.T) {
	root, e := reviewTree(t)
	outside := filepath.Join(filepath.Dir(root), "victim.py")
	os.WriteFile(outside, []byte("x = 1\n"), 0o644)
	os.Symlink(outside, filepath.Join(root, "link.py"))
	os.WriteFile(filepath.Join(root, "lib.py"), []byte("def run():\n    pass\n"), 0o644)
	e.Invalidate()
	e.Actor = "agent:test"
	for _, d := range []review.Decision{
		{Path: "../victim.py"},
		{Path: "link.py"},
		{Path: "./glossmark.yaml"},
		{Path: "x/../glossmark.yaml"},
		{Path: "lib.py", Scope: "file\n# @gloss implements .Doer scope=file"},
		{Path: "lib.py", Scope: "nope"},
		{Path: "lib.py", Target: ".Doer\n# @gloss implements .Doer"},
	} {
		d.Action, d.Line, d.Relation = review.ActWriteLink, 1, "implements"
		if d.Target == "" {
			d.Target = ".Doer"
		}
		if r := e.Decide(d); r.OK {
			t.Errorf("write-link %q scope %q target %q was allowed: %+v", d.Path, d.Scope, d.Target, r)
		}
	}
	if b, _ := os.ReadFile(outside); string(b) != "x = 1\n" {
		t.Errorf("a file outside the repository was written:\n%s", b)
	}
	b, _ := os.ReadFile(filepath.Join(root, "lib.py"))
	if strings.Contains(string(b), "@gloss") {
		t.Errorf("a rejected marker reached lib.py:\n%s", b)
	}
}

// TestAgentUndoesOnlyItsOwn: an agent cannot walk back a person's decision.
func TestAgentUndoesOnlyItsOwn(t *testing.T) {
	_, e := reviewTree(t)
	soft := itemByPath(t, e, review.KindProposedLink, "soft.py")
	if r := e.Decide(review.Decision{ID: soft.ID, Action: review.ActAccept}); !r.OK {
		t.Fatalf("accept: %+v", r)
	}
	e.Actor = "agent:test"
	if r := e.Decide(review.Decision{Action: review.ActUndo}); r.OK || !strings.Contains(r.Error, "human:test made it") {
		t.Errorf("an agent undid a person's decision: %+v", r)
	}
	e.Actor = "human:test"
	if r := e.Decide(review.Decision{Action: review.ActUndo}); !r.OK {
		t.Errorf("a person could not undo their own decision: %+v", r)
	}
}

// TestWalkSkipsWhatItCannotScan: a symlink out of the repository, a FIFO and
// a dangling link are skipped with GM119, and the build still finishes.
func TestWalkSkipsWhatItCannotScan(t *testing.T) {
	tmp := t.TempDir()
	writeTree(t, tmp, map[string]string{
		"outside.py":                 "# @gloss implements z:Leak\n",
		"repo/glossmark.yaml":        "owns:\n  z: { iri: https://x/z/, file: ontology/z.gloss.yaml, tier: core }\n",
		"repo/ontology/z.gloss.yaml": "concepts:\n  Leak: { kind: entity }\n",
		"repo/ok.py":                 "x = 1\n",
	})
	root := filepath.Join(tmp, "repo")
	if err := os.Symlink(filepath.Join(tmp, "outside.py"), filepath.Join(root, "out.py")); err != nil {
		t.Skip("no symlinks:", err)
	}
	os.Symlink(filepath.Join(root, "missing.py"), filepath.Join(root, "dangling.py"))
	if err := mkfifo(filepath.Join(root, "pipe.py")); err != nil {
		t.Log("no FIFO:", err)
	}
	done := make(chan string)
	go func() { out, _ := capture(t, "build", "--root", root); done <- out }()
	var out string
	select {
	case out = <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("gloss build hung")
	}
	for _, f := range []string{"out.py", "dangling.py"} {
		if !strings.Contains(out, f+":1: warning: not scanned") {
			t.Errorf("%s: no GM119\n%s", f, out)
		}
	}
	if g, _ := os.ReadFile(filepath.Join(root, ".glossmark", "graph.jsonl")); strings.Contains(string(g), "out.py") {
		t.Error("a file outside the repository was scanned")
	}
}

// TestBlankedFingerprintIsStale: clearing an accepted row's
// anchor_fingerprint by hand does not clear its stale state.
func TestBlankedFingerprintIsStale(t *testing.T) {
	root := tiny(t, map[string]string{"a.py": "# @gloss implements .Doer\ndef a(): pass\n"})
	gloss(t, "build", "-q", "--root", root)
	lp := filepath.Join(root, ".glossmark", "ledger.jsonl")
	b, _ := os.ReadFile(lp)
	os.WriteFile(lp, []byte(regexp.MustCompile(`"anchor_fingerprint":"[^"]*"`).ReplaceAllString(string(b), `"anchor_fingerprint":""`)), 0o644)
	os.WriteFile(filepath.Join(root, "a.py"), []byte("# @gloss implements .Doer\ndef a(x): pass\n"), 0o644)
	gloss(t, "build", "-q", "--root", root)
	if b, _ := os.ReadFile(lp); !strings.Contains(string(b), `"status":"stale"`) {
		t.Fatalf("a blanked fingerprint hid a signature change:\n%s", b)
	}
}
