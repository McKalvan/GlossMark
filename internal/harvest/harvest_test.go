// @gloss verifies gmi:EvidenceHarvester scope=file

package harvest

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func harvestLibrary(t *testing.T) []Evidence {
	t.Helper()
	ev, err := Harvest("testdata/library", Options{})
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

// TestHarvestSignals: every signal family is read from the fixture, each
// observation carries its unit and line, and what is not evidence is skipped.
func TestHarvestSignals(t *testing.T) {
	ev := harvestLibrary(t)
	got := map[string]Evidence{}
	for _, e := range ev {
		got[e.Signal+" "+e.Surface] = e
		if e.Source.Family != FamilyOf(e.Signal) || !strings.HasPrefix(e.ID, "ev:") {
			t.Errorf("%+v: family or id", e)
		}
	}
	want := map[string]string{ // signal surface -> unit
		"type-decl LoanRow":        "services/circulation",
		"table loans":              "services/circulation",
		"column patron_id":         "services/circulation",
		"foreign-key loans":        "services/circulation",
		"field loan_period_days":   "services/circulation",
		"enum-type LoanStatus":     "services/circulation",
		"enum-value OVERDUE":       "services/circulation",
		"enum-type Format":         "services/circulation",
		"enum-value dvd":           "services/circulation",
		"function-decl renew_loan": "services/circulation",
		"guard-message an overdue loan cannot be renewed": "services/circulation",
		"guard-message hold queue is not empty":           "services/circulation",
		"route /loans":                                    "services/circulation",
		"route /{loan_id}/renew":                          "services/circulation",
		"test-name overdue loan cannot be renewed":        "services/circulation",
		"table patrons":                                   "services/circulation",
		"column card_number":                              "services/circulation",
		"rename borrowers":                                "services/circulation",
		"module loans":                                    "services/circulation",
		"ui-string Place hold":                            "apps/opac",
		"ui-string Hold queue":                            "apps/opac",
		"type-decl HoldProps":                             "apps/opac",
		"doc-heading Loan period":                         "",
		"doc-definition Hold":                             "",
		"doc-phrase reading list":                         "",
		"glossary-entry Patron":                           "",
		"issue-title Fines for late returns":              "",
	}
	for k, unit := range want {
		e, ok := got[k]
		if !ok {
			t.Errorf("missing %s", k)
			continue
		}
		if e.Unit != unit {
			t.Errorf("%s: unit %q, want %q", k, e.Unit, unit)
		}
	}
	if e := got["column patron_id"]; e.Source.Path != "services/circulation/loans/models.py" || e.Source.Line != 19 || e.Context[0] != "table:loans" {
		t.Errorf("patron_id = %+v", e)
	}
	for _, k := range []string{"doc-heading Not a heading inside a fence", "function-decl _private_helper", "field local_counter", "field id", "function-decl test_overdue_loan_cannot_be_renewed"} {
		if _, ok := got[k]; ok {
			t.Errorf("unexpected %s", k)
		}
	}
}

// TestClusters: names come from the most authoritative exact surface,
// categories triangulate intent against what is built, a prose phrase needs
// to recur or be named in code, and renames and aliases are linked, not merged.
func TestClusters(t *testing.T) {
	cs := Clusters(harvestLibrary(t))
	by := map[string]Cluster{}
	for _, c := range cs {
		by[c.Key] = c
	}
	cases := []struct{ key, name, category string }{
		{"hold", "Hold", Corroborated},
		{"patron", "Patron", Corroborated},
		{"loan period", "Loan period", Corroborated},
		{"hold queu", "hold queue", Corroborated},
		{"interlibrary loan", "interlibrary loan", DocsOnly},
		{"plac hold", "Place hold", CodeOnlyVisible},
		{"renewal", "Renewal", CodeOnlyInternal},
	}
	for _, c := range cases {
		got, ok := by[c.key]
		if !ok {
			t.Errorf("no cluster %q", c.key)
			continue
		}
		if got.Name != c.name || got.Category != c.category {
			t.Errorf("%q: name %q category %s, want %q %s", c.key, got.Name, got.Category, c.name, c.category)
		}
	}
	if _, ok := by["read list"]; ok {
		t.Errorf("a phrase in one document and no code proposed a concept")
	}
	if p := by["patron"]; !contains(p.Related, "renamed-from:borrow") || !contains(p.Related, "alias:borrow") {
		t.Errorf("patron related = %v", p.Related)
	}
	if !contains(by["loan period"].NamedAs, "doc-phrase") || !contains(by["loan period"].NamedAs, "doc-heading") {
		t.Errorf("loan period named as %v", by["loan period"].NamedAs)
	}
	if cs[0].Key != "loan" {
		t.Errorf("first candidate %q, want the one named in the most ways", cs[0].Key)
	}
}

func TestKey(t *testing.T) {
	cases := map[string]string{
		"LoanRow":               "loan",
		"BaseExercise":          "exerci",
		"is_record":             "record",
		"session_id":            "session",
		"created_at":            "",
		"Overview":              "",
		"Open questions":        "",
		"/candidates/{cid}":     "candidat",
		"Record attempt":        "record attempt",
		"a b c d e f":           "",
		"How the gateway works": "gateway work",
	}
	for in, want := range cases {
		if got := Key(in); got != want {
			t.Errorf("Key(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidate(t *testing.T) {
	ok := Evidence{Signal: "doc-heading", Surface: "Renewals", Source: Source{Family: FamilyDocConnector, URI: "gdrive://1AbC", Locator: "heading:Renewals"}}
	if err := ok.Validate(); err != nil || ok.ID == "" {
		t.Errorf("connector heading: %v, id %q", err, ok.ID)
	}
	issue := Evidence{Signal: "issue-title", Surface: "Fines", Source: Source{URI: "https://tracker/1"}}
	if err := issue.Validate(); err != nil || issue.Source.Family != FamilyTracker {
		t.Errorf("issue: %v, family %q", err, issue.Source.Family)
	}
	bad := []Evidence{
		{Signal: "rumour", Surface: "x", Source: Source{Path: "a"}},
		{Signal: "table", Surface: "x", Source: Source{Family: FamilyDoc, Path: "a"}},
		{Signal: "table", Surface: " ", Source: Source{Path: "a"}},
		{Signal: "table", Surface: "loans"},
	}
	for _, e := range bad {
		if err := e.Validate(); err == nil {
			t.Errorf("%+v validated", e)
		}
	}
}

// TestGitRenames: a file renamed in the history is a retired surface of the
// new name; a move that keeps the name is not.
func TestGitRenames(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	body := "class Borrower:\n    card_number: str\n    branch: str\n    email: str\n"
	if err := os.WriteFile(filepath.Join(dir, "borrower.py"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "loan.py"), []byte("class Loan:\n    due_on: str\n    patron: str\n    item: str\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "first")
	if err := os.MkdirAll(filepath.Join(dir, "loans"), 0o755); err != nil {
		t.Fatal(err)
	}
	git("mv", "borrower.py", "patron.py")
	git("mv", "loan.py", "loans/loan.py")
	git("commit", "-q", "-m", "rename")
	ev, err := Harvest(dir, Options{Git: true})
	if err != nil {
		t.Fatal(err)
	}
	var renames []string
	for _, e := range ev {
		if e.Signal == "rename" {
			renames = append(renames, e.Surface+"->"+strings.Join(e.Context, ","))
			if e.Source.Rev == "" || e.Source.Family != FamilyHistory {
				t.Errorf("rename %+v", e)
			}
		}
	}
	if len(renames) != 1 || renames[0] != "borrower->renamed-to:patron,from:borrower.py" {
		t.Errorf("renames = %v", renames)
	}
}

// TestSchemaMatchesSignals: schemas/evidence.schema.json lists exactly the
// signals and families the harvester knows.
func TestSchemaMatchesSignals(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "schemas", "evidence.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		OneOf []struct {
			Properties struct {
				Signal struct {
					Enum []string `json:"enum"`
				} `json:"signal"`
				Source struct {
					Properties struct {
						Family struct {
							Enum []string `json:"enum"`
						} `json:"family"`
					} `json:"properties"`
				} `json:"source"`
			} `json:"properties"`
		} `json:"oneOf"`
	}
	if err := json.Unmarshal(b, &s); err != nil || len(s.OneOf) != 2 {
		t.Fatalf("schema: %v", err)
	}
	rec := s.OneOf[1].Properties
	want := Signals()
	sort.Strings(want)
	if strings.Join(rec.Signal.Enum, ",") != strings.Join(want, ",") {
		t.Errorf("schema signals %v, code %v", rec.Signal.Enum, want)
	}
	fams := map[string]bool{}
	for _, sg := range want {
		fams[FamilyOf(sg)] = true
	}
	fams[FamilyDocConnector] = true
	var wf []string
	for f := range fams {
		wf = append(wf, f)
	}
	sort.Strings(wf)
	if strings.Join(rec.Source.Properties.Family.Enum, ",") != strings.Join(wf, ",") {
		t.Errorf("schema families %v, code %v", rec.Source.Properties.Family.Enum, wf)
	}
}
