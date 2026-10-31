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
	// Issue #158: a hook that sets the actor for its own command only.
	if c := run("agent:claude-code", true)["agent hook"]; c.Status != Warn || !strings.Contains(c.Msg, "briefing only") {
		t.Errorf("hook without CLAUDE_ENV_FILE = %+v", c)
	}
	write(".claude/settings.json", `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo export GLOSS_ACTOR=agent:claude-code >> \"${CLAUDE_ENV_FILE:-/dev/null}\"; GLOSS_ACTOR=agent:claude-code gloss context"}]}]}}`)
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
		// adoption: is unset and the tree is at level 1: worth knowing, nothing to fix.
		if name == "adoption" {
			if c.Status != Info || !strings.Contains(c.Msg, "Written: level 1 (manifests)") {
				t.Errorf("adoption = %+v, want info at level 1", c)
			}
			continue
		}
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
	// A malformed date waives nothing (GM610); doctor says so (#175).
	os.WriteFile(filepath.Join(root, "glossmark.yaml"), []byte("owns:\n  z: { iri: https://x/z/, tier: core }\npolicy:\n  waivers:\n    - { finding: z:A, owner: a, expires: next-week }\n    - { finding: z:C, owner: a, expires: 2027-06-30 }\n"), 0o644)
	if ws, err = load.Load(root); err != nil {
		t.Fatal(err)
	}
	res, _ = extract.Build(ws)
	if c := at("2025-01-01"); c.Status != Warn || !strings.HasPrefix(c.Msg, `z:A (expires "next-week" is not a YYYY-MM-DD date)`) || !strings.Contains(c.Msg, "GM610") {
		t.Errorf("malformed waiver = %+v", c)
	}
}

// TestDoctorHookPaths: the MCP server is found when .mcp.json runs gloss by
// an absolute path (issue #225), an absolute path in committed agent config
// is a warning, and the uncommitted settings.local.json hook counts (#226).
func TestDoctorHookPaths(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	hook := func() Check {
		t.Helper()
		d := &doctor{ws: &load.Workspace{Root: root}}
		d.hook()
		return d.out[0]
	}
	cmd := func(bin string) string {
		return `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo export GLOSS_ACTOR=agent:claude-code >> \"${CLAUDE_ENV_FILE:-/dev/null}\"; GLOSS_ACTOR=agent:claude-code ` + bin + ` context --dir \"$PWD\""}]}]}}`
	}
	mcp := func(bin string) string {
		return `{"mcpServers":{"glossmark":{"command":"` + bin + `","args":["mcp"],"env":{"GLOSS_ACTOR":"agent:claude-code"}}}}`
	}
	write(".claude/settings.json", cmd("/tmp/claude-0/gloss"))
	write(".mcp.json", mcp("/tmp/claude-0/gloss"))
	if c := hook(); c.Status != Warn || strings.Contains(c.Msg, "no MCP server") || !strings.Contains(c.Msg, ".mcp.json runs /tmp/claude-0/gloss") || !strings.Contains(c.Msg, ".claude/settings.json runs /tmp/claude-0/gloss") {
		t.Errorf("absolute paths = %+v", c)
	}
	// What gloss hook install now writes when PATH does not find it.
	write(".claude/settings.json", cmd("gloss"))
	write(".claude/settings.local.json", cmd("'/opt/my tools/gloss'"))
	write(".mcp.json", mcp("gloss"))
	if c := hook(); c.Status != OK {
		t.Errorf("by name, local path = %+v", c)
	}
	// Only the local hook, and a server under another name that runs gloss.
	write(".claude/settings.json", `{}`)
	write(".mcp.json", `{"mcpServers":{"gm":{"command":"C:\\tools\\gloss.exe","args":["mcp"],"env":{"GLOSS_ACTOR":"agent:x"}}}}`)
	if c := hook(); c.Status != Warn || !strings.Contains(c.Msg, `.mcp.json runs C:\tools\gloss.exe`) {
		t.Errorf("local hook, Windows path = %+v", c)
	}
	// A server that is not gloss does not count.
	write(".mcp.json", `{"mcpServers":{"other":{"command":"/usr/bin/glossary","args":["mcp"]}}}`)
	if c := hook(); c.Status != Warn || !strings.Contains(c.Msg, "no MCP server") {
		t.Errorf("unrelated server = %+v", c)
	}
}

