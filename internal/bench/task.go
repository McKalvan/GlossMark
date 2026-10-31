// @gloss implements gmi:AgentBenchmark scope=file

// Package bench is the agent benchmark (issue #271): a task is a realistic
// ticket against one of the showcase repositories with the domain pitfalls it
// tests, and the scorer compares the tree an agent leaves behind with the
// tree it started from, in the terms the gate, the drift report and the
// linter already use. bench/run.sh drives it; docs/BENCHMARK.md describes
// the method.
package bench

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Task is one benchmark ticket, read from bench/tasks/<id>.yaml.
type Task struct {
	// ID names the task; it defaults to the file's base name.
	ID string `yaml:"id"`
	// Repo is the workspace the ticket is against, relative to the
	// glossmark-examples checkout (showcase/claims-events).
	Repo  string `yaml:"repo"`
	Title string `yaml:"title"`
	// Prompt is the ticket as the agent receives it, in the words a
	// product owner would use: it never mentions GlossMark.
	Prompt string `yaml:"prompt"`
	// Pitfalls document what the ticket tests; the assertions check them.
	Pitfalls []Pitfall `yaml:"pitfalls"`
	// Assertions are the machine-checkable expectations on the result.
	// The standard set (check-clean, no-new-finding, no-unproposed-concept,
	// no-duplicate-concept) is added unless Defaults is false.
	Assertions []Assertion `yaml:"assertions"`
	Defaults   *bool       `yaml:"defaults,omitempty"`
}

// Pitfall is one domain trap the ticket sets: a rule that must not be
// weakened, a concept that must be reused rather than duplicated, a
// published value that must follow the code, a deprecated term.
type Pitfall struct {
	Kind    string `yaml:"kind"` // rule | reuse | published-value | deprecated | boundary
	Concept string `yaml:"concept,omitempty"`
	Note    string `yaml:"note,omitempty"`
}

// Pitfall kinds.
var pitfallKinds = map[string]bool{"rule": true, "reuse": true, "published-value": true, "deprecated": true, "boundary": true}

// Assertion is one machine-checkable expectation on the resulting tree.
// Each kind reads the fields it needs; the rest stay empty.
type Assertion struct {
	Kind string `yaml:"kind"`
	// Why says what a failure means in domain terms; it is echoed in the
	// result so a reader of the report needs no task file.
	Why string `yaml:"why,omitempty"`
	// link, no-link: an inline marker with Relation (any when empty) to
	// Concept, on a file under Under (any when empty).
	Relation string `yaml:"relation,omitempty"`
	Concept  string `yaml:"concept,omitempty"`
	Under    string `yaml:"under,omitempty"`
	// no-new-concept, no-unproposed-concept: CURIEs that may appear.
	Allow []string `yaml:"allow,omitempty"`
	// forbidden-terms: words that must not name anything new; Paths adds
	// changed files (globs) whose content is scanned too.
	Terms []string `yaml:"terms,omitempty"`
	Paths []string `yaml:"paths,omitempty"`
	// file-matches, file-unchanged, file-changed: Path; Pattern is a regexp.
	Path    string `yaml:"path,omitempty"`
	Pattern string `yaml:"pattern,omitempty"`
	// no-new-finding: Kinds (default violation, value-drift, dangling),
	// narrowed to Boundary or Concept when set. finding-closed: ID.
	Kinds    []string `yaml:"kinds,omitempty"`
	Boundary string   `yaml:"boundary,omitempty"`
	ID       string   `yaml:"id,omitempty"`
	// concept-value: the ontology value of Concept matches Pattern.
	// gate: Is names the class the change must get, Max the strictest it may.
	Is  string `yaml:"is,omitempty"`
	Max string `yaml:"max,omitempty"`
	// check-clean: Warnings makes a warning fail it too.
	Warnings bool `yaml:"warnings,omitempty"`
}

// Assertion kinds and the category each scores under.
const (
	CategoryRule    = "rule"    // a domain rule or a published value was broken
	CategoryConcept = "concept" // a concept was invented, duplicated or not reused
	CategoryNaming  = "naming"  // the wrong term names something
	CategoryGate    = "gate"    // what the gate said about the change
)

