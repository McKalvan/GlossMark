// @gloss implements gmi:HookGuard scope=file

// Package guard turns a Claude Code hook payload into GlossMark feedback at
// edit time: before an edit, the file's criticality, published-seam concepts
// and whether it is outside the acting agent's may-modify remit; after an
// edit, the lint findings in that file; when the agent stops, whether gloss
// check is clean and the committed graph matches the tree.
//
// The guard fails open: callers print nothing when anything here errors.
package guard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
)

// Events the guard handles (gloss hook guard --event).
const (
	EventPreEdit  = "pre-edit"
	EventPostEdit = "post-edit"
	EventStop     = "stop"
)

// Payload is the part of a Claude Code hook's stdin the guard reads.
type Payload struct {
	HookEventName string `json:"hook_event_name,omitempty"`
	ToolName      string `json:"tool_name,omitempty"`
	ToolInput     struct {
		FilePath string `json:"file_path,omitempty"`
	} `json:"tool_input"`
	Cwd string `json:"cwd,omitempty"`
	// AgentType names the subagent the hook fires inside, if any.
	AgentType      string `json:"agent_type,omitempty"`
	StopHookActive bool   `json:"stop_hook_active,omitempty"`
}

// ParsePayload decodes a hook payload.
func ParsePayload(b []byte) (*Payload, error) {
	var p Payload
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Output is what the guard prints on stdout, in Claude Code's hook output shape.
type Output struct {
	HookSpecificOutput *Specific `json:"hookSpecificOutput,omitempty"`
	// Decision is "block" on Stop; Reason says why and how to fix it.
	Decision string `json:"decision,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Specific is the event-specific part of a PreToolUse or PostToolUse output.
type Specific struct {
	HookEventName            string `json:"hookEventName"`
	AdditionalContext        string `json:"additionalContext,omitempty"`
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
}

// Actor is the actor the guard judges the edit for: agent:<agent_type> when
// the hook fires inside a subagent that has an agent file in
// .claude/agents/, otherwise the session's GLOSS_ACTOR.
func Actor(root string, p *Payload, sessionActor string) string {
	name := p.AgentType
	if name != "" && !strings.ContainsAny(name, `/\`) && name != "." && name != ".." {
		if st, err := os.Stat(filepath.Join(root, ".claude", "agents", name+".md")); err == nil && !st.IsDir() {
			return "agent:" + name
		}
	}
	return sessionActor
}

// RelPath makes the payload's file path repository-relative. A relative path
// is taken against the payload's cwd, else wd. It reports false for a path
// outside the repository or a payload without a file.
func RelPath(root string, p *Payload, wd string) (string, bool) {
	f := p.ToolInput.FilePath
	if f == "" {
		return "", false
	}
	if !filepath.IsAbs(f) {
		base := p.Cwd
		if base == "" {
			base = wd
		}
		f = filepath.Join(base, f)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	if r, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootAbs = r
	}
	if d, err := filepath.EvalSymlinks(filepath.Dir(f)); err == nil {
		f = filepath.Join(d, filepath.Base(f))
	}
	rel, err := filepath.Rel(rootAbs, filepath.Clean(f))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// PreEdit describes what an edit to the repository-relative file touches,
// for the actor. It returns nil when nothing is notable: an unclassified or
// low-risk file, off the published seam, inside the actor's remit.
func PreEdit(ws *load.Workspace, g *graph.Graph, rel, actor string) *Output {
	pol := ws.Config.EffectivePolicy()
	ts := gate.Touched(g, ws, []string{rel})
	if len(ts) == 0 {
		return nil
	}
	ft := ts[0]
	var facts []string
	ask := ""
	if ft.Criticality != "" {
		listed := pol.GuardAsks(ft.Criticality)
		if listed || model.CriticalityRank(ft.Criticality) >= model.CriticalityRank("high") {
			facts = append(facts, fmt.Sprintf("criticality %s (via %s)", ft.Criticality, ft.CriticalityFrom))
		}
		if listed {
			ask = fmt.Sprintf("policy.guard-ask lists %s: %s is %s (via %s)", ft.Criticality, rel, ft.Criticality, ft.CriticalityFrom)
		}
	}
	// The published-seam concepts; a core name already given as some concept's
	// "published as" is not repeated.
	var seam []string
	as := map[string]bool{}
	for _, c := range ft.Concepts {
		if c.Published != "" {
			seam = append(seam, c.Curie+" (published as "+c.Published+")")
			as[c.Published] = true
		}
	}
	for _, c := range ft.Concepts {
		if c.Published == "" && len(c.PublishedBy) > 0 && !as[c.Curie] {
			seam = append(seam, c.Curie)
		}
	}
	if len(seam) > 0 {
		facts = append(facts, "on the published seam: "+strings.Join(capList(seam, 3), ", "))
	}
	if governance(rel) {
		facts = append(facts, "a governance file, every edit to it is HARD-gated")
	}
	var lines []string
	if len(facts) > 0 {
		lines = append(lines, fmt.Sprintf("GlossMark: %s: %s. Say so before you change it and keep the change minimal.", rel, strings.Join(facts, "; ")))
	}
	if remit, outside := outsideRemit(g, ft, actor); len(outside) > 0 {
		lines = append(lines, fmt.Sprintf("GlossMark: %s is outside %s's may-modify remit (%s): it touches %s. Links you write here are HARD-gated; if the task needs this edit, say why in your summary.",
			rel, actor, strings.Join(remit, ", "), strings.Join(capList(outside, 4), ", ")))
		if ask == "" && pol.GuardAsks(model.GuardAskRemit) {
			ask = fmt.Sprintf("policy.guard-ask lists remit: %s is outside %s's may-modify remit", rel, actor)
		}
	}
	if len(lines) == 0 && ask == "" {
		return nil
	}
	s := &Specific{HookEventName: "PreToolUse", AdditionalContext: strings.Join(lines, "\n")}
	if ask != "" {
		s.PermissionDecision, s.PermissionDecisionReason = "ask", ask
	}
	return &Output{HookSpecificOutput: s}
}

// governance reports whether the file is an ontology file, a manifest or glossmark.yaml.
func governance(rel string) bool {
	return rel == load.ConfigName || strings.HasSuffix(rel, ".gloss.yaml")
}

// outsideRemit returns the actor's remit and the concepts the file touches
// directly that it does not cover. Without a named agent that has a
// may-modify remit, or for a file that links nothing, it returns nothing.
func outsideRemit(g *graph.Graph, ft gate.FileTouch, actor string) ([]string, []string) {
	if !strings.HasPrefix(actor, "agent:") {
		return nil, nil
	}
	remit, ok := gate.AgentRemit(g, actor)
	if !ok {
		return nil, nil
	}
	concepts := map[string]bool{}
	for _, c := range ft.Concepts {
		concepts[c.Curie] = true
	}
	// Code, tests and docs: the concepts the file links to. Ontology files and
	// manifests: the concepts they define or declare.
	var direct []string
	if len(ft.Links) > 0 {
		for _, l := range ft.Links {
			if l.Relation != "may-modify" && l.Relation != "instructs" && concepts[l.Target] {
				direct = append(direct, l.Target)
			}
		}
	} else if governance(ft.Path) {
		for c := range concepts {
			direct = append(direct, c)
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, c := range direct {
		if !seen[c] && !model.MatchAnyCurie(remit, c) {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return remit, out
}

// PostEdit reports the diagnostics that point at the edited file, or nil when it is clean.
func PostEdit(rel string, diags []marker.Diagnostic) *Output {
	var mine []marker.Diagnostic
	for _, d := range diags {
		if filepath.ToSlash(d.Path) == rel {
			mine = append(mine, d)
		}
	}
	if len(mine) == 0 {
		return nil
	}
	sort.SliceStable(mine, func(i, j int) bool {
		if (mine[i].Severity == "error") != (mine[j].Severity == "error") {
			return mine[i].Severity == "error"
		}
		return mine[i].Line < mine[j].Line
	})
	var sb strings.Builder
	fmt.Fprintf(&sb, "GlossMark: gloss check flags %s in %s; fix it before you finish:", plural(len(mine), "finding"), rel)
	const max = 5
	for i, d := range mine {
		if i == max {
			fmt.Fprintf(&sb, "\n- and %d more (run gloss check)", len(mine)-max)
			break
		}
		fmt.Fprintf(&sb, "\n- line %d: %s %s: %s", d.Line, d.Severity, d.Code, d.Msg)
	}
	return &Output{HookSpecificOutput: &Specific{HookEventName: "PostToolUse", AdditionalContext: sb.String()}}
}

// StopState is what the Stop guard judges.
type StopState struct {
	// Diags are every extract and check diagnostic; only errors count.
	Diags []marker.Diagnostic
	// GraphState is GraphCurrent's verdict: "" when current, else why not.
	GraphState string
	// Proposals awaiting a human; mentioned, never a reason to block.
	Proposals int
}

// Stop returns the reason the agent should not stop yet, or "" when the tree
// is clean: no gloss check errors and a current .glossmark/graph.jsonl.
func Stop(st StopState) string {
	var errs []marker.Diagnostic
	for _, d := range st.Diags {
		if d.Severity == "error" {
			errs = append(errs, d)
		}
	}
	var problems, fixes []string
	if len(errs) > 0 {
		problems = append(problems, fmt.Sprintf("gloss check reports %s (first: %s)", plural(len(errs), "error"), errs[0]))
		fixes = append(fixes, "fix them and re-run gloss check")
	}
	if st.GraphState != "" {
		problems = append(problems, st.GraphState)
		fixes = append(fixes, "run gloss build -q and include .glossmark/ in the change")
	}
	if len(problems) == 0 {
		return ""
	}
	reason := "GlossMark: " + strings.Join(problems, "; ") + ". To finish, " + strings.Join(fixes, ", then ") + "."
	if st.Proposals > 0 {
		reason += fmt.Sprintf(" (%s await a human; they do not block.)", plural(st.Proposals, "proposal"))
	}
	return reason
}

// GraphCurrent compares the committed .glossmark/graph.jsonl with the graph
// rebuilt from the tree, ignoring the header line that names the generator
// (the same comparison gloss doctor makes). It returns "" when they match.
func GraphCurrent(ws *load.Workspace, g *graph.Graph) (string, error) {
	rel := filepath.ToSlash(filepath.Join(load.StateDir, "graph.jsonl"))
	committed, err := os.ReadFile(ws.StatePath("graph.jsonl"))
	if err != nil {
		return rel + " does not exist", nil
	}
	var buf bytes.Buffer
	if err := g.WriteJSONL(&buf); err != nil {
		return "", err
	}
	if !bytes.Equal(graphBody(committed), graphBody(buf.Bytes())) {
		return rel + " is stale", nil
	}
	return "", nil
}

// graphBody returns a graph.jsonl without its header line.
func graphBody(b []byte) []byte {
	b = bytes.TrimSpace(b)
	if bytes.HasPrefix(b, []byte(`{"type":"context"`)) {
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			return b[i+1:]
		}
		return nil
	}
	return b
}

func capList(xs []string, n int) []string {
	if len(xs) <= n {
		return xs
	}
	return append(append([]string{}, xs[:n]...), fmt.Sprintf("%d more", len(xs)-n))
}

func plural(n int, s string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, s)
	}
	return fmt.Sprintf("%d %ss", n, s)
}
