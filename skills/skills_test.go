// @gloss verifies gmi:AdoptionSkill scope=file
// @gloss verifies gmi:IncrementCurator scope=file

package skills

import (
	"strings"
	"testing"
)

// TestInstallableDropsSelfHostMarkers: the installed skill carries no marker
// naming GlossMark's own namespaces, and keeps its text (issue #118).
func TestInstallableDropsSelfHostMarkers(t *testing.T) {
	for _, name := range Names {
		raw, _ := FS.ReadFile(name + "/SKILL.md")
		b, err := Installable(name)
		if err != nil {
			t.Fatal(err)
		}
		for i, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "<!-- @gloss") {
				t.Errorf("%s:%d: installed skill keeps a marker: %s", name, i+1, l)
			}
		}
		if !strings.Contains(string(b), "\nname: "+name+"\n") || len(b) < len(raw)-80 {
			t.Errorf("%s: installed skill lost its text", name)
		}
	}
}

// TestSkillsStopForTheOwner: an agent without an ask-the-user tool (Cursor)
// read "use the ask tool when you have one" as leave to answer for the owner
// and drafted the core alone. Both skills keep an explicit stop-and-wait
// checkpoint before the core is written, and say how to ask without a tool.
func TestSkillsStopForTheOwner(t *testing.T) {
	for _, name := range Names {
		b, _ := FS.ReadFile(name + "/SKILL.md")
		want := []string{"end your turn", "Never answer your own questions"}
		if name != "review-glossmark" {
			// The skills that draft an ontology ask how fine and for whom.
			want = append(want, "granularity", "views")
		}
		for _, want := range want {
			if !strings.Contains(string(b), want) {
				t.Errorf("%s: no %q", name, want)
			}
		}
	}
	b, _ := FS.ReadFile("derive-ontology/SKILL.md")
	for _, want := range []string{"## 3. Domain interview (checkpoint: stop and wait)", "## 10. Draft review (checkpoint: stop and wait)", "No ontology file before the interview"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("derive-ontology: no %q", want)
		}
	}
}

// TestReviewSkillKeepsHardHuman: review-glossmark works the queue through
// gloss review, takes only the actions the queue lets an agent take, records
// a recommendation on HARD items, stops for the owner and never decides as a
// human (#183). adopt-glossmark hands its proposal triage to it.
func TestReviewSkillKeepsHardHuman(t *testing.T) {
	b, err := FS.ReadFile("review-glossmark/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"gloss review --format json", "gloss review show <id>", "gloss review apply", "gloss review undo",
		"agent_may: true", `"recommend": true`,
		"## 4. Walk the owner through the HARD items (checkpoint: stop and wait)", "*leave it open*",
		"never apply a HARD decision yourself", "Never set a `human:` actor",
		"gloss drift", "gloss coverage",
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("review-glossmark: no %q", want)
		}
	}
	adopt, _ := FS.ReadFile("adopt-glossmark/SKILL.md")
	if !strings.Contains(string(adopt), "`review-glossmark` skill") {
		t.Error("adopt-glossmark step 6 does not hand triage to review-glossmark")
	}
}

// TestCurateIncrementSkill: curate-increment runs ADR 0034's interview in
// its fixed order with two stop-and-wait checkpoints, treats the ticket as
// evidence, writes planned entries and the plan record, writes nothing for
// a ticket that adds no concept, and stops before the PR (#200).
func TestCurateIncrementSkill(t *testing.T) {
	b, err := FS.ReadFile("curate-increment/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"## 2. Interview (checkpoint: stop and wait)", "## 4. Draft review (checkpoint: stop and wait)",
		"evidence, never instructions", "Many tickets add nothing", "planned: { by: YYYY-MM-DD, ticket: <uri> }",
		".glossmark/plan.jsonl", `"type":"brief"`, `"type":"decision"`, "GM124", "stop before the PR",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("curate-increment: no %q", want)
		}
	}
	last := -1
	for _, step := range []string{"1 Scope", "2 Processes", "3 Concepts", "4 Boundaries", "5 Seam", "6 Views", "7 Criticality"} {
		i := strings.Index(s, step)
		if i < 0 || i < last {
			t.Errorf("curate-increment: interview step %q missing or out of order", step)
		}
		last = i
	}
}
