// @gloss verifies gmi:RelationProfile scope=file

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/mcp"
)

// TestStarterProfileMCP (issue #261): in a starter repository the MCP
// server's instructions and propose_link's relation description list only
// the starter relations and name the switch, and propose_link carries a
// hint for an advanced relation; a full repository shows every relation.
func TestStarterProfileMCP(t *testing.T) {
	for _, relations := range []string{"starter", "full"} {
		root := t.TempDir()
		for rel, content := range map[string]string{
			"glossmark.yaml":        "relations: " + relations + "\nowns:\n  i: { iri: https://x/i/, tier: implementation }\n",
			"ontology/i.gloss.yaml": "concepts:\n  Doer: { kind: process, internal: true }\n  Page: { kind: system-type, internal: true }\n",
			".gloss.yaml":           "boundary: b:root\nlevel: system\ndefault-namespace: i\n",
			"app.py":                "# @gloss implements .Doer\ndef do(): pass\n",
		} {
			p := filepath.Join(root, rel)
			os.MkdirAll(filepath.Dir(p), 0o755)
			os.WriteFile(p, []byte(content), 0o644)
		}
		var buf strings.Builder
		req := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n"
		if err := mcp.New(&mcpBackend{root: root}, strings.NewReader(req), &buf, "test").Serve(); err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 {
			t.Fatalf("relations: %s: %d responses:\n%s", relations, len(lines), buf.String())
		}
		var init struct {
			Result struct{ Instructions string } `json:"result"`
		}
		json.Unmarshal([]byte(lines[0]), &init)
		var list struct {
			Result struct {
				Tools []struct {
					Name        string
					InputSchema struct {
						Properties map[string]struct{ Description string }
					} `json:"inputSchema"`
				}
			} `json:"result"`
		}
		json.Unmarshal([]byte(lines[1]), &list)
		desc := ""
		for _, tl := range list.Result.Tools {
			if tl.Name == "propose_link" {
				desc = tl.InputSchema.Properties["relation"].Description
			}
		}
		raw, errText := mcpTool(t, root, "propose_link", map[string]any{"path": "page.py", "relation": "presents", "target": "i:Page"})
		if errText != "" {
			t.Fatalf("propose_link: %s", errText)
		}
		var pl struct {
			Proposals []struct{ Hint string } `json:"proposals"`
		}
		json.Unmarshal(raw, &pl)
		hint := ""
		if len(pl.Proposals) == 1 {
			hint = pl.Proposals[0].Hint
		}
		if relations == "starter" {
			if !strings.Contains(init.Result.Instructions, "inline: `consumes`, `documents`, `implements`, `produces`, `represents`, `verifies`. ") || !strings.Contains(init.Result.Instructions, "relations: full") {
				t.Errorf("starter instructions:\n%s", init.Result.Instructions)
			}
			if desc != "consumes | documents | implements | produces | represents | verifies (the starter profile; relations: full in glossmark.yaml unlocks the rest)" {
				t.Errorf("starter propose_link relation: %q", desc)
			}
			if !strings.Contains(hint, "presents is outside the starter relation profile") || !strings.Contains(hint, "GM250") {
				t.Errorf("starter propose_link presents hint: %q", hint)
			}
			continue
		}
		if !strings.Contains(init.Result.Instructions, "`presents`") || strings.Contains(init.Result.Instructions, "starter") {
			t.Errorf("full instructions:\n%s", init.Result.Instructions)
		}
		if desc != "implements | represents | verifies | ..." || hint != "" {
			t.Errorf("full propose_link relation %q, hint %q", desc, hint)
		}
	}
}
