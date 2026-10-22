// @gloss verifies gmi:GraphBrowser scope=file

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/browse"
)

// gloss review node and browse_node show the same node, and change nothing.
func TestReviewNode(t *testing.T) {
	root, _ := reviewTree(t)
	before := ledgerStatus(t, root, "status")
	code, out := gloss(t, "review", "node", "i:Doer", "--format", "json", "--root", root)
	var cli browse.Node
	if code != 0 || json.Unmarshal([]byte(out), &cli) != nil || cli.Ref != "i:Doer" || len(cli.In) == 0 {
		t.Fatalf("review node: exit %d\n%s", code, out)
	}
	raw, errText := mcpTool(t, root, "browse_node", map[string]any{"ref": "i:Doer"})
	var m browse.Node
	if errText != "" || json.Unmarshal(raw, &m) != nil || len(m.In) != len(cli.In) {
		t.Errorf("browse_node: %s %s", errText, raw)
	}
	if raw, _ := mcpTool(t, root, "browse_node", map[string]any{}); !strings.Contains(string(raw), `"index"`) {
		t.Errorf("browse_node without ref: %s", raw)
	}
	if code, out := gloss(t, "review", "node", "soft.py", "--root", root); code != 0 || !strings.Contains(out, "→ implements: i:Doer?") {
		t.Errorf("review node text: exit %d\n%s", code, out)
	}
	if after := ledgerStatus(t, root, "status"); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Errorf("browsing changed the ledger: %v → %v", before, after)
	}
}
