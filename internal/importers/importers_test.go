// @gloss verifies gmi:VocabularyImporter scope=file

package importers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/check"
	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/ontoedit"
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

// TestEnumValueDoesNotOverwriteGlossaryConcept: an enum value whose bare name
// is already a concept that is not is-a the enum lands as <Enum><Value>, and
// re-running either importer afterwards is a no-op.
func TestEnumValueDoesNotOverwriteGlossaryConcept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lib.gloss.yaml")
	os.WriteFile(path, []byte("namespace: lib\nconcepts: {}\n"), 0o644)
	gl := "## Lost Item\n\n**Def.** An item a patron cannot return.\n**Status:** implemented\n"
	en := "FineType:\n  LOST_ITEM: Replacement cost charged when an item is declared lost.\n  LATE: Charged per day overdue.\n"
	if r, err := Apply(path, ParseGlossary(gl), "human:test", false); err != nil || len(r.Added) != 1 {
		t.Fatalf("glossary import: %+v %v", r, err)
	}
	ev, err := ParseEnums([]byte(en), true)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Apply(path, ev, "human:test", false)
	if err != nil || len(r.Updated) != 0 || strings.Join(r.Added, ",") != "FineType,FineTypeLostItem,Late" {
		t.Fatalf("enums import: %+v %v", r, err)
	}
	if doc, _, _ := ontoedit.Get(path, "LostItem", "doc"); doc != "An item a patron cannot return." {
		t.Errorf("glossary doc overwritten: %q", doc)
	}
	if isa, ok, _ := ontoedit.Get(path, "FineTypeLostItem", "is-a"); !ok || isa != "FineType" {
		t.Errorf("FineTypeLostItem is-a = %q (exists %v)", isa, ok)
	}
	before, _ := os.ReadFile(path)
	for i := 0; i < 2; i++ {
		if r, _ := Apply(path, ParseGlossary(gl), "human:test", false); len(r.Added)+len(r.Updated) != 0 {
			t.Errorf("glossary re-run changed %+v", r)
		}
		if r, _ := Apply(path, ev, "human:test", false); len(r.Added)+len(r.Updated) != 0 {
			t.Errorf("enums re-run changed %+v", r)
		}
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Errorf("re-runs rewrote the file:\n%s", after)
	}
}

// A brownfield staff glossary (issue #136): a section heading is not a
// concept, kinds are read from the definition, and a deprecated status
// names its successor.
const libraryGlossary = `# Circulation glossary

## Patron

**Def.** A person registered to borrow items.

## Checkout

**Def.** Lending an item to a patron for a loan period.

## Loan Period

**Def.** How long a patron may keep an item before it is due.

## Claims Returned

**Def.** Recording a patron's claim that an overdue item was returned.

## Fine Cap

**Def.** A fine never exceeds the replacement cost of the item.

## Retired terms

Older names that still appear in reports, forms and the database.

### Borrower

**Def.** A patron, in the old circulation system.
**Status:** deprecated in 2016 (CIRC-412); use Patron

### Ticket

**Def.** A fine slip.
**Status:** retired; replaced by Fine Notice
`

