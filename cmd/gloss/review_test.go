// @gloss verifies gmi:ReviewCommands scope=file

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/model"
)

// ledgerRows reads the ledger's entries keyed by marker id.
func ledgerRows(t *testing.T, root string) map[string]model.LedgerEntry {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, ".glossmark", "ledger.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]model.LedgerEntry{}
	for _, l := range strings.Split(string(b), "\n") {
		var e model.LedgerEntry
		if json.Unmarshal([]byte(l), &e) == nil && e.MarkerID != "" {
			out[e.MarkerID] = e
		}
	}
	return out
}

// rowFor returns the one ledger row whose targets include target.
func rowFor(t *testing.T, rows map[string]model.LedgerEntry, target string) model.LedgerEntry {
	t.Helper()
	var found []model.LedgerEntry
	for _, e := range rows {
		for _, x := range e.Targets {
			if x == target && e.Relation != "concept" {
				found = append(found, e)
			}
		}
	}
	if len(found) != 1 {
		t.Fatalf("rows targeting %s: %+v, want one", target, found)
	}
	return found[0]
}

// Issue #158 (K16, K13): propose --from-new carries the marker's ledger row
// to the id the rewrite gives it, and reject --concept --remove carries it
// back, so one proposal keeps one row and no row turns removed without a
// decision.
func TestRewriteCarriesMarkerID(t *testing.T) {
	root := workspace(t, "library/circulation")
	gloss(t, "build", "-q", "--root", root)
	before := rowFor(t, ledgerRows(t, root), "circ:NEW:DamageCheck")
	if code, _ := gloss(t, "propose", "--from-new", "--realizes", "lib:Return", "--doc", "Inspecting a returned item.", "--root", root); code != 0 {
		t.Fatalf("propose --from-new: exit %d", code)
	}
	gloss(t, "build", "-q", "--root", root)
	rows := ledgerRows(t, root)
	if _, ok := rows[before.MarkerID]; ok {
		t.Errorf("the old id %s still has a row: %+v", before.MarkerID, rows[before.MarkerID])
	}
	after := rowFor(t, rows, "circ:DamageCheck")
	if after.Status != model.StatusProposed || after.Created != before.Created || after.Actor != before.Actor || after.Rationale != before.Rationale ||
		len(after.PreviousIDs) != 1 || after.PreviousIDs[0] != before.MarkerID {
		t.Errorf("row after --from-new = %+v, want the history of %+v under a new id", after, before)
	}
	if code, _ := gloss(t, "reject", "--concept", "circ:DamageCheck", "--reason", "the bindery owns damage", "--remove", "--root", root); code != 0 {
		t.Fatalf("reject --concept --remove: exit %d", code)
	}
	gloss(t, "build", "-q", "--root", root)
	rows = ledgerRows(t, root)
	for id, e := range rows {
		if e.Status == model.StatusRemoved {
			t.Errorf("%s is removed without a decision: %+v", id, e)
		}
	}
	if c := rows["concept:circ:DamageCheck"]; c.Status != model.StatusRejected || c.ReviewReason != "the bindery owns damage" {
		t.Errorf("concept row = %+v, want rejected with the reason", c)
	}
	back := rowFor(t, rows, "circ:NEW:DamageCheck")
	if back.MarkerID != before.MarkerID || back.Created != before.Created || len(back.PreviousIDs) != 1 || back.PreviousIDs[0] != after.MarkerID {
		t.Errorf("row after reject --remove = %+v, want %s back with %s as its previous id", back, before.MarkerID, after.MarkerID)
	}
}

// Issue #158 (K22): status prints the reason diff gives for a proposal's
// gate, and no gate on stale or rejected links.
func TestStatusGateReasons(t *testing.T) {
	root := tiny(t, map[string]string{
		"low.py":   "# @gloss? implements .Doer conf=0.2\ndef low(): pass\n",
		"high.py":  "# @gloss? implements .Doer conf=0.95\ndef high(): pass\n",
		"stale.py": "# @gloss implements .Doer\ndef stale(): pass\n",
		"no.py":    "# @gloss? implements .Doer conf=0.9\ndef no(): pass\n",
	})
	gloss(t, "build", "-q", "--root", root)
	gloss(t, "reject", "no.py:1", "--reason", "not it", "--root", root)
	os.WriteFile(filepath.Join(root, "stale.py"), []byte("# @gloss implements .Doer\ndef stale(): return 1\n"), 0o644)
	gloss(t, "build", "-q", "--root", root)
	_, out := gloss(t, "status", "--root", root)
	lines := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		for _, f := range []string{"low.py", "high.py", "stale.py", "no.py"} {
			if strings.Contains(l, " "+f+" ") {
				lines[f] = l
			}
		}
	}
	if !strings.Contains(lines["low.py"], "[HARD: conf 0.20 below threshold") {
		t.Errorf("low-confidence proposal: %q", lines["low.py"])
	}
	if !strings.HasSuffix(lines["high.py"], "[SOFT: auto-accept eligible]") {
		t.Errorf("eligible proposal: %q", lines["high.py"])
	}
	for _, f := range []string{"stale.py", "no.py"} {
		if l := lines[f]; l == "" || strings.Contains(l, "[") {
			t.Errorf("%s: %q, want a row without a gate", f, l)
		}
	}
}

