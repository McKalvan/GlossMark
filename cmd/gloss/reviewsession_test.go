// @gloss verifies gmi:ReviewEngine scope=file

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/mcp"
	"github.com/mckalvan/glossmark/internal/review"
)

// reviewTree is a workspace with one of each review item: a SOFT proposal,
// a proposal of a new concept (HARD), a stale link and a rogue marker.
func reviewTree(t *testing.T) (string, *review.Engine) {
	t.Helper()
	// The undo journal lives under the user cache directory outside git.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	// Decisions run as a person unless a test says otherwise.
	t.Setenv("GLOSS_ACTOR", "human:test")
	root := tiny(t, map[string]string{
		"soft.py":  "# @gloss? implements .Doer conf=0.95\ndef soft(): pass\n",
		"stale.py": "# @gloss implements .Doer scope=file\ndef stale(): pass\n",
	})
	gloss(t, "build", "-q", "--root", root)
	os.WriteFile(filepath.Join(root, "stale.py"), []byte("# @gloss implements .Doer scope=file\ndef stale(): return 1\n"), 0o644)
	os.WriteFile(filepath.Join(root, "rogue.py"), []byte("# @gloss? implements i:NEW:Thing\ndef rogue(): pass\n"), 0o644)
	gloss(t, "build", "-q", "--root", root)
	e, err := newEngine(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, e
}

func itemByPath(t *testing.T, e *review.Engine, kind, path string) review.Item {
	t.Helper()
	items, err := e.Queue()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Kind == kind && it.Path == path {
			return it
		}
	}
	t.Fatalf("no %s item for %s in %+v", kind, path, items)
	return review.Item{}
}

// The queue lists each kind with its actions and who may take them.
func TestReviewQueue(t *testing.T) {
	_, e := reviewTree(t)
	soft := itemByPath(t, e, review.KindProposedLink, "soft.py")
	if a := soft.Action(review.ActAccept); a == nil || a.Gate != "SOFT" || !a.AgentMay {
		t.Errorf("soft proposal accept: %+v", a)
	}
	rogue := itemByPath(t, e, review.KindProposedLink, "rogue.py")
	if a := rogue.Action(review.ActAccept); a == nil || a.Gate != "HARD" || a.AgentMay {
		t.Errorf("new-concept proposal accept: %+v", a)
	}
	stale := itemByPath(t, e, review.KindStaleLink, "stale.py")
	if a := stale.Action(review.ActReconfirm); a == nil || a.Gate != "AUTO" || !a.AgentMay {
		t.Errorf("stale reconfirm: %+v", a)
	}
	f := itemByPath(t, e, review.KindFinding, "rogue.py")
	if f.Finding != "rogue" || f.Action(review.ActPropose) == nil || f.Action(review.ActWaive) == nil || f.Action(review.ActWaive).AgentMay {
		t.Errorf("rogue finding: %+v", f)
	}
	d, err := e.Detail(stale.ID)
	if err != nil || len(d.Context) == 0 || len(d.Concepts) != 1 || !d.Concepts[0].Exists || d.Concepts[0].Curie != "i:Doer" {
		t.Errorf("detail: %+v, %v", d, err)
	}
}

// An agent takes AUTO and SOFT decisions and only recommends HARD ones; a
// recommendation reads as outdated once its item changes and goes when a
// person decides the item.
func TestReviewAuthority(t *testing.T) {
	root, e := reviewTree(t)
	e.Actor = "agent:test"
	stale := itemByPath(t, e, review.KindStaleLink, "stale.py")
	if r := e.Decide(review.Decision{ID: stale.ID, Action: review.ActReconfirm}); !r.OK || r.Undo == "" {
		t.Fatalf("agent reconfirm: %+v", r)
	}
	if st := ledgerStatus(t, root, "stale.py"); st[0] != "accepted" {
		t.Errorf("after reconfirm: %v", st)
	}
	rogue := itemByPath(t, e, review.KindProposedLink, "rogue.py")
	r := e.Decide(review.Decision{ID: rogue.ID, Action: review.ActAccept})
	if r.OK || !strings.Contains(r.Error, review.ErrNeedsHuman.Error()) {
		t.Fatalf("agent accepting a HARD proposal: %+v", r)
	}
	if r := e.Decide(review.Decision{ID: rogue.ID, Action: review.ActReject, Reason: "no such concept", Recommend: true}); !r.OK || !r.Recommended {
		t.Fatalf("agent recommendation: %+v", r)
	}
	if got := itemByPath(t, e, review.KindProposedLink, "rogue.py"); got.Recommendation == nil || got.Recommendation.By != "agent:test" || got.Recommendation.Outdated {
		t.Fatalf("recommendation on the item: %+v", got.Recommendation)
	}
	os.WriteFile(filepath.Join(root, "rogue.py"), []byte("# @gloss? implements i:NEW:Thing scope=file\ndef rogue(): pass\n"), 0o644)
	e.Invalidate()
	// The marker changed, so its id did too: the old recommendation has no
	// item, and a fresh one on the new item is not outdated.
	rogue = itemByPath(t, e, review.KindProposedLink, "rogue.py")
	e.Decide(review.Decision{ID: rogue.ID, Action: review.ActReject, Reason: "no such concept", Recommend: true})
	e.Actor = "human:test"
	if r := e.Decide(review.Decision{ID: rogue.ID, Action: review.ActReject, Reason: "no such concept", Remove: true}); !r.OK {
		t.Fatalf("human reject: %+v", r)
	}
	b, _ := os.ReadFile(filepath.Join(root, ".glossmark", review.RecommendationsFile))
	if strings.Contains(string(b), rogue.ID) {
		t.Errorf("the decided item's recommendation is still there:\n%s", b)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "rogue.py")); strings.Contains(string(b), "@gloss") {
		t.Errorf("reject --remove left the marker:\n%s", b)
	}
	if !errors.Is(review.ErrNeedsHuman, review.ErrNeedsHuman) || review.IsAgent("human:test") || !review.IsAgent("agent:x") {
		t.Error("IsAgent")
	}
}

