// @gloss verifies gmi:HookGuard scope=file

package e2e

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	glossOnce sync.Once
	glossPath string
	glossErr  error
)

// glossBinary builds cmd/gloss once for the tests that drive it as a
// harness would: a hook payload on stdin, JSON on stdout.
func glossBinary(t *testing.T) string {
	t.Helper()
	glossOnce.Do(func() {
		dir, err := os.MkdirTemp("", "gloss-e2e-")
		if err != nil {
			glossErr = err
			return
		}
		goBin, err := exec.LookPath("go")
		if err != nil {
			goBin = filepath.Join(runtime.GOROOT(), "bin", "go")
		}
		glossPath = filepath.Join(dir, "gloss")
		cmd := exec.Command(goBin, "build", "-o", glossPath, "github.com/mckalvan/glossmark/cmd/gloss")
		if out, err := cmd.CombinedOutput(); err != nil {
			glossErr = err
			glossPath = string(out)
		}
	})
	if glossErr != nil {
		t.Fatalf("go build ./cmd/gloss: %v\n%s", glossErr, glossPath)
	}
	return glossPath
}

type run struct {
	stdout, stderr string
	code           int
}

// gloss runs the binary in dir with stdin and extra environment.
func gloss(t *testing.T, dir, stdin string, env []string, args ...string) run {
	t.Helper()
	cmd := exec.Command(glossBinary(t), args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "GLOSS_ACTOR="), env...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return run{out.String(), errb.String(), code}
}

// guardOut decodes the guard's stdout; an empty stdout decodes to nil.
func guardOut(t *testing.T, r run) map[string]any {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("gloss hook guard exited %d: %s", r.code, r.stderr)
	}
	if strings.TrimSpace(r.stdout) == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &m); err != nil {
		t.Fatalf("guard output is not JSON: %v\n%s", err, r.stdout)
	}
	return m
}

func specific(m map[string]any) map[string]any {
	s, _ := m["hookSpecificOutput"].(map[string]any)
	return s
}

