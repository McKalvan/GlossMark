// @gloss verifies gmi:AgentBenchmark scope=file

package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture writes a small two-tier workspace: a shipping service whose
// manifest declares a subscription nothing implements yet (the ticket) and
// a published weight limit bound to a constant (the pitfall).
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "glossmark.yaml", "owns:\n  sh: { iri: https://x/sh/, tier: core }\n  shi: { iri: https://x/shi/, tier: implementation }\n")
	write(t, root, "ontology/sh.gloss.yaml", `namespace: sh
concepts:
  Order: { kind: entity, doc: "What a customer asked for." }
  Shipping: { kind: process, consumes: [Order], doc: "Sending an order." }
  WeightLimit: { kind: constraint, constrains: [Shipping], criticality: high, doc: "The heaviest parcel we ship." }
  DispatchFailed: { kind: event, aliases: [dispatch failure], doc: "The carrier refused a parcel." }
`)
	write(t, root, "ontology/shi.gloss.yaml", `namespace: shi
concepts:
  ShipHandler: { kind: process, realizes: sh:Shipping, doc: "ship()." }
  WeightRule:
    kind: constraint
    realizes: sh:WeightLimit
    published: { as: sh:WeightLimit }
    value: "30 kg"
    value-from: { path: svc/limits.py, symbol: MAX_KG }
    doc: "MAX_KG."
`)
	write(t, root, ".gloss.yaml", "boundary: b:root\nlevel: system\nrealizes: inherit\n")
	write(t, root, "svc/.gloss.yaml", "boundary: b:svc\nlevel: container\npart-of: b:root\nrealizes: [sh:Shipping, shi:WeightRule]\nsubscribes: [sh:DispatchFailed]\nconsumes: [sh:Order]\ndefault-namespace: shi\n")
	write(t, root, "svc/limits.py", "# @gloss implements .WeightRule\nMAX_KG = 30\n")
	write(t, root, "svc/ship.py", "# @gloss implements .ShipHandler\n# @gloss consumes sh:Order\ndef ship(order): pass\n")
	write(t, root, "svc/test_ship.py", "# @gloss verifies shi:ShipHandler, shi:WeightRule\ndef test_ship(): pass\n")
	return root
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// copyTree copies a fixture so a head can be edited beside its base.
func copyTree(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	files, err := hashTree(src)
	if err != nil {
		t.Fatal(err)
	}
	for rel, b := range files {
		write(t, dst, rel, string(b))
	}
	return dst
}

func ticket() *Task {
	return &Task{
		ID: "notify-on-dispatch-failed", Repo: "fixture", Prompt: "When the carrier refuses a parcel, tell the customer.",
		Pitfalls: []Pitfall{{Kind: "reuse", Concept: "sh:DispatchFailed"}, {Kind: "published-value", Concept: "shi:WeightRule"}},
		Assertions: []Assertion{
			{Kind: "link", Relation: "subscribes", Concept: "sh:DispatchFailed", Under: "svc"},
			{Kind: "finding-closed", ID: "gap:b:svc:subscribes:sh:DispatchFailed"},
			{Kind: "forbidden-terms", Terms: []string{"ShipmentFailed", "DeliveryFailed"}},
			{Kind: "file-unchanged", Path: "svc/limits.py"},
			{Kind: "gate", Max: "SOFT"},
		},
	}
}

func results(s *Score) map[string]Result {
	m := map[string]Result{}
	for _, r := range s.Results {
		m[r.Kind] = r
	}
	return m
}

