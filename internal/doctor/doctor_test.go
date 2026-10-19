// @gloss verifies gmi:Doctor scope=file

package doctor

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/rollup"
)

func TestDoctor(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=A", "GIT_AUTHOR_EMAIL=a@x", "GIT_COMMITTER_NAME=A", "GIT_COMMITTER_EMAIL=a@x")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Thing: { kind: entity, doc: One thing (a, b) }\n")
	write(".gloss.yaml", "boundary: o:b\nlevel: container\n")
	write("a.py", "# @gloss represents z:Thing\nclass T: pass\n")
	write(".gitignore", ".glossmark/ledger.jsonl\n")
	write(".github/workflows/gloss.yml", "jobs:\n  g:\n    steps:\n      - uses: mckalvan/glossmark@main\n")
	git("init", "-q")
	git("add", ".")
	git("commit", "-q", "-m", "init")

	run := func(actor string, path bool) map[string]Check {
		t.Helper()
		ws, err := load.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		res, _ := extract.Build(ws)
		rollup.Derive(res.Graph, ws.Vocab)
		// Like gloss build: the ledger stamps actors onto the graph before it is compared.
		if lg, err := ledger.Open(ws.StatePath(ledger.FileName)); err == nil {
			lg.Sync(res.Graph, res.Markers, "human:a", time.Now())
		}
		diags := append([]marker.Diagnostic{}, res.Diags...)
		diags = append(diags, check.Run(ws, res.Graph)...)
		lookPath := func(string) (string, error) { return "/usr/local/bin/gloss", nil }
		if !path {
			lookPath = func(string) (string, error) { return "", errors.New("not found") }
		}
		out := map[string]Check{}
		for _, c := range Run(ws, res.Graph, diags, Env{Actor: actor, LookPath: lookPath}) {
			out[c.Name] = c
		}
		return out
	}
	by := run("", false)
	want := map[string]string{"gloss on PATH": Fail, "agent hook": Warn, "graph": Fail, "ledger": Warn, "lockfile": OK, "CI pins": Fail, "ontology docs": Fail}
	for name, st := range want {
		if by[name].Status != st {
			t.Errorf("%s = %+v, want %s", name, by[name], st)
		}
	}
	if !strings.Contains(by["CI pins"].Msg, "@main") || !strings.Contains(by["ontology docs"].Msg, "GM304/GM305") {
		t.Errorf("messages: %+v %+v", by["CI pins"], by["ontology docs"])
	}
	// Fix everything: quote the doc, build and commit the graph, track the ledger, pin the Action, install the hook.
	write("ontology/z.gloss.yaml", "concepts:\n  Thing: { kind: entity, doc: \"One thing (a, b).\" }\n")
	write(".gitignore", "")
	write(".github/workflows/gloss.yml", "jobs:\n  g:\n    steps:\n      - uses: mckalvan/glossmark@v0.1.0\n")
	write(".claude/settings.json", `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"GLOSS_ACTOR=agent:claude-code gloss context"}]}]}}`)
	write(".mcp.json", `{"mcpServers":{"glossmark":{"command":"gloss","args":["mcp"],"env":{"GLOSS_ACTOR":"agent:claude-code"}}}}`)
	ws, _ := load.Load(root)
	res, _ := extract.Build(ws)
	rollup.Derive(res.Graph, ws.Vocab)
	lg, _ := ledger.Open(ws.StatePath(ledger.FileName))
	lg.Sync(res.Graph, res.Markers, "human:a", time.Now())
	if err := lg.Save(); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Dir(ws.StatePath("graph.jsonl")), 0o755)
	f, _ := os.Create(ws.StatePath("graph.jsonl"))
	if err := res.Graph.WriteJSONL(f); err != nil {
		t.Fatal(err)
	}
	f.Close()
	git("add", ".")
	git("commit", "-q", "-m", "fixed")
	by = run("agent:claude-code", true)
	for name, c := range by {
		if c.Status != OK {
			t.Errorf("%s = %+v, want ok", name, c)
		}
	}
	// A stale graph is caught: change a marker without rebuilding.
	write("a.py", "# @gloss represents z:Thing\nclass T2: pass\n")
	if c := run("agent:claude-code", true)["graph"]; c.Status != Fail || !strings.Contains(c.Msg, "stale") {
		t.Errorf("stale graph = %+v", c)
	}
	_ = graph.New
}
