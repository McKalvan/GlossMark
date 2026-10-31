// @gloss implements gmi:Doctor scope=file

// Package doctor checks that a repository's GlossMark setup works end to end:
// the hook can find gloss, agents are attributed, the committed graph matches
// the tree, the review record is in git, imports are locked, CI pins a
// release, and ontology docs are intact. Each check is one line with a fix.
package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/rollup"
	"github.com/mckalvan/glossmark/internal/version"
)

// Status of one check.
const (
	OK   = "ok"
	Info = "info" // nothing to fix, but worth knowing; never fails the run
	Warn = "warn"
	Fail = "fail"
)

// Check is one line of the report.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Msg    string `json:"msg"`
	Fix    string `json:"fix,omitempty"`
}

// Env is what the checks read from the environment, injectable for tests.
type Env struct {
	Actor    string // GLOSS_ACTOR
	LookPath func(string) (string, error)
	// Version runs `<path> version` and returns the first line of its output.
	Version func(path string) (string, error)
	// Now is the time waivers are judged at (zero: time.Now()).
	Now time.Time
}

// Run performs every check. diags are the build and lint diagnostics of the
// current tree.
func Run(ws *load.Workspace, g *graph.Graph, diags []marker.Diagnostic, env Env) []Check {
	if env.LookPath == nil {
		env.LookPath = exec.LookPath
	}
	if env.Version == nil {
		env.Version = runVersion
	}
	d := &doctor{ws: ws, g: g, diags: diags, env: env, git: isGitRepo(ws.Root)}
	d.binary()
	d.actor()
	d.hook()
	d.integrations()
	d.graph()
	d.ledger()
	d.lock()
	d.pins()
	d.docs()
	d.waivers()
	d.autoAccept()
	return d.out
}

// autoAccept reports the auto-accept rule in force: the conf threshold,
// the precision and share it measured, and where it came from (ADR 0024).
// It warns when no conf meets the target, when soft-conf-threshold is
// overridden, and when the repository's calibration file is missing.
func (d *doctor) autoAccept() {
	cfg := d.ws.Config
	a := cfg.AutoAccept()
	notes := cfg.AutoAcceptNotes()
	msg := strings.TrimPrefix(a.String(), "auto-accept: ")
	switch {
	case !a.Met:
		d.add("auto-accept", Warn, msg, "lower suggest.auto-accept-precision in "+load.ConfigName+", or measure this repository with gloss suggest --calibrate")
	case len(notes) > 0:
		d.add("auto-accept", Warn, msg+"; "+strings.Join(notes, "; "), "")
	default:
		d.add("auto-accept", OK, msg, "")
	}
}

// waivers lists waivers that have expired or expire within 30 days, so a
// deadline is seen before drift starts failing on it. No line without waivers.
func (d *doctor) waivers() {
	ws := d.ws.Config.EffectivePolicy().Waivers
	if len(ws) == 0 {
		return
	}
	now := d.env.Now
	if now.IsZero() {
		now = time.Now()
	}
	var invalid, expired, soon []string
	for _, w := range ws {
		exp, err := time.Parse(rollup.WaiverDate, w.Expires)
		if err != nil {
			invalid = append(invalid, fmt.Sprintf("%s (expires %q is not a YYYY-MM-DD date)", w.Finding, w.Expires))
			continue
		}
		switch {
		case rollup.WaiverExpired(exp, now):
			expired = append(expired, fmt.Sprintf("%s (expired %s)", w.Finding, w.Expires))
		case rollup.WaiverExpired(exp, now.AddDate(0, 0, 30)):
			soon = append(soon, fmt.Sprintf("%s (expires %s)", w.Finding, w.Expires))
		}
	}
	switch {
	case len(invalid) > 0:
		d.add("waivers", Warn, strings.Join(append(append(invalid, expired...), soon...), ", ")+"; a waiver without a valid date waives nothing (GM610)", "write expires: as YYYY-MM-DD in "+load.ConfigName)
	case len(expired) > 0:
		d.add("waivers", Warn, strings.Join(append(expired, soon...), ", "), "fix the finding, renew the date or remove the waiver in "+load.ConfigName)
	case len(soon) > 0:
		d.add("waivers", Warn, "expiring within 30 days: "+strings.Join(soon, ", "), "fix the finding before then, or renew the date")
	default:
		d.add("waivers", OK, fmt.Sprintf("%d waiver(s), none expiring within 30 days", len(ws)), "")
	}
}

