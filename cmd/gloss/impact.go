// @gloss implements gmi:Gate, gmi:AgentsSnippet scope=file
// @gloss produces gm:Impact scope=file

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/agents"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/skills"
)

func init() {
	register(command{"impact", "what a change touches: concepts, constraints, published seam, boundaries, tests and blast radius for changed files (or the given paths)", cmdImpact})
	register(command{"context", "compact session-start briefing for an agent working in a directory (--dir) or on a feature (--concept); meant for a SessionStart hook", cmdContext})
	register(command{"hook", "gloss hook install: register the SessionStart hook in .claude/settings.json and the MCP server in .mcp.json (--agent claude-code|cursor|copilot: a ready-made integration for that agent)", cmdHook})
}

func cmdImpact(args []string) int {
	fs, root := newFlags("impact")
	base := fs.String("base", "", "git ref; when set, the changed files come from git diff against it and the AUTO/SOFT/HARD classification is included")
	format := fs.String("format", "text", "text | json")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	files := fs.Args()
	for i, f := range files {
		if filepath.IsAbs(f) {
			if r, err := filepath.Rel(p.WS.Root, f); err == nil {
				files[i] = filepath.ToSlash(r)
			}
		}
	}
	if *base != "" {
		ch, err := changedFiles(p.WS.Root, *base)
		if err != nil {
			return fail(err)
		}
		files = append(files, attributable(p.WS, ch)...)
	}
	if len(files) == 0 {
		ch, err := changedFiles(p.WS.Root, "HEAD")
		if err == nil {
			files = attributable(p.WS, ch)
		}
	}
	if len(files) == 0 {
		fmt.Println("gloss impact: no changed files (pass paths or --base <ref>)")
		return 0
	}
	touched := gate.Touched(p.Graph, p.WS, uniqStrings(files))
	var im *gate.Impact
	if *base != "" {
		bg, err := loadBaseGraph(p.WS, *base)
		if err != nil {
			return fail(err)
		}
		pol := p.WS.Config.EffectivePolicy()
		after := rollup.Compare(p.Graph, p.WS, rollup.Options{Ledger: p.Ledger.Entries, Policy: pol, Now: time.Now()})
		before := baseReport(p.WS, *base, bg, pol)
		im = gate.Classify(bg, p.Graph, p.WS, gate.Options{Base: *base, Policy: pol, HeadLedger: p.Ledger.Entries, ChangedFiles: files, BeforeReport: before, AfterReport: after})
	}
	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{"files": touched, "impact": im, "slices": slicesTouched(p.Graph, p.WS, touched)})
		return 0
	}
	fmt.Print(gate.RenderTouched(touched))
	fmt.Print(renderSlicesTouched(slicesTouched(p.Graph, p.WS, touched)))
	if im != nil {
		fmt.Println()
		fmt.Print(gate.RenderText(im))
	}
	return 0
}

// attributable drops the changed files no concept can own: the tool's own
// state (.glossmark/), and excluded and generated paths. Listing them would
// ask for a marker that cannot be written.
func attributable(ws *load.Workspace, files []string) []string {
	var out []string
	for _, f := range files {
		if !ws.Ignored(f) {
			out = append(out, f)
		}
	}
	return out
}

func uniqStrings(xs []string) []string {
	m := map[string]bool{}
	var out []string
	for _, x := range xs {
		if x != "" && !m[x] {
			m[x] = true
			out = append(out, x)
		}
	}
	return out
}

func cmdContext(args []string) int {
	fs, root := newFlags("context")
	dir := fs.String("dir", "", "directory whose nearest manifest scopes the briefing (default: current directory)")
	max := fs.Int("max", 0, "maximum concepts to list (0: as many as the budget allows)")
	budget := fs.Int("budget", 2000, "approximate token budget for the whole briefing (0: unlimited)")
	changed := fs.Bool("changed", false, "open the briefing with what the uncommitted changes touch (gloss impact of the working tree)")
	var concepts []string
	fs.Func("concept", "brief on a feature instead of a directory: the slice of this concept, boundary or path (repeatable; see gloss slice)", func(s string) error { concepts = append(concepts, s); return nil })
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	if len(concepts) > 0 {
		txt, err := conceptBriefing(p, concepts, *budget)
		if err != nil {
			return fail(err)
		}
		fmt.Print(txt)
		return 0
	}
	d := *dir
	if d == "" {
		d, _ = os.Getwd()
	}
	fmt.Print(briefing(p, d, *max, *budget, *changed))
	return 0
}

