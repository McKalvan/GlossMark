// @gloss verifies gm:Increment, gmi:PlanRecord scope=file

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mckalvan/glossmark/internal/agents"
	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/ontoedit"
)

// TestIncrementAgentSurfaces is ADR 0034 P2's done-when on the library
// example (#200): a curated increment's plan record is read; the session-start briefing for the
// directory names the open plan item; impact_of's plan block reports it open
// while the code does not reach it and closed once it does.
func TestIncrementAgentSurfaces(t *testing.T) {
	root := filepath.Join(t.TempDir(), "circulation")
	copyTree(t, example(t, "library/circulation"), root)
	libPath := filepath.Join(root, "ontology", "lib.gloss.yaml")
	renew := filepath.Join(root, "services", "circulation", "loans", "renew.py")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The curation PR: lib:Renewal planned, nothing implements it yet.
	const ticket = "https://github.com/acme/library/issues/412"
	by := time.Now().AddDate(0, 0, 30).Format(model.PlanDate)
	if err := ontoedit.RemoveConcept(libPath, "Renewal"); err != nil {
		t.Fatal(err)
	}
	if err := ontoedit.AddConcept(libPath, "Renewal", ontoedit.ConceptSpec{Kind: "process", Doc: "Extending a loan's due date before it falls due.", Planned: true, PlannedBy: by, PlannedTicket: ticket}); err != nil {
		t.Fatal(err)
	}
	write(renew, "def renew_loan(loan_id: str) -> None:\n    pass\n")
	write(filepath.Join(root, ".glossmark", "plan.jsonl"),
		`{"type":"plan","format":1,"generator":"curate-increment"}`+"\n"+
			`{"type":"brief","increment":"`+ticket+`","date":"2026-10-24","actor":"agent:test","owner":"library","answers":{"scope":"circulation only"}}`+"\n"+
			`{"type":"decision","increment":"`+ticket+`","subject":"lib:Renewal","decision":"added","reason":"Patrons ask to keep items longer.","actor":"agent:test"}`+"\n")
	ws, g, _, _ := build(t, root)
	if len(ws.Plan) != 2 || ws.Plan[1].Subject != "lib:Renewal" {
		t.Errorf("plan lines = %+v", ws.Plan)
	}

	rep := compareNow(g, ws, time.Now())
	dir := "services/circulation/loans"
	brief := agents.Context(ws, g, ws.ManifestFor(dir+"/renew.py"), rep, agents.ContextOptions{Dir: dir})
	if !strings.Contains(brief, "Open plan items here") || !strings.Contains(brief, "planned concept lib:Renewal") {
		t.Errorf("briefing lacks the open plan item:\n%s", brief)
	}
	// A file under the boundary that links nothing yet touches the plan
	// through its boundary.
	pt := gate.PlanItemsTouched(g, ws, rep, gate.Touched(g, ws, []string{"services/circulation/loans/renew.py"}))
	if pt == nil || len(pt.Open) == 0 || len(pt.Closes) != 0 {
		t.Errorf("before: plan touch %+v", pt)
	}

	// The implementation PR: the handler implements it.
	write(renew, "# @gloss implements lib:Renewal\ndef renew_loan(loan_id: str) -> None:\n    pass\n")
	ws2, g2, _, _ := build(t, root)
	rep2 := compareNow(g2, ws2, time.Now())
	pt = gate.PlanItemsTouched(g2, ws2, rep2, gate.Touched(g2, ws2, []string{"services/circulation/loans/renew.py"}))
	if pt == nil || len(pt.Closes) != 1 || !strings.Contains(pt.Closes[0], "lib:Renewal is implemented") {
		t.Errorf("after: plan touch %+v", pt)
	}
	brief = agents.Context(ws2, g2, ws2.ManifestFor(dir+"/renew.py"), rep2, agents.ContextOptions{Dir: dir})
	if !strings.Contains(brief, "Closed plan items here") {
		t.Errorf("briefing lacks the closed plan item:\n%s", brief)
	}
}
