// @gloss verifies gmi:EvidenceHarvester scope=file

package harvest

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
		"rule-sentence A patron must never hold more than five items at once.": "",
		"rule-sentence A hold expires after seven days and cannot be renewed.": "",
		"rule-sentence An overdue loan cannot be renewed by the patron.":       "services/circulation",
		"ui-string Loan report":   "apps/dashboard",
		"ui-string Overdue loans": "apps/dashboard",
		"module Loan_Report":      "apps/dashboard",
		"transition overdue":      "services/circulation",
		"transition RETURNED":     "services/circulation",
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
	if e := got["column patron_id"]; e.Source.Path != "services/circulation/loans/models.py" || e.Source.Line != 21 || e.Context[0] != "table:loans" {
		t.Errorf("patron_id = %+v", e)
	}
	if e := got["transition overdue"]; strings.Join(e.Context, " ") != "attr:status of:LoanPatch in:mark_overdue" {
		t.Errorf("overdue transition context %v", e.Context)
	}
	if e := got["transition RETURNED"]; strings.Join(e.Context, " ") != "attr:status of:loan in:return_loan" {
		t.Errorf("returned transition context %v", e.Context)
	}
	rules := 0
	for _, e := range ev {
		if e.Signal == "rule-sentence" {
			rules++
		}
	}
	if rules != 4 {
		t.Errorf("%d rule sentences, want 4 (one per rule, none for a sentence without a rule word)", rules)
	}
	if e := got["rule-sentence A patron must never hold two loans of one copy."]; e.Source.Line != 1 {
		t.Errorf("module docstring rule: %+v", e)
	}
	if e := got["value-set BLOCKING_STATUSES"]; strings.Join(e.Context, " ") != "member:OVERDUE member:lost" {
		t.Errorf("BLOCKING_STATUSES = %+v", e)
	}
	if e := got["value-set FORMATS"]; strings.Join(e.Context, " ") != "member:book member:dvd" {
		t.Errorf("FORMATS = %+v", e)
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

func TestSentencesAndTransitions(t *testing.T) {
	got := sentences("Keys are encrypted, e.g. with Fernet v1.2 at rest. The API never returns a key! Is it? 3 retries at most.")
	want := []string{"Keys are encrypted, e.g. with Fernet v1.2 at rest.", "The API never returns a key!", "Is it?", "3 retries at most."}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("sentences = %q", got)
	}
	if rs := ruleSentences("The API never returns a key. Keys are stored. A session holds at most one turn."); len(rs) != 2 {
		t.Errorf("ruleSentences = %q", rs)
	}
	cases := []struct{ line, value, attr, of string }{
		{`        row.state = SessionState.VOIDED`, "VOIDED", "state", "row"},
		{`    s.Status = StatusClosed`, "StatusClosed", "Status", "s"},
		{`        self._update(row.id, SessionPatch(state="submitted", submitted_at=now))`, "submitted", "state", "SessionPatch"},
		{`UPDATE sessions SET state = 'graded' WHERE id = 1;`, "graded", "state", ""},
		{`    item.status = "deleted"`, "deleted", "status", "item"},
		{`        self.status = status`, "", "", ""},
		{`    row.state = None`, "", "", ""},
		{`    if row.state == "reading":`, "", "", ""},
	}
	for _, c := range cases {
		v, a, o := transition(c.line)
		if v != c.value || (v != "" && (strings.ToLower(a) != strings.ToLower(c.attr) || o != c.of)) {
			t.Errorf("transition(%q) = %q %q %q, want %q %q %q", c.line, v, a, o, c.value, c.attr, c.of)
		}
	}
}