// briefing renders gloss context for a directory (absolute or repo-relative).
func briefing(p *pipeline, dir string, max, budget int, changed bool) string {
	rel := dir
	if filepath.IsAbs(rel) {
		rel, _ = filepath.Rel(p.WS.Root, rel)
	}
	rel = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(rel)), "./")
	if rel == "." {
		rel = ""
	}
	probe := "x"
	if rel != "" {
		probe = rel + "/x"
	}
	m := p.WS.ManifestFor(probe)
	rep := rollup.Compare(p.Graph, p.WS, rollup.Options{Ledger: p.Ledger.Entries, Policy: p.WS.Config.EffectivePolicy(), Now: time.Now()})
	opt := agents.ContextOptions{Dir: rel, MaxConcepts: max, Budget: budget, Actor: os.Getenv("GLOSS_ACTOR")}
	if changed {
		if files, err := changedFiles(p.WS.Root, "HEAD"); err == nil && len(attributable(p.WS, files)) > 0 {
			files = attributable(p.WS, files)
			ts := gate.Touched(p.Graph, p.WS, uniqStrings(files))
			if len(ts) > 8 {
				ts = ts[:8]
			}
			opt.Changed = gate.RenderTouched(ts)
		}
	}
	return agents.Context(p.WS, p.Graph, m, rep, opt)
}

func cmdHook(args []string) int {
	fs, root := newFlags("hook")
	noMCP := fs.Bool("no-mcp", false, "do not register the MCP server in .mcp.json")
	noHook := fs.Bool("no-hook", false, "do not register the SessionStart hook")
	actor := fs.String("actor", "agent:claude-code", "GLOSS_ACTOR the hook and the MCP server run with, so links an agent writes are attributed to it (agent:cursor for the Cursor files unless set)")
	skill := fs.Bool("skill", false, "also install the shipped skills (adopt-glossmark, derive-ontology, review-glossmark, curate-increment) into .claude/skills/, which Claude Code, Cursor and Copilot all read; before gloss init, only the skills")
	cursor := fs.Bool("cursor", false, "same as --agent cursor")
	var agentFlags []string
	fs.Func("agent", "also set up a ready-made integration (repeatable, or comma-separated): claude-code (a plugin in "+pluginDir+" with the skills, the SessionStart hook and the MCP server, listed in "+marketplacePath+"), cursor (an always-applied rule in .cursor/rules/glossmark.mdc with the vocabulary, and .cursor/mcp.json) or copilot (a GlossMark section in "+copilotPath+" and "+vscodeMCPPath+")", func(s string) error {
		agentFlags = append(agentFlags, s)
		return nil
	})
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 1 || fs.Arg(0) != "install" {
		fmt.Fprintln(os.Stderr, "usage: gloss hook install [--no-mcp] [--no-hook] [--skill] [--agent claude-code|cursor|copilot] [--actor agent:name]")
		return 2
	}
	if *cursor {
		agentFlags = append(agentFlags, "cursor")
	}
	agentList, err := parseAgents(agentFlags)
	if err != nil {
		return fail(err)
	}
	// Each integration runs as its own agent unless --actor says otherwise.
	actorFor := func(agent string) string { return defaultActors[agent] }
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "actor" {
			actorFor = func(string) string { return *actor }
		}
	})
	// Before gloss init there is no workspace to hook into, but the skills
	// are how an adoption starts (derive-ontology runs before init), and the
	// agent files are how each agent finds them: the Claude Code plugin
	// bundles them, the Cursor rule and the Copilot instructions point at
	// .claude/skills/, so those two install the skills too.
	if *skill || len(agentList) > 0 {
		dir := *root
		if dir == "" {
			cwd, _ := os.Getwd()
			if r, err := load.FindRoot(cwd); err == nil {
				dir = r
			} else {
				dir = cwd
			}
		}
		if _, err := os.Stat(filepath.Join(dir, load.ConfigName)); err != nil {
			var what []string
			if *skill || hasAgent(agentList, "cursor") || hasAgent(agentList, "copilot") {
				if err := installSkills(dir); err != nil {
					return fail(err)
				}
				what = append(what, "the skills")
			}
			for _, a := range agentList {
				// No workspace, so no MCP server to register for Cursor or
				// Copilot; the plugin is one unit and keeps its server.
				if err := installAgent(dir, a, actorFor(a), "", true, false); err != nil {
					return fail(err)
				}
				what = append(what, agentLabel[a])
			}
			fmt.Printf("gloss hook: no %s yet, so only %s were installed; gloss init installs the hook and the MCP server\n", load.ConfigName, strings.Join(what, ", "))
			return 0
		}
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	if err := installHooks(p.WS.Root, glossCommand(), *actor, !*noHook, !*noMCP); err != nil {
		return fail(err)
	}
	if *skill {
		if err := installSkills(p.WS.Root); err != nil {
			return fail(err)
		}
	}
	if len(agentList) == 0 {
		return 0
	}
	// The rule and the instructions carry the vocabulary of the whole
	// repository, as gloss agents prints it, so an agent without MCP still
	// knows the names.
	snippet := agents.Snippet(p.WS, p.Graph, nil)
	for _, a := range agentList {
		if err := installAgent(p.WS.Root, a, actorFor(a), snippet, !*noHook, !*noMCP); err != nil {
			return fail(err)
		}
	}
	return 0
}

