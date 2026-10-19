package importers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/rollup"
)

const glossary = `# AgentArena glossary

Intro text that is not a concept.

## Grading

**Def.** The process of scoring a submission against a rubric, per criterion.
**Aliases:** marking, scoring
**Code:** ` + "`grading/`" + `
**UI:** Grade tab
**Status:** implemented

## Lesson

- **Def.**: A unit of teaching, made of quizzes, readings and an exercise.
- **Status:** implemented

## Leaderboard

A ranking of agents by score. Updated nightly.

**Status:** planned

## Legacy Score

**Def.** The old single-number score.
**Status:** deprecated

` + "```" + `
## Not A Heading Inside A Fence
` + "```" + `
`

const enums = `enums:
  SubmissionState:
    description: Where a submission is in grading.
    values:
      - { name: queued, description: Waiting for a grader. }
      - { name: graded }
  Difficulty: [easy, medium, hard]
`

func TestParseGlossary(t *testing.T) {
	es := ParseGlossary(glossary)
	names := map[string]Entry{}
	for _, e := range es {
		names[e.Name] = e
	}
	if len(es) != 4 {
		t.Fatalf("entries = %+v", es)
	}
	g := names["Grading"]
	if g.Kind != "process" || !strings.HasPrefix(g.Doc, "The process of scoring a submission against a rubric, per criterion.") || strings.Join(g.Aliases, "|") != "marking|scoring" || g.Proposed {
		t.Errorf("Grading = %+v", g)
	}
	if l := names["Lesson"]; !strings.Contains(l.Doc, "quizzes, readings") || l.Proposed {
		t.Errorf("Lesson = %+v", l)
	}
	if lb := names["Leaderboard"]; lb.Doc != "A ranking of agents by score." || !lb.Proposed {
		t.Errorf("Leaderboard = %+v", lb)
	}
	if ls := names["LegacyScore"]; !ls.Deprecated || ls.Kind != "measure" {
		t.Errorf("LegacyScore = %+v", ls)
	}
}

// TestImportRoundTrip: import a glossary and an enums file into an empty
// namespace, check passes, a re-run changes nothing, a changed definition
// updates only that doc, and the status export reads the result.
func TestImportRoundTrip(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("glossmark.yaml", "owns:\n  arena: { iri: https://x/arena/, tier: core }\n")
	write("ontology/arena.gloss.yaml", "# keep this header comment\nnamespace: arena\nconcepts: {}\n")
	path := filepath.Join(root, "ontology", "arena.gloss.yaml")
	r, err := Apply(path, ParseGlossary(glossary), "human:test", false)
	if err != nil || len(r.Added) != 4 {
		t.Fatalf("glossary import: %+v %v", r, err)
	}
	ev, err := ParseEnums([]byte(enums), true)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := Apply(path, ev, "human:test", false); err != nil || len(r.Added) != 7 {
		t.Fatalf("enums import: %+v %v", r, err)
	}
	before, _ := os.ReadFile(path)
	if !strings.Contains(string(before), "# keep this header comment") || !strings.Contains(string(before), `doc: "The process of scoring a submission against a rubric, per criterion."`) {
		t.Errorf("file:\n%s", before)
	}
	ws, err := load.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	res, _ := extract.Build(ws)
	rollup.Derive(res.Graph, ws.Vocab)
	for _, d := range check.Run(ws, res.Graph) {
		if d.Severity == "error" || d.Code == "GM304" || d.Code == "GM305" {
			t.Errorf("check: %s", d)
		}
	}
	if n := res.Graph.NodeByCurie("arena:Queued"); n == nil || len(res.Graph.Objects(n.ID, "is-a")) != 1 {
		t.Errorf("enum value Queued is-a SubmissionState: %+v", n)
	}
	if n := res.Graph.NodeByCurie("arena:Leaderboard"); n == nil || !n.Proposed {
		t.Errorf("a planned glossary entry is proposed: %+v", n)
	}
	// Re-run: nothing changes.
	if r, _ := Apply(path, ParseGlossary(glossary), "human:test", false); len(r.Added)+len(r.Updated) != 0 {
		t.Errorf("re-run changed %+v", r)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Error("re-run rewrote the file")
	}
	// A changed definition updates only that doc line.
	g2 := strings.Replace(glossary, "A unit of teaching, made of quizzes, readings and an exercise.", "A unit of teaching.", 1)
	if r, _ := Apply(path, ParseGlossary(g2), "human:test", false); len(r.Updated) != 1 || r.Updated[0] != "Lesson" {
		t.Errorf("update = %+v", r)
	}
	after, _ = os.ReadFile(path)
	diff := 0
	bl, al := strings.Split(string(before), "\n"), strings.Split(string(after), "\n")
	for i := range bl {
		if i < len(al) && bl[i] != al[i] {
			diff++
		}
	}
	if diff != 1 || len(bl) != len(al) {
		t.Errorf("one line should change, got %d (lines %d → %d)", diff, len(bl), len(al))
	}
	// Status export.
	ws, _ = load.Load(root)
	res, _ = extract.Build(ws)
	rollup.Derive(res.Graph, ws.Vocab)
	rows := rollup.ConceptStatus(res.Graph, rollup.Compare(res.Graph, ws, rollup.Options{}))
	st := map[string]string{}
	for _, r := range rows {
		st[r.Concept] = r.Status
	}
	if st["arena:Leaderboard"] != "proposed" || st["arena:Grading"] != "orphan" {
		t.Errorf("status = %v", st)
	}
}