var assertionKinds = map[string]string{
	"check-clean":           CategoryRule,
	"no-new-finding":        CategoryRule,
	"finding-closed":        CategoryRule,
	"file-matches":          CategoryRule,
	"file-unchanged":        CategoryRule,
	"file-changed":          CategoryRule,
	"concept-value":         CategoryRule,
	"no-unproposed-concept": CategoryConcept,
	"no-duplicate-concept":  CategoryConcept,
	"no-new-concept":        CategoryConcept,
	"link":                  CategoryConcept,
	"no-link":               CategoryConcept,
	"forbidden-terms":       CategoryNaming,
	"gate":                  CategoryGate,
}

// Category returns the score category of an assertion kind ("" when unknown).
func Category(kind string) string { return assertionKinds[kind] }

// Kinds lists the assertion kinds, sorted.
func Kinds() []string {
	ks := make([]string, 0, len(assertionKinds))
	for k := range assertionKinds {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// defaultAssertions are checked on every task unless defaults: false.
func defaultAssertions() []Assertion {
	return []Assertion{
		{Kind: "check-clean", Why: "the markers the agent wrote must lint"},
		{Kind: "no-new-finding", Why: "no boundary exceeds its manifest, no published value drifts, nothing dangles"},
		{Kind: "no-unproposed-concept", Why: "a concept the agent needs ships as a proposal, never as a hand edit"},
		{Kind: "no-duplicate-concept", Why: "an existing concept is reused, not named again"},
	}
}

// Effective returns the task's assertions with the standard set in front.
func (t *Task) Effective() []Assertion {
	if t.Defaults != nil && !*t.Defaults {
		return t.Assertions
	}
	return append(defaultAssertions(), t.Assertions...)
}

// ReadTask reads and validates one task file.
func ReadTask(path string) (*Task, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Task
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	if t.ID == "" {
		t.ID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	return &t, nil
}

// ReadTasks reads every *.yaml under dir, sorted by file name.
func ReadTasks(dir string) ([]*Task, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var ts []*Task
	for _, p := range paths {
		t, err := ReadTask(p)
		if err != nil {
			return nil, err
		}
		ts = append(ts, t)
	}
	if len(ts) == 0 {
		return nil, fmt.Errorf("no task files under %s", dir)
	}
	return ts, nil
}

// Validate checks the fields each assertion kind needs, so a typo in a
// task file fails before an agent has run for an hour.
func (t *Task) Validate() error {
	var errs []string
	add := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }
	if t.Repo == "" {
		add("repo: is required (showcase/<name>)")
	}
	if strings.TrimSpace(t.Prompt) == "" {
		add("prompt: is required")
	}
	if strings.Contains(strings.ToLower(t.Prompt), "glossmark") || strings.Contains(t.Prompt, "@gloss") {
		add("prompt: must not mention GlossMark; the conditions add that context, the ticket does not")
	}
	for i, p := range t.Pitfalls {
		if !pitfallKinds[p.Kind] {
			add("pitfalls[%d]: unknown kind %q", i, p.Kind)
		}
	}
	for i, a := range t.Assertions {
		if _, ok := assertionKinds[a.Kind]; !ok {
			add("assertions[%d]: unknown kind %q (one of %s)", i, a.Kind, strings.Join(Kinds(), ", "))
			continue
		}
		need := func(ok bool, field string) {
			if !ok {
				add("assertions[%d] %s: %s is required", i, a.Kind, field)
			}
		}
		switch a.Kind {
		case "link", "no-link":
			need(a.Concept != "", "concept")
		case "forbidden-terms":
			need(len(a.Terms) > 0, "terms")
		case "file-matches":
			need(a.Path != "", "path")
			need(a.Pattern != "", "pattern")
		case "file-unchanged", "file-changed":
			need(a.Path != "", "path")
		case "finding-closed":
			need(a.ID != "", "id")
		case "concept-value":
			need(a.Concept != "", "concept")
			need(a.Pattern != "", "pattern")
		case "gate":
			need(a.Is != "" || a.Max != "", "is or max")
			for _, c := range []string{a.Is, a.Max} {
				if c != "" && c != "AUTO" && c != "SOFT" && c != "HARD" {
					add("assertions[%d] gate: %q is not AUTO, SOFT or HARD", i, c)
				}
			}
		}
		if a.Pattern != "" {
			if _, err := regexp.Compile(a.Pattern); err != nil {
				add("assertions[%d] %s: pattern: %v", i, a.Kind, err)
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}