// agentLabel names each integration in messages.
var agentLabel = map[string]string{"claude-code": "the Claude Code plugin", "cursor": "the Cursor rule", "copilot": "the Copilot instructions"}

func hasAgent(list []string, name string) bool {
	for _, a := range list {
		if a == name {
			return true
		}
	}
	return false
}

// installAgent writes one integration. snippet is the vocabulary ("" before
// gloss init); hook and mcp mirror --no-hook and --no-mcp, which apply to
// the Cursor and Copilot halves the same way (the plugin is one unit).
func installAgent(root, agent, actor, snippet string, hook, mcp bool) error {
	switch agent {
	case "claude-code":
		return installClaudePlugin(root, actor)
	case "cursor":
		if hook {
			if err := installCursorRule(root, actor, snippet); err != nil {
				return err
			}
		}
		if mcp {
			return registerMCP(root, filepath.Join(".cursor", "mcp.json"), actor)
		}
		return nil
	case "copilot":
		if hook {
			if err := installCopilotInstructions(root, actor, snippet); err != nil {
				return err
			}
		}
		if mcp {
			return registerMCPIn(root, vscodeMCPPath, "servers", actor, map[string]any{"type": "stdio"}, false)
		}
		return nil
	}
	return fmt.Errorf("--agent %s: unknown agent", agent)
}

// installSkills copies the shipped skills into .claude/skills/. Cursor reads
// that directory too, so one copy serves both.
func installSkills(root string) error {
	for _, name := range skills.Names {
		b, err := skills.Installable(name)
		if err != nil {
			return err
		}
		path := filepath.Join(root, ".claude", "skills", name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return err
		}
		fmt.Printf("gloss hook: skill %s installed in %s\n", name, relOrAbs(root, path))
	}
	return nil
}

// cursorRulePath is where gloss hook install --cursor writes its rule.
var cursorRulePath = filepath.Join(".cursor", "rules", "glossmark.mdc")