func editPayload(path, agentType string) string {
	b, _ := json.Marshal(map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Edit", "tool_input": map[string]any{"file_path": path}, "agent_type": agentType})
	return string(b)
}

func paymentsCopy(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	copyTree(t, example(t, "payments/payments-api"), root)
	return root
}

func appendFile(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString(s)
}

func writeFile(t *testing.T, path, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestGuardPreEdit: before an edit the guard names a critical file's level
// and its source, warns a subagent outside its may-modify remit, asks only
// when policy.guard-ask says so, and says nothing about an unremarkable file.
func TestGuardPreEdit(t *testing.T) {
	root := paymentsCopy(t)
	pre := func(path, agent string, env ...string) map[string]any {
		return guardOut(t, gloss(t, root, editPayload(path, agent), env, "hook", "guard", "--event", "pre-edit"))
	}

	m := pre("services/api/handlers/authorize.py", "")
	s := specific(m)
	if s == nil || s["hookEventName"] != "PreToolUse" || !strings.Contains(s["additionalContext"].(string), "criticality critical (via pa:AuthorizeHandler)") {
		t.Fatalf("critical file: want its level and source, got %v", m)
	}
	if _, ok := s["permissionDecision"]; ok {
		t.Errorf("no guard-ask policy: the guard must not ask, got %v", s)
	}

	// A subagent named refund-engineer has .claude/agents/refund-engineer.md
	// with may-modify pay:Refund, pay:Refunding, pa:*; the migration represents pay:Payment.
	s = specific(pre(filepath.Join(root, "migrations", "0042_payments.sql"), "refund-engineer"))
	ctx, _ := s["additionalContext"].(string)
	if !strings.Contains(ctx, "outside agent:refund-engineer's may-modify remit") || !strings.Contains(ctx, "pay:Payment") {
		t.Errorf("out-of-remit edit should be reported, got %q", ctx)
	}
	if _, ok := s["permissionDecision"]; ok {
		t.Errorf("out of remit warns by default, it must not ask: %v", s)
	}
	// The same remit applies when the session actor names the agent.
	if s := specific(pre("migrations/0042_payments.sql", "", "GLOSS_ACTOR=agent:refund-engineer")); s == nil || !strings.Contains(s["additionalContext"].(string), "outside agent:refund-engineer's") {
		t.Errorf("GLOSS_ACTOR=agent:refund-engineer should apply the remit, got %v", s)
	}
	// Without an agent file the subagent name is not an actor: no remit line.
	if s := specific(pre("migrations/0042_payments.sql", "nobody")); s == nil || strings.Contains(s["additionalContext"].(string), "remit") {
		t.Errorf("an agent_type without an agent file has no remit, got %v", s)
	}
	// Inside the remit, unclassified, off the seam: nothing to say.
	if m := pre("services/api/handlers/refund.py", "refund-engineer"); m != nil {
		t.Errorf("refund.py for refund-engineer is unremarkable, got %v", m)
	}
	// A file outside the repository is not the guard's business.
	if m := pre(filepath.Join(t.TempDir(), "x.py"), ""); m != nil {
		t.Errorf("file outside the repository: want no output, got %v", m)
	}

	appendFile(t, filepath.Join(root, "glossmark.yaml"), "policy:\n  guard-ask: [remit]\n")
	s = specific(pre("migrations/0042_payments.sql", "refund-engineer"))
	if s["permissionDecision"] != "ask" || !strings.Contains(s["permissionDecisionReason"].(string), "remit") {
		t.Errorf("guard-ask: [remit] should ask outside the remit, got %v", s)
	}
	if s := specific(pre("services/api/handlers/authorize.py", "")); s["permissionDecision"] != nil {
		t.Errorf("guard-ask: [remit] must not ask for a critical file in no agent's remit, got %v", s)
	}

	writeFile(t, filepath.Join(root, "glossmark.yaml"), strings.Replace(readFile(t, filepath.Join(root, "glossmark.yaml")), "guard-ask: [remit]", "guard-ask: [critical]", 1))
	if s := specific(pre("services/api/handlers/authorize.py", "")); s["permissionDecision"] != "ask" || !strings.Contains(s["permissionDecisionReason"].(string), "critical") {
		t.Errorf("guard-ask: [critical] should ask for a critical file, got %v", s)
	}

	// The published seam: metrics.py implements a measure published as zoo:ProductionCapacity.
	hf := example(t, "horse-factory")
	s = specific(guardOut(t, gloss(t, hf, editPayload("services/horse-factory/breeding/metrics.py", ""), nil, "hook", "guard", "--event", "pre-edit")))
	if ctx, _ := s["additionalContext"].(string); !strings.Contains(ctx, "on the published seam: impl:FoalingPipelineThroughput (published as zoo:ProductionCapacity).") {
		t.Errorf("published-seam concepts should be named once, got %q", ctx)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestGuardPostEdit: after an edit the guard reports the lint findings that
// point at that file, and nothing for a clean one.
func TestGuardPostEdit(t *testing.T) {
	root := paymentsCopy(t)
	post := func(path string) map[string]any {
		return guardOut(t, gloss(t, root, editPayload(path, ""), nil, "hook", "guard", "--event", "post-edit"))
	}
	if m := post("services/api/handlers/refund.py"); m != nil {
		t.Errorf("clean file: want no output, got %v", m)
	}
	// pay:Settlement is outside services/api's allowed list (GM502), and the
	// same marker on two consecutive functions is GM131.
	writeFile(t, filepath.Join(root, "services/api/handlers/settle.py"),
		"# @gloss implements pay:Settlement\ndef settle():\n    pass\n\n# @gloss implements pa:CaptureHandler\ndef a():\n    pass\n\n# @gloss implements pa:CaptureHandler\ndef b():\n    pass\n\n# @gloss implements pa:CaptureHandler\ndef c():\n    pass\n")
	s := specific(post(filepath.Join(root, "services/api/handlers/settle.py")))
	ctx, _ := s["additionalContext"].(string)
	if s["hookEventName"] != "PostToolUse" || !strings.Contains(ctx, "GM502") || !strings.Contains(ctx, "pay:Settlement") {
		t.Errorf("post-edit should report GM502 for settle.py, got %v", s)
	}
	if !strings.Contains(ctx, "GM131") {
		t.Errorf("post-edit should report GM131 for the repeated marker, got %q", ctx)
	}
	if strings.Contains(ctx, "refund.py") {
		t.Errorf("post-edit reports only the edited file, got %q", ctx)
	}
}

// TestGuardStop: the Stop guard blocks on a check error and on a stale graph,
// names the fix, and stays quiet when Claude Code is already continuing from a
// Stop hook or the policy says report.
func TestGuardStop(t *testing.T) {
	root := paymentsCopy(t)
	stop := func(payload string) run {
		return gloss(t, root, payload, nil, "hook", "guard", "--event", "stop")
	}
	if r := gloss(t, root, "", nil, "build", "-q"); r.code != 0 {
		t.Fatalf("gloss build: %d %s", r.code, r.stderr)
	}
	if m := guardOut(t, stop(`{"hook_event_name":"Stop","stop_hook_active":false}`)); m != nil {
		t.Fatalf("clean tree, current graph: want no output, got %v", m)
	}

	// A stale graph alone blocks, with the build fix and no check complaint.
	writeFile(t, filepath.Join(root, "services/api/handlers/void.py"), "# @gloss implements pa:CaptureHandler\ndef void():\n    pass\n")
	m := guardOut(t, stop(`{"stop_hook_active":false}`))
	reason, _ := m["reason"].(string)
	if m["decision"] != "block" || !strings.Contains(reason, "stale") || !strings.Contains(reason, "gloss build -q") || strings.Contains(reason, "gloss check reports") {
		t.Errorf("stale graph should block with gloss build -q, got %v", m)
	}
	if !strings.Contains(reason, "proposal") {
		t.Errorf("a blocking reason mentions the open proposals (refund.py has one), got %q", reason)
	}

	// A check error blocks too.
	writeFile(t, filepath.Join(root, "services/api/handlers/void.py"), "# @gloss implements pay:DoesNotExist\ndef void():\n    pass\n")
	m = guardOut(t, stop(`{}`))
	reason, _ = m["reason"].(string)
	if m["decision"] != "block" || !strings.Contains(reason, "gloss check reports") || !strings.Contains(reason, "void.py") {
		t.Errorf("check error should block and name it, got %v", m)
	}

	// Never block twice in a row.
	if m := guardOut(t, stop(`{"stop_hook_active":true}`)); m != nil {
		t.Errorf("stop_hook_active: want no output, got %v", m)
	}

	// guard-stop: report prints the reason and lets the agent stop.
	appendFile(t, filepath.Join(root, "glossmark.yaml"), "policy:\n  guard-stop: report\n")
	r := stop(`{}`)
	if m := guardOut(t, r); m != nil || !strings.Contains(r.stderr, "gloss check reports") {
		t.Errorf("guard-stop: report must not block and should print the reason on stderr, got %v / %q", m, r.stderr)
	}
}

// TestGuardFailsOpen: unreadable payloads, a directory without glossmark.yaml
// and a broken config print nothing and exit 0.
func TestGuardFailsOpen(t *testing.T) {
	root := paymentsCopy(t)
	for _, ev := range []string{"pre-edit", "post-edit", "stop"} {
		for _, in := range []string{"", "not json", `{"tool_input":"x"}`} {
			if m := guardOut(t, gloss(t, root, in, nil, "hook", "guard", "--event", ev)); m != nil {
				t.Errorf("%s with %q: want no output, got %v", ev, in, m)
			}
		}
		if m := guardOut(t, gloss(t, t.TempDir(), editPayload("x.py", ""), nil, "hook", "guard", "--event", ev)); m != nil {
			t.Errorf("%s without glossmark.yaml: want no output, got %v", ev, m)
		}
	}
	broken := paymentsCopy(t)
	appendFile(t, filepath.Join(broken, "glossmark.yaml"), "policy:\n  guard-stop: sometimes\n")
	if m := guardOut(t, gloss(t, broken, `{}`, nil, "hook", "guard", "--event", "stop")); m != nil {
		t.Errorf("invalid config: want no output, got %v", m)
	}
}

// TestHookInstallGuards: --guards registers one entry per event, re-running
// replaces rather than duplicates, other hooks survive, and --skills installs
// named skills only.
func TestHookInstallGuards(t *testing.T) {
	root := paymentsCopy(t)
	settings := filepath.Join(root, ".claude", "settings.json")
	writeFile(t, settings, `{"permissions":{"allow":["Bash(ls)"]},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo pre"}]}],"Stop":[{"hooks":[{"type":"command","command":"echo done"}]}]}}`)
	for i := 0; i < 2; i++ {
		if r := gloss(t, root, "", nil, "hook", "install", "--guards", "--no-mcp", "--actor", "agent:tester"); r.code != 0 {
			t.Fatalf("hook install --guards: %d %s", r.code, r.stderr)
		}
	}
	var cfg struct {
		Permissions map[string]any `json:"permissions"`
		Hooks       map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(readFile(t, settings)), &cfg); err != nil {
		t.Fatal(err)
	}
	count := func(event, sub string) (n int, matcher string) {
		for _, g := range cfg.Hooks[event] {
			for _, h := range g.Hooks {
				if strings.Contains(h.Command, sub) {
					n++
					matcher = g.Matcher
				}
			}
		}
		return
	}
	want := map[string]string{
		"PreToolUse":  "GLOSS_ACTOR=agent:tester gloss hook guard --event pre-edit 2>/dev/null || true",
		"PostToolUse": "GLOSS_ACTOR=agent:tester gloss hook guard --event post-edit 2>/dev/null || true",
		"Stop":        "GLOSS_ACTOR=agent:tester gloss hook guard --event stop",
	}
	for event, cmd := range want {
		n, matcher := count(event, "gloss hook guard")
		if exact, _ := count(event, cmd); n != 1 || exact != 1 {
			t.Errorf("%s: want exactly one guard entry %q, got %d (%d exact)", event, cmd, n, exact)
		}
		if event != "Stop" && matcher != "Edit|Write" {
			t.Errorf("%s matcher: want Edit|Write, got %q", event, matcher)
		}
	}
	if n, m := count("PreToolUse", "echo pre"); n != 1 || m != "Bash" {
		t.Errorf("unrelated PreToolUse hook lost: %+v", cfg.Hooks["PreToolUse"])
	}
	if n, _ := count("Stop", "echo done"); n != 1 {
		t.Errorf("unrelated Stop hook lost: %+v", cfg.Hooks["Stop"])
	}
	if n, _ := count("SessionStart", "gloss context"); n != 1 {
		t.Errorf("SessionStart briefing: want one entry, got %+v", cfg.Hooks["SessionStart"])
	}
	if cfg.Permissions == nil {
		t.Error("other settings must survive")
	}

	if r := gloss(t, root, "", nil, "hook", "install", "--no-mcp", "--no-hook", "--skills", "adopt-glossmark"); r.code != 0 {
		t.Fatalf("--skills adopt-glossmark: %d %s", r.code, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "adopt-glossmark", "SKILL.md")); err != nil {
		t.Errorf("--skills adopt-glossmark should install it: %v", err)
	}
	if r := gloss(t, root, "", nil, "hook", "install", "--no-mcp", "--no-hook", "--skills", "no-such-skill"); r.code == 0 || !strings.Contains(r.stderr, "unknown skill") {
		t.Errorf("--skills with an unknown name should fail, got %d %q", r.code, r.stderr)
	}
}
