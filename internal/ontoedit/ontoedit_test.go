// @gloss verifies gmi:OntologyEditor scope=file

package ontoedit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const flowFile = `# ontology/impl.gloss.yaml  (implementation tier)
namespace: impl
version: 0.3.0
concepts:
  HorseBuilder:               { kind: process, realizes: zoo:Breeding }
  HoofAssembler:              { kind: process, part-of: HorseBuilder, internal: true }     # unpublished: private to this repo
  Throughput:
    kind: measure
    criticality: low                 # telemetry
    realizes: zoo:ProductionCapacity
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
	p := filepath.Join(t.TempDir(), "impl.gloss.yaml")
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
	if err := AddConcept(p, "Gelding", ConceptSpec{Kind: "process", Realizes: []string{"zoo:Breeding"}, Doc: "Castration step: colts, not fillies.", Aliases: []string{"castration", "cut-off step"}, Proposed: true, ProposedBy: "agent:bot@m"}); err != nil {
		t.Fatal(err)
	}
	got := read(t, p)
	want := strings.Replace(flowFile, "  }\nrelations:", "  }\n"+
		"  Gelding:\n"+
		"    kind: process\n"+
		"    realizes: zoo:Breeding\n"+
		"    doc: \"Castration step: colts, not fillies.\"\n"+
		"    aliases: [castration, cut-off step]\n"+
		"    proposed: true\n"+
		"    proposed-by: agent:bot@m\n"+
		"relations:", 1)
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if err := AddConcept(p, "Gelding", ConceptSpec{Kind: "process"}); err == nil {
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
