// @gloss implements gmi:HookGuard scope=file

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mckalvan/glossmark/internal/agents"
	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/guard"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
)

const guardUsage = "usage: gloss hook guard --event pre-edit|post-edit|stop  (reads the Claude Code hook payload on stdin)"

// cmdHookGuard runs one guard event on the hook payload read from stdin. It
// fails open: when the payload, the repository or the graph cannot be read it
// prints nothing and exits 0, so a broken setup never stops an agent. It never
// exits 2, which Claude Code would read as a blocking error.
func cmdHookGuard(args []string) (code int) {
	fs, root := newFlags("hook guard")
	event := fs.String("event", "", "pre-edit (before an Edit or Write) | post-edit (after one) | stop (when the agent finishes)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	switch *event {
	case guard.EventPreEdit, guard.EventPostEdit, guard.EventStop:
	default:
		fmt.Fprintln(os.Stderr, guardUsage)
		return 1
	}
	defer func() {
		if r := recover(); r != nil {
			code = 0
		}
	}()
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		return 0
	}
	payload, err := guard.ParsePayload(in)
	if err != nil {
		return 0
	}
	if *event == guard.EventStop && payload.StopHookActive {
		return 0
	}
	r := *root
	if r == "" && payload.Cwd != "" {
		if r, err = load.FindRoot(payload.Cwd); err != nil {
			return 0
		}
	}
	p, err := buildGraph(r, false)
	if err != nil {
		return 0
	}
	wd, _ := os.Getwd()
	var out *guard.Output
	switch *event {
	case guard.EventPreEdit:
		rel, ok := guard.RelPath(p.WS.Root, payload, wd)
		if !ok {
			return 0
		}
		out = guard.PreEdit(p.WS, p.Graph, rel, guard.Actor(p.WS.Root, payload, os.Getenv("GLOSS_ACTOR")))
	case guard.EventPostEdit:
		rel, ok := guard.RelPath(p.WS.Root, payload, wd)
		if !ok {
			return 0
		}
		out = guard.PostEdit(rel, append(append([]marker.Diagnostic{}, p.Diags...), check.Run(p.WS, p.Graph)...))
	case guard.EventStop:
		state, err := guard.GraphCurrent(p.WS, p.Graph)
		if err != nil {
			return 0
		}
		reason := guard.Stop(guard.StopState{
			Diags:      append(append([]marker.Diagnostic{}, p.Diags...), check.Run(p.WS, p.Graph)...),
			GraphState: state,
			Proposals:  countProposals(p),
		})
		if reason == "" {
			return 0
		}
		if p.WS.Config.EffectivePolicy().GuardStop == model.GuardStopReport {
			fmt.Fprintln(os.Stderr, reason)
			return 0
		}
		out = &guard.Output{Decision: "block", Reason: reason}
	}
	if out == nil {
		return 0
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.Encode(out)
	return 0
}

// countProposals counts proposed links and this repository's proposed
// concepts, the way gloss check reports them.
func countProposals(p *pipeline) int {
	n := 0
	for _, fm := range p.Res.Markers {
		if fm.Marker.Proposed {
			n++
		}
	}
	for _, c := range p.Graph.NodesOf(model.NodeConcept) {
		if c.Proposed && !c.Imported {
			n++
		}
	}
	return n
}

// guardMarker identifies the hook commands gloss hook install --guards owns.
const guardMarker = "gloss hook guard"

// installGuards registers the edit and stop guards in .claude/settings.json.
// Re-running replaces the guard commands an earlier install wrote and keeps
// every other hook.
func installGuards(root, actor string) error {
	path := filepath.Join(root, ".claude", "settings.json")
	settings := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &settings); err != nil {
			return fmt.Errorf("%s: %w", relOrAbs(root, path), err)
		}
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	for _, event := range agents.GuardEvents {
		entry := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": agents.GuardCommand(event, actor)}}}
		if event != "Stop" {
			entry["matcher"] = agents.GuardMatcher
		}
		list, _ := hooks[event].([]any)
		var kept []any
		at := -1
		for _, it := range list {
			group, ok := it.(map[string]any)
			inner, _ := group["hooks"].([]any)
			if !ok || inner == nil {
				kept = append(kept, it)
				continue
			}
			var rest []any
			for _, h := range inner {
				if hm, ok := h.(map[string]any); ok {
					if cmd, _ := hm["command"].(string); strings.Contains(cmd, guardMarker) {
						continue
					}
				}
				rest = append(rest, h)
			}
			if len(rest) < len(inner) && at < 0 {
				at = len(kept)
			}
			if len(rest) == 0 {
				continue
			}
			group["hooks"] = rest
			kept = append(kept, group)
		}
		if at < 0 {
			kept = append(kept, entry)
		} else {
			kept = append(kept[:at], append([]any{entry}, kept[at:]...)...)
		}
		hooks[event] = kept
	}
	settings["hooks"] = hooks
	if err := writeJSON(path, settings); err != nil {
		return err
	}
	fmt.Printf("gloss hook: guards registered in %s: PreToolUse and PostToolUse on Edit|Write (criticality, remit and lint feedback), Stop (blocks on gloss check errors or a stale graph unless policy.guard-stop is report)\n", relOrAbs(root, path))
	return nil
}
