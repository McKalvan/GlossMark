// @gloss verifies gmi:SourcePlugin scope=file

package wire

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/infer"
)

// The test binary doubles as the plugin: GLOSS_WIRE_TEST picks what it
// does when a Client starts it (the helper-process pattern).
func TestMain(m *testing.M) {
	switch os.Getenv("GLOSS_WIRE_TEST") {
	case "":
		os.Exit(m.Run())
	case "ok":
		if err := Serve(&fakeSource{}, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "v99":
		var m Message
		json.NewDecoder(os.Stdin).Decode(&m)
		fmt.Println(`{"type":"indexed","contract":99,"name":"fake"}`)
	case "crash":
		var m Message
		json.NewDecoder(os.Stdin).Decode(&m)
		fmt.Fprintln(os.Stderr, "boom: no such spec")
		os.Exit(3)
	case "garbage":
		fmt.Println("hello, I am not JSON")
	case "reader":
		// Asks for file content by glob, then answers.
		dec := json.NewDecoder(os.Stdin)
		var m Message
		dec.Decode(&m)
		fmt.Println(`{"type":"read","globs":["*.spec"]}`)
		var files Message
		dec.Decode(&files)
		var names []string
		for _, f := range files.Contents {
			names = append(names, f.Path+"="+strings.TrimSpace(f.Content))
		}
		items := map[string][]infer.Item{"a.spec": {{Symbol: strings.Join(names, ";"), Role: "x", Relation: "documents", Line: 1, End: 1}}}
		json.NewEncoder(os.Stdout).Encode(Message{Type: TypeIndexed, Contract: Contract, Name: "fake", Items: items})
	case "lateCrash":
		if err := Serve(&fakeSource{crashOnHints: true}, os.Stdin, os.Stdout); err != nil {
			os.Exit(1)
		}
	}
	os.Exit(0)
}

// fakeSource uses every part of the vocabulary and declares a prior.
type fakeSource struct{ crashOnHints bool }

func (f *fakeSource) Name() string { return "fake" }
func (f *fakeSource) Priors() map[string]float64 {
	return map[string]float64{"spec": 0.8}
}
func (f *fakeSource) Index(files infer.Files, v infer.Vocabulary) error {
	for _, c := range v.Concepts() {
		if c.CURIE == "x:Missing" {
			return errors.New("the vocabulary has x:Missing")
		}
	}
	return nil
}
func (f *fakeSource) Items(p string) []infer.Item {
	if p != "a.spec" {
		return nil
	}
	return []infer.Item{{Symbol: "A", Role: "x", Relation: "documents", Line: 1, End: 2, Target: infer.Target{Concept: "x:A", Basis: "spec"}}}
}
func (f *fakeSource) Hints(d infer.Decl) []infer.Hint {
	if f.crashOnHints {
		os.Exit(2)
	}
	return []infer.Hint{{Target: infer.Target{Concept: "x:" + d.Symbol, Basis: "spec", Terms: d.Body}, Relation: "implements", Line: d.Line}}
}

// vocabStub answers Named by name and CommonAncestor with the first curie.
type vocabStub struct{ named map[string]string }

func (v vocabStub) Concepts() []infer.Concept {
	return []infer.Concept{{CURIE: "x:A", Name: "A", Kind: "entity"}, {CURIE: "x:B", Name: "B", Kind: "event"}}
}
func (v vocabStub) Named(name string, kinds ...string) []infer.Concept {
	if c, ok := v.named[name]; ok {
		return []infer.Concept{{CURIE: c, Name: name}}
	}
	return nil
}
func (v vocabStub) CommonAncestor(curies []string) string {
	if len(curies) == 0 {
		return ""
	}
	return curies[0]
}

func plugin(t *testing.T, mode string) (*Client, infer.Files) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GLOSS_WIRE_TEST", mode)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.spec"), []byte("spec a\n"), 0o644)
	os.WriteFile(filepath.Join(root, "b.spec"), []byte("spec b\n"), 0o644)
	os.WriteFile(filepath.Join(root, "c.txt"), []byte("c\n"), 0o644)
	files := infer.Files{Root: root, List: []infer.File{{Path: "a.spec"}, {Path: "b.spec", Dir: "b"}, {Path: "c.txt", Test: true}}}
	c := New("fake", []string{exe}, "test", root)
	t.Cleanup(c.Close)
	return c, files
}

func TestRoundTrip(t *testing.T) {
	c, files := plugin(t, "ok")
	v := vocabStub{named: map[string]string{"A": "x:A"}}
	if err := c.Index(files, v); err != nil {
		t.Fatal(err)
	}
	if its := c.Items("a.spec"); len(its) != 1 || its[0].Concept != "x:A" || its[0].Basis != "spec" || its[0].End != 2 {
		t.Errorf("items: %+v", its)
	}
	if c.Items("b.spec") != nil {
		t.Error("b.spec has items")
	}
	if p := c.Priors(); p["spec"] != 0.8 {
		t.Errorf("priors: %v", p)
	}
	hs := c.Hints(infer.Decl{Path: "x.py", Symbol: "Handle", Role: "func", Line: 7, Body: "def Handle(): pass"})
	if len(hs) != 1 || hs[0].Concept != "x:Handle" || hs[0].Relation != "implements" || hs[0].Line != 7 || hs[0].Terms != "def Handle(): pass" {
		t.Errorf("hints: %+v", hs)
	}
	if c.Err() != nil {
		t.Error(c.Err())
	}
	c.Close()
	if c.Hints(infer.Decl{Symbol: "After"}) != nil {
		t.Error("a closed client answered")
	}
}

