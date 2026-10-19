// @gloss verifies gmi:Scaffolder scope=file

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
)

// TestInitRole: gloss init --role writes .claude/agents/<name>.md in an
// adopted repository, shows it in --dry-run without writing, never
// overwrites it, refuses a name with ':' or '/', and leaves gloss check clean
// with the role's remit in the graph.
func TestInitRole(t *testing.T) {
	root := t.TempDir()
	copyTree(t, example(t, "horse-factory"), root)
	path := filepath.Join(root, ".claude", "agents", "foaling-engineer.md")
	ws0, g0, res0, _ := build(t, root)
	baseline := append(res0.Diags, check.Run(ws0, g0)...)
	args := []string{"init", "--role", "foaling-engineer", "--may-modify", "impl:Foaling*, zoo:Foal", "--instructs", "impl:FoalingPipeline", "--description", "Changes the foaling pipeline."}

	// From a subdirectory: the file lands at the repository root.
	sub := filepath.Join(root, "services")
	r := gloss(t, sub, "", nil, append(args, "--dry-run")...)
	if r.code != 0 || !strings.Contains(r.stdout, "create .claude/agents/foaling-engineer.md") {
		t.Fatalf("--dry-run: %d %s %s", r.code, r.stdout, r.stderr)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("--dry-run wrote the role file")
	}
	if r := gloss(t, sub, "", nil, args...); r.code != 0 {
		t.Fatalf("init --role: %d %s", r.code, r.stderr)
	}
	content := readFile(t, path)
	for _, want := range []string{"---\nname: foaling-engineer\n", "<!-- @gloss instructs impl:FoalingPipeline scope=file -->", "<!-- @gloss may-modify impl:Foaling*, zoo:Foal scope=file -->", "GLOSS_ACTOR=agent:foaling-engineer"} {
		if !strings.Contains(content, want) {
			t.Errorf("role file lacks %q:\n%s", want, content)
		}
	}

	// The example carries one deliberate GM502; the role file adds nothing.
	ws, g, res, _ := build(t, root)
	diags := append(res.Diags, check.Run(ws, g)...)
	if len(diags) != len(baseline) {
		t.Errorf("gloss check after init --role: %d diagnostics, %d before:\n%v", len(diags), len(baseline), diags)
	}
	for _, d := range diags {
		if d.Severity == "error" || strings.Contains(d.String(), "foaling-engineer") {
			t.Errorf("gloss check after init --role: %s", d)
		}
	}
	remit := 0
	for _, fm := range res.Markers {
		if fm.Path == ".claude/agents/foaling-engineer.md" && fm.Marker.Relation == "may-modify" {
			remit += len(fm.Marker.Targets)
		}
	}
	if remit != 2 {
		t.Errorf("want 2 may-modify targets from the role file, got %d", remit)
	}

	writeFile(t, path, "mine\n")
	if r := gloss(t, root, "", nil, args...); r.code != 0 || !strings.Contains(r.stdout, "skip") {
		t.Errorf("second init --role: %d %s", r.code, r.stdout)
	}
	if readFile(t, path) != "mine\n" {
		t.Error("init --role overwrote an existing role file")
	}
	for _, bad := range []string{"agent:foaling", "team/foaling"} {
		if r := gloss(t, root, "", nil, "init", "--role", bad, "--may-modify", "zoo:Foal"); r.code == 0 {
			t.Errorf("--role %q should be refused", bad)
		}
	}
}