// TestDoctorActorUnset: an unset GLOSS_ACTOR is information, not a pass and
// not a failure (issue #230).
func TestDoctorActorUnset(t *testing.T) {
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-q", root)
	if err := cmd.Run(); err != nil {
		t.Skip("git unavailable")
	}
	exec.Command("git", "-C", root, "config", "user.name", "Alice").Run()
	d := &doctor{ws: &load.Workspace{Root: root}}
	d.actor()
	c := d.out[0]
	if c.Status != Info || !strings.Contains(c.Msg, "human:alice") || Failed(d.out) {
		t.Errorf("unset actor = %+v", c)
	}
	if s := String(d.out); !strings.HasPrefix(s, "· GLOSS_ACTOR: unset") {
		t.Errorf("rendered = %q", s)
	}
}

// TestDoctorAutoAccept (ADR 0024): doctor reports the auto-accept rule in
// force, and warns when no conf meets the target or when the precision
// target overrides soft-conf-threshold.
func TestDoctorAutoAccept(t *testing.T) {
	at := func(cfg string) Check {
		t.Helper()
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, "glossmark.yaml"), []byte("owns:\n  z: { iri: https://x/z/, tier: core }\n"+cfg), 0o644)
		os.WriteFile(filepath.Join(root, "cal.tsv"), []byte("name-exact/type\t10\t1\n"), 0o644)
		ws, err := load.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		res, _ := extract.Build(ws)
		for _, c := range Run(ws, res.Graph, nil, Env{Actor: "agent:x", LookPath: func(string) (string, error) { return "", errors.New("none") }}) {
			if c.Name == "auto-accept" {
				return c
			}
		}
		return Check{}
	}
	if c := at(""); c.Status != OK || !strings.Contains(c.Msg, "derived from the target precision 90.0%") || !strings.Contains(c.Msg, "of proposals") {
		t.Errorf("default = %+v", c)
	}
	if c := at("suggest:\n  calibration: cal.tsv\n"); c.Status != Warn || !strings.HasPrefix(c.Msg, "nothing; no conf reaches") || !strings.Contains(c.Msg, "cal.tsv") {
		t.Errorf("unreachable target = %+v", c)
	}
	if c := at("suggest:\n  auto-accept-precision: 0.8\npolicy:\n  soft-conf-threshold: 0.7\n"); c.Status != Warn || !strings.Contains(c.Msg, "soft-conf-threshold 0.70 is ignored") {
		t.Errorf("both set = %+v", c)
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

// TestDoctorCIShaAndComputedMatrix: a full commit sha is a pin, and a
// workflow whose root: comes from a matrix it computes with fromJSON covers
// the workspace, noted as not checked (#175).
func TestDoctorCIShaAndComputedMatrix(t *testing.T) {
	top := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(top, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("ff/dispatch/glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n")
	write("ff/dispatch/ontology/z.gloss.yaml", "concepts: {}\n")
	write(".github/workflows/showcase.yml", "jobs:\n  g:\n    strategy:\n      matrix:\n        include: ${{ fromJSON(needs.matrix.outputs.workspaces) }}\n    steps:\n      - uses: mckalvan/glossmark@0123456789abcdef0123456789abcdef01234567\n        with:\n          root: ${{ matrix.root }}\n")
	if out, err := exec.Command("git", "-C", top, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	ws, err := load.Load(filepath.Join(top, "ff/dispatch"))
	if err != nil {
		t.Fatal(err)
	}
	res, _ := extract.Build(ws)
	for _, c := range Run(ws, res.Graph, nil, Env{LookPath: func(string) (string, error) { return "", errors.New("no") }}) {
		if c.Name == "CI pins" {
			if c.Status != OK || !strings.Contains(c.Msg, "showcase.yml (root: from a computed matrix") {
				t.Errorf("CI pins = %+v", c)
			}
			return
		}
	}
	t.Fatal("no CI pins check")
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

// TestDoctorIntegrations (issue #267): one line per ready-made integration
// gloss hook install --agent wrote, none for one that is not there; each
// warns on a missing piece or an absolute path, and when no hook is in
// .claude/settings.json the integrations stand in for it.
func TestDoctorIntegrations(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	run := func() map[string]Check {
		t.Helper()
		d := &doctor{ws: &load.Workspace{Root: root}}
		d.hook()
		d.integrations()
		out := map[string]Check{}
		for _, c := range d.out {
			out[c.Name] = c
		}
		return out
	}
	hook := func(bin string) string {
		return `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo export GLOSS_ACTOR=agent:claude-code >> \"${CLAUDE_ENV_FILE:-/dev/null}\"; GLOSS_ACTOR=agent:claude-code ` + bin + ` context --dir \"$PWD\""}]}]}}`
	}
	mcp := func(key, bin, actor string) string {
		return `{"` + key + `":{"glossmark":{"command":"` + bin + `","args":["mcp"],"env":{"GLOSS_ACTOR":"` + actor + `"}}}}`
	}
	// Nothing installed: the agent hook line warns, no integration lines.
	cs := run()
	if c := cs["agent hook"]; c.Status != Warn {
		t.Errorf("nothing = %+v", c)
	}
	for _, n := range []string{"claude-code plugin", "cursor", "copilot"} {
		if _, ok := cs[n]; ok {
			t.Errorf("%s reported though not installed", n)
		}
	}
	// The three integrations, complete and by name.
	write(".claude/plugins/glossmark/.claude-plugin/plugin.json", `{"name":"glossmark","version":"1"}`)
	for _, s := range skillNames {
		write(".claude/plugins/glossmark/skills/"+s+"/SKILL.md", "---\nname: "+s+"\n---\n")
	}
	write(".claude/plugins/glossmark/hooks/hooks.json", hook("gloss"))
	write(".claude/plugins/glossmark/.mcp.json", mcp("mcpServers", "gloss", "agent:claude-code"))
	write(".claude-plugin/marketplace.json", `{"name":"repo-plugins","owner":{"name":"x"},"plugins":[{"name":"glossmark","source":"./.claude/plugins/glossmark"}]}`)
	write(".cursor/rules/glossmark.mdc", "---\nalwaysApply: true\n---\n- run `GLOSS_ACTOR=agent:cursor gloss context --dir <dir>`\n")
	write(".cursor/mcp.json", mcp("mcpServers", "gloss", "agent:cursor"))
	write(".github/copilot-instructions.md", "# Rules\n\n## GlossMark\n\n- ask concepts_for_path first\n")
	write(".vscode/mcp.json", mcp("servers", "gloss", "agent:copilot"))
	cs = run()
	if c := cs["agent hook"]; c.Status != OK || !strings.Contains(c.Msg, "the Claude Code plugin, the Cursor rule, the Copilot instructions installed instead") {
		t.Errorf("stand-in = %+v", c)
	}
	if c := cs["claude-code plugin"]; c.Status != OK || !strings.Contains(c.Msg, "claude plugin install glossmark@repo-plugins") {
		t.Errorf("plugin = %+v", c)
	}
	if c := cs["cursor"]; c.Status != OK {
		t.Errorf("cursor = %+v", c)
	}
	if c := cs["copilot"]; c.Status != OK {
		t.Errorf("copilot = %+v", c)
	}
	// Absolute paths, a missing skill, a missing actor and an unlisted
	// plugin each warn with the install command as the fix.
	write(".claude/plugins/glossmark/hooks/hooks.json", hook("/opt/gm/gloss"))
	os.Remove(filepath.Join(root, ".claude/plugins/glossmark/skills/curate-increment/SKILL.md"))
	write(".claude-plugin/marketplace.json", `{"name":"repo-plugins","plugins":[]}`)
	write(".vscode/mcp.json", mcp("servers", "C:\\\\tools\\\\gloss.exe", "agent:copilot"))
	write(".cursor/mcp.json", `{"mcpServers":{"glossmark":{"command":"gloss","args":["mcp"]}}}`)
	cs = run()
	c := cs["claude-code plugin"]
	for _, want := range []string{"skills missing: curate-increment", "hooks/hooks.json runs /opt/gm/gloss", "does not list the plugin"} {
		if c.Status != Warn || !strings.Contains(c.Msg, want) || c.Fix != "gloss hook install --agent claude-code" {
			t.Errorf("plugin lacks %q: %+v", want, c)
		}
	}
	if c := cs["copilot"]; c.Status != Warn || !strings.Contains(c.Msg, `.vscode/mcp.json runs C:\tools\gloss.exe`) || c.Fix != "gloss hook install --agent copilot" {
		t.Errorf("copilot = %+v", c)
	}
	if c := cs["cursor"]; c.Status != Warn || !strings.Contains(c.Msg, "sets no GLOSS_ACTOR") || c.Fix != "gloss hook install --agent cursor" {
		t.Errorf("cursor = %+v", c)
	}
}

// TestDoctorAdoption (issue #258): doctor names the level in force, the
// rung the tree has reached and what the next one adds; a mismatch is an
// Info with the line that fixes it, never a failure.
func TestDoctorAdoption(t *testing.T) {
	at := func(cfg string, manifest bool) Check {
		t.Helper()
		root := t.TempDir()
		write := func(rel, content string) {
			p := filepath.Join(root, rel)
			os.MkdirAll(filepath.Dir(p), 0o755)
			os.WriteFile(p, []byte(content), 0o644)
		}
		write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n"+cfg)
		write("ontology/z.gloss.yaml", "concepts:\n  Thing: { kind: entity }\n")
		if manifest {
			write("svc/.gloss.yaml", "boundary: o:svc\nlevel: container\n")
			write("svc/a.py", "# @gloss represents z:Thing scope=file\nclass T: pass\n")
		}
		ws, err := load.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		res, _ := extract.Build(ws)
		rollup.Derive(res.Graph, ws.Vocab)
		for _, c := range Run(ws, res.Graph, nil, Env{Actor: "agent:x", LookPath: func(string) (string, error) { return "", errors.New("none") }}) {
			if c.Name == "adoption" {
				return c
			}
		}
		return Check{}
	}
	// Unset means level 3; a glossary-only tree is at 0: say so and name level 1.
	if c := at("", false); c.Status != Info || !strings.Contains(c.Msg, "level 3 (full), adoption: unset") || !strings.Contains(c.Msg, "Written: level 0 (vocabulary)") || !strings.Contains(c.Msg, "Next, level 1 (manifests): write one .gloss.yaml manifest") || !strings.Contains(c.Fix, "set adoption: 0") {
		t.Errorf("unset on a glossary = %+v", c)
	}
	// The level matches what is written.
	if c := at("adoption: 0\n", false); c.Status != OK || !strings.Contains(c.Msg, "level 0 (vocabulary) fails on nothing") || !strings.Contains(c.Msg, "Next, level 1") {
		t.Errorf("level 0 = %+v", c)
	}
	if c := at("adoption: 1\n", true); c.Status != OK || !strings.Contains(c.Msg, "Written: level 1 (manifests) (1 manifest(s), 1 link(s) from code)") || !strings.Contains(c.Msg, "Next, level 2 (criticality)") || !strings.Contains(c.Msg, "(no criticality: declared)") {
		t.Errorf("level 1 = %+v", c)
	}
	// The tree has more than the level enforces: suggest raising it.
	if c := at("adoption: 0\n", true); c.Status != Info || !strings.Contains(c.Fix, "raise adoption: in glossmark.yaml to 1") {
		t.Errorf("level 0 with manifests = %+v", c)
	}
}