func TestScoreGoodDiff(t *testing.T) {
	base := fixture(t)
	head := copyTree(t, base)
	// The right change: a consumer that subscribes to the existing event.
	write(t, head, "svc/notify.py", "# @gloss subscribes sh:DispatchFailed\ndef on_dispatch_failed(evt): pass\n")
	s, err := ScoreDirs(ticket(), base, head, Options{Condition: "glossmark"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Mistakes.Total != 0 || s.Passed != s.Total {
		t.Errorf("good diff: %d mistakes, %d/%d passed\n%s", s.Mistakes.Total, s.Passed, s.Total, RenderText(s))
	}
	if s.Total != 9 {
		t.Errorf("%d assertions, want 4 default + 5 from the task", s.Total)
	}
	if s.Gate == "HARD" {
		t.Errorf("gate %s for a link inside the manifest's declaration", s.Gate)
	}
	if len(s.NewConcepts) != 0 || len(s.NewLinks) != 1 || s.NewLinks[0] != "svc/notify.py subscribes sh:DispatchFailed" {
		t.Errorf("new concepts %v, new links %v", s.NewConcepts, s.NewLinks)
	}
	if len(s.ChangedFiles) != 1 || s.ChangedFiles[0] != "svc/notify.py" {
		t.Errorf("changed files %v", s.ChangedFiles)
	}
	if !contains(s.ClosedFindings, "gap:b:svc:subscribes:sh:DispatchFailed") {
		t.Errorf("gap not closed: %v", s.ClosedFindings)
	}
}

func TestScoreBadDiff(t *testing.T) {
	base := fixture(t)
	head := copyTree(t, base)
	// Every pitfall at once: a NEW: concept that renames the existing event,
	// a hand-added concept with an alias the ontology already has, and the
	// published constant changed without its value.
	write(t, head, "svc/notify.py", "# @gloss? subscribes sh:NEW:ShipmentFailed conf=0.8\ndef on_shipment_failed(evt): pass\n")
	write(t, head, "ontology/sh.gloss.yaml", `namespace: sh
concepts:
  Order: { kind: entity, doc: "What a customer asked for." }
  Shipping: { kind: process, consumes: [Order], doc: "Sending an order." }
  WeightLimit: { kind: constraint, constrains: [Shipping], criticality: high, doc: "The heaviest parcel we ship." }
  DispatchFailed: { kind: event, aliases: [dispatch failure], doc: "The carrier refused a parcel." }
  DeliveryFailed: { kind: event, aliases: [dispatch failure], doc: "The carrier refused a parcel (again)." }
`)
	write(t, head, "svc/limits.py", "# @gloss implements .WeightRule\nMAX_KG = 50\n")
	s, err := ScoreDirs(ticket(), base, head, Options{Condition: "bare"})
	if err != nil {
		t.Fatal(err)
	}
	r := results(s)
	for _, kind := range []string{"link", "finding-closed", "forbidden-terms", "file-unchanged", "gate", "no-new-finding", "no-unproposed-concept", "no-duplicate-concept"} {
		if r[kind].OK {
			t.Errorf("%s passed on the bad diff: %s", kind, r[kind].Msg)
		}
	}
	if !r["check-clean"].OK {
		t.Errorf("check-clean failed: a NEW: marker is a proposal, not a lint error: %s", r["check-clean"].Msg)
	}
	want := Mistakes{Rule: 3, Concept: 3, Naming: 1, Gate: 1, Total: 7}
	if s.Mistakes != want {
		t.Errorf("mistakes %+v, want %+v\n%s", s.Mistakes, want, RenderText(s))
	}
	if s.Gate != "HARD" {
		t.Errorf("gate %s: a drifted published value is HARD", s.Gate)
	}
	states := map[string]NewConcept{}
	for _, c := range s.NewConcepts {
		states[c.Curie] = c
	}
	if c := states["sh:DeliveryFailed"]; c.State != "added" || c.DuplicateOf != "sh:DispatchFailed" {
		t.Errorf("sh:DeliveryFailed: %+v", c)
	}
	if c := states["sh:ShipmentFailed"]; c.State != "rogue" {
		t.Errorf("sh:ShipmentFailed: %+v", c)
	}
	if !strings.Contains(r["no-new-finding"].Msg, "value-drift") {
		t.Errorf("no-new-finding: %s", r["no-new-finding"].Msg)
	}
	if !strings.Contains(r["forbidden-terms"].Msg, `"ShipmentFailed"`) || !strings.Contains(r["forbidden-terms"].Msg, `"DeliveryFailed"`) {
		t.Errorf("forbidden-terms: %s", r["forbidden-terms"].Msg)
	}
}

func TestReport(t *testing.T) {
	base := fixture(t)
	good := copyTree(t, base)
	write(t, good, "svc/notify.py", "# @gloss subscribes sh:DispatchFailed\ndef on_dispatch_failed(evt): pass\n")
	bad := copyTree(t, base)
	write(t, bad, "svc/notify.py", "# @gloss? subscribes sh:NEW:ShipmentFailed conf=0.8\n")
	g, err := ScoreDirs(ticket(), base, good, Options{Condition: "glossmark"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ScoreDirs(ticket(), base, bad, Options{Condition: "bare"})
	if err != nil {
		t.Fatal(err)
	}
	md := RenderMarkdown([]*Score{g, b, b})
	for _, want := range []string{
		"| notify-on-dispatch-failed | bare | 2 |",
		"| notify-on-dispatch-failed | glossmark | 1 | 0.00 |",
		"| bare | 2 |",
		"| glossmark | 1 | 0.00 |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report lacks %q:\n%s", want, md)
		}
	}
	if b.Mistakes.Total == 0 {
		t.Errorf("bad diff scored no mistakes:\n%s", RenderText(b))
	}
}

// TestTaskFiles: every task in bench/tasks parses, validates and names one
// of the showcase repositories.
func TestTaskFiles(t *testing.T) {
	dir, err := filepath.Abs(filepath.Join("..", "..", "bench", "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	ts, err := ReadTasks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) < 8 {
		t.Errorf("%d tasks, the benchmark wants at least 8", len(ts))
	}
	for _, task := range ts {
		if !strings.HasPrefix(task.Repo, "showcase/") {
			t.Errorf("%s: repo %q is not a showcase repository", task.ID, task.Repo)
		}
		if len(task.Pitfalls) == 0 || len(task.Assertions) == 0 {
			t.Errorf("%s: a task names its pitfalls and asserts on them", task.ID)
		}
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		task Task
		want string
	}{
		{"no repo", Task{Prompt: "x"}, "repo: is required"},
		{"mentions the tool", Task{Repo: "showcase/x", Prompt: "run gloss check"}, ""},
		{"mentions GlossMark", Task{Repo: "showcase/x", Prompt: "add a @gloss marker"}, "must not mention GlossMark"},
		{"unknown kind", Task{Repo: "showcase/x", Prompt: "x", Assertions: []Assertion{{Kind: "exists"}}}, `unknown kind "exists"`},
		{"link without concept", Task{Repo: "showcase/x", Prompt: "x", Assertions: []Assertion{{Kind: "link"}}}, "concept is required"},
		{"bad gate", Task{Repo: "showcase/x", Prompt: "x", Assertions: []Assertion{{Kind: "gate", Max: "MEDIUM"}}}, "not AUTO, SOFT or HARD"},
		{"bad regexp", Task{Repo: "showcase/x", Prompt: "x", Assertions: []Assertion{{Kind: "file-matches", Path: "a", Pattern: "("}}}, "pattern:"},
		{"ok", Task{Repo: "showcase/x", Prompt: "x", Assertions: []Assertion{{Kind: "gate", Is: "HARD"}, {Kind: "forbidden-terms", Terms: []string{"a"}}}}, ""},
	}
	for _, c := range cases {
		err := c.task.Validate()
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: unexpected %v", c.name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
