// @gloss verifies gmi:AgentsSnippet scope=file

package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/rollup"
)

func TestRulesBlock(t *testing.T) {
	b := RulesBlock()
	if !strings.HasPrefix(b, RulesBegin+"\n"+RulesHeading+"\n") || !strings.HasSuffix(b, "\n"+RulesEnd) {
		t.Errorf("block must run from %q to %q:\n%s", RulesBegin, RulesEnd, b)
	}
	if len(Rules) != 10 || Rules[4] != ProposalSentence {
		t.Fatalf("rule 5 must be ProposalSentence verbatim; %d rules", len(Rules))
	}
	if !strings.Contains(b, "\n5. "+ProposalSentence+"\n") {
		t.Errorf("the rules block must state ProposalSentence verbatim as rule 5:\n%s", b)
	}
	for _, s := range append([]string{RulesDocURL, "\n10. **Report the impact.**"}, RuleSkills...) {
		if !strings.Contains(b, s) {
			t.Errorf("rules block lacks %q", s)
		}
	}
	// The block is read in full by every agent: keep it near the 600-token
	// target (the briefing's estimate is four characters a token).
	if len(b) > 3000 {
		t.Errorf("rules block is %d characters; keep it under 3000", len(b))
	}
	in := MCPInstructions()
	if !strings.Contains(in, ProposalSentence) || strings.Contains(in, "**") || !strings.Contains(in, "search_concepts") {
		t.Errorf("MCP instructions must quote the rules without emphasis:\n%s", in)
	}
}

func TestUpsertRules(t *testing.T) {
	vocab := "<!-- gloss:begin -->\nvocabulary\n<!-- gloss:end -->\n"
	for name, doc := range map[string]string{
		"empty":           "",
		"no newline":      "# Agents\nBe kind.",
		"with vocabulary": "# Agents\n\nBe kind.\n\n" + vocab,
	} {
		out, err := UpsertRules(doc)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(out, doc) {
			t.Errorf("%s: existing text changed:\n%s", name, out)
		}
		if st, _ := CheckRules(out); st != RulesCurrent {
			t.Errorf("%s: block not current after upsert:\n%s", name, out)
		}
		again, _ := UpsertRules(out)
		if again != out {
			t.Errorf("%s: a second upsert changed the file:\n%s", name, again)
		}
		// An old block is replaced in place; text on both sides stays.
		old := strings.Replace(out, "**Leave it clean.**", "**Leave it tidy.**", 1)
		old = strings.Replace(old, RulesBegin, "<!-- gloss:rules:begin (an older release) -->", 1)
		old += "\nAfter.\n"
		if st, _ := CheckRules(old); st != RulesStale {
			t.Errorf("%s: an edited block should be stale", name)
		}
		fixed, _ := UpsertRules(old)
		if fixed != out+"\nAfter.\n" {
			t.Errorf("%s: replacing a stale block:\n%s", name, fixed)
		}
	}
	if st, _ := CheckRules("# Agents\n"); st != RulesMissing {
		t.Errorf("no block should be missing")
	}
	if _, err := UpsertRules("x\n" + RulesBegin + "\nhalf a block\n"); err == nil {
		t.Errorf("a begin marker without an end marker must be refused, not overwritten")
	}
}

func TestClaudeImport(t *testing.T) {
	if !HasClaudeImport("# Claude\n\n  @AGENTS.md\n") || HasClaudeImport("Read @AGENTS.md first.\n") || HasClaudeImport("") {
		t.Error("HasClaudeImport must match only a line of its own")
	}
}

func TestAllowedLine(t *testing.T) {
	short := rollup.Allowed{Patterns: []string{"aa:*", "arena:Grading"}, Derived: true}
	if got := allowedLine(short); got != "Allowed targets: aa:*, arena:Grading (others are HARD-gated, derived from what the manifest declares)" {
		t.Errorf("short list: %s", got)
	}
	long := rollup.Allowed{Patterns: []string{"gmi:*"}, Derived: true}
	for i := 0; i < 12; i++ {
		long.Patterns = append(long.Patterns, "gm:C"+string(rune('a'+i)))
	}
	got := allowedLine(long)
	if !strings.HasPrefix(got, "Allowed targets: gmi:*, everything this boundary") || !strings.Contains(got, "(12 concepts)") || strings.Contains(got, "gm:Ca") {
		t.Errorf("a long derived list collapses to its globs and a count: %s", got)
	}
	long.Derived = false
	if got := allowedLine(long); !strings.Contains(got, "gmi:*, the 12 concepts listed in allowed:") {
		t.Errorf("a long written list: %s", got)
	}
}

func TestInstalledSkills(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"gloss-review", "adopt-glossmark", "someone-elses"} {
		p := filepath.Join(root, ".claude", "skills", n, "SKILL.md")
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("---\nname: "+n+"\n---\n"), 0o644)
	}
	os.MkdirAll(filepath.Join(root, ".claude", "skills", "gloss-link"), 0o755) // no SKILL.md
	if got := strings.Join(InstalledSkills(root), ","); got != "adopt-glossmark,gloss-review" {
		t.Errorf("installed skills = %s", got)
	}
	if got := InstalledSkills(t.TempDir()); len(got) != 0 {
		t.Errorf("no skills installed, got %v", got)
	}
}
