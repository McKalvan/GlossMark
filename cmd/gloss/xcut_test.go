// @gloss verifies gmi:Gate scope=file

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/gate"
)

// TestDecoratorRemovalDiff: deleting a guard decorator above a handler is a
// change gloss diff reports, at the handler's criticality (ADR 0014, #72),
// and a graph committed by another release is re-extracted rather than read,
// so fingerprints that release computed differently do not read as changes.
func TestDecoratorRemovalDiff(t *testing.T) {
	root, write, git := gitRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Approving: { kind: process, criticality: high }\n  Listing: { kind: process }\n")
	write(".gloss.yaml", "boundary: o:b\nlevel: container\n")
	const handlers = "# @gloss implements z:Approving\n@bp.post(\"/approve\")\n@requires_role(\"approver\")\ndef approve(): pass\n\n\n# @gloss implements z:Listing\ndef listing(): pass\n"
	write("h.py", handlers)
	if code := cmdBuild([]string{"--root", root, "-q"}); code != 0 {
		t.Fatalf("gloss build exited %d", code)
	}
	// Stand in for a graph committed by another release that fingerprinted
	// every anchor differently.
	gp := filepath.Join(root, ".glossmark", "graph.jsonl")
	b, err := os.ReadFile(gp)
	if err != nil {
		t.Fatal(err)
	}
	old := regexp.MustCompile(`sha256:[0-9a-f]+`).ReplaceAllString(strings.Replace(string(b), `"generator":"gloss `, `"generator":"gloss 0.0.1-older+`, 1), "sha256:0000000000000000")
	os.WriteFile(gp, []byte(old), 0o644)
	git("add", ".")
	git("commit", "-q", "-m", "base")
	write("h.py", strings.Replace(handlers, "@requires_role(\"approver\")\n", "", 1))
	out := captureFunc(t, func() { cmdDiff([]string{"--root", root, "--base", "HEAD", "--format", "json"}) })
	var im gate.Impact
	if err := json.Unmarshal(out, &im); err != nil {
		t.Fatalf("diff json: %v\n%s", err, out)
	}
	var got []string
	for _, ch := range im.Changes {
		if ch.Kind != "concept-changed" {
			got = append(got, ch.Kind+" "+ch.Msg+" "+ch.Symbol+" "+ch.Criticality)
		}
	}
	// The ledger accepted the link with the guard in place, so it is stale
	// too; the undecorated listing() is untouched.
	want := []string{
		"stale stale implements z:Approving  high",
		`decorators-changed implements z:Approving decorators changed: removed @requires_role("approver") approve high`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("changes = %q, want %q", got, want)
	}
}