// Undo restores every file a decision changed, and refuses when one has
// changed again since unless forced.
func TestReviewUndo(t *testing.T) {
	root, e := reviewTree(t)
	read := func(rel string) string { b, _ := os.ReadFile(filepath.Join(root, rel)); return string(b) }
	beforeSrc, beforeLedger, beforeGraph := read("soft.py"), read(".glossmark/ledger.jsonl"), read(".glossmark/graph.jsonl")
	soft := itemByPath(t, e, review.KindProposedLink, "soft.py")
	if r := e.Decide(review.Decision{ID: soft.ID, Action: review.ActAccept}); !r.OK {
		t.Fatalf("accept: %+v", r)
	}
	if read("soft.py") == beforeSrc {
		t.Fatal("accept did not flip the marker")
	}
	if _, err := e.Undo(false); err != nil {
		t.Fatal(err)
	}
	if read("soft.py") != beforeSrc || read(".glossmark/ledger.jsonl") != beforeLedger || read(".glossmark/graph.jsonl") != beforeGraph {
		t.Error("undo did not restore the source, ledger and graph")
	}
	if r := e.Decide(review.Decision{ID: soft.ID, Action: review.ActAccept}); !r.OK {
		t.Fatalf("accept again: %+v", r)
	}
	os.WriteFile(filepath.Join(root, "soft.py"), []byte(read("soft.py")+"# edited later\n"), 0o644)
	if _, err := e.Undo(false); err == nil || !strings.Contains(err.Error(), "soft.py changed since") {
		t.Fatalf("undo over a later edit: %v", err)
	}
	if _, err := e.Undo(true); err != nil || read("soft.py") != beforeSrc {
		t.Errorf("forced undo: %v\n%s", err, read("soft.py"))
	}
	if _, err := e.Undo(false); err == nil {
		t.Error("undo with an empty journal should fail")
	}
}

