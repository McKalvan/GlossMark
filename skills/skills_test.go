// @gloss verifies gmi:AdoptionSkill scope=file

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
		for _, want := range []string{"end your turn", "Never answer your own questions"} {
			if !strings.Contains(string(b), want) {
				t.Errorf("%s: no %q", name, want)
			}
		}
	}
	for _, name := range []string{"adopt-glossmark", "derive-ontology"} {
		b, _ := FS.ReadFile(name + "/SKILL.md")
		for _, want := range []string{"granularity", "views"} {
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

// TestReviewSkillKeepsHardDecisionsHuman: the review skill takes only the
// actions the queue marks agent_may, recommends on HARD ones, and hands the
// owner's answers back for the owner to apply rather than applying them under
// a human actor (issue #183).
func TestReviewSkillKeepsHardDecisionsHuman(t *testing.T) {
	b, err := FS.ReadFile("review-glossmark/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"`agent_may`",
		`"recommend": true`,
		"no setting `GLOSS_ACTOR` to a human's name",
		"you still do not apply them",
		"gloss review show",
		"gloss review undo",
		"## 4. Walk the owner through it (checkpoint: stop and wait)",
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("review-glossmark: no %q", want)
		}
	}
}
