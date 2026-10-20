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
	"github.com/mckalvan/glossmark/internal/version"
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

	onPath := "gloss " + version.String() // what `gloss version` on PATH prints first
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
		for _, c := range Run(ws, res.Graph, diags, Env{Actor: actor, LookPath: lookPath, Version: func(string) (string, error) { return onPath, nil }}) {
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
	// An unrelated program named gloss first on PATH is caught (issue #37);
	// another GlossMark release is a warning.
	onPath = "Frames per second: 60.0"
	if c := run("agent:claude-code", true)["gloss on PATH"]; c.Status != Fail || !strings.Contains(c.Msg, "is not GlossMark") {
		t.Errorf("unrelated gloss = %+v", c)
	}
	onPath = "gloss 9.9.9"
	if c := run("agent:claude-code", true)["gloss on PATH"]; c.Status != Warn || !strings.Contains(c.Msg, "gloss 9.9.9") {
		t.Errorf("other release = %+v", c)
	}
	onPath = "gloss " + version.String()
	// A graph written by another release (different generator in the header) is still current.
	gp := ws.StatePath("graph.jsonl")
	gb, _ := os.ReadFile(gp)
	os.WriteFile(gp, []byte(strings.Replace(string(gb), "gloss 0.0.0-dev", "gloss 0.1.0", 1)), 0o644)
	if c := run("agent:claude-code", true)["graph"]; c.Status != OK {
		t.Errorf("graph from another release = %+v, want ok", c)
	}
	// A stale graph is caught: change a marker without rebuilding.
	write("a.py", "# @gloss represents z:Thing\nclass T2: pass\n")
	if c := run("agent:claude-code", true)["graph"]; c.Status != Fail || !strings.Contains(c.Msg, "stale") {
		t.Errorf("stale graph = %+v", c)
	}
	_ = graph.New
}

// TestDoctorWaivers: doctor lists waivers that expired or expire within 30
// days; expires is inclusive (#102).
func TestDoctorWaivers(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "glossmark.yaml"), []byte("owns:\n  z: { iri: https://x/z/, tier: core }\npolicy:\n  waivers:\n    - { finding: z:A, owner: a, expires: 2026-01-31 }\n    - { finding: z:B, owner: a, expires: 2026-11-10 }\n    - { finding: z:C, owner: a, expires: 2027-06-30 }\n"), 0o644)
	ws, err := load.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	res, _ := extract.Build(ws)
	at := func(day string) Check {
		now, _ := time.Parse("2006-01-02", day)
		env := Env{Actor: "agent:x", LookPath: func(string) (string, error) { return "", errors.New("none") }, Now: now.Add(12 * time.Hour)}
		for _, c := range Run(ws, res.Graph, nil, env) {
			if c.Name == "waivers" {
				return c
			}
		}
		return Check{}
	}
	if c := at("2026-10-20"); c.Status != Warn || c.Msg != "z:A (expired 2026-01-31), z:B (expires 2026-11-10)" {
		t.Errorf("waivers = %+v", c)
	}
	if c := at("2026-01-31"); c.Status != Warn || !strings.HasPrefix(c.Msg, "expiring within 30 days: z:A (expires 2026-01-31)") {
		t.Errorf("on the expiry day the waiver still holds: %+v", c)
	}
	if c := at("2025-01-01"); c.Status != OK {
		t.Errorf("waivers far from expiry = %+v", c)
	}
}

// TestDoctorCIAtGitRoot: a workspace below the git root is covered by a
// workflow at the git root that runs the Action with its root: (issue #36),
// and not by one that names another workspace.
func TestDoctorCIAtGitRoot(t *testing.T) {
	top := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(top, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	for _, ws := range []string{"showcase/clinic-booking", "showcase/clinic-booking-v2"} {
		write(ws+"/glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n")
		write(ws+"/ontology/z.gloss.yaml", "concepts: {}\n")
	}
	write(".github/workflows/showcase.yml", "jobs:\n  g:\n    strategy: { matrix: { root: [showcase/clinic-booking] } }\n    steps:\n      - uses: mckalvan/glossmark@v0.1.0\n        with: { root: \"${{ matrix.root }}\" }\n")
	if out, err := exec.Command("git", "-C", top, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	pins := func(rel string) Check {
		ws, err := load.Load(filepath.Join(top, rel))
		if err != nil {
			t.Fatal(err)
		}
		res, _ := extract.Build(ws)
		for _, c := range Run(ws, res.Graph, nil, Env{LookPath: func(string) (string, error) { return "", errors.New("no") }}) {
			if c.Name == "CI pins" {
				return c
			}
		}
		t.Fatal("no CI pins check")
		return Check{}
	}
	if c := pins("showcase/clinic-booking"); c.Status != OK || !strings.Contains(c.Msg, "showcase.yml") {
		t.Errorf("covered workspace: %+v", c)
	}
	if c := pins("showcase/clinic-booking-v2"); c.Status != Warn || !strings.Contains(c.Msg, "git root") {
		t.Errorf("uncovered workspace: %+v", c)
	}
}

func TestMentionsRoot(t *testing.T) {
	for text, want := range map[string]bool{
		"root: showcase/clinic-booking\n":        true,
		"root: ./showcase/clinic-booking/\n":     true,
		"root: [a, \"showcase/clinic-booking\"]": true,
		"root: showcase/clinic-booking-v2\n":     false,
		"root: x/showcase/clinic-booking\n":      false,
		"root: showcase/clinic-booking/web\n":    false,
	} {
		if got := mentionsRoot(text, "showcase/clinic-booking"); got != want {
			t.Errorf("%q: %v, want %v", text, got, want)
		}
	}
}