// cursorRule is the always-applied Cursor rule. Cursor has no SessionStart
// hook, so the rule tells the agent to fetch the briefing itself, to carry
// GLOSS_ACTOR on every gloss command, and that deriving or adopting an
// ontology is a conversation with the user, not something to finish alone.
func cursorRule(actor, bin string) string {
	a := shellWord(actor)
	b := shellWord(bin)
	return `---
description: GlossMark - link code to the domain ontology with @gloss comments
alwaysApply: true
---
<!-- generated by gloss hook install --cursor; rerun it to update -->

# GlossMark

This repository links code to a domain ontology with one-line ` + "`@gloss`" + ` comments.

- Before working in a directory, run ` + "`GLOSS_ACTOR=" + a + " " + b + " context --dir <that directory> --changed --budget 2000`" + ` and read the briefing: the boundary, the concepts you may link to, the current state.
- Run every gloss command with ` + "`GLOSS_ACTOR=" + a + "`" + ` so the links you write are attributed to you. The glossmark MCP server (search_concepts, concepts_for_path, propose_link, propose_concept, impact_of, slice_of, history_of) already runs as that actor.
- Write ` + "`@gloss?`" + ` for every link you add; a human removes the ` + "`?`" + `.
- ` + agents.ProposalSentence + `

## Adopting GlossMark or deriving the ontology

If asked to add GlossMark, derive the ontology or find the core concepts, follow ` + "`.claude/skills/derive-ontology/SKILL.md`" + ` and then ` + "`.claude/skills/adopt-glossmark/SKILL.md`" + ` (` + "`gloss hook install --skill`" + ` installs them). The core ontology is the user's domain, not yours:

- Do the legwork first, then interview the user about core processes, core concepts, granularity and the views they want, before you write any ontology file.
- Ask in chat as numbered questions with lettered choices, your recommendation first, then **end your turn and wait**. Never answer your own questions, never assume the recommended option, and never keep drafting while a question is open.

## Planning a feature in the ontology first

If asked to plan a ticket or feature in the ontology, or to curate an increment, follow ` + "`.claude/skills/curate-increment/SKILL.md`" + `. Interview the user before writing anything, write only planned entries and the plan record, and stop before the PR.

## Reviewing proposals, stale links and findings

If asked to clean up the review backlog, triage ` + "`@gloss?`" + ` proposals or fix stale links, follow ` + "`.claude/skills/review-glossmark/SKILL.md`" + `. Decide AUTO and SOFT items through ` + "`gloss review apply`" + `, recommend on HARD ones, and leave HARD decisions to a person.
`
}

