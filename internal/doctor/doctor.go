// @gloss implements gmi:Doctor scope=file

// Package doctor checks that a repository's GlossMark setup works end to end:
// the hook can find gloss, agents are attributed, the committed graph matches
// the tree, the review record is in git, imports are locked, CI pins a
// release, ontology docs are intact, and AGENTS.md carries the current rules
// block. Each check is one line with a fix.
package doctor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/agents"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/version"
)

// Status of one check.
const (
	OK   = "ok"
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
}

// Run performs every check. diags are the build and lint diagnostics of the
// current tree.
func Run(ws *load.Workspace, g *graph.Graph, diags []marker.Diagnostic, env Env) []Check {
	if env.LookPath == nil {
		env.LookPath = exec.LookPath
	}
	d := &doctor{ws: ws, g: g, diags: diags, env: env, git: isGitRepo(ws.Root)}
	d.binary()
	d.actor()
	d.hook()
	d.rules()
	d.graph()
	d.ledger()
	d.lock()
	d.pins()
	d.docs()
	return d.out
}

// String renders the report, one line per check.
func String(checks []Check) string {
	var sb strings.Builder
	for _, c := range checks {
		glyph := map[string]string{OK: "✓", Warn: "!", Fail: "✗"}[c.Status]
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
			"curl -fsSL https://raw.githubusercontent.com/mckalvan/glossmark/main/install.sh | sh, or go install github.com/mckalvan/glossmark/cmd/gloss@v0.1.0, then add the directory to PATH")
		return
	}
	d.add("gloss on PATH", OK, fmt.Sprintf("%s (this build is %s)", p, version.String()), "")
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
	d.add("GLOSS_ACTOR", OK, fmt.Sprintf("unset; new markers by a human are attributed to %s. Agents need it set (gloss hook install writes it into the hook and the MCP server)", def), "")
}

func (d *doctor) hook() {
	settings, _ := os.ReadFile(filepath.Join(d.ws.Root, ".claude", "settings.json"))
	mcp, _ := os.ReadFile(filepath.Join(d.ws.Root, ".mcp.json"))
	hasHook := bytes.Contains(settings, []byte("gloss context"))
	hasMCP := bytes.Contains(mcp, []byte(`"gloss"`))
	switch {
	case !hasHook && !hasMCP:
		d.add("agent hook", Warn, "no SessionStart hook in .claude/settings.json and no MCP server in .mcp.json", "gloss hook install")
	case !hasHook:
		d.add("agent hook", Warn, "MCP server registered but no SessionStart hook", "gloss hook install")
	case !hasMCP:
		d.add("agent hook", Warn, "SessionStart hook registered but no MCP server in .mcp.json", "gloss hook install")
	default:
		missing := []string{}
		if !bytes.Contains(settings, []byte("GLOSS_ACTOR=")) {
			missing = append(missing, "the hook command")
		}
		if !bytes.Contains(mcp, []byte("GLOSS_ACTOR")) {
			missing = append(missing, ".mcp.json env")
		}
		if len(missing) > 0 {
			d.add("agent hook", Warn, "installed, but GLOSS_ACTOR is missing from "+strings.Join(missing, " and ")+": links an agent writes are attributed to the git user", "gloss hook install (rewrites both with GLOSS_ACTOR=agent:claude-code)")
			return
		}
		d.add("agent hook", OK, "SessionStart hook and MCP server registered with GLOSS_ACTOR", "")
	}
}

func (d *doctor) rules() {
	const fix = "gloss agents --rules --write AGENTS.md"
	am, err := os.ReadFile(filepath.Join(d.ws.Root, "AGENTS.md"))
	if err != nil {
		d.add("agent rules", Warn, "no AGENTS.md: agents get no GlossMark rules unless a hook briefs them", fix)
		return
	}
	switch st, err := agents.CheckRules(string(am)); {
	case err != nil:
		d.add("agent rules", Fail, "AGENTS.md: "+err.Error(), "")
		return
	case st == agents.RulesMissing:
		d.add("agent rules", Warn, "AGENTS.md has no GlossMark rules block", fix)
		return
	case st == agents.RulesStale:
		d.add("agent rules", Warn, "the rules block in AGENTS.md differs from what this build renders", fix)
		return
	}
	if cm, err := os.ReadFile(filepath.Join(d.ws.Root, "CLAUDE.md")); err == nil && !agents.HasClaudeImport(string(cm)) {
		d.add("agent rules", Warn, "AGENTS.md is current, but CLAUDE.md does not import it: Claude Code reads AGENTS.md on its own only when there is no CLAUDE.md", "add a line "+agents.ClaudeImport+" to CLAUDE.md")
		return
	}
	d.add("agent rules", OK, "AGENTS.md carries the current rules block", "")
}

func (d *doctor) graph() {
	path := d.ws.StatePath("graph.jsonl")
	rel := filepath.ToSlash(filepath.Join(load.StateDir, "graph.jsonl"))
	committed, err := os.ReadFile(path)
	if err != nil {
		d.add("graph", Fail, rel+" does not exist", "gloss build, then commit .glossmark/")
		return
	}
	var buf bytes.Buffer
	if err := d.g.WriteJSONL(&buf); err != nil {
		d.add("graph", Fail, "cannot rebuild the graph: "+err.Error(), "")
		return
	}
	// The header line names the generator; a graph committed by one release
	// and checked by another is still current when every record matches.
	if !bytes.Equal(graphBody(committed), graphBody(buf.Bytes())) {
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
	}
	d.add("lockfile", OK, load.LockName+" pins every import", "")
}

var (
	usesRe = regexp.MustCompile(`(?im)^\s*-?\s*uses:\s*["']?mckalvan/glossmark@([^\s"'#]+)`)
	revRe  = regexp.MustCompile(`(?is)repo:\s*https://github\.com/mckalvan/glossmark[^\n]*\n\s*rev:\s*["']?([^\s"'#]+)`)
)

func (d *doctor) pins() {
	var found, unpinned []string
	matches, _ := filepath.Glob(filepath.Join(d.ws.Root, ".github", "workflows", "*.y*ml"))
	for _, f := range matches {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, m := range usesRe.FindAllStringSubmatch(string(b), -1) {
			rel, _ := filepath.Rel(d.ws.Root, f)
			found = append(found, filepath.ToSlash(rel))
			if !strings.HasPrefix(m[1], "v") {
				unpinned = append(unpinned, fmt.Sprintf("%s uses @%s", filepath.ToSlash(rel), m[1]))
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(d.ws.Root, ".pre-commit-config.yaml")); err == nil {
		if m := revRe.FindStringSubmatch(strings.ToLower(string(b))); m != nil {
			found = append(found, ".pre-commit-config.yaml")
			if !strings.HasPrefix(m[1], "v") {
				unpinned = append(unpinned, ".pre-commit-config.yaml rev: "+m[1])
			}
		}
	}
	switch {
	case len(found) == 0:
		d.add("CI pins", Warn, "no GitHub workflow or pre-commit config references GlossMark", "add the Action (docs/QUICKSTART.md, section 7) pinned to a release: uses: mckalvan/glossmark@v0.1.0")
	case len(unpinned) > 0:
		d.add("CI pins", Fail, strings.Join(unpinned, "; ")+": an unpinned ref builds from source on every run and changes under you", "pin a release tag, e.g. @v0.1.0 / rev: v0.1.0")
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
