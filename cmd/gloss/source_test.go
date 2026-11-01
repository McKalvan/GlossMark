// @gloss verifies gmi:SourcePlugin scope=file

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/infer"
	"github.com/mckalvan/glossmark/internal/infer/wire"
)

// TestSourceServeSpeaksTheProtocol: gloss source serve asyncapi answers an
// index message with the built-in source's name and items, then hints.
func TestSourceServeSpeaksTheProtocol(t *testing.T) {
	root := t.TempDir()
	spec := "asyncapi: 3.0.0\nchannels:\n  claimsFiled:\n    address: claims.filed.v1\n    messages:\n      ClaimFiled:\n        name: ClaimFiled\n"
	os.WriteFile(filepath.Join(root, "contract.yaml"), []byte(spec), 0o644)
	var in strings.Builder
	enc := json.NewEncoder(&in)
	enc.Encode(wire.Message{Type: wire.TypeIndex, Contract: wire.Contract, Root: root, Files: []infer.File{{Path: "contract.yaml"}}, Vocabulary: nil})
	enc.Encode(wire.Message{Type: wire.TypeHints, Decl: &infer.Decl{Path: "x.py", Symbol: "publish", Role: "func", Line: 1, Body: `producer.send("claims.filed.v1")`}})
	r, w, _ := os.Pipe()
	oldIn := os.Stdin
	os.Stdin = r
	go func() { w.Write([]byte(in.String())); w.Close() }()
	code, out := gloss(t, "source", "serve", "asyncapi")
	os.Stdin = oldIn
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	dec := json.NewDecoder(strings.NewReader(out))
	var answers []wire.Message
	for dec.More() {
		var m wire.Message
		if err := dec.Decode(&m); err != nil {
			t.Fatal(err)
		}
		answers = append(answers, m)
	}
	// The source asks the vocabulary for the message's event (named), gets
	// no answer here (the test sends none), so the item has no concept;
	// the protocol still carries it.
	var indexed *wire.Message
	for i := range answers {
		if answers[i].Type == wire.TypeIndexed {
			indexed = &answers[i]
		}
	}
	if indexed == nil || indexed.Contract != wire.Contract || indexed.Name != "asyncapi" {
		t.Fatalf("answers: %+v", answers)
	}
	if its := indexed.Items["contract.yaml"]; len(its) != 1 || its[0].Symbol != "claimsFiled" || its[0].Relation != "documents" {
		t.Errorf("items: %+v", indexed.Items)
	}
}

// TestSourceList names the built-ins and what PATH holds.
func TestSourceList(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell plugin")
	}
	root := workspace(t, "library/circulation")
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "gloss-source-graphql"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	code, out := gloss(t, "source", "list", "--root", root)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	for _, want := range []string{"asyncapi     builtin", "openapi      builtin", "graphql      external   PATH: gloss-source-graphql", "code         core"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// TestSuggestFailsOnContractMismatch: a plugin on PATH that speaks another
// contract version fails gloss suggest, naming both versions (#209).
func TestSuggestFailsOnContractMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell plugin")
	}
	root := workspace(t, "library/circulation")
	bin := t.TempDir()
	script := "#!/bin/sh\nread line\necho '{\"type\":\"indexed\",\"contract\":99,\"name\":\"future\"}'\n"
	os.WriteFile(filepath.Join(bin, "gloss-source-future"), []byte(script), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var code int
	stderr := captureStderr(t, func() { code, _ = gloss(t, "suggest", "--root", root) })
	if code != 1 {
		t.Errorf("exit %d, want 1: %s", code, stderr)
	}
	for _, want := range []string{"source plugin future", "v99", "v1", "glossmark.yaml"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
	// Without the plugin, suggest runs.
	t.Setenv("PATH", os.Getenv("PATH")[len(bin)+1:])
	if code, _ := gloss(t, "suggest", "--root", root); code != 0 {
		t.Errorf("without the plugin: exit %d", code)
	}
}

// TestSuggestReportsABrokenPlugin: a plugin that prints no protocol is a
// GM700 warning, and suggest still exits 0 with the other sources.
func TestSuggestReportsABrokenPlugin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell plugin")
	}
	root := workspace(t, "library/circulation")
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "gloss-source-broken"), []byte("#!/bin/sh\necho 'oops' >&2\nexit 7\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var code int
	stderr := captureStderr(t, func() { code, _ = gloss(t, "suggest", "--root", root, "--format", "json") })
	if code != 0 {
		t.Errorf("exit %d: %s", code, stderr)
	}
	for _, want := range []string{"source plugin broken", "[GM700]", "oops", "external sources: broken"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
	if _, out := gloss(t, "suggest", "--root", root, "--source", "nope"); out != "" {
		t.Error("--source nope printed proposals")
	}
}
