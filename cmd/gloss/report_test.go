// @gloss verifies gmi:ComplianceReporter scope=file

package main

import (
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/report"
)

// complianceTree is a workspace with one audited rule: a core constraint
// realized by an implementation constraint that publishes a value bound to
// code, an accepted implements link, a proposed link (rejected by the test),
// a verifying test, a source-url and a waiver on the rule (issue #269).
func complianceTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"glossmark.yaml": "owns:\n  c: { iri: https://x.test/c/, tier: core, owner: compliance-office }\n  i: { iri: https://x.test/i/, tier: implementation }\n" +
			"source-url: https://host.test/r/blob/main/{path}#L{line}\n" +
			"policy:\n  waivers:\n    - { finding: c:Limit, owner: audit-lead, expires: 2099-01-01, reason: \"accepted risk until the rewrite\" }\n",
		"ontology/c.gloss.yaml": "concepts:\n  Limit:\n    kind: constraint\n    criticality: high\n    doc: \"The largest amount <b>approved</b> without review.\"\n  Other: { kind: process }\n",
		"ontology/i.gloss.yaml": "concepts:\n  LimitRule:\n    kind: constraint\n    realizes: c:Limit\n    published: { as: c:Limit }\n    value: \"5\"\n    value-from: { path: app/settings.py, symbol: LIMIT }\n    doc: \"LIMIT in settings.\"\n",
		".gloss.yaml":           "boundary: o:app\nlevel: system\nlabel: The app\nrealizes: [c:Limit, c:Other]\ndefault-namespace: i\nowner: team-rules\n",
		"app/settings.py":       "# @gloss implements .LimitRule\nLIMIT = 5\n",
		"app/rules.py":          "# @gloss? implements c:Limit conf=0.6\ndef check(): pass\n\n# @gloss implements c:Other\ndef other(): pass\n",
		"tests/test_rules.py":   "# @gloss verifies c:Limit\ndef test_limit(): pass\n",
	})
	t.Setenv("GLOSS_ACTOR", "human:reviewer")
	if msg, code := capture(t, "build", "--root", root, "-q"); code != 0 {
		t.Fatalf("build: %d\n%s", code, msg)
	}
	if msg, code := capture(t, "reject", "--root", root, "app/rules.py:1", "--reason", "wrong concept"); code != 0 {
		t.Fatalf("reject: %d\n%s", code, msg)
	}
	return root
}

