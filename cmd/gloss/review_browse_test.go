// @gloss verifies gmi:GraphBrowser scope=file

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

// v builds the viewer and hands the browser a deep link to the slice.
func TestViewerOpener(t *testing.T) {
	root, e := reviewTree(t)
	got := filepath.Join(t.TempDir(), "url")
	script := filepath.Join(t.TempDir(), "browser")
	os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$1\" > "+got+"\n"), 0o755)
	t.Setenv("BROWSER", script)
	msg, err := viewerOpener(e)("i:Doer")
	if err != nil || !strings.Contains(msg, "i:Doer") {
		t.Fatalf("open: %q %v", msg, err)
	}
	var b []byte
	for i := 0; i < 50 && len(b) == 0; i++ {
		time.Sleep(20 * time.Millisecond)
		b, _ = os.ReadFile(got)
	}
	want := "file://" + filepath.ToSlash(filepath.Join(root, ".glossmark", "viewer.html")) + "#slice=i:Doer"
	if string(b) != want {
		t.Errorf("browser got %q, want %q", b, want)
	}
	if _, err := os.Stat(filepath.Join(root, ".glossmark", "viewer.html")); err != nil {
		t.Errorf("viewer not built: %v", err)
	}
	t.Setenv("BROWSER", "")
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	if runtime.GOOS == "linux" {
		if msg, _ := viewerOpener(e)(""); !strings.Contains(msg, "no browser here, open file://") {
			t.Errorf("headless: %q", msg)
		}
	}
}
