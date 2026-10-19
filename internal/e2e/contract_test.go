// @gloss verifies gmi:AdoptionSkill scope=file
// @gloss verifies gmi:ClaudeCodePlugin scope=file

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/mckalvan/glossmark/internal/agents"
	"github.com/mckalvan/glossmark/skills"
)

// The contract tests pin what an agent reads outside the Go code: the shipped
// skills, QUICKSTART.md and the Claude Code plugin say what the constants in
// internal/agents say, and the plugin runs the hooks gloss hook install writes.

// repoRoot is the root of this repository.
func repoRoot(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// pluginActor is the actor the plugin's hooks and MCP server run as: the
// default of gloss hook install --actor.
const pluginActor = "agent:claude-code"

// TestShippedSkillsContract: every skills/<name>/SKILL.md opens with
// frontmatter naming its directory and saying when to use it, states the
// proposal sentence verbatim, and is embedded byte for byte; skills.Names
// lists exactly the directories; every skill the rules block names ships.
func TestShippedSkillsContract(t *testing.T) {
	root := repoRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	names := append([]string(nil), skills.Names...)
	sort.Strings(dirs)
	sort.Strings(names)
	if !reflect.DeepEqual(dirs, names) {
		t.Errorf("skills.Names %v must list exactly the directories under skills/ %v", skills.Names, dirs)
	}
	for _, dir := range dirs {
		b, err := os.ReadFile(filepath.Join(root, "skills", dir, "SKILL.md"))
		if err != nil {
			t.Errorf("skills/%s: %v", dir, err)
			continue
		}
		doc := string(b)
		if !strings.HasPrefix(doc, "---\n") {
			t.Errorf("skills/%s/SKILL.md: line 1 must be the frontmatter's ---", dir)
			continue
		}
		end := strings.Index(doc[4:], "\n---\n")
		if end < 0 {
			t.Errorf("skills/%s/SKILL.md: frontmatter is not closed by ---", dir)
			continue
		}
		var fm struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		}
		if err := yaml.Unmarshal([]byte(doc[4:4+end+1]), &fm); err != nil {
			t.Errorf("skills/%s/SKILL.md: frontmatter: %v", dir, err)
		}
		if fm.Name != dir {
			t.Errorf("skills/%s/SKILL.md: name %q must equal the directory", dir, fm.Name)
		}
		if strings.TrimSpace(fm.Description) == "" {
			t.Errorf("skills/%s/SKILL.md: description is empty", dir)
		}
		if !strings.Contains(doc, agents.ProposalSentence) {
			t.Errorf("skills/%s/SKILL.md must state agents.ProposalSentence verbatim", dir)
		}
		if !strings.Contains(doc, "<!-- @gloss instructs ") {
			t.Errorf("skills/%s/SKILL.md: no instructs marker", dir)
		}
		if emb, err := skills.FS.ReadFile(dir + "/SKILL.md"); err != nil || string(emb) != doc {
			t.Errorf("skills/%s/SKILL.md is not embedded as is (%v)", dir, err)
		}
	}
	for _, n := range agents.RuleSkills {
		if _, err := os.Stat(filepath.Join(root, "skills", n, "SKILL.md")); err != nil {
			t.Errorf("the rules block names skill %s, which does not ship: %v", n, err)
		}
	}
}

// TestQuickstartContract: QUICKSTART.md quotes the proposal sentence verbatim
// and names every shipped skill.
func TestQuickstartContract(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "QUICKSTART.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), agents.ProposalSentence) {
		t.Error("docs/QUICKSTART.md must quote agents.ProposalSentence verbatim")
	}
	for _, n := range skills.Names {
		if !strings.Contains(string(b), "`"+n+"`") {
			t.Errorf("docs/QUICKSTART.md does not name the shipped skill %s", n)
		}
	}
}

type hookGroup struct {
	Matcher string `json:"matcher,omitempty"`
	Hooks   []struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	} `json:"hooks"`
}