func TestReportComplianceJSON(t *testing.T) {
	root := complianceTree(t)
	out, code := capture(t, "report", "compliance", "--root", root, "--format", "json")
	if code != 0 {
		t.Fatalf("report: %d\n%s", code, out)
	}
	var rep report.Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	// The default selection: the constraint, not the process.
	if len(rep.Concepts) != 1 || rep.Concepts[0].CURIE != "c:Limit" {
		t.Fatalf("selected %+v, want c:Limit alone", rep.Concepts)
	}
	c := rep.Concepts[0]
	if c.Criticality != "high" || c.NamespaceOwner != "compliance-office" || c.Status != "implemented" {
		t.Errorf("concept: %+v", *c)
	}
	if len(c.Boundaries) != 1 || c.Boundaries[0].CURIE != "o:app" || c.Boundaries[0].Owner != "team-rules" {
		t.Errorf("boundaries: %+v", c.Boundaries)
	}
	if len(c.Realizers) != 1 || c.Realizers[0].CURIE != "i:LimitRule" {
		t.Errorf("realizers: %+v", c.Realizers)
	}
	// The accepted implements link through the realizer; the rejected
	// proposal is dropped from the graph and lives on in the history.
	if len(c.Artifacts) != 1 || c.Artifacts[0].Path != "app/settings.py" || c.Artifacts[0].Via != "i:LimitRule" || c.Artifacts[0].Status != "accepted" ||
		c.Artifacts[0].URL != "https://host.test/r/blob/main/app/settings.py#L1" {
		t.Errorf("artifacts: %+v", c.Artifacts)
	}
	if len(c.Tests) != 1 || c.Tests[0].Path != "tests/test_rules.py" || c.Tests[0].Relation != "verifies" {
		t.Errorf("tests: %+v", c.Tests)
	}
	if len(c.Values) != 1 || c.Values[0].Value != "5" || c.Values[0].BoundTo != "app/settings.py:2" || c.Values[0].Live != "LIMIT = 5" || c.Values[0].Status != "accepted" {
		t.Errorf("values: %+v", c.Values)
	}
	kinds := map[string]report.Event{}
	for _, ev := range c.History {
		kinds[ev.Kind] = ev
	}
	rej, ok := kinds["rejected"]
	if !ok || rej.Actor != "human:reviewer" || rej.Reason != "wrong concept" || rej.Path != "app/rules.py" {
		t.Errorf("history lacks the rejection: %+v", c.History)
	}
	if _, ok := kinds["proposed"]; !ok {
		t.Errorf("history lacks the proposal: %+v", c.History)
	}
	if _, ok := kinds["value-bound"]; !ok {
		t.Errorf("history lacks the value binding: %+v", c.History)
	}
	if len(c.Waivers) != 1 || c.Waivers[0].Owner != "audit-lead" || c.Waivers[0].Expires != "2099-01-01" || c.Waivers[0].Expired {
		t.Errorf("waivers: %+v", c.Waivers)
	}
	for i := 1; i < len(c.History); i++ {
		if c.History[i-1].When > c.History[i].When {
			t.Errorf("history out of order at %d: %+v", i, c.History)
		}
	}
}

// TestReportComplianceSelection: --concept names the rows, --criticality
// floors them, and a mistyped concept is an error with a hint.
func TestReportComplianceSelection(t *testing.T) {
	root := complianceTree(t)
	cases := []struct {
		args []string
		want []string
		code int
	}{
		{[]string{"--concept", "c:Other"}, []string{"c:Other"}, 0},
		{[]string{"--concept", "c:Other,c:Limit"}, []string{"c:Other", "c:Limit"}, 0},
		{[]string{"--criticality", "high"}, []string{"c:Limit", "i:LimitRule"}, 0},
		{[]string{"--criticality", "critical"}, nil, 0},
		{[]string{"--since", "2099-01-01"}, []string{"c:Limit"}, 0},
	}
	for _, tc := range cases {
		out, code := capture(t, append([]string{"report", "compliance", "--root", root, "--format", "json"}, tc.args...)...)
		if code != tc.code {
			t.Errorf("%v: exit %d, want %d\n%s", tc.args, code, tc.code, out)
			continue
		}
		var rep report.Report
		if err := json.Unmarshal([]byte(out), &rep); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		var got []string
		for _, c := range rep.Concepts {
			got = append(got, c.CURIE)
			if len(tc.args) > 0 && tc.args[0] == "--since" && len(c.History) != 0 {
				t.Errorf("--since 2099: history %+v, want none", c.History)
			}
		}
		if strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Errorf("%v: got %v, want %v", tc.args, got, tc.want)
		}
	}
	for _, bad := range [][]string{
		{"--concept", "c:Limti"},
		{"--format", "pdf"},
		{"--criticality", "severe"},
		{"--since", "yesterday"},
		{},
	} {
		args := append([]string{"report", "compliance", "--root", root}, bad...)
		if len(bad) == 0 {
			args = []string{"report", "--root", root}
		}
		out, code := capture(t, args...)
		if code == 0 {
			t.Errorf("%v: exit 0, want an error\n%s", bad, out)
		}
		if len(bad) > 0 && bad[0] == "--concept" && !strings.Contains(out, `did you mean "c:Limit"`) {
			t.Errorf("mistyped concept: %s", out)
		}
	}
}

