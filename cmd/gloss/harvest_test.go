// @gloss verifies gmi:EvidenceHarvester scope=file

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/harvest"
)

// TestHarvestCommand: evidence round-trips through --in, external evidence
// is validated by --ingest, and --cluster summarises candidates.
func TestHarvestCommand(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "harvest", "testdata", "library")
	code, out := gloss(t, "harvest", "--root", root, "--no-git")
	if code != 0 {
		t.Fatalf("harvest exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.Contains(lines[0], `"type":"evidence"`) || !strings.Contains(lines[0], `"format":1`) {
		t.Errorf("header = %s", lines[0])
	}
	var e harvest.Evidence
	if err := json.Unmarshal([]byte(lines[1]), &e); err != nil || e.ID == "" || e.Signal == "" {
		t.Errorf("first record %s: %v", lines[1], err)
	}
	dir := t.TempDir()
	saved := filepath.Join(dir, "evidence.jsonl")
	if err := os.WriteFile(saved, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	ext := filepath.Join(dir, "connector.jsonl")
	if err := os.WriteFile(ext, []byte(`{"signal":"doc-heading","surface":"Interlibrary loan","source":{"family":"doc-connector","uri":"gdrive://prd","locator":"heading:ILL"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, merged := gloss(t, "harvest", "--in", saved, "--ingest", ext)
	if code != 0 || len(strings.Split(strings.TrimSpace(merged), "\n")) != len(lines)+1 || !strings.Contains(merged, "gdrive://prd") {
		t.Errorf("ingest exit %d, %d lines", code, len(strings.Split(strings.TrimSpace(merged), "\n")))
	}
	code, text := gloss(t, "harvest", "--in", saved, "--ingest", ext, "--cluster", "--format", "text")
	if code != 0 || !strings.Contains(text, "candidate concepts:") || !strings.Contains(text, "Interlibrary loan") {
		t.Errorf("cluster text exit %d:\n%s", code, text)
	}
	bad := filepath.Join(dir, "bad.jsonl")
	if err := os.WriteFile(bad, []byte(`{"signal":"table","surface":"loans","source":{"family":"doc","path":"x"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _ := gloss(t, "harvest", "--in", saved, "--ingest", bad); code != 1 {
		t.Errorf("a record whose family contradicts its signal was ingested (exit %d)", code)
	}
	if code, _ := gloss(t, "harvest", "--in", saved, "--format", "text"); code != 1 {
		t.Errorf("--format text without --cluster: exit %d", code)
	}
}