// installCursorRule writes the Cursor rule, with the AGENTS.md vocabulary
// of the whole repository after it when there is one (snippet is "" before
// gloss init).
func installCursorRule(root, actor, snippet string) error {
	path := filepath.Join(root, cursorRulePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Committed like .mcp.json, so it names gloss, not this machine's path.
	rule := cursorRule(actor, "gloss")
	if snippet != "" {
		rule += "\n" + vocabularySection(snippet)
	}
	if err := os.WriteFile(path, []byte(rule), 0o644); err != nil {
		return err
	}
	fmt.Printf("gloss hook: Cursor rule written to %s (Cursor has no SessionStart hook; the rule asks the agent for gloss context)\n", relOrAbs(root, path))
	return nil
}

// glossCommand is how this machine's hook invokes gloss: by name when PATH
// finds this binary, else by this binary's absolute path, so another
// program named gloss earlier on PATH cannot silently stand in (issue #37).
// Only the uncommitted .claude/settings.local.json gets the path (#226).
func glossCommand() string {
	self, err := os.Executable()
	if err != nil {
		return "gloss"
	}
	return glossCommandFor(self, exec.LookPath)
}

func glossCommandFor(self string, lookPath func(string) (string, error)) string {
	if r, err := filepath.EvalSymlinks(self); err == nil {
		self = r
	}
	if strings.Contains(self, "go-build") {
		return "gloss" // go run or go test: a temporary binary
	}
	if p, err := lookPath("gloss"); err == nil {
		if r, err := filepath.EvalSymlinks(p); err == nil && r == self {
			return "gloss"
		}
	}
	return self
}

// shellWord quotes s for a shell command line when it needs it.
func shellWord(s string) string {
	if strings.ContainsAny(s, " '\"$\\") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}

// hookCommand is the SessionStart hook: it appends `export GLOSS_ACTOR=actor`
// to $CLAUDE_ENV_FILE, which Claude Code sources before every shell command
// of the session, so a `gloss build` the agent runs later is attributed to it
// too, not only the briefing (issue #158). Outside Claude Code the variable
// is unset and the line goes to /dev/null. Subagents share the session's
// shell environment, so each sets its own actor on its gloss commands
// (docs/QUICKSTART.md).
func hookCommand(actor, bin string) string {
	a := shellWord(actor)
	return fmt.Sprintf("echo export GLOSS_ACTOR=%s >> \"${CLAUDE_ENV_FILE:-/dev/null}\"; GLOSS_ACTOR=%s %s context --dir \"$PWD\" --changed --budget 2000 2>/dev/null || true", shellWord(a), a, shellWord(bin))
}

// installHooks registers the SessionStart hook and the MCP server for a
// repository, both running with GLOSS_ACTOR=actor. bin is glossCommand():
// the committed files always run gloss by name, so a clone on another
// machine works; when PATH does not find this binary, the hook that runs it
// by path goes to .claude/settings.local.json, which is not committed
// (issue #226).
func installHooks(root, bin, actor string, hook, mcp bool) error {
	if hook {
		path := filepath.Join(root, ".claude", "settings.json")
		if err := registerHook(path, hookCommand(actor, "gloss")); err != nil {
			return err
		}
		fmt.Printf("gloss hook: SessionStart hook registered in %s\n", relOrAbs(root, path))
		local := filepath.Join(root, ".claude", "settings.local.json")
		if bin != "gloss" {
			if err := registerHook(local, hookCommand(actor, bin)); err != nil {
				return err
			}
			fmt.Printf("gloss hook: PATH does not find this gloss, so the hook in %s (not committed) runs %s\n", relOrAbs(root, local), bin)
			if exec.Command("git", "-C", root, "rev-parse", "--git-dir").Run() == nil &&
				exec.Command("git", "-C", root, "check-ignore", "-q", local).Run() != nil {
				fmt.Println("gloss hook: add .claude/settings.local.json to .gitignore; it holds this machine's path")
			}
		} else if _, err := os.Stat(local); err == nil {
			// gloss is on PATH now: drop the path an earlier install wrote.
			if err := registerHook(local, ""); err != nil {
				return err
			}
		}
	}
	if mcp {
		if err := registerMCP(root, ".mcp.json", actor); err != nil {
			return err
		}
	}
	if bin != "gloss" && mcp {
		fmt.Printf("gloss hook: the MCP server runs gloss by name: put %s on PATH\n", filepath.Dir(bin))
	}
	fmt.Printf("gloss hook: new sessions will receive 'gloss context' for their directory and write links as %s (the hook exports it to the session's shell); agents can call search_concepts, concepts_for_path, propose_link, propose_concept, impact_of, slice_of, history_of\n", actor)
	fmt.Println("gloss hook: commit .glossmark/ledger.jsonl; it is the review record (accepted, rejected, stale) and who wrote each link")
	return nil
}

// registerHook puts command in the SessionStart hooks of a Claude Code
// settings file, replacing an earlier gloss context hook; an empty command
// only removes that hook.
func registerHook(path, command string) error {
	settings := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, &settings)
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	entry := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command}}}
	old, _ := hooks["SessionStart"].([]any)
	list := []any{}
	present := false
	for _, it := range old {
		if s, _ := json.Marshal(it); strings.Contains(string(s), "gloss context") || strings.Contains(string(s), "gloss") && strings.Contains(string(s), " context --dir") {
			// An earlier install: replace it so the actor line is current.
			if command != "" && !present {
				list = append(list, entry)
			}
			present = true
			continue
		}
		list = append(list, it)
	}
	if command != "" && !present {
		list = append(list, entry)
	}
	if command == "" && !present {
		return nil
	}
	hooks["SessionStart"] = list
	if len(list) == 0 {
		delete(hooks, "SessionStart")
	}
	settings["hooks"] = hooks
	return writeJSON(path, settings)
}

// registerMCP adds the glossmark server to an MCP config file (rel to root)
// in the mcpServers format Claude Code and Cursor share. The file is
// committed, so the server runs gloss by name; a path an earlier install
// wrote is replaced, a command of the user's own is kept.
func registerMCP(root, rel, actor string) error {
	return registerMCPIn(root, filepath.ToSlash(rel), "mcpServers", actor, nil, false)
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // keep ">" readable in hook commands
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