func TestServeAsksTheVocabulary(t *testing.T) {
	// The served source's Index fails on a concept the stub lists only
	// when asked through Concepts: the vocabulary crossed the wire.
	c, files := plugin(t, "ok")
	v := vocabStub{named: map[string]string{}}
	if err := c.Index(files, vocabWith{v, "x:Missing"}); err == nil || !strings.Contains(err.Error(), "x:Missing") || !strings.Contains(err.Error(), "GM700") {
		t.Errorf("err: %v", err)
	}
}

type vocabWith struct {
	vocabStub
	extra string
}

func (v vocabWith) Concepts() []infer.Concept {
	return append(v.vocabStub.Concepts(), infer.Concept{CURIE: v.extra, Name: "Missing"})
}

func TestContractMismatchNamesBoth(t *testing.T) {
	c, files := plugin(t, "v99")
	err := c.Index(files, vocabStub{})
	var ce *ContractError
	if !errors.As(err, &ce) || ce.Got != 99 {
		t.Fatalf("err: %v", err)
	}
	if s := err.Error(); !strings.Contains(s, "v99") || !strings.Contains(s, fmt.Sprintf("v%d", Contract)) || !strings.Contains(s, "fake") {
		t.Errorf("message names neither version or the plugin: %s", s)
	}
}

func TestCrashIsReportedNotFatal(t *testing.T) {
	for _, tc := range []struct{ mode, want string }{
		{"crash", "boom: no such spec"},
		{"garbage", "not the wire protocol"},
	} {
		c, files := plugin(t, tc.mode)
		err := c.Index(files, vocabStub{})
		var ce *ContractError
		if err == nil || errors.As(err, &ce) {
			t.Fatalf("%s: err %v", tc.mode, err)
		}
		if s := err.Error(); !strings.Contains(s, tc.want) || !strings.Contains(s, "[GM700]") || !strings.Contains(s, "source plugin fake") {
			t.Errorf("%s: %s", tc.mode, s)
		}
		if c.Hints(infer.Decl{Symbol: "x"}) != nil {
			t.Errorf("%s: a failed plugin answered", tc.mode)
		}
	}
}

func TestLateCrashKeepsErr(t *testing.T) {
	c, files := plugin(t, "lateCrash")
	if err := c.Index(files, vocabStub{}); err != nil {
		t.Fatal(err)
	}
	if hs := c.Hints(infer.Decl{Symbol: "x"}); hs != nil {
		t.Errorf("hints after a crash: %+v", hs)
	}
	if err := c.Err(); err == nil || !strings.Contains(err.Error(), "exited before answering hints") {
		t.Errorf("err: %v", err)
	}
}

func TestReadByGlob(t *testing.T) {
	c, files := plugin(t, "reader")
	if err := c.Index(files, vocabStub{}); err != nil {
		t.Fatal(err)
	}
	if its := c.Items("a.spec"); len(its) != 1 || its[0].Symbol != "a.spec=spec a;b.spec=spec b" {
		t.Errorf("read answered %+v", its)
	}
}

func TestServeRejectsOtherContract(t *testing.T) {
	in := strings.NewReader(`{"type":"index","contract":2,"root":"/x"}` + "\n")
	var out strings.Builder
	err := Serve(&fakeSource{}, in, &out)
	if err == nil || !strings.Contains(err.Error(), "v2") || !strings.Contains(err.Error(), "v1") {
		t.Errorf("err: %v", err)
	}
	if !strings.Contains(out.String(), `"contract":1`) {
		t.Errorf("the answer does not name our version: %s", out.String())
	}
}

func TestDiscoverFromPath(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"gloss-source-graphql", "gloss-source-code", "gloss-source-Bad", "gloss-sourceless", "gloss-source-noexec"} {
		mode := os.FileMode(0o755)
		if name == "gloss-source-noexec" {
			mode = 0o644
		}
		os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), mode)
	}
	os.Mkdir(filepath.Join(dir, "gloss-source-dir"), 0o755)
	t.Setenv("PATH", dir)
	cs, err := Discover(nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range cs {
		names = append(names, c.Name()+"@"+c.Origin())
	}
	want := "graphql@PATH"
	if isWindows() {
		want = "graphql@PATH noexec@PATH"
	}
	if got := strings.Join(names, " "); got != want {
		t.Errorf("discovered %q, want %q", got, want)
	}
}

func isWindows() bool { return strings.EqualFold(filepath.VolumeName(os.TempDir()), "c:") }