// TestReportComplianceCSV: one row per piece of evidence, so a spreadsheet
// filter on the row column gives the artifacts, the tests, the decisions or
// the waivers of a rule.
func TestReportComplianceCSV(t *testing.T) {
	root := complianceTree(t)
	out, code := capture(t, "report", "compliance", "--root", root, "--format", "csv")
	if code != 0 {
		t.Fatalf("report: %d\n%s", code, out)
	}
	rows, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(rows[0], ",") != strings.Join(report.CSVHeader, ",") {
		t.Errorf("header %v", rows[0])
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[h] = i
	}
	counts := map[string]int{}
	for _, r := range rows[1:] {
		if len(r) != len(rows[0]) {
			t.Errorf("row has %d columns, want %d: %v", len(r), len(rows[0]), r)
		}
		if r[col["concept"]] != "c:Limit" || r[col["criticality"]] != "high" || r[col["owner"]] != "team-rules" {
			t.Errorf("row context: %v", r)
		}
		counts[r[col["row"]]]++
		switch r[col["row"]] {
		case "artifact":
			if r[col["path"]] != "app/settings.py" || r[col["line"]] != "1" || !strings.HasPrefix(r[col["url"]], "https://host.test/") || r[col["state"]] != "accepted" {
				t.Errorf("artifact row: %v", r)
			}
		case "decision":
			if r[col["when"]] == "" || r[col["record"]] == "" {
				t.Errorf("decision row without a date or record: %v", r)
			}
			if r[col["state"]] == "rejected" && (r[col["reason"]] != "wrong concept" || r[col["actor"]] != "human:reviewer") {
				t.Errorf("rejection row: %v", r)
			}
		case "waiver":
			if r[col["actor"]] != "audit-lead" || r[col["when"]] != "2099-01-01" || r[col["state"]] != "active" {
				t.Errorf("waiver row: %v", r)
			}
		}
	}
	for row, want := range map[string]int{"concept": 1, "boundary": 1, "artifact": 1, "test": 1, "value": 1, "waiver": 1} {
		if counts[row] != want {
			t.Errorf("%d %s rows, want %d", counts[row], row, want)
		}
	}
	if counts["decision"] < 4 {
		t.Errorf("%d decision rows, want the proposal, its rejection, the accepted links and the value binding", counts["decision"])
	}
}

// TestReportComplianceHTMLAndMarkdown: the readable forms carry the plain
// headings, link paths to the source-url, and escape what the ontology says.
func TestReportComplianceHTMLAndMarkdown(t *testing.T) {
	root := complianceTree(t)
	html, code := capture(t, "report", "compliance", "--root", root, "--format", "html")
	if code != 0 {
		t.Fatalf("html: %d\n%s", code, html)
	}
	for _, want := range []string{
		"<title>Compliance report</title>",
		"Where it is enforced", "How it is verified", "Published value", "Decision history", "Open findings and waivers",
		`href="https://host.test/r/blob/main/app/settings.py#L1"`,
		"&lt;b&gt;approved&lt;/b&gt;",
		"human:reviewer rejected the link implements → c:Limit in app/rules.py", "wrong concept",
		"Waiver on <code>c:Limit</code> by audit-lead, expires 2099-01-01",
		"@media print",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html lacks %q", want)
		}
	}
	if strings.Contains(html, "<b>approved</b>") || strings.Contains(html, "<script") {
		t.Errorf("html carries unescaped ontology text or a script")
	}
	md, code := capture(t, "report", "compliance", "--root", root)
	if code != 0 {
		t.Fatalf("markdown: %d\n%s", code, md)
	}
	for _, want := range []string{"# Compliance report", "## Limit (`c:Limit`)", "| `c:Limit` | high | implemented | 1 | 1 | 0 |",
		"[`app/settings.py:1 (LIMIT)`](https://host.test/r/blob/main/app/settings.py#L1)", "- Implemented in: The app (`o:app`), owner team-rules",
		"| `i:LimitRule` | 5 | [`app/settings.py:2`](https://host.test/r/blob/main/app/settings.py#L2) | `LIMIT = 5` | accepted |"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
}