// String renders the report, one line per check.
func String(checks []Check) string {
	var sb strings.Builder
	for _, c := range checks {
		glyph := map[string]string{OK: "✓", Info: "·", Warn: "!", Fail: "✗"}[c.Status]
		fmt.Fprintf(&sb, "%s %s: %s", glyph, c.Name, c.Msg)
		if c.Fix != "" {
			fmt.Fprintf(&sb, " → %s", c.Fix)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// Failed reports whether any check failed.
func Failed(checks []Check) bool {
	for _, c := range checks {
		if c.Status == Fail {
			return true
		}
	}
	return false
}

type doctor struct {
	ws    *load.Workspace
	g     *graph.Graph
	diags []marker.Diagnostic
	env   Env
	git   bool
	out   []Check
}

func (d *doctor) add(name, status, msg, fix string) {
	d.out = append(d.out, Check{Name: name, Status: status, Msg: msg, Fix: fix})
}

func isGitRepo(root string) bool {
	return exec.Command("git", "-C", root, "rev-parse", "--git-dir").Run() == nil
}

func (d *doctor) binary() {
	p, err := d.env.LookPath("gloss")
	if err != nil {
		d.add("gloss on PATH", Fail, "not found; the SessionStart hook and the MCP server run `gloss` by name and fail silently without it",
			"curl -fsSL https://raw.githubusercontent.com/mckalvan/glossmark/main/install.sh | sh, or go install github.com/mckalvan/glossmark/cmd/gloss@v0.3.0, then add the directory to PATH")
		return
	}
	// The hook and the MCP server run whatever `gloss` PATH finds, so ask it
	// what it is instead of reporting this build's version.
	fix := "put GlossMark's gloss first on PATH (the MCP server runs gloss by name; gloss hook install points only the hook, in the uncommitted .claude/settings.local.json, at this binary's absolute path)"
	first, err := d.env.Version(p)
	got := strings.Fields(first)
	if err != nil || len(got) != 2 || got[0] != "gloss" {
		d.add("gloss on PATH", Fail, fmt.Sprintf("%s is not GlossMark (`%s version` printed %q)", p, p, first), fix)
		return
	}
	if got[1] != version.String() {
		d.add("gloss on PATH", Warn, fmt.Sprintf("%s is gloss %s, this build is %s", p, got[1], version.String()), fix)
		return
	}
	d.add("gloss on PATH", OK, fmt.Sprintf("%s (gloss %s)", p, got[1]), "")
}

// runVersion runs `<path> version` with a timeout and returns its first line.
func runVersion(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "version").Output()
	first, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return first, err
}

func (d *doctor) actor() {
	if d.env.Actor != "" {
		if !strings.HasPrefix(d.env.Actor, "agent:") && !strings.HasPrefix(d.env.Actor, "human:") {
			d.add("GLOSS_ACTOR", Warn, fmt.Sprintf("%q has no agent: or human: prefix; the remit gate only applies to agent: actors", d.env.Actor), "export GLOSS_ACTOR=agent:claude-code@model or human:alice")
			return
		}
		d.add("GLOSS_ACTOR", OK, d.env.Actor, "")
		return
	}
	def := ledger.DefaultActor(d.ws.Root)
	if def == "human:unknown" {
		d.add("GLOSS_ACTOR", Warn, "unset and git user.name is empty: new markers would be attributed to human:unknown", "git config user.name, or export GLOSS_ACTOR")
		return
	}
	d.add("GLOSS_ACTOR", Info, fmt.Sprintf("unset; new markers by a human are attributed to %s. Agents need it set (gloss hook install writes it into the MCP server and exports it to the agent's shell from the SessionStart hook)", def), "")
}

func (d *doctor) hook() {
	read := func(rel string) []byte {
		b, _ := os.ReadFile(filepath.Join(d.ws.Root, filepath.FromSlash(rel)))
		return b
	}
	hooks, bins := glossHooks(read(".claude/settings.json"))
	// When PATH does not find this gloss, gloss hook install writes the hook
	// with its absolute path to the uncommitted settings.local.json.
	local, _ := glossHooks(read(".claude/settings.local.json"))
	hooks = append(hooks, local...)
	srv, hasMCP := glossMCP(read(".mcp.json"))
	hasHook := len(hooks) > 0
	if !hasHook && !hasMCP {
		// A ready-made integration (gloss hook install --agent, #267) stands
		// in: the Claude Code plugin carries the hook and the server, the
		// Cursor rule and the Copilot instructions ask for the briefing
		// since neither agent has a SessionStart hook. Each gets its own
		// line from integrations().
		if names := d.installedIntegrations(); len(names) > 0 {
			d.add("agent hook", OK, "none in .claude/settings.json and .mcp.json; "+strings.Join(names, ", ")+" installed instead (checked below)", "")
			return
		}
	}
	switch {
	case !hasHook && !hasMCP:
		d.add("agent hook", Warn, "no SessionStart hook in .claude/settings.json and no MCP server in .mcp.json", "gloss hook install (--agent claude-code|cursor|copilot for a ready-made integration)")
	case !hasHook:
		d.add("agent hook", Warn, "MCP server registered but no SessionStart hook", "gloss hook install")
	case !hasMCP:
		d.add("agent hook", Warn, "SessionStart hook registered but no MCP server in .mcp.json", "gloss hook install")
	default:
		cmds := strings.Join(hooks, "\n")
		missing := []string{}
		if !strings.Contains(cmds, "GLOSS_ACTOR=") {
			missing = append(missing, "the hook command")
		}
		if _, ok := srv.Env["GLOSS_ACTOR"]; !ok {
			missing = append(missing, ".mcp.json env")
		}
		if len(missing) > 0 {
			d.add("agent hook", Warn, "installed, but GLOSS_ACTOR is missing from "+strings.Join(missing, " and ")+": links an agent writes are attributed to the git user", "gloss hook install (rewrites both with GLOSS_ACTOR=agent:claude-code)")
			return
		}
		// Committed files run gloss by name: a path to one machine's binary
		// fails silently on every other clone (issue #226).
		var abs []string
		for _, b := range bins {
			if absPath(b) {
				abs = append(abs, ".claude/settings.json runs "+b)
			}
		}
		if absPath(srv.Command) {
			abs = append(abs, ".mcp.json runs "+srv.Command)
		}
		if len(abs) > 0 {
			d.add("agent hook", Warn, strings.Join(abs, ", ")+": committed agent config names gloss by an absolute path, which a teammate's clone does not have", "gloss hook install (committed files then run gloss by name; a path this machine needs goes to the uncommitted .claude/settings.local.json)")
			return
		}
		if !strings.Contains(cmds, "CLAUDE_ENV_FILE") {
			d.add("agent hook", Warn, "the hook sets GLOSS_ACTOR for the briefing only: a gloss build the agent runs from its shell is attributed to the git user", "gloss hook install (the hook then exports GLOSS_ACTOR through $CLAUDE_ENV_FILE)")
			return
		}
		d.add("agent hook", OK, "SessionStart hook and MCP server registered with GLOSS_ACTOR", "")
	}
}

// hookBinRe finds the program a hook command runs `context` with: a word,
// or a single-quoted path with spaces.
var hookBinRe = regexp.MustCompile(`('[^']*'|[^\s';]+)\s+context\b`)

// glossHooks returns the SessionStart hook commands in a Claude Code settings
// file that run gloss context, and the gloss each one runs.
func glossHooks(b []byte) (cmds, bins []string) {
	var s struct {
		Hooks struct {
			SessionStart []struct {
				Hooks []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"SessionStart"`
		} `json:"hooks"`
	}
	json.Unmarshal(b, &s)
	for _, e := range s.Hooks.SessionStart {
		for _, h := range e.Hooks {
			if m := hookBinRe.FindStringSubmatch(h.Command); m != nil && isGloss(m[1]) {
				cmds = append(cmds, h.Command)
				bins = append(bins, strings.Trim(m[1], "'"))
			}
		}
	}
	return cmds, bins
}

// mcpServer is one entry of an mcpServers file.
type mcpServer struct {
	Command string         `json:"command"`
	Args    []string       `json:"args"`
	Env     map[string]any `json:"env"`
}

// glossMCP finds the server that runs gloss mcp in an mcpServers file: the
// one gloss hook install names glossmark, or any whose command is gloss by
// name or by path (issue #225), with mcp among its args.
func glossMCP(b []byte) (mcpServer, bool) {
	var c struct {
		Servers map[string]mcpServer `json:"mcpServers"`
	}
	json.Unmarshal(b, &c)
	names := make([]string, 0, len(c.Servers))
	for n := range c.Servers {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s := c.Servers[n]
		hasMCP := false
		for _, a := range s.Args {
			hasMCP = hasMCP || a == "mcp"
		}
		if hasMCP && (n == "glossmark" || isGloss(s.Command)) {
			return s, true
		}
	}
	return mcpServer{}, false
}

// isGloss reports whether a command names gloss, by name or by path.
func isGloss(cmd string) bool {
	cmd = strings.Trim(cmd, `'"`)
	if i := strings.LastIndexAny(cmd, `/\`); i >= 0 {
		cmd = cmd[i+1:]
	}
	return cmd == "gloss" || cmd == "gloss.exe"
}

// absPath reports whether a command is an absolute path (/usr/local/bin/gloss,
// ~/bin/gloss, C:\tools\gloss.exe) rather than a name PATH resolves.
func absPath(cmd string) bool {
	return strings.HasPrefix(cmd, "/") || strings.HasPrefix(cmd, "~") || len(cmd) > 2 && cmd[1] == ':' && (cmd[2] == '\\' || cmd[2] == '/')
}

func (d *doctor) graph() {
	path := d.ws.StatePath("graph.jsonl")
	rel := filepath.ToSlash(filepath.Join(load.StateDir, "graph.jsonl"))
	committed, err := os.ReadFile(path)
	if err != nil {
		d.add("graph", Fail, rel+" does not exist", "gloss build, then commit .glossmark/")
		return
	}
	same, err := d.g.Matches(committed)
	if err != nil {
		d.add("graph", Fail, "cannot rebuild the graph: "+err.Error(), "")
		return
	}
	if !same {
		d.add("graph", Fail, rel+" is stale: rebuilding from the tree gives a different graph", "gloss build (the pre-commit hook keeps it current: ln -s ../../hooks/pre-commit .git/hooks/pre-commit)")
		return
	}
	if d.git && !tracked(d.ws.Root, rel) {
		d.add("graph", Fail, rel+" is current but not in git", "git add "+rel)
		return
	}
	d.add("graph", OK, rel+" matches the tree", "")
}

func (d *doctor) ledger() {
	rel := filepath.ToSlash(filepath.Join(load.StateDir, ledger.FileName))
	if _, err := os.Stat(d.ws.StatePath(ledger.FileName)); err != nil {
		d.add("ledger", Warn, rel+" does not exist yet", "gloss build writes it; commit it: it is the review record (who wrote each link, what was accepted or rejected)")
		return
	}
	if !d.git {
		d.add("ledger", OK, rel+" exists (not a git repository, cannot check tracking)", "")
		return
	}
	if ignored(d.ws.Root, rel) {
		d.add("ledger", Fail, rel+" is git-ignored: accepted, rejected and stale decisions evaporate on every clone and CI run", "remove it from .gitignore and git add "+rel)
		return
	}
	if !tracked(d.ws.Root, rel) {
		d.add("ledger", Fail, rel+" is not in git: review decisions are lost on clone", "git add "+rel)
		return
	}
	d.add("ledger", OK, rel+" is tracked in git", "")
}

func (d *doctor) lock() {
	if len(d.ws.Resolved) == 0 {
		d.add("lockfile", OK, "no imports, nothing to lock", "")
		return
	}
	if d.ws.Lock == nil {
		d.add("lockfile", Fail, fmt.Sprintf("%d import(s) but no %s: upstream changes arrive unreviewed", len(d.ws.Resolved), load.LockName), "gloss lock, then commit "+load.LockName)
		return
	}
	for _, g := range d.diags {
		if g.Code == "GM107" {
			d.add("lockfile", Warn, g.Msg, "gloss lock after reviewing the upstream change")
			return
		}
		if g.Code == "GM116" {
			d.add("lockfile", Warn, g.Msg, "gloss lock --update after reviewing the upstream change")
			return
		}
	}
	d.add("lockfile", OK, load.LockName+" pins every import", "")
}

var (
	usesRe = regexp.MustCompile(`(?im)^\s*-?\s*uses:\s*["']?mckalvan/glossmark@([^\s"'#]+)`)
	revRe  = regexp.MustCompile(`(?is)repo:\s*https://github\.com/mckalvan/glossmark[^\n]*\n\s*rev:\s*["']?([^\s"'#]+)`)
	shaRe  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	// A root: taken from a matrix the workflow computes (include: ${{
	// fromJSON(...) }}) names no workspace in the file; it may be this one.
	matrixRootRe = regexp.MustCompile(`(?i)root:\s*["']?\$\{\{\s*matrix\.`)
	fromJSONRe   = regexp.MustCompile(`(?i)matrix:[^\n]*\n?[^\n]*\$\{\{\s*fromjson\(`)
)

// pinned reports whether a ref is a release tag or a full commit sha.
func pinned(ref string) bool {
	return strings.HasPrefix(ref, "v") || shaRe.MatchString(ref)
}

// GitRoot returns the top level of the git repository holding dir, or "".
func GitRoot(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return filepath.Clean(strings.TrimSpace(string(out)))
}

func (d *doctor) pins() {
	var found, unpinned []string
	// GitHub runs workflows only at the git root; a workspace below it is
	// covered by a workflow there that runs the Action with its root:.
	scan := func(base, wsRel string) {
		matches, _ := filepath.Glob(filepath.Join(base, ".github", "workflows", "*.y*ml"))
		for _, f := range matches {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			computed := false
			if wsRel != "" && !mentionsRoot(string(b), wsRel) {
				if computed = matrixRootRe.Match(b) && fromJSONRe.Match(b); !computed {
					continue
				}
			}
			for _, m := range usesRe.FindAllStringSubmatch(string(b), -1) {
				rel, _ := filepath.Rel(d.ws.Root, f)
				if computed {
					found = append(found, filepath.ToSlash(rel)+" (root: from a computed matrix, not checked against this workspace)")
				} else {
					found = append(found, filepath.ToSlash(rel))
				}
				if !pinned(m[1]) {
					unpinned = append(unpinned, fmt.Sprintf("%s uses @%s", filepath.ToSlash(rel), m[1]))
				}
			}
		}
		if b, err := os.ReadFile(filepath.Join(base, ".pre-commit-config.yaml")); err == nil {
			if m := revRe.FindStringSubmatch(strings.ToLower(string(b))); m != nil {
				rel, _ := filepath.Rel(d.ws.Root, filepath.Join(base, ".pre-commit-config.yaml"))
				found = append(found, filepath.ToSlash(rel))
				if !pinned(m[1]) {
					unpinned = append(unpinned, filepath.ToSlash(rel)+" rev: "+m[1])
				}
			}
		}
	}
	scan(d.ws.Root, "")
	gitRoot := ""
	if d.git {
		root, _ := filepath.EvalSymlinks(d.ws.Root)
		if g := GitRoot(d.ws.Root); g != "" && g != filepath.Clean(root) && g != filepath.Clean(d.ws.Root) {
			gitRoot = g
			if rel, err := filepath.Rel(g, root); err == nil {
				scan(g, filepath.ToSlash(rel))
			}
		}
	}
	switch {
	case len(found) == 0 && gitRoot != "":
		d.add("CI pins", Warn, "no GitHub workflow at the git root ("+gitRoot+") or in this workspace references GlossMark for this workspace", "add the Action pinned to a release to a workflow at the git root, with root: set to this workspace: uses: mckalvan/glossmark@v0.3.0")
	case len(found) == 0:
		d.add("CI pins", Warn, "no GitHub workflow or pre-commit config references GlossMark", "add the Action (docs/QUICKSTART.md, section 7) pinned to a release: uses: mckalvan/glossmark@v0.3.0")
	case len(unpinned) > 0:
		d.add("CI pins", Fail, strings.Join(unpinned, "; ")+": an unpinned ref builds from source on every run and changes under you", "pin a release tag or a full commit sha, e.g. @v0.3.0 / rev: v0.3.0")
	default:
		sort.Strings(found)
		d.add("CI pins", OK, "release pinned in "+strings.Join(uniq(found), ", "), "")
	}
}

func (d *doctor) docs() {
	n := 0
	for _, g := range d.diags {
		if g.Code == "GM304" || g.Code == "GM305" {
			n++
		}
	}
	errs := 0
	for _, g := range d.diags {
		if g.Severity == "error" && g.Code != "GM305" {
			errs++
		}
	}
	switch {
	case n > 0:
		d.add("ontology docs", Fail, fmt.Sprintf("%d concept(s) have a doc cut at a comma or a null key (GM304/GM305); the truncated text is what agents read", n), "quote the docs or write the entries in block style; gloss check lists them")
	case errs > 0:
		d.add("ontology docs", Warn, fmt.Sprintf("docs intact, but gloss check reports %d error(s)", errs), "gloss check")
	default:
		d.add("ontology docs", OK, "no truncated docs or null keys; gloss check reports no errors", "")
	}
}

// mentionsRoot reports whether a workflow at the git root names the
// workspace at rel (in a root: input, a matrix entry or a working
// directory) as a whole path: "showcase/clinic-booking" or
// "./showcase/clinic-booking/", not "showcase/clinic-booking-v2".
func mentionsRoot(text, rel string) bool {
	rel = strings.Trim(rel, "/")
	at := func(i int) byte {
		if i < 0 || i >= len(text) {
			return ' '
		}
		return text[i]
	}
	for i := 0; ; {
		j := strings.Index(text[i:], rel)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(rel)
		i = end
		before, after := at(start-1), at(end)
		if before == '/' && at(start-2) == '.' && !pathChar(at(start-3)) && at(start-3) != '/' {
			before = ' ' // ./showcase/...
		}
		if after == '/' && !pathChar(at(end+1)) {
			after = ' ' // showcase/clinic-booking/
		}
		if !pathChar(before) && before != '/' && !pathChar(after) && after != '/' {
			return true
		}
	}
}

func pathChar(c byte) bool {
	return c == '-' || c == '_' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func tracked(root, rel string) bool {
	return exec.Command("git", "-C", root, "ls-files", "--error-unmatch", "--", rel).Run() == nil
}

func ignored(root, rel string) bool {
	return exec.Command("git", "-C", root, "check-ignore", "-q", "--", rel).Run() == nil
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// Ready-made agent integrations (issue #267). Each is a set of committed
// files gloss hook install --agent writes; doctor reads them the way the
// agent does and reports one line per integration it finds, none for one
// that is not installed. Committed files must run gloss by name (#226).
const (
	pluginDir       = ".claude/plugins/glossmark"
	marketplacePath = ".claude-plugin/marketplace.json"
	cursorRulePath  = ".cursor/rules/glossmark.mdc"
	cursorMCPPath   = ".cursor/mcp.json"
	copilotPath     = ".github/copilot-instructions.md"
	vscodeMCPPath   = ".vscode/mcp.json"
)

// skillNames are the skills the plugin must carry.
var skillNames = []string{"adopt-glossmark", "derive-ontology", "review-glossmark", "curate-increment"}

func (d *doctor) read(rel string) []byte {
	b, _ := os.ReadFile(filepath.Join(d.ws.Root, filepath.FromSlash(rel)))
	return b
}

func (d *doctor) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(d.ws.Root, filepath.FromSlash(rel)))
	return err == nil
}

// installedIntegrations names the integrations whose main file exists.
func (d *doctor) installedIntegrations() []string {
	var out []string
	if d.exists(pluginDir + "/.claude-plugin/plugin.json") {
		out = append(out, "the Claude Code plugin")
	}
	if d.exists(cursorRulePath) {
		out = append(out, "the Cursor rule")
	}
	if bytes.Contains(d.read(copilotPath), []byte("## GlossMark")) {
		out = append(out, "the Copilot instructions")
	}
	return out
}

func (d *doctor) integrations() {
	d.claudePlugin()
	d.cursor()
	d.copilot()
}

// claudePlugin checks the plugin as Claude Code loads it: a manifest, the
// four skills, a SessionStart hook in hooks/hooks.json and the MCP server
// in .mcp.json, both with GLOSS_ACTOR and running gloss by name, and the
// marketplace entry that installs it.
func (d *doctor) claudePlugin() {
	manifest := pluginDir + "/.claude-plugin/plugin.json"
	if !d.exists(manifest) {
		return
	}
	fix := "gloss hook install --agent claude-code"
	var m struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(d.read(manifest), &m); err != nil || m.Name == "" {
		d.add("claude-code plugin", Warn, manifest+" is not a plugin manifest with a name", fix)
		return
	}
	var problems []string
	var missing []string
	for _, s := range skillNames {
		if !d.exists(pluginDir + "/skills/" + s + "/SKILL.md") {
			missing = append(missing, s)
		}
	}
	if len(missing) > 0 {
		problems = append(problems, "skills missing: "+strings.Join(missing, ", "))
	}
	cmds, bins := glossHooks(d.read(pluginDir + "/hooks/hooks.json"))
	switch {
	case len(cmds) == 0:
		problems = append(problems, "no SessionStart hook running gloss context in hooks/hooks.json")
	case !strings.Contains(strings.Join(cmds, "\n"), "GLOSS_ACTOR="):
		problems = append(problems, "the hook sets no GLOSS_ACTOR")
	default:
		for _, b := range bins {
			if absPath(b) {
				problems = append(problems, "hooks/hooks.json runs "+b+" (an absolute path; the plugin is installed on every teammate's machine)")
			}
		}
	}
	srv, ok := glossMCP(d.read(pluginDir + "/.mcp.json"))
	switch {
	case !ok:
		problems = append(problems, "no MCP server running gloss mcp in .mcp.json")
	case absPath(srv.Command):
		problems = append(problems, ".mcp.json runs "+srv.Command+" (an absolute path)")
	default:
		if _, ok := srv.Env["GLOSS_ACTOR"]; !ok {
			problems = append(problems, "the MCP server sets no GLOSS_ACTOR")
		}
	}
	market, listed := marketplaceLists(d.read(marketplacePath), m.Name)
	if !listed {
		problems = append(problems, marketplacePath+" does not list the plugin, so claude plugin install cannot find it")
	}
	if len(problems) > 0 {
		d.add("claude-code plugin", Warn, pluginDir+": "+strings.Join(problems, "; "), fix)
		return
	}
	d.add("claude-code plugin", OK, fmt.Sprintf("%s has the skills, the SessionStart hook and the MCP server, all running gloss by name with GLOSS_ACTOR; install with claude plugin install %s@%s --scope project", pluginDir, m.Name, market), "")
}

// marketplaceLists reports whether a marketplace file lists a plugin by
// name, and the marketplace's name.
func marketplaceLists(b []byte, plugin string) (string, bool) {
	var m struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name string `json:"name"`
		} `json:"plugins"`
	}
	json.Unmarshal(b, &m)
	for _, p := range m.Plugins {
		if p.Name == plugin {
			return m.Name, true
		}
	}
	return m.Name, false
}

// briefingRule reports whether a rules or instructions file asks the agent
// for gloss context or the MCP server, as the generated ones do.
func briefingRule(b []byte) bool {
	return bytes.Contains(b, []byte("gloss context")) || bytes.Contains(b, []byte(" context --dir")) || bytes.Contains(b, []byte("concepts_for_path"))
}

// mcpProblems checks an MCP config file written for an agent: the server
// is there, runs gloss by name and carries GLOSS_ACTOR. key is the map the
// file keeps its servers under (mcpServers, or servers for VS Code).
func mcpProblems(b []byte, rel, key string) []string {
	var raw map[string]json.RawMessage
	json.Unmarshal(b, &raw)
	wrapped, _ := json.Marshal(map[string]json.RawMessage{"mcpServers": raw[key]})
	srv, ok := glossMCP(wrapped)
	switch {
	case !ok:
		return []string{"no MCP server running gloss mcp in " + rel}
	case absPath(srv.Command):
		return []string{rel + " runs " + srv.Command + ": committed agent config names gloss by an absolute path, which a teammate's clone does not have"}
	}
	if _, ok := srv.Env["GLOSS_ACTOR"]; !ok {
		return []string{"the MCP server in " + rel + " sets no GLOSS_ACTOR: links the agent writes are attributed to the git user"}
	}
	return nil
}

// cursor checks the always-applied rule and .cursor/mcp.json. Cursor has
// no SessionStart hook, so the rule must ask for the briefing itself.
func (d *doctor) cursor() {
	rule := d.read(cursorRulePath)
	if len(rule) == 0 {
		return
	}
	fix := "gloss hook install --agent cursor"
	var problems []string
	if !bytes.Contains(rule, []byte("alwaysApply: true")) {
		problems = append(problems, cursorRulePath+" is not always applied (alwaysApply: true)")
	}
	if !briefingRule(rule) {
		problems = append(problems, cursorRulePath+" does not ask for gloss context")
	}
	if m := hookBinRe.FindSubmatch(rule); m != nil && absPath(strings.Trim(string(m[1]), "'`")) {
		problems = append(problems, cursorRulePath+" runs "+strings.Trim(string(m[1]), "'`")+" (an absolute path)")
	}
	problems = append(problems, mcpProblems(d.read(cursorMCPPath), cursorMCPPath, "mcpServers")...)
	if len(problems) > 0 {
		d.add("cursor", Warn, strings.Join(problems, "; "), fix)
		return
	}
	d.add("cursor", OK, "rule always applied and MCP server registered in "+cursorMCPPath+" with GLOSS_ACTOR, running gloss by name", "")
}

// copilot checks the GlossMark section of the Copilot instructions and the
// VS Code MCP config (servers, not mcpServers).
func (d *doctor) copilot() {
	instr := d.read(copilotPath)
	if !bytes.Contains(instr, []byte("## GlossMark")) {
		return
	}
	fix := "gloss hook install --agent copilot"
	var problems []string
	if !briefingRule(instr) {
		problems = append(problems, copilotPath+" does not ask for gloss context or concepts_for_path")
	}
	problems = append(problems, mcpProblems(d.read(vscodeMCPPath), vscodeMCPPath, "servers")...)
	if len(problems) > 0 {
		d.add("copilot", Warn, strings.Join(problems, "; "), fix)
		return
	}
	d.add("copilot", OK, "GlossMark section in "+copilotPath+" and MCP server registered in "+vscodeMCPPath+" with GLOSS_ACTOR, running gloss by name", "")
}