// Waive, write-link and edit-doc, and apply reading JSON Lines.
func TestReviewEdits(t *testing.T) {
	root, e := reviewTree(t)
	read := func(rel string) string { b, _ := os.ReadFile(filepath.Join(root, rel)); return string(b) }
	f := itemByPath(t, e, review.KindFinding, "rogue.py")
	if r := e.Decide(review.Decision{ID: f.ID, Action: review.ActWaive, Owner: "alice", Expires: "2999-01-01", Reason: "decided next sprint"}); !r.OK {
		t.Fatalf("waive: %+v", r)
	}
	if code, out := gloss(t, "check", "--root", root); code != 0 || strings.Contains(out, "GM61") {
		t.Errorf("check after waive: exit %d\n%s\n%s", code, out, read("glossmark.yaml"))
	}
	for _, it := range mustQueue(t, e) {
		if it.ID == f.ID {
			t.Errorf("waived finding still queued: %+v", it)
		}
	}
	// A second waiver goes into the same list.
	os.WriteFile(filepath.Join(root, "rogue2.py"), []byte("# @gloss? implements i:NEW:Other\ndef rogue2(): pass\n"), 0o644)
	e.Invalidate()
	f2 := itemByPath(t, e, review.KindFinding, "rogue2.py")
	if r := e.Decide(review.Decision{ID: f2.ID, Action: review.ActWaive, Owner: "bob", Expires: "2999-01-01"}); !r.OK {
		t.Fatalf("second waive: %+v\n%s", r, read("glossmark.yaml"))
	}
	if n := strings.Count(read("glossmark.yaml"), "waivers:"); n != 1 || strings.Count(read("glossmark.yaml"), "finding:") != 2 {
		t.Errorf("two waivers in one list:\n%s", read("glossmark.yaml"))
	}

	os.WriteFile(filepath.Join(root, "lib.py"), []byte("class Lib:\n    def run(self):\n        pass\n"), 0o644)
	e.Invalidate()
	e.Actor = "agent:test"
	if r := e.Decide(review.Decision{Action: review.ActWriteLink, Path: "lib.py", Line: 2, Relation: "implements", Target: ".Doer", Accepted: true}); r.OK {
		t.Errorf("an agent wrote an accepted link: %+v", r)
	}
	if r := e.Decide(review.Decision{Action: review.ActWriteLink, Path: "lib.py", Line: 2, Relation: "implements", Target: ".Doer", Conf: 0.9}); !r.OK {
		t.Fatalf("write-link: %+v", r)
	}
	if got := strings.Split(read("lib.py"), "\n")[1]; got != "    # @gloss? implements .Doer conf=0.90" {
		t.Errorf("written marker: %q", got)
	}
	itemByPath(t, e, review.KindProposedLink, "lib.py")
	if r := e.Decide(review.Decision{Action: review.ActEditDoc, Curie: "i:Doer", Doc: "Does the thing."}); r.OK {
		t.Errorf("an agent edited a doc: %+v", r)
	}

	e.Actor = "human:test"
	in := strings.NewReader(`{"action":"edit-doc","curie":"i:Doer","doc":"Does the thing."}
# a comment line
{"id":"nope","action":"accept"}
`)
	var out strings.Builder
	if code := applyDecisions(e, in, &out); code != 1 {
		t.Errorf("apply with a failing line: exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var r1, r2 review.Result
	if len(lines) != 2 || json.Unmarshal([]byte(lines[0]), &r1) != nil || json.Unmarshal([]byte(lines[1]), &r2) != nil || !r1.OK || r2.OK {
		t.Fatalf("apply results:\n%s", out.String())
	}
	if !strings.Contains(read("ontology/i.gloss.yaml"), `doc: "Does the thing."`) {
		t.Errorf("edit-doc:\n%s", read("ontology/i.gloss.yaml"))
	}
}

// Propose answers a rogue marker with a proposed concept.
func TestReviewPropose(t *testing.T) {
	root, e := reviewTree(t)
	e.Actor = "agent:test"
	f := itemByPath(t, e, review.KindFinding, "rogue.py")
	if r := e.Decide(review.Decision{ID: f.ID, Action: review.ActPropose, Doc: "A thing."}); !r.OK {
		t.Fatalf("propose: %+v", r)
	}
	b, _ := os.ReadFile(filepath.Join(root, "ontology/i.gloss.yaml"))
	if !strings.Contains(string(b), "Thing:") || !strings.Contains(string(b), "proposed: true") {
		t.Errorf("propose did not add the concept:\n%s", b)
	}
	c := itemByPath(t, e, review.KindProposedConcept, "ontology/i.gloss.yaml")
	if c.Concept != "i:Thing" || c.Action(review.ActAccept).AgentMay {
		t.Errorf("proposed concept item: %+v", c)
	}
}

func mustQueue(t *testing.T, e *review.Engine) []review.Item {
	t.Helper()
	items, err := e.Queue()
	if err != nil {
		t.Fatal(err)
	}
	return items
}

// mcpTool calls one MCP tool through the server and returns its structured
// content, or the error text with isError.
func mcpTool(t *testing.T, root, name string, args any) (json.RawMessage, string) {
	t.Helper()
	a, _ := json.Marshal(args)
	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":` + string(a) + `}}` + "\n"
	var buf strings.Builder
	if err := mcp.New(&mcpBackend{root: root}, strings.NewReader(req), &buf, "test").Serve(); err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Result struct {
			IsError           bool                    `json:"isError"`
			Content           []struct{ Text string } `json:"content"`
			StructuredContent json.RawMessage         `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(buf.String()), &resp); err != nil {
		t.Fatalf("%v: %s", err, buf.String())
	}
	if resp.Result.IsError {
		return nil, resp.Result.Content[0].Text
	}
	return resp.Result.StructuredContent, ""
}

// The review tools over MCP, as an agent: list, reconfirm, refuse a HARD
// accept, recommend, recommend a doc edit, undo.
func TestReviewMCP(t *testing.T) {
	root, _ := reviewTree(t)
	t.Setenv("GLOSS_ACTOR", "agent:test")
	var list struct {
		Tools []struct{ Name string } `json:"tools"`
	}
	var buf strings.Builder
	mcp.New(&mcpBackend{root: root}, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n"), &buf, "test").Serve()
	var lr struct{ Result json.RawMessage }
	json.Unmarshal([]byte(buf.String()), &lr)
	json.Unmarshal(lr.Result, &list)
	names := map[string]bool{}
	for _, tl := range list.Tools {
		names[tl.Name] = true
	}
	for _, n := range []string{"review_queue", "review_item", "review_decide", "review_undo", "write_link", "edit_concept_doc", "lookup_concept"} {
		if !names[n] {
			t.Errorf("tools/list lacks %s", n)
		}
	}
	raw, errText := mcpTool(t, root, "review_queue", map[string]any{"kind": []string{"stale-link", "proposed-link"}})
	var q struct {
		Actor string        `json:"actor"`
		Items []review.Item `json:"items"`
	}
	if errText != "" || json.Unmarshal(raw, &q) != nil || q.Actor != "agent:test" || len(q.Items) != 3 {
		t.Fatalf("review_queue: %s %s", errText, raw)
	}
	var staleID, rogueID string
	for _, it := range q.Items {
		switch it.Path {
		case "stale.py":
			staleID = it.ID
		case "rogue.py":
			rogueID = it.ID
		}
	}
	if raw, errText := mcpTool(t, root, "review_item", map[string]any{"id": staleID}); errText != "" || !strings.Contains(string(raw), `"concepts"`) {
		t.Errorf("review_item: %s %s", errText, raw)
	}
	if _, errText := mcpTool(t, root, "review_decide", map[string]any{"id": staleID, "action": "reconfirm"}); errText != "" {
		t.Fatalf("reconfirm: %s", errText)
	}
	if _, errText := mcpTool(t, root, "review_decide", map[string]any{"id": rogueID, "action": "accept"}); !strings.Contains(errText, "HARD") {
		t.Errorf("agent accept of a HARD proposal: %q", errText)
	}
	if raw, errText := mcpTool(t, root, "review_decide", map[string]any{"id": rogueID, "action": "reject", "reason": "no such concept", "recommend": true}); errText != "" || !strings.Contains(string(raw), `"recommended": true`) && !strings.Contains(string(raw), `"recommended":true`) {
		t.Errorf("recommend: %s %s", errText, raw)
	}
	if _, errText := mcpTool(t, root, "edit_concept_doc", map[string]any{"curie": "i:Doer", "doc": "Does it.", "reason": "it had none"}); errText != "" {
		t.Fatalf("edit_concept_doc: %s", errText)
	}
	raw, _ = mcpTool(t, root, "review_queue", map[string]any{"kind": []string{"recommendation"}})
	if !strings.Contains(string(raw), `"edit:doc:i:Doer"`) {
		t.Errorf("recommended doc edit not queued: %s", raw)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "ontology/i.gloss.yaml")); strings.Contains(string(b), "Does it.") {
		t.Error("an agent's doc edit was applied")
	}
	if raw, errText := mcpTool(t, root, "review_undo", map[string]any{}); errText != "" || !strings.Contains(string(raw), "undid") {
		t.Errorf("review_undo: %s %s", errText, raw)
	}
}

// A person applies a recommended doc edit, and dismisses a recommendation.
func TestReviewApplyRecommendation(t *testing.T) {
	root, e := reviewTree(t)
	e.Actor = "agent:test"
	if r := e.Decide(review.Decision{Action: review.ActEditDoc, Curie: "i:Doer", Doc: "Does it.", Recommend: true}); !r.OK {
		t.Fatalf("recommend doc: %+v", r)
	}
	soft := itemByPath(t, e, review.KindProposedLink, "soft.py")
	e.Decide(review.Decision{ID: soft.ID, Action: review.ActAccept, Recommend: true})
	if r := e.Decide(review.Decision{ID: "edit:doc:i:Doer", Action: review.ActApply}); r.OK {
		t.Errorf("an agent applied a doc edit: %+v", r)
	}
	e.Actor = "human:test"
	if r := e.Decide(review.Decision{ID: "edit:doc:i:Doer", Action: review.ActApply}); !r.OK {
		t.Fatalf("apply: %+v", r)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "ontology/i.gloss.yaml")); !strings.Contains(string(b), `doc: "Does it."`) {
		t.Errorf("doc not applied:\n%s", b)
	}
	if r := e.Decide(review.Decision{ID: soft.ID, Action: review.ActDismiss}); !r.OK {
		t.Fatalf("dismiss: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(root, ".glossmark", review.RecommendationsFile)); !os.IsNotExist(err) {
		t.Errorf("recommendations file left behind: %v", err)
	}
}
