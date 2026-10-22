// @gloss implements gmi:GraphBrowser scope=file

package mcp

import (
	"encoding/json"

	"github.com/mckalvan/glossmark/internal/browse"
)

var browseTools = []tool{
	{"browse_node", "Read-only: any node of the graph (a concept or boundary by CURIE, a file by path or path#symbol) with every edge in and out grouped by relation, each neighbour's ref to browse on, and the drift findings on it. Without ref: every boundary and owned concept, where browsing starts.",
		obj(map[string]any{"ref": str("CURIE, repo-relative path or path#symbol (optional)")})},
}

// callBrowse handles browse_node; handled is false for any other name.
func (s *Server) callBrowse(name string, args json.RawMessage) (any, bool, error) {
	if name != "browse_node" {
		return nil, false, nil
	}
	r, ok := s.backend.(Reviewer)
	if !ok {
		return nil, false, nil
	}
	e, err := r.ReviewEngine()
	if err != nil {
		return nil, true, err
	}
	e.Invalidate()
	st, err := e.State()
	if err != nil {
		return nil, true, err
	}
	var a struct {
		Ref string `json:"ref"`
	}
	json.Unmarshal(args, &a)
	if a.Ref == "" {
		return map[string]any{"index": browse.Index(st.Graph)}, true, nil
	}
	n, err := browse.Lookup(st.Graph, st.Report, a.Ref)
	return n, true, err
}
