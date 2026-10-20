// @gloss verifies gmi:Gate scope=file

package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mckalvan/glossmark/internal/rollup"
)

// TestBaseSummary: the base side of diff and impact is computed over the base
// tree's own files (#104), and impact reports it at all (#81), whether the
// base graph is extracted from the tree or read from a committed graph.jsonl.
func TestBaseSummary(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@x", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@x")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Doing: { kind: process }\n")
	write(".gloss.yaml", "boundary: o:b\nlevel: container\n")
	write("a.py", "# @gloss implements z:Doing\ndef a(): pass\n")
	write("b.py", "def b(): pass\n")
	git("init", "-q")
	git("add", ".")
	git("commit", "-q", "-m", "base")
	// Two unmarked files added after the base: base 1 of 2, head 1 of 4.
	write("c.py", "def c(): pass\n")
	write("d.py", "def d(): pass\n")
	for _, committed := range []bool{false, true} {
		if committed {
			os.Remove(filepath.Join(root, "c.py"))
			os.Remove(filepath.Join(root, "d.py"))
			if code := cmdBuild([]string{"--root", root, "-q"}); code != 0 {
				t.Fatalf("gloss build exited %d", code)
			}
			if _, err := os.Stat(filepath.Join(root, ".glossmark", "graph.jsonl")); err != nil {
				t.Fatal(err)
			}
			git("add", ".")
			git("commit", "-q", "-m", "graph")
			write("c.py", "def c(): pass\n")
			write("d.py", "def d(): pass\n")
		}
		p, err := buildGraph(root, false)
		if err != nil {
			t.Fatal(err)
		}
		bg, err := loadBaseGraph(p.WS, "HEAD")
		if err != nil {
			t.Fatal(err)
		}
		before := baseReport(p.WS, "HEAD", bg, p.WS.Config.EffectivePolicy()).Metrics
		if before.InScopeFiles != 2 || before.CoveredFiles != 1 {
			t.Errorf("committed=%v: base files %d/%d, want 1/2", committed, before.CoveredFiles, before.InScopeFiles)
		}
		out := capture(t, func() { cmdImpact([]string{"--root", root, "--base", "HEAD", "--format", "json"}) })
		var res struct {
			Impact struct{ Before, After rollup.Metrics }
		}
		if err := json.Unmarshal(out, &res); err != nil {
			t.Fatalf("impact json: %v\n%s", err, out)
		}
		if res.Impact.Before.InScopeFiles != 2 || res.Impact.Before.CoveredFiles != 1 || res.Impact.After.InScopeFiles != 4 {
			t.Errorf("committed=%v: impact before %+v after %+v", committed, res.Impact.Before, res.Impact.After)
		}
	}
}

// capture returns what f writes to stdout.
func capture(t *testing.T, f func()) []byte {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan []byte)
	go func() { b, _ := io.ReadAll(r); done <- b }()
	f()
	w.Close()
	os.Stdout = old
	return <-done
}
