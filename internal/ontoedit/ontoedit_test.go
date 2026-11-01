// @gloss verifies gmi:OntologyEditor scope=file

package ontoedit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const flowFile = `# ontology/circ.gloss.yaml  (implementation tier)
namespace: circ
version: 0.3.0
concepts:
  CatalogIntake:              { kind: process, realizes: lib:Cataloging }
  MarcParser:                 { kind: process, part-of: CatalogIntake, internal: true }     # unpublished: private to this repo
  Throughput:
    kind: measure
    criticality: low                 # telemetry
    realizes: lib:LoanPeriod
    doc: |
      Multi-line block scalar.
      Second line.
  Wide: {
    kind: resource,
    internal: true,
  }
relations:
  produces: { lift: range-lcs }
`

func tmp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "circ.gloss.yaml")
	if content != "" {
		os.WriteFile(p, []byte(content), 0o644)
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAddConceptPreservesFormatting(t *testing.T) {
	p := tmp(t, flowFile)
	if err := AddConcept(p, "DamageCheck", ConceptSpec{Kind: "process", Realizes: []string{"lib:Cataloging"}, Doc: "Damage check: books, not DVDs.", Aliases: []string{"condition check", "wear review"}, Proposed: true, ProposedBy: "agent:bot@m"}); err != nil {
		t.Fatal(err)
	}
	got := read(t, p)
	want := strings.Replace(flowFile, "  }\nrelations:", "  }\n"+
		"  DamageCheck:\n"+
		"    kind: process\n"+
		"    realizes: lib:Cataloging\n"+
		"    doc: \"Damage check: books, not DVDs.\"\n"+
		"    aliases: [condition check, wear review]\n"+
		"    proposed: true\n"+
		"    proposed-by: agent:bot@m\n"+
		"relations:", 1)
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if err := AddConcept(p, "DamageCheck", ConceptSpec{Kind: "process"}); err == nil {
		t.Error("adding an existing concept must fail")
	}
	// The other entries are untouched byte for byte.
	for _, line := range strings.Split(flowFile, "\n") {
		if !strings.Contains(got, line) {
			t.Errorf("line lost or reflowed: %q", line)
		}
	}
}

func TestAddConceptEmptyFiles(t *testing.T) {
	cases := map[string]string{
		"":                             "concepts:\n  X:\n    kind: entity\n",
		"namespace: a\n":               "namespace: a\n\nconcepts:\n  X:\n    kind: entity\n",
		"namespace: a\nconcepts: {}\n": "namespace: a\nconcepts:\n  X:\n    kind: entity\n",
		"concepts:\n":                  "concepts:\n  X:\n    kind: entity\n",
	}
	for in, want := range cases {
		p := tmp(t, in)
		if in == "" {
			p = filepath.Join(t.TempDir(), "new.gloss.yaml")
		}
		if err := AddConcept(p, "X", ConceptSpec{Kind: "entity"}); err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if got := read(t, p); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestSetProposedAndRemove(t *testing.T) {
	src := "concepts:\n" +
		"  A: { kind: process, proposed: true, proposed-by: agent:x }   # keep me\n" +
		"  B:\n    kind: process\n    proposed: true\n    proposed-by: \"agent:y\"\n" +
		"  C: { kind: process }\n" +
		"  D:\n    kind: process\n"
	p := tmp(t, src)
	for _, n := range []string{"A", "B"} {
		if ok, _ := IsProposed(p, n); !ok {
			t.Errorf("%s should be proposed", n)
		}
		if err := SetProposed(p, n, false, ""); err != nil {
			t.Fatal(err)
		}
		if ok, _ := IsProposed(p, n); ok {
			t.Errorf("%s should be accepted", n)
		}
	}
	want := "concepts:\n  A: { kind: process }   # keep me\n  B:\n    kind: process\n  C: { kind: process }\n  D:\n    kind: process\n"
	if got := read(t, p); got != want {
		t.Errorf("after accept:\n%s\nwant:\n%s", got, want)
	}
	if err := SetProposed(p, "C", true, "agent:z"); err != nil {
		t.Fatal(err)
	}
	if err := SetProposed(p, "D", true, "agent:z"); err != nil {
		t.Fatal(err)
	}
	want = "concepts:\n  A: { kind: process }   # keep me\n  B:\n    kind: process\n  C: { kind: process, proposed: true, proposed-by: agent:z }\n  D:\n    kind: process\n    proposed: true\n    proposed-by: agent:z\n"
	if got := read(t, p); got != want {
		t.Errorf("after re-propose:\n%s\nwant:\n%s", got, want)
	}
	if err := RemoveConcept(p, "D"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveConcept(p, "A"); err != nil {
		t.Fatal(err)
	}
	want = "concepts:\n  B:\n    kind: process\n  C: { kind: process, proposed: true, proposed-by: agent:z }\n"
	if got := read(t, p); got != want {
		t.Errorf("after remove:\n%s\nwant:\n%s", got, want)
	}
	if err := RemoveConcept(p, "Nope"); err == nil {
		t.Error("removing a missing concept must fail")
	}
}

func TestRemoveMultiLineEntries(t *testing.T) {
	p := tmp(t, flowFile)
	if err := RemoveConcept(p, "Throughput"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveConcept(p, "Wide"); err != nil {
		t.Fatal(err)
	}
	got := read(t, p)
	if strings.Contains(got, "Second line") || strings.Contains(got, "internal: true,") || !strings.Contains(got, "relations:\n  produces") {
		t.Errorf("multi-line extents wrong:\n%s", got)
	}
}

func TestSetField(t *testing.T) {
	src := "concepts:\n" +
		"  A:          { kind: entity, doc: \"Old, with comma.\" }   # keep\n" +
		"  B:          { kind: entity }\n" +
		"  C:\n    kind: process\n    doc: Old block doc.   # note\n" +
		"  D:\n    kind: process\n"
	p := tmp(t, src)
	for _, c := range []struct{ name, key, val string }{
		{"A", "doc", Quote("New, also with comma.")},
		{"B", "doc", Quote("Added.")},
		{"B", "aliases", List([]string{"bee", "big b"})},
		{"C", "doc", Quote("New block doc.")},
		{"D", "aliases", List([]string{"dee"})},
	} {
		if err := SetField(p, c.name, c.key, c.val); err != nil {
			t.Fatalf("%s.%s: %v", c.name, c.key, err)
		}
	}
	want := "concepts:\n" +
		"  A:          { kind: entity, doc: \"New, also with comma.\" }   # keep\n" +
		"  B:          { kind: entity, doc: \"Added.\", aliases: [bee, big b] }\n" +
		"  C:\n    kind: process\n    doc: \"New block doc.\"   # note\n" +
		"  D:\n    kind: process\n    aliases: [dee]\n"
	if got := read(t, p); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if v, ok, _ := Get(p, "B", "aliases"); !ok || v != "bee, big b" {
		t.Errorf("Get = %q %v", v, ok)
	}
}

// TestPublishCopy: the published copy keeps only the selected concepts, in
// the owner's formatting, and states the tier (issues #129, #96).
func TestPublishCopy(t *testing.T) {
	src := tmp(t, flowFile)
	dst := filepath.Join(t.TempDir(), "out.gloss.yaml")
	keep := map[string]bool{"CatalogIntake": true, "Throughput": true}
	if err := PublishCopy(src, dst, func(n string) bool { return keep[n] }, "implementation"); err != nil {
		t.Fatal(err)
	}
	want := `# ontology/circ.gloss.yaml  (implementation tier)
namespace: circ
version: 0.3.0
tier: implementation
concepts:
  CatalogIntake:              { kind: process, realizes: lib:Cataloging }
  Throughput:
    kind: measure
    criticality: low                 # telemetry
    realizes: lib:LoanPeriod
    doc: |
      Multi-line block scalar.
      Second line.
relations:
  produces: { lift: range-lcs }
`
	if got := read(t, dst); got != want {
		t.Fatalf("published copy:\n%s\nwant:\n%s", got, want)
	}
	if read(t, src) != flowFile {
		t.Fatal("source file changed")
	}
	// An existing tier: line is corrected in place; a comment above a dropped entry goes with it.
	src = tmp(t, "tier: core\nconcepts:\n  A: { kind: entity }\n  # private helper\n  B: { kind: process }\n")
	if err := PublishCopy(src, dst, func(n string) bool { return n == "A" }, "implementation"); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t, dst), "tier: implementation\nconcepts:\n  A: { kind: entity }\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestAddConceptBinding: gloss bind writes a published value with the code
// it is bound to, in the documented field order (#272).
func TestAddConceptBinding(t *testing.T) {
	p := tmp(t, "namespace: tide\nconcepts:\n  Other: { kind: process, internal: true }\n")
	spec := ConceptSpec{Kind: "constraint", Realizes: []string{"claim:AutoApprovalLimit"}, PublishedAs: "claim:AutoApprovalLimit",
		Value: "5000", ValueFrom: &ValueFrom{Path: "services/assessment/settings.py", Symbol: "AUTO_APPROVE_LIMIT_EUR"}, Doc: "AUTO_APPROVE_LIMIT_EUR in the assessment settings."}
	if err := AddConcept(p, "AutoApprovalRule", spec); err != nil {
		t.Fatal(err)
	}
	want := "namespace: tide\nconcepts:\n  Other: { kind: process, internal: true }\n" +
		"  AutoApprovalRule:\n" +
		"    kind: constraint\n" +
		"    realizes: claim:AutoApprovalLimit\n" +
		"    published: { as: claim:AutoApprovalLimit }\n" +
		"    value: \"5000\"\n" +
		"    value-from: { path: services/assessment/settings.py, symbol: AUTO_APPROVE_LIMIT_EUR }\n" +
		"    doc: \"AUTO_APPROVE_LIMIT_EUR in the assessment settings.\"\n"
	if got := read(t, p); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	// The same fields on an existing entry, one key each, keep its style.
	if err := SetField(p, "Other", "value-from", ValueFrom{Path: "a b.py", Symbol: "X"}.Render()); err != nil {
		t.Fatal(err)
	}
	if got := read(t, p); !strings.Contains(got, `  Other: { kind: process, internal: true, value-from: { path: a b.py, symbol: X } }`) {
		t.Errorf("SetField value-from on a flow entry:\n%s", got)
	}
}
