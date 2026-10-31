package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A mapped link (a links: entry, ADR 0045) is reviewed like a marker:
// status names its source, accept drops proposed: true from the entry,
// reject --remove deletes the entry, and gloss review node says where the
// link is written.
func TestMappedLinkReview(t *testing.T) {
	t.Setenv("GLOSS_ACTOR", "human:test")
	root := tiny(t, map[string]string{
		".gloss.yaml": "boundary: o:b\nlevel: container\ndefault-namespace: i\nlinks:\n  - { symbol: run, implements: .Doer, conf: 0.9, proposed: true }\n  - path: \"*.py\"\n    verifies: z:Doing\n    proposed: true\n",
		"svc.py":      "def run(): pass\n",
	})
	manifest := filepath.Join(root, ".gloss.yaml")
	if code, out := gloss(t, "status", "--root", root); code != 0 || !strings.Contains(out, "svc.py (manifest)  implements .Doer") {
		t.Fatalf("status: exit %d\n%s", code, out)
	}
	if code, out := gloss(t, "review", "node", "svc.py#run", "--root", root); code != 0 || !strings.Contains(out, "→ implements: i:Doer? [manifest .gloss.yaml:5]") {
		t.Errorf("review node: exit %d\n%s", code, out)
	}
	if code, out := gloss(t, "accept", ".gloss.yaml:5", "--root", root); code != 0 || !strings.Contains(out, "accept .gloss.yaml:5 implements .Doer") {
		t.Fatalf("accept: exit %d\n%s", code, out)
	}
	b, _ := os.ReadFile(manifest)
	if !strings.Contains(string(b), "{ symbol: run, implements: .Doer, conf: 0.9 }") {
		t.Errorf("accept left the entry as:\n%s", b)
	}
	if code, out := gloss(t, "reject", ".gloss.yaml:6", "--reason", "not a test", "--remove", "--root", root); code != 0 || !strings.Contains(out, "reject .gloss.yaml:6 verifies z:Doing") {
		t.Fatalf("reject: exit %d\n%s", code, out)
	}
	b, _ = os.ReadFile(manifest)
	if strings.Contains(string(b), "verifies") || strings.Contains(string(b), "proposed") {
		t.Errorf("reject --remove left the manifest as:\n%s", b)
	}
	if st := ledgerStatus(t, root, "svc.py"); len(st) != 2 || !strings.Contains(strings.Join(st, ","), "accepted") || !strings.Contains(strings.Join(st, ","), "rejected") {
		t.Errorf("ledger states %v, want accepted and rejected", st)
	}
}