// TestReference: the same vocabulary in Turtle, RDF/XML and JSON-LD reads
// as the same terms, English labels first, with is-a from subClassOf and
// part-of from BFO's part of; a restriction is not a parent (#203).
func TestReference(t *testing.T) {
	want := []string{
		"Lending Agreement is-a [https://ref.example/lending#Loan] part-of []",
		"Loan Item is-a [] part-of [https://ref.example/lending#Loan]",
		"Loan is-a [] part-of []",
		"Overdue Loan is-a [https://ref.example/lending#Loan] part-of []",
		"Patron is-a [] part-of []",
	}
	for _, f := range []string{"lending.ttl", "lending.rdf", "lending.jsonld"} {
		terms, err := ReadReference(filepath.Join("testdata", "reference", f))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		var got []string
		for _, rt := range terms {
			got = append(got, fmt.Sprintf("%s is-a %v part-of %v", rt.Label, rt.Parents, rt.Wholes))
		}
		sort.Strings(got)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("%s:\n%s\nwant\n%s", f, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
	ev, err := Reference([]string{"testdata/reference/lending.ttl"})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ev {
		if err := e.Validate(); err != nil || e.Signal != "reference-term" || e.Source.Family != FamilyReference || e.Source.Locator == "" {
			t.Errorf("%+v: %v", e, err)
		}
	}
}

// TestReferenceInClusters: a term only the reference names is no candidate;
// a reference label picks the cluster name when the repository's own
// surfaces disagree; reference structure links two candidates that both
// have evidence of their own, and nothing else.
func TestReferenceInClusters(t *testing.T) {
	ref, err := Reference([]string{"testdata/reference/lending.ttl"})
	if err != nil {
		t.Fatal(err)
	}
	own := []Evidence{
		newEvidence("", "doc-heading", "Patrons", "docs/lending.md", 3),
		newEvidence("", "type-decl", "Patron", "app/patron.py", 1),
		newEvidence("", "type-decl", "Loan", "app/loan.py", 1),
		newEvidence("", "type-decl", "OverdueLoan", "app/loan.py", 9),
		newEvidence("", "function-decl", "sign_lending_agreement", "app/loan.py", 20),
	}
	by := func(cs []Cluster) map[string]Cluster {
		m := map[string]Cluster{}
		for _, c := range cs {
			m[c.Key] = c
		}
		return m
	}
	before := by(Clusters(own))
	patron, loan, overdue := Key("Patron"), Key("Loan"), Key("OverdueLoan")
	if before[patron].Name != "Patrons" {
		t.Fatalf("without a reference the doc names it: %q", before[patron].Name)
	}
	after := by(Clusters(append(append([]Evidence{}, own...), ref...)))
	if after[patron].Name != "Patron" {
		t.Errorf("with the reference: %q, want Patron", after[patron].Name)
	}
	if _, ok := after[Key("Lending Agreement")]; ok {
		t.Error("lending agreement is named only by the reference, yet it is a candidate")
	}
	if _, ok := after[Key("Loan Item")]; ok {
		t.Error("loan item is named only by the reference, yet it is a candidate")
	}
	if !slices.Contains(after[overdue].Related, "is-a:"+loan) || !slices.Contains(after[loan].Related, "has-subtype:"+overdue) {
		t.Errorf("overdue loan related %v, loan related %v", after[overdue].Related, after[loan].Related)
	}
	for _, r := range after[loan].Related {
		if strings.Contains(r, "lending") || strings.Contains(r, "item") {
			t.Errorf("loan related %v names a reference-only term", after["loan"].Related)
		}
	}
}

// TestTopics: queue, event and cron names in code, Terraform and a
// Kubernetes CronJob are topic evidence with their kind; a lower-case UI
// event is not (#203).
func TestTopics(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("worker/returns.py", "QUEUE_NAME = \"overdue-notices\"\n\ndef on_return(bus, loan):\n    bus.publish(\"loan.returned\", loan)\n    button.emit(\"click\")\n")
	write("infra/main.tf", "resource \"aws_sqs_queue\" \"hold_ready\" {\n  name = \"hold-ready\"\n}\n")
	write("deploy/reminders.yaml", "apiVersion: batch/v1\nkind: CronJob\nmetadata:\n  name: send-reminders\nspec:\n  schedule: \"0 9 * * *\"\n")
	ev, err := Harvest(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range ev {
		if e.Signal == "topic" {
			got = append(got, e.Surface+" "+e.Context[0])
			if e.Source.Family != FamilyCode {
				t.Errorf("%+v: family", e)
			}
		}
	}
	sort.Strings(got)
	if want := []string{"hold_ready kind:queue", "loan.returned kind:event", "overdue-notices kind:queue", "send-reminders kind:cron"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("topics = %v, want %v", got, want)
	}
}

// TestGitSeen: every surface is dated from the history. A term renamed
// away carries the commit that added it and the commit that retired it; a
// current one, the commit that added it and HEAD (#203).
func TestGitSeen(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	git := func(date string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date+"T12:00:00Z", "GIT_COMMITTER_DATE="+date+"T12:00:00Z")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(rel, body string) {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("2024-01-02", "init", "-q")
	write("borrower.py", "class Borrower:\n    card_number: str\n    branch: str\n    email: str\n")
	git("2024-01-02", "add", ".")
	git("2024-01-02", "commit", "-q", "-m", "first")
	git("2024-03-04", "mv", "borrower.py", "patron.py")
	write("patron.py", "class Patron:\n    card_number: str\n    branch: str\n    email: str\n")
	git("2024-03-04", "add", ".")
	git("2024-03-04", "commit", "-q", "-m", "rename")
	write("notes.md", "# Notes\n")
	git("2024-05-06", "add", ".")
	git("2024-05-06", "commit", "-q", "-m", "later")
	ev, err := Harvest(dir, Options{Git: true})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range ev {
		got[e.Signal+" "+e.Surface] = e.FirstSeen + ".." + e.LastSeen
	}
	if s := got["rename borrower"]; s != "2024-01-02..2024-03-04" {
		t.Errorf("retired borrower seen %q", s)
	}
	if s := got["type-decl Patron"]; s != "2024-03-04..2024-05-06" {
		t.Errorf("current Patron seen %q (all: %v)", s, got)
	}
	var cl Cluster
	for _, c := range Clusters(ev) {
		if c.Key == Key("borrower") {
			cl = c
		}
	}
	if cl.FirstSeen != "2024-01-02" || cl.LastSeen != "2024-03-04" {
		t.Errorf("borrower cluster seen %s..%s", cl.FirstSeen, cl.LastSeen)
	}
	noGit, _ := Harvest(dir, Options{})
	for _, e := range noGit {
		if e.FirstSeen != "" || e.LastSeen != "" {
			t.Errorf("without git: %+v", e)
		}
	}
}