func readHooks(t *testing.T, path string) map[string][]hookGroup {
	t.Helper()
	var cfg struct {
		Hooks map[string][]hookGroup `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(readFile(t, path)), &cfg); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return cfg.Hooks
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &m); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return m
}

// TestClaudeCodePluginContract: the plugin at the repository root runs the
// hook lines gloss hook install --guards writes and the MCP server gloss hook
// install registers, for actor agent:claude-code, and its manifests name it.
func TestClaudeCodePluginContract(t *testing.T) {
	root := repoRoot(t)
	hooks := readHooks(t, filepath.Join(root, "hooks", "hooks.json"))

	// Against the constants in internal/agents.
	want := map[string]hookGroup{"SessionStart": {}}
	for _, ev := range agents.GuardEvents {
		want[ev] = hookGroup{}
	}
	if len(hooks) != len(want) {
		t.Errorf("hooks/hooks.json events: %v", hooks)
	}
	for ev := range want {
		groups := hooks[ev]
		if len(groups) != 1 || len(groups[0].Hooks) != 1 || groups[0].Hooks[0].Type != "command" {
			t.Errorf("hooks/hooks.json %s: want one command hook, got %+v", ev, groups)
			continue
		}
		cmd, matcher := groups[0].Hooks[0].Command, groups[0].Matcher
		wantCmd, wantMatcher := agents.GuardCommand(ev, pluginActor), agents.GuardMatcher
		if ev == "SessionStart" {
			wantCmd, wantMatcher = agents.SessionStartCommand(pluginActor), ""
		}
		if ev == "Stop" {
			wantMatcher = ""
		}
		if cmd != wantCmd || matcher != wantMatcher {
			t.Errorf("hooks/hooks.json %s: (%q, %q), want (%q, %q)", ev, matcher, cmd, wantMatcher, wantCmd)
		}
	}

	// Against what the binary writes.
	ws := paymentsCopy(t)
	if r := gloss(t, ws, "", nil, "hook", "install", "--guards"); r.code != 0 {
		t.Fatalf("gloss hook install --guards: %d %s", r.code, r.stderr)
	}
	if got := readHooks(t, filepath.Join(ws, ".claude", "settings.json")); !reflect.DeepEqual(got, hooks) {
		t.Errorf("hooks/hooks.json differs from what gloss hook install --guards writes:\nplugin:  %+v\ninstall: %+v", hooks, got)
	}
	mcp := readJSON(t, filepath.Join(root, ".mcp.json"))
	if got := readJSON(t, filepath.Join(ws, ".mcp.json")); !reflect.DeepEqual(got, mcp) {
		t.Errorf(".mcp.json differs from what gloss hook install writes:\nrepo:    %v\ninstall: %v", mcp, got)
	}
	srv, _ := mcp["mcpServers"].(map[string]any)["glossmark"].(map[string]any)
	env, _ := srv["env"].(map[string]any)
	if srv["command"] != "gloss" || !reflect.DeepEqual(srv["args"], []any{"mcp"}) || env["GLOSS_ACTOR"] != pluginActor {
		t.Errorf(".mcp.json must run gloss mcp with GLOSS_ACTOR=%s: %v", pluginActor, srv)
	}

	// The manifests.
	plugin := readJSON(t, filepath.Join(root, ".claude-plugin", "plugin.json"))
	market := readJSON(t, filepath.Join(root, ".claude-plugin", "marketplace.json"))
	if plugin["name"] != "glossmark" || plugin["version"] == nil || plugin["description"] == nil {
		t.Errorf("plugin.json: %v", plugin)
	}
	owner, _ := market["owner"].(map[string]any)
	plugins, _ := market["plugins"].([]any)
	if market["name"] == nil || owner["name"] == nil || len(plugins) != 1 {
		t.Fatalf("marketplace.json needs name, owner.name and one plugin: %v", market)
	}
	entry, _ := plugins[0].(map[string]any)
	if entry["name"] != plugin["name"] || entry["source"] != "./" {
		t.Errorf("marketplace.json plugin entry %v must name %v with source ./", entry, plugin["name"])
	}
}
