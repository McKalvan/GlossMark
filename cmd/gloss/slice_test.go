package main

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/mcp"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata golden files")

// TestSliceCaptureRecorded: gloss slice pay:Capture over the payments
// landscape returns the recorded result (issue #34): the capture
// implementation concepts, their artifacts, the boundary that realizes it,
// pay:CaptureConfirmed as context and the settlement worker as the
// subscriber on the other side, and nothing of refunding.
func TestSliceCaptureRecorded(t *testing.T) {
	api := filepath.Join("..", "..", "examples", "payments", "payments-api")
	sw := filepath.Join("..", "..", "examples", "payments", "settlement-worker")
	code, out := gloss(t, "slice", "pay:Capture", "--repo", api, "--repo", sw)
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	golden := filepath.Join("testdata", "slice-capture.txt")
	if *updateGolden {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(golden, []byte(out), 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if out != string(want) {
		t.Errorf("gloss slice pay:Capture differs from %s (go test ./cmd/gloss -run TestSliceCaptureRecorded -update rewrites it):\n%s", golden, out)
	}
	for _, s := range []string{"pa:CaptureHandler", "pa:CaptureEmitter", "capture.py#capture", "acme:payments-api", "pay:CaptureConfirmed", "acme:settlement-worker", "sw:BatchSettler"} {
		if !strings.Contains(out, s) {
			t.Errorf("slice lacks %s", s)
		}
	}
	for _, s := range []string{"Refunding", "refund.py"} {
		if strings.Contains(out, s) {
			t.Errorf("slice contains unrelated %s", s)
		}
	}
	if code, _ := gloss(t, "slice", "pay:Capture", "--context", "2", "--root", api); code != 2 {
		t.Errorf("--context 2 exits %d, want 2", code)
	}
	if code, _ := gloss(t, "slice", "pay:Capture", "--follow", "calls", "--root", api); code != 2 {
		t.Errorf("--follow calls exits %d, want 2", code)
	}
}

// TestSliceSameEverywhere: the CLI, the MCP tool, a saved slice in
// views.yaml and the viewer's precomputed index return the same nodes for
// the same seed, and impact and context --concept use the saved slice.
func TestSliceSameEverywhere(t *testing.T) {
	dir := workspace(t, "payments/payments-api")
	writeTree(t, dir, map[string]string{"ontology/views.yaml": "views:\n  capture:\n    doc: Everything about capturing a payment.\n    slice: [pay:Capture]\n    zoom: code\n"})
	ids := func(nodes []struct {
		ID string `json:"id"`
	}) []string {
		var out []string
		for _, n := range nodes {
			out = append(out, n.ID)
		}
		sort.Strings(out)
		return out
	}
	type sliceJSON struct {
		Nodes []struct {
			ID string `json:"id"`
		} `json:"nodes"`
	}
	// CLI.
	code, out := gloss(t, "slice", "pay:Capture", "--format", "json", "--root", dir)
	if code != 0 {
		t.Fatalf("slice exit %d: %s", code, out)
	}
	var cli sliceJSON
	if err := json.Unmarshal([]byte(out), &cli); err != nil {
		t.Fatal(err)
	}
	want := ids(cli.Nodes)
	if len(want) < 8 {
		t.Fatalf("slice too small: %v", want)
	}
	// MCP.
	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"slice_of","arguments":{"seeds":["pay:Capture"]}}}` + "\n"
	var buf strings.Builder
	if err := mcp.New(&mcpBackend{root: dir}, strings.NewReader(req), &buf, "test").Serve(); err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Result struct {
			StructuredContent sliceJSON `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(buf.String()), &resp); err != nil {
		t.Fatalf("%v: %s", err, buf.String())
	}
	if got := ids(resp.Result.StructuredContent.Nodes); !reflect.DeepEqual(got, want) {
		t.Errorf("MCP slice_of = %v\nCLI = %v", got, want)
	}
	// Saved slice and viewer index.
	vj := filepath.Join(t.TempDir(), "views.json")
	if code, out := gloss(t, "view", "--json", "-o", vj, "--root", dir); code != 0 {
		t.Fatalf("view exit %d: %s", code, out)
	}
	b, _ := os.ReadFile(vj)
	var data struct {
		Views []struct {
			Name  string   `json:"name"`
			Slice []string `json:"slice"`
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		} `json:"views"`
		Slices struct {
			IDs []string           `json:"ids"`
			Of  map[string][][]int `json:"of"`
		} `json:"slices"`
	}
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Views) != 1 || data.Views[0].Name != "capture" || !reflect.DeepEqual(data.Views[0].Slice, []string{"pay:Capture"}) {
		t.Fatalf("views = %+v", data.Views)
	}
	if got := ids(data.Views[0].Nodes); !reflect.DeepEqual(got, want) {
		t.Errorf("saved slice view = %v\nCLI = %v", got, want)
	}
	var idx []string
	for _, m := range data.Slices.Of["pay:Capture"] {
		idx = append(idx, data.Slices.IDs[m[0]])
	}
	sort.Strings(idx)
	if !reflect.DeepEqual(idx, want) {
		t.Errorf("viewer index = %v\nCLI = %v", idx, want)
	}
	// Impact names the saved slice a change touches; a refund change does not touch it.
	if code, out := gloss(t, "impact", "services/api/handlers/capture.py", "--root", dir); code != 0 || !strings.Contains(out, "Saved slices touched") || !strings.Contains(out, "capture [pay:Capture]") {
		t.Errorf("impact (exit %d) does not name the capture slice:\n%s", code, out)
	}
	if _, out := gloss(t, "impact", "services/api/handlers/refund.py", "--root", dir); strings.Contains(out, "Saved slices touched") {
		t.Errorf("a refund change touches the capture slice:\n%s", out)
	}
	// context --concept briefs on the feature.
	if code, out := gloss(t, "context", "--concept", "pay:Capture", "--root", dir); code != 0 || !strings.Contains(out, "feature slice of pay:Capture") || !strings.Contains(out, "pa:CaptureHandler") {
		t.Errorf("context --concept (exit %d):\n%s", code, out)
	}
	// slice and include together are refused.
	writeTree(t, dir, map[string]string{"ontology/views.yaml": "views:\n  bad:\n    slice: [pay:Capture]\n    include: [pay:*]\n"})
	if code, out := gloss(t, "view", "--json", "-o", vj, "--root", dir); code == 0 && !strings.Contains(out, "slice and include") {
		t.Errorf("slice with include accepted (exit %d): %s", code, out)
	}
}
