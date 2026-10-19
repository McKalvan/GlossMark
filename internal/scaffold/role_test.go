// @gloss verifies gmi:Scaffolder scope=file

package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRolePlan: a role agent file carries the subagent frontmatter, the
// instructs and may-modify markers and the actor line; bad names and empty
// remits are refused; an existing file is skipped, never overwritten.
func TestRolePlan(t *testing.T) {
	root := t.TempDir()
	for _, bad := range []string{"", "agent:refund", "team/refund", "Refund-Engineer", "refund engineer", "-refund"} {
		if _, err := RolePlan(root, Role{Name: bad, MayModify: "pay:Refund"}); err == nil {
			t.Errorf("role name %q should be refused", bad)
		}
	}
	if _, err := RolePlan(root, Role{Name: "refund-engineer"}); err == nil || !strings.Contains(err.Error(), "remit") {
		t.Errorf("a role without --may-modify should be refused, got %v", err)
	}
	if _, err := RolePlan(root, Role{Name: "refund-engineer", MayModify: "Refund"}); err == nil {
		t.Error("a may-modify target without a prefix should be refused")
	}
	if _, err := RolePlan(root, Role{Name: "refund-engineer", MayModify: "pay:Refund", Instructs: "pay:*"}); err == nil {
		t.Error("--instructs takes one concept, not a glob")
	}

	p, err := RolePlan(root, Role{Name: "refund-engineer", MayModify: "pay:Refund,pay:Refunding , pa:*", Instructs: "pay:Refunding", Description: "Implements and fixes refund flows in services/api."})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 1 || p.Files[0].Path != ".claude/agents/refund-engineer.md" || p.Files[0].Action != Create {
		t.Fatalf("plan = %+v", p.Files)
	}
	want := "---\nname: refund-engineer\ndescription: Implements and fixes refund flows in services/api.\n---\n\n" +
		"<!-- @gloss instructs pay:Refunding scope=file -->\n" +
		"<!-- @gloss may-modify pay:Refund, pay:Refunding, pa:* scope=file -->\n\n" +
		"# Refund engineer\n\n"
	if got := p.Files[0].Content; !strings.HasPrefix(got, want) || !strings.Contains(got, "`GLOSS_ACTOR=agent:refund-engineer`") || !strings.Contains(got, "HARD-gated") {
		t.Errorf("role file:\n%s", got)
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, ".claude", "agents", "refund-engineer.md"), []byte("mine\n"), 0o644)
	p2, err := RolePlan(root, Role{Name: "refund-engineer", MayModify: "pay:Refund"})
	if err != nil {
		t.Fatal(err)
	}
	if p2.Files[0].Action != Skip {
		t.Errorf("an existing role file must be skipped, got %s", p2.Files[0].Action)
	}
	p2.Apply()
	if b, _ := os.ReadFile(filepath.Join(root, ".claude", "agents", "refund-engineer.md")); string(b) != "mine\n" {
		t.Errorf("role file overwritten: %q", b)
	}

	// Without a description the remit describes the role, quoted for YAML;
	// without --instructs there is no instructs marker.
	c := RoleFile(Role{Name: "ops", MayModify: "pa:*"})
	if !strings.Contains(c, "description: \"Changes the code linked to pa:*. Use for tasks within that remit.\"\n") || strings.Contains(c, "instructs") {
		t.Errorf("default role file:\n%s", c)
	}
}
