// @gloss implements gmi:Scaffolder scope=file

package scaffold

import (
	"fmt"
	"regexp"
	"strings"
)

// Role describes a role agent: a Claude Code subagent with a GlossMark remit.
type Role struct {
	Name        string // the subagent's name; its actor is agent:<Name>
	MayModify   string // the remit: concept CURIEs and globs, comma-separated
	Instructs   string // the concept the role is about (optional)
	Description string // when Claude Code should use the subagent (default: derived from the remit)
}

// roleName is a Claude Code subagent name: lower-case words joined by
// hyphens. It never holds ':' or '/', so agent:<name> is one actor and
// .claude/agents/<name>.md one file.
var roleName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// RolePath is the repo-relative path of a role agent's file.
func RolePath(name string) string { return ".claude/agents/" + name + ".md" }

// RolePlan plans the one file gloss init --role writes:
// .claude/agents/<name>.md, skipped when it exists.
func RolePlan(root string, r Role) (*Plan, error) {
	if !roleName.MatchString(r.Name) {
		return nil, fmt.Errorf("role name %q: use lower-case letters, digits and hyphens, such as refund-engineer (no ':' or '/')", r.Name)
	}
	var targets []string
	for _, t := range strings.Split(r.MayModify, ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !strings.Contains(t, ":") || strings.ContainsAny(t, " \t") {
			return nil, fmt.Errorf("--may-modify target %q: write a CURIE or a glob, such as pay:Refund or pa:*", t)
		}
		targets = append(targets, t)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("a role needs a remit: --may-modify \"pay:Refund, pa:*\"")
	}
	instructs := strings.TrimSpace(r.Instructs)
	if instructs != "" && (!strings.Contains(instructs, ":") || strings.ContainsAny(instructs, " \t,*")) {
		return nil, fmt.Errorf("--instructs %q: write one concept CURIE, such as pay:Refunding", instructs)
	}
	p := &Plan{Root: root, Name: r.Name}
	f := File{Path: RolePath(r.Name), Action: Create, Content: RoleFile(Role{Name: r.Name, MayModify: strings.Join(targets, ", "), Instructs: instructs, Description: r.Description}), Note: "role agent agent:" + r.Name}
	if exists(root, f.Path) {
		f.Action, f.Note = Skip, "exists; left as is"
	}
	p.Files = append(p.Files, f)
	return p, nil
}

// RoleFile renders a role agent's file: Claude Code subagent frontmatter,
// the instructs and may-modify markers, and the two rules that make the
// remit work.
func RoleFile(r Role) string {
	desc := strings.TrimSpace(r.Description)
	if desc == "" {
		desc = "Changes the code linked to " + r.MayModify + ". Use for tasks within that remit."
	}
	title := strings.ReplaceAll(r.Name, "-", " ")
	title = strings.ToUpper(title[:1]) + title[1:]
	var sb strings.Builder
	fmt.Fprintf(&sb, "---\nname: %s\ndescription: %s\n---\n\n", r.Name, yamlScalar(desc))
	if r.Instructs != "" {
		fmt.Fprintf(&sb, "<!-- @gloss instructs %s scope=file -->\n", r.Instructs)
	}
	fmt.Fprintf(&sb, "<!-- @gloss may-modify %s scope=file -->\n\n", r.MayModify)
	fmt.Fprintf(&sb, "# %s\n\n", title)
	sb.WriteString("You are a role agent with a GlossMark remit. The GlossMark rules in AGENTS.md apply.\n\n")
	sb.WriteString("- Your remit is the `may-modify` line above. A link you write outside it is HARD-gated; if your task needs one, say why in your summary.\n")
	fmt.Fprintf(&sb, "- Run with `GLOSS_ACTOR=agent:%s` so the briefing shows only your remit and the ledger attributes your links to you.\n", r.Name)
	return sb.String()
}

// yamlScalar quotes s when YAML would read it as something other than a
// plain string.
func yamlScalar(s string) string {
	if strings.ContainsAny(s, ":#{}[]&*!|>'\"%@`") || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "?") {
		return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
	}
	return s
}