// Issue #158 (K23): the PROV export names its agents as prov:Agent nodes and
// carries the author's rationale beside the reviewer's reason.
// @gloss verifies gmi:ReviewLedger
func TestExportPROVAgents(t *testing.T) {
	root := workspace(t, "library/circulation")
	t.Setenv("GLOSS_ACTOR", "human:reviewer")
	if code, _ := gloss(t, "reject", "services/circulation/loans/returns.py:10", "--reason", "the bindery owns damage", "--root", root); code != 0 {
		t.Fatalf("reject: exit %d", code)
	}
	path := filepath.Join(root, "prov.jsonld")
	if code, _ := gloss(t, "export", "--prov", path, "--root", root); code != 0 {
		t.Fatalf("export --prov: exit %d", code)
	}
	b, _ := os.ReadFile(path)
	var doc struct {
		Context map[string]any   `json:"@context"`
		Graph   []map[string]any `json:"@graph"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	agents := map[string][]any{}
	var rejected map[string]any
	for _, n := range doc.Graph {
		if ts, ok := n["@type"].([]any); ok && len(ts) > 0 && ts[0] == "prov:Agent" {
			agents[n["@id"].(string)] = ts
		}
		if n["gm:reviewReason"] == "the bindery owns damage" {
			rejected = n
		}
	}
	if rejected == nil || rejected["gm:rationale"] != "inspecting returned items has no concept yet" || rejected["gm:reviewedBy"] != "urn:gloss:actor:human:reviewer" {
		t.Fatalf("rejected activity = %v", rejected)
	}
	for _, ref := range []string{rejected["prov:wasAssociatedWith"].(string), rejected["gm:reviewedBy"].(string)} {
		if ts := agents[ref]; len(ts) != 2 || ts[1] != "prov:Person" {
			t.Errorf("agent %s = %v, want prov:Agent and prov:Person", ref, ts)
		}
	}
	if ref, _ := doc.Context["gm:reviewedBy"].(map[string]any); ref["@type"] != "@id" {
		t.Errorf("gm:reviewedBy is not an IRI in the context: %v", doc.Context["gm:reviewedBy"])
	}
}

// Issue #158 (K11): the generated block names id=, writes verifies and not
// its alias, gives criticality a column, tags a proposed concept and does
// not double a doc's full stop.
// @gloss verifies gmi:AgentsSnippet
func TestAgentsSnippetBlock(t *testing.T) {
	root := workspace(t, "library/circulation")
	addConcept(t, root, "circ.gloss.yaml", "  ShelfCheck: { kind: process, realizes: lib:Return, proposed: true, doc: \"Checking an item's shelf mark.\" }")
	code, out := gloss(t, "agents", "--root", root)
	if code != 0 {
		t.Fatalf("agents: exit %d", code)
	}
	for _, want := range []string{"[id=<stable-id>]", "| Concept | Kind | Criticality | Notes |", "| `lib:Patron` | entity | critical | ", "| `circ:ShelfCheck` | process | ", "PROPOSED; Checking an item's shelf mark; realizes lib:Return"} {
		if !strings.Contains(out, want) {
			t.Errorf("agents output lacks %q", want)
		}
	}
	for _, bad := range []string{"`tests`", ".;"} {
		if strings.Contains(out, bad) {
			t.Errorf("agents output contains %q", bad)
		}
	}
}

// Issue #158 (K3): the SessionStart hook exports GLOSS_ACTOR through
// $CLAUDE_ENV_FILE, so commands the agent runs later carry the actor.
func TestHookExportsActor(t *testing.T) {
	root := tiny(t, nil)
	if code, _ := gloss(t, "hook", "install", "--no-mcp", "--actor", "agent:prep-cook", "--root", root); code != 0 {
		t.Fatalf("hook install: exit %d", code)
	}
	b, _ := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct{ Command string } `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(b, &settings); err != nil || len(settings.Hooks["SessionStart"]) != 1 {
		t.Fatalf("settings: %v\n%s", err, b)
	}
	cmd := settings.Hooks["SessionStart"][0].Hooks[0].Command
	env := filepath.Join(t.TempDir(), "env")
	sh := exec.Command("sh", "-c", cmd)
	sh.Dir = root
	sh.Env = append(os.Environ(), "CLAUDE_ENV_FILE="+env, "PATH=")
	if out, err := sh.CombinedOutput(); err != nil {
		t.Fatalf("hook command %q: %v\n%s", cmd, err, out)
	}
	if got, _ := os.ReadFile(env); string(got) != "export GLOSS_ACTOR=agent:prep-cook\n" {
		t.Errorf("CLAUDE_ENV_FILE = %q", got)
	}
	// Outside Claude Code the line goes nowhere and the hook still succeeds.
	sh = exec.Command("sh", "-c", cmd)
	sh.Dir = root
	sh.Env = []string{"PATH="}
	if out, err := sh.CombinedOutput(); err != nil {
		t.Errorf("hook without CLAUDE_ENV_FILE: %v\n%s", err, out)
	}
}