func TestParseLibraryGlossary(t *testing.T) {
	got := map[string]Entry{}
	for _, e := range ParseGlossary(libraryGlossary) {
		got[e.Name] = e
	}
	if _, ok := got["RetiredTerms"]; ok {
		t.Error("a section heading above deeper headings became a concept")
	}
	for name, want := range map[string]string{"Patron": "entity:name", "Checkout": "process:definition", "LoanPeriod": "constraint:definition",
		"ClaimsReturned": "process:definition", "FineCap": "constraint:definition", "Borrower": "entity:name"} {
		if e := got[name]; e.Kind+":"+e.KindFrom != want {
			t.Errorf("%s: %s:%s, want %s", name, e.Kind, e.KindFrom, want)
		}
	}
	if b := got["Borrower"]; !b.Deprecated || b.ReplacedBy != "Patron" {
		t.Errorf("Borrower = %+v", b)
	}
	if tk := got["Ticket"]; tk.ReplacedBy != "FineNotice" {
		t.Errorf("Ticket = %+v", tk)
	}
	// A section heading with a Def. line is still a concept.
	es := ParseGlossary("## Loan\n\n**Def.** Lending an item.\n\n### Renewal\n\n**Def.** Extending a loan.\n")
	if len(es) != 2 {
		t.Errorf("entries = %+v", es)
	}
	path := filepath.Join(t.TempDir(), "lib.gloss.yaml")
	os.WriteFile(path, []byte("namespace: lib\nconcepts: {}\n"), 0o644)
	r, err := Apply(path, ParseGlossary(libraryGlossary), "human:test", false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Kinds["Checkout"] != "process, guessed from the definition" {
		t.Errorf("kinds = %v", r.Kinds)
	}
	if rb, _, _ := ontoedit.Get(path, "Borrower", "replaced-by"); rb != "Patron" {
		t.Errorf("Borrower replaced-by = %q", rb)
	}
}

// TestEnumValueNames (issue #136, L7): prefixed names every value with its enum.
func TestEnumValueNames(t *testing.T) {
	src := []byte("FineType: [OVERDUE, DAMAGE]\nLoanStatus: [ACTIVE, OVERDUE]\n")
	names := func(prefixed bool) string {
		es, err := ParseEnumsNamed(src, true, prefixed)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, e := range es {
			out = append(out, e.Name)
		}
		return strings.Join(out, ",")
	}
	if got := names(false); got != "FineType,LoanStatus,Overdue,Damage,Active,LoanStatusOverdue" {
		t.Errorf("bare = %s", got)
	}
	if got := names(true); got != "FineType,LoanStatus,FineTypeOverdue,FineTypeDamage,LoanStatusActive,LoanStatusOverdue" {
		t.Errorf("prefixed = %s", got)
	}
}

// TestImportRecord (issue #136, L8): with the import record, a doc improved
// by hand survives a re-run until the source changes, and a concept deleted
// after an import is not added again.
func TestImportRecord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lib.gloss.yaml")
	recPath := filepath.Join(dir, ".glossmark", RecordName)
	os.WriteFile(path, []byte("namespace: lib\nconcepts: {}\n"), 0o644)
	src := "PatronType: [ADULT, JUVENILE]\nLoanStatus: [ACTIVE]\n"
	run := func(src string) *Result {
		t.Helper()
		rec, err := OpenRecord(recPath)
		if err != nil {
			t.Fatal(err)
		}
		es, _ := ParseEnums([]byte(src), false)
		r, err := ApplyRecorded(path, "lib", es, "human:test", false, rec)
		if err != nil {
			t.Fatal(err)
		}
		if err := rec.Save(); err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := run(src); len(r.Added) != 2 {
		t.Fatalf("first import: %+v", r)
	}
	hand := "The kind of card a patron holds; sets the borrowing limit."
	if err := ontoedit.SetField(path, "PatronType", "doc", ontoedit.Quote(hand)); err != nil {
		t.Fatal(err)
	}
	if err := ontoedit.RemoveConcept(path, "LoanStatus"); err != nil {
		t.Fatal(err)
	}
	r := run(src)
	if len(r.Added)+len(r.Updated) != 0 || strings.Join(r.Kept, ",") != "PatronType" || len(r.Skipped) != 1 || !strings.Contains(r.Skipped[0], "LoanStatus: removed") {
		t.Errorf("re-run: %+v", r)
	}
	if doc, _, _ := ontoedit.Get(path, "PatronType", "doc"); doc != hand {
		t.Errorf("hand-edited doc replaced: %q", doc)
	}
	// The source changes: the doc follows it.
	r = run("PatronType: [ADULT, JUVENILE, SENIOR]\nLoanStatus: [ACTIVE]\n")
	if strings.Join(r.Updated, ",") != "PatronType" {
		t.Errorf("source change: %+v", r)
	}
	if doc, _, _ := ontoedit.Get(path, "PatronType", "doc"); doc != "One of: ADULT, JUVENILE, SENIOR." {
		t.Errorf("doc after a source change: %q", doc)
	}
	b, _ := os.ReadFile(recPath)
	if !strings.HasPrefix(string(b), `{"type":"header","format":1`) || !strings.Contains(string(b), `"name":"PatronType"`) {
		t.Errorf("record:\n%s", b)
	}
}

// TestEnumInstances (#205): a catalogue enum named in Instances is imported
// without its values, which come back as instances, while a lifecycle enum
// in the same file keeps its values as concepts.
func TestEnumInstances(t *testing.T) {
	src := []byte("Plan: [Free, Pro, Team]\nLoanStatus: [ACTIVE, OVERDUE]\n")
	es, insts, err := ParseEnumsWith(src, EnumOptions{Values: true, Instances: []string{"plan"}})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range es {
		names = append(names, e.Name)
	}
	if got := strings.Join(names, ","); got != "Plan,LoanStatus,Active,Overdue" {
		t.Errorf("concepts = %s", got)
	}
	var vals []string
	for _, in := range insts {
		vals = append(vals, in.Enum+"/"+in.Name)
	}
	if got := strings.Join(vals, ","); got != "Plan/Free,Plan/Pro,Plan/Team" {
		t.Errorf("instances = %s", got)
	}
	// Without Instances the import is what it was.
	old, _ := ParseEnumsNamed(src, true, false)
	if len(old) != 7 {
		t.Errorf("--values without --instances: %d entries, want 7", len(old))
	}
}
