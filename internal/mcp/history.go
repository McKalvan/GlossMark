// @gloss implements gmi:McpServer scope=file

package mcp

import (
	"encoding/json"
)

// Historian is implemented by a backend that reads the repository's git
// history (ADR 0040). A backend without it does not serve history_of.
type Historian interface {
	// History returns the history of subjects (all when empty) since a git
	// ref ("" picks the default), compacted for an agent.
	History(subjects []string, since, step string, all bool, limit int) (any, error)
}

var historyTools = []tool{
	{"history_of", "How concepts and boundaries got to where they are (docs/adr/0040-ontology-history.md): each commit's gate against its parent, newest first, for the subjects given (followed through replaced-by) or for everything. Each step has the commit, date, author, title, status (built, unbuilt, skipped), gate and its SOFT and HARD changes, realization moves and version bumps. since defaults to the merge base with the default branch on a branch, else the last version bump of the subjects' namespaces. Call it before changing a concept to learn what was decided about it.",
		obj(map[string]any{"curies": map[string]any{"type": "array", "items": str("Concept or boundary CURIE to follow (optional: all when empty)")},
			"since": str("Git ref the history starts after (optional)"), "step": str("commit | first-parent | version (optional; default history.step in glossmark.yaml, else commit)"),
			"all":   map[string]any{"type": "boolean", "description": "Also return AUTO steps and AUTO changes"},
			"limit": map[string]any{"type": "integer", "description": "Most recent steps to return (default 20)"}})},
}

// callHistory handles history_of; handled is false for any other name.
func (s *Server) callHistory(name string, args json.RawMessage) (any, bool, error) {
	h, ok := s.backend.(Historian)
	if !ok || name != "history_of" {
		return nil, false, nil
	}
	var a struct {
		Curies []string `json:"curies"`
		Since  string   `json:"since"`
		Step   string   `json:"step"`
		All    bool     `json:"all"`
		Limit  int      `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, true, err
	}
	if a.Limit <= 0 {
		a.Limit = 20
	}
	res, err := h.History(a.Curies, a.Since, a.Step, a.All, a.Limit)
	return res, true, err
}
