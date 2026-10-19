// @gloss verifies gmi:McpServer scope=file

package mcp

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/agents"
)

// TestToolContract pins the MCP surface agents depend on: the server name,
// every tool's name and its required arguments, as docs/AGENT-CONTRACT.md
// lists them. Adding a tool or an optional argument is MINOR and only needs
// a new row here; renaming or removing either, or making an argument
// required, is MAJOR.
func TestToolContract(t *testing.T) {
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}, "\n")
	var out strings.Builder
	if err := New(nil, strings.NewReader(in), &out, "test").Serve(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want two responses, got %q", out.String())
	}
	var init struct {
		Result struct {
			ServerInfo   struct{ Name string } `json:"serverInfo"`
			Instructions string                `json:"instructions"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &init); err != nil {
		t.Fatal(err)
	}
	if init.Result.ServerInfo.Name != "glossmark" {
		t.Errorf("server name: want glossmark, got %q", init.Result.ServerInfo.Name)
	}
	if init.Result.Instructions != agents.MCPInstructions() {
		t.Errorf("instructions must be agents.MCPInstructions():\n%s", init.Result.Instructions)
	}
	var list struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				InputSchema struct {
					Required []string `json:"required"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &list); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"lookup_concept":    {"curie"},
		"search_concepts":   {"query"},
		"concepts_for_path": {"path"},
		"propose_link":      {"path", "relation", "target"},
		"impact_of":         nil,
		"propose_concept":   {"curie", "kind"},
		"context":           nil,
		"drift":             nil,
	}
	got := map[string][]string{}
	for _, tl := range list.Result.Tools {
		req := append([]string(nil), tl.InputSchema.Required...)
		sort.Strings(req)
		got[tl.Name] = req
	}
	for name, req := range want {
		req = append([]string(nil), req...)
		sort.Strings(req)
		g, ok := got[name]
		if !ok {
			t.Errorf("tool %s is missing; removing or renaming a tool is a MAJOR change", name)
			continue
		}
		if !reflect.DeepEqual(g, req) {
			t.Errorf("tool %s: required arguments %v, want %v; changing them is a MAJOR change", name, g, req)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("tool %s is not in this test or docs/AGENT-CONTRACT.md; add it to both", name)
		}
	}
}
