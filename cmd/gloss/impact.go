// @gloss implements gmi:Gate, gmi:AgentsSnippet scope=file

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/agents"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/rollup"
)

func init() {
	register(command{"impact", "what a change touches: concepts, constraints, published seam, boundaries, tests and blast radius for changed files (or the given paths)", cmdImpact})
	register(command{"context", "compact session-start briefing for an agent working in a directory (--dir); meant for a SessionStart hook", cmdContext})
	register(command{"hook", "gloss hook install: register the SessionStart hook in .claude/settings.json and the MCP server in .mcp.json", cmdHook})
}

func cmdImpact(args []string) int {
	fs, root := newFlags("impact")
	base := fs.String("base", "", "git ref; when set, the changed files come from git diff against it and the AUTO/SOFT/HARD classification is included")
	format := fs.String("format", "text", "text | json")
	if err := fs.Parse(flagsLast(args)); err != nil {
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
		files = append(files, ch...)
	}
	if len(files) == 0 {
		ch, err := changedFiles(p.WS.Root, "HEAD")
		if err == nil {
			files = ch
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
		im = gate.Classify(bg, p.Graph, p.WS, gate.Options{Base: *base, Policy: pol, HeadLedger: p.Ledger.Entries, ChangedFiles: files, AfterReport: after})
	}
	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{"files": touched, "impact": im})
		return 0
	}
	fmt.Print(gate.RenderTouched(touched))
	if im != nil {
		fmt.Println()
		fmt.Print(gate.RenderText(im))
	}
	return 0
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
	max := fs.Int("max", 60, "maximum concepts to list")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	d := *dir
	if d == "" {
		d, _ = os.Getwd()
	}
	rel := d
	if filepath.IsAbs(rel) {
		rel, _ = filepath.Rel(p.WS.Root, rel)
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	m := p.WS.ManifestFor(strings.TrimPrefix(rel+"/x", "./"))
	rep := rollup.Compare(p.Graph, p.WS, rollup.Options{Ledger: p.Ledger.Entries, Policy: p.WS.Config.EffectivePolicy(), Now: time.Now()})
	fmt.Print(agents.Context(p.WS, p.Graph, m, rep, *max))
	return 0
}

func cmdHook(args []string) int {
	fs, root := newFlags("hook")
	noMCP := fs.Bool("no-mcp", false, "do not register the MCP server in .mcp.json")
	noHook := fs.Bool("no-hook", false, "do not register the SessionStart hook")
	actor := fs.String("actor", "agent:claude-code", "GLOSS_ACTOR the hook and the MCP server run with, so links an agent writes are attributed to it")
	if err := fs.Parse(flagsLast(args)); err != nil {
		return 2
	}
	if fs.NArg() != 1 || fs.Arg(0) != "install" {
		fmt.Fprintln(os.Stderr, "usage: gloss hook install [--no-mcp] [--no-hook] [--actor agent:name]")
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	if !*noHook {
		path := filepath.Join(p.WS.Root, ".claude", "settings.json")
		settings := map[string]any{}
		if b, err := os.ReadFile(path); err == nil {
			json.Unmarshal(b, &settings)
		}
		hooks, _ := settings["hooks"].(map[string]any)
		if hooks == nil {
			hooks = map[string]any{}
		}
		entry := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": fmt.Sprintf("GLOSS_ACTOR=%s gloss context --dir \"$PWD\" 2>/dev/null || true", *actor)}}}
		list, _ := hooks["SessionStart"].([]any)
		present := false
		for i, it := range list {
			if b, _ := json.Marshal(it); strings.Contains(string(b), "gloss context") {
				// An earlier install: replace it so the actor line is current.
				list[i] = entry
				present = true
			}
		}
		if !present {
			list = append(list, entry)
		}
		hooks["SessionStart"] = list
		settings["hooks"] = hooks
		if err := writeJSON(path, settings); err != nil {
			return fail(err)
		}
		fmt.Printf("gloss hook: SessionStart hook registered in %s\n", relOrAbs(p.WS.Root, path))
	}
	if !*noMCP {
		path := filepath.Join(p.WS.Root, ".mcp.json")
		cfg := map[string]any{}
		if b, err := os.ReadFile(path); err == nil {
			json.Unmarshal(b, &cfg)
		}
		servers, _ := cfg["mcpServers"].(map[string]any)
		if servers == nil {
			servers = map[string]any{}
		}
		srv, _ := servers["glossmark"].(map[string]any)
		if srv == nil {
			srv = map[string]any{"command": "gloss", "args": []string{"mcp"}}
		}
		env, _ := srv["env"].(map[string]any)
		if env == nil {
			env = map[string]any{}
		}
		env["GLOSS_ACTOR"] = *actor
		srv["env"] = env
		servers["glossmark"] = srv
		cfg["mcpServers"] = servers
		if err := writeJSON(path, cfg); err != nil {
			return fail(err)
		}
		fmt.Printf("gloss hook: MCP server registered in %s\n", relOrAbs(p.WS.Root, path))
	}
	fmt.Printf("gloss hook: new sessions will receive 'gloss context' for their directory and write links as %s; agents can call search_concepts, concepts_for_path, propose_link, propose_concept, impact_of\n", *actor)
	fmt.Println("gloss hook: commit .glossmark/ledger.jsonl; it is the review record (accepted, rejected, stale) and who wrote each link")
	return 0
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
