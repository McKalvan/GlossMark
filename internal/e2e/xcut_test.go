// @gloss verifies gmi:Gate, gmi:DriftComparator scope=file

package e2e

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mckalvan/glossmark/internal/ledger"
	"github.com/mckalvan/glossmark/internal/model"
)

// TestDecoratorFingerprintMigrates: a link accepted before the decorator
// stack joined the fingerprint (ADR 0014) stays accepted after the upgrade,
// and is stale once a decorator is then deleted.
func TestDecoratorFingerprintMigrates(t *testing.T) {
	root, write := tempRepo(t)
	write("glossmark.yaml", "owns:\n  z: { iri: https://x/z/, tier: core }\n")
	write("ontology/z.gloss.yaml", "concepts:\n  Approving: { kind: process }\n")
	const src = "# @gloss implements z:Approving\n@requires_role(\"approver\")\ndef approve(): pass\n"
	write("h.py", src)
	_, g, res, _ := build(t, root)
	lp := filepath.Join(t.TempDir(), "ledger.jsonl")
	lg, _ := ledger.Open(lp)
	lg.Sync(g, res.Markers, "human:test", time.Now())
	if len(res.Markers) != 1 {
		t.Fatalf("%d markers", len(res.Markers))
	}
	id := res.Markers[0].ID
	// What the previous release recorded: the signature line alone.
	lg.Entries[id].AnchorFingerprint = res.Markers[0].Anchor.Legacy
	if lg.Entries[id].AnchorFingerprint == "" || lg.Entries[id].AnchorFingerprint == res.Markers[0].Anchor.Fingerprint {
		t.Fatalf("no legacy fingerprint for a decorated anchor: %+v", res.Markers[0].Anchor)
	}
	lg.Sync(g, res.Markers, "human:test", time.Now())
	if e := lg.Entries[id]; e.Status != model.StatusAccepted || e.AnchorFingerprint != res.Markers[0].Anchor.Fingerprint {
		t.Errorf("after upgrade: %s %s, want accepted with the new fingerprint", e.Status, e.AnchorFingerprint)
	}
	write("h.py", "# @gloss implements z:Approving\ndef approve(): pass\n")
	_, g, res, _ = build(t, root)
	lg.Sync(g, res.Markers, "human:test", time.Now())
	if e := lg.Entries[id]; e.Status != model.StatusStale {
		t.Errorf("guard deleted: %s, want stale", e.Status)
	}
}
