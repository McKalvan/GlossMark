// @gloss implements gmi:ReviewEngine scope=file

package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/mckalvan/glossmark/internal/review"
)

// Reviewer is implemented by a backend that serves review sessions (ADR
// 0033). A backend without it serves the other tools only.
type Reviewer interface {
	// ReviewEngine returns the session engine; decisions run as the
	// server's GLOSS_ACTOR.
	ReviewEngine() (*review.Engine, error)
}

var arr = func(desc string) map[string]any { return map[string]any{"type": "array", "items": str(desc)} }

var reviewTools = []tool{
	{"review_queue", "Everything that waits on a reviewer: proposed and stale links, proposed concepts, drifted published values and drift findings. Each item has an id, a summary, its actions with their gate (AUTO/SOFT/HARD) and agent_may (whether you may take it or only recommend it), and any recommendation already recorded.",
		obj(map[string]any{"kind": arr("Item kinds to keep: proposed-link, stale-link, proposed-concept, value-drift, finding, or a finding kind such as untested"), "path": arr("Files or directories the items must be under")})},
	{"review_item", "One review item with what a reviewer reads before deciding: the marker's lines, what changed in the file since the link was last reviewed (for a stale link), and the doc and kind of each target concept.",
		obj(map[string]any{"id": str("Item id from review_queue")}, "id")},
	{"review_decide", "Take or recommend a decision on a review item. Actions: accept, reconfirm, reject (reason, remove), confirm, waive (owner, expires, reason), propose (kind, doc). You may take an action whose agent_may is true; for any other, set recommend: true and give a reason: a human sees the recommendation in gloss review and decides. Each decision returns an undo id; review_undo reverts the last one.",
		obj(map[string]any{"id": str("Item id"), "action": str("accept | reconfirm | reject | confirm | waive | propose"), "reason": str("Why: required for reject, recorded with a recommendation"),
			"remove": map[string]any{"type": "boolean", "description": "reject: also delete the marker line or concept entry"}, "force": map[string]any{"type": "boolean"},
			"owner": str("waive: who owns the waiver"), "expires": str("waive: YYYY-MM-DD"), "kind": str("propose: concept kind (default from the relation)"), "doc": str("propose: one sentence"),
			"recommend": map[string]any{"type": "boolean", "description": "Record the decision for a human instead of taking it"}}, "id", "action")},
	{"review_undo", "Revert the last review decision: every file it changed (source, ontology, ledger, graph) goes back. Refuses when a file changed again since, unless force.",
		obj(map[string]any{"force": map[string]any{"type": "boolean"}})},
	{"write_link", "Insert a @gloss? marker into a file before a line, in the file's comment style and indented like that line, then rebuild. The link is a proposal a human accepts; use propose_link first to see its gate.",
		obj(map[string]any{"path": str("Repo-relative file"), "line": map[string]any{"type": "integer", "description": "1-based line the marker goes above (the declaration it binds to); 0 or 1 for the top of the file"},
			"relation": str("implements | represents | verifies | ..."), "target": str("Concept CURIE, .Local, or ns:NEW:Name"), "scope": str("file | symbol | block | line (optional)"),
			"conf": map[string]any{"type": "number"}, "note": str("Rationale (optional)")}, "path", "relation", "target")},
	{"edit_concept_doc", "Recommend a new doc for a concept this repository owns. A definition is a human decision, so this records a recommendation unless the server runs as a person.",
		obj(map[string]any{"curie": str("ns:Name"), "doc": str("One sentence: what the concept is"), "reason": str("Why the doc should change")}, "curie", "doc")},
}

// toolList is every tool this backend serves.
func (s *Server) toolList() []tool {
	if _, ok := s.backend.(Reviewer); ok {
		return append(append(append([]tool{}, tools...), reviewTools...), browseTools...)
	}
	return tools
}

// callReview handles the review tools; handled is false for any other name.
func (s *Server) callReview(name string, args json.RawMessage) (res any, handled bool, err error) {
	r, ok := s.backend.(Reviewer)
	if !ok {
		return nil, false, nil
	}
	switch name {
	case "review_queue", "review_item", "review_decide", "review_undo", "write_link", "edit_concept_doc":
	default:
		return nil, false, nil
	}
	e, err := r.ReviewEngine()
	if err != nil {
		return nil, true, err
	}
	// Every call reads the tree as it is now: the agent edits it between calls.
	e.Invalidate()
	switch name {
	case "review_queue":
		var a struct {
			Kind []string `json:"kind"`
			Path []string `json:"path"`
		}
		json.Unmarshal(args, &a)
		items, err := e.Queue()
		if err != nil {
			return nil, true, err
		}
		return map[string]any{"actor": e.Actor, "items": review.Filter(items, a.Kind, a.Path)}, true, nil
	case "review_item":
		var a struct {
			ID string `json:"id"`
		}
		json.Unmarshal(args, &a)
		d, err := e.Detail(a.ID)
		return d, true, err
	case "review_undo":
		var a struct {
			Force bool `json:"force"`
		}
		json.Unmarshal(args, &a)
		msg, err := e.Undo(a.Force)
		return map[string]any{"message": msg}, true, err
	}
	var d review.Decision
	if err := json.Unmarshal(args, &d); err != nil {
		return nil, true, fmt.Errorf("invalid arguments: %w", err)
	}
	switch name {
	case "write_link":
		d.Action, d.Accepted = review.ActWriteLink, false
	case "edit_concept_doc":
		// A doc edit is HARD: an agent's becomes a recommended edit in the
		// queue, which a person applies or dismisses.
		d.Action, d.Recommend = review.ActEditDoc, review.IsAgent(e.Actor)
	}
	res2 := e.Decide(d)
	if res2.Error != "" && !res2.OK {
		return res2, true, fmt.Errorf("%s", res2.Error)
	}
	return res2, true, nil
}
