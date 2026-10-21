package ledger

import (
	"reflect"
	"testing"

	"github.com/mckalvan/glossmark/internal/model"
)

// Rename moves a row to a marker's new id with its history (ADR 0027): it
// replaces a removed row an older release left there, keeps a live one, and
// never lists the id it lands on among its previous ids.
func TestRename(t *testing.T) {
	l := &Ledger{Entries: map[string]*model.LedgerEntry{
		"m-a":    {MarkerID: "m-a", Status: model.StatusProposed, Actor: "agent:x", Created: "t0"},
		"m-b":    {MarkerID: "m-b", Status: model.StatusRemoved, PreviousIDs: []string{"m-old"}},
		"m-live": {MarkerID: "m-live", Status: model.StatusAccepted},
	}}
	if !l.Rename("m-a", "m-b") {
		t.Fatal("rename onto a removed row refused")
	}
	e := l.Entries["m-b"]
	if _, ok := l.Entries["m-a"]; ok || e.MarkerID != "m-b" || e.Actor != "agent:x" || e.Created != "t0" || e.Status != model.StatusProposed ||
		!reflect.DeepEqual(e.PreviousIDs, []string{"m-old", "m-a"}) {
		t.Errorf("after rename: %+v", e)
	}
	if !l.Rename("m-b", "m-a") || !reflect.DeepEqual(l.Entries["m-a"].PreviousIDs, []string{"m-old", "m-b"}) {
		t.Errorf("rename back: %+v", l.Entries["m-a"])
	}
	if l.Rename("m-a", "m-live") || l.Entries["m-live"].Status != model.StatusAccepted {
		t.Errorf("rename onto a live row went through")
	}
	if l.Rename("m-missing", "m-c") {
		t.Errorf("rename of a missing row went through")
	}
}
