// @gloss verifies gmi:ReviewCommands scope=file

package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/model"
)

// TestReacceptStale: an accepted link whose declaration changed is stale, and
// `gloss accept <path:line>` re-accepts it against the new declaration without
// editing the source, recording the reviewer; the next build keeps it accepted.
func TestReacceptStale(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) { writeFile(t, filepath.Join(root, rel), content) }
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n  i: { iri: https://x/i/, tier: implementation }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Doing: { kind: process }\n")
	write("ontology/i.gloss.yaml", "concepts:\n  Doer: { kind: process, realizes: z:Doing }\n")
	write(".gloss.yaml", "boundary: o:b\nlevel: container\ndefault-namespace: i\n")
	write("a.py", "# @gloss implements .Doer\ndef go(x): pass\n")
	write("b.py", "# @gloss implements .Doer\ndef other(x): pass\n")
	alice := []string{"GLOSS_ACTOR=human:alice"}
	status := func(path string) model.LedgerEntry {
		t.Helper()
		lg, err := ledger.Open(filepath.Join(root, ".glossmark", "ledger.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range lg.Entries {
			if e.Path == path {
				return *e
			}
		}
		t.Fatalf("no ledger entry for %s", path)
		return model.LedgerEntry{}
	}

	if r := gloss(t, root, "", alice, "build", "-q"); r.code != 0 {
		t.Fatalf("build: %d %s", r.code, r.stderr)
	}
	write("a.py", "# @gloss implements .Doer\ndef go(x, y): pass\n")
	if r := gloss(t, root, "", alice, "build", "-q"); r.code != 0 {
		t.Fatalf("build: %d %s", r.code, r.stderr)
	}
	if e := status("a.py"); e.Status != model.StatusStale {
		t.Fatalf("a changed declaration should make the link stale: %+v", e)
	}

	r := gloss(t, root, "", []string{"GLOSS_ACTOR=human:bob"}, "accept", "a.py:1")
	if r.code != 0 || !strings.Contains(r.stdout, "re-accept a.py:1 implements") {
		t.Fatalf("accept of a stale link: %d %s %s", r.code, r.stdout, r.stderr)
	}
	if got := readFile(t, filepath.Join(root, "a.py")); !strings.HasPrefix(got, "# @gloss implements") {
		t.Errorf("re-accepting must not edit the marker:\n%s", got)
	}
	if e := status("a.py"); e.Status != model.StatusAccepted || e.ReviewedBy != "human:bob" {
		t.Errorf("the stale link should be accepted by bob: %+v", e)
	}
	if r := gloss(t, root, "", alice, "build", "-q"); r.code != 0 {
		t.Fatalf("build: %d %s", r.code, r.stderr)
	}
	if e := status("a.py"); e.Status != model.StatusAccepted {
		t.Errorf("the next build should keep the re-accepted link accepted: %+v", e)
	}

	// An accepted, current link is neither proposed nor stale.
	r = gloss(t, root, "", alice, "accept", "b.py:1")
	if !strings.Contains(r.stderr, "not proposed or stale") {
		t.Errorf("accepting a current link should say there is nothing to do: %d %s %s", r.code, r.stdout, r.stderr)
	}
}
