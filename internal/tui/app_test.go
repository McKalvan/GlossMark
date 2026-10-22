// @gloss verifies gmi:ReviewTerminal scope=file

package tui

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mckalvan/glossmark/internal/review"
)

// fakeEngine records decisions and serves a fixed queue.
type fakeEngine struct {
	items     []review.Item
	decisions []review.Decision
	undos     int
	result    review.Result
}

func (f *fakeEngine) Queue() ([]review.Item, error) { return f.items, nil }
func (f *fakeEngine) Detail(id string) (*review.Detail, error) {
	for _, it := range f.items {
		if it.ID == id {
			return &review.Detail{Item: it, Context: []string{"   1  # marker"}, History: "commit abc 2026-10-01 alice\n+added\n-removed",
				Concepts: []review.ConceptInfo{{Curie: "i:Doer", Exists: true, Kind: "process", Tier: "implementation", Doc: "Does the thing."}}}, nil
		}
	}
	return nil, errors.New("no item")
}
func (f *fakeEngine) Decide(d review.Decision) review.Result {
	f.decisions = append(f.decisions, d)
	if f.result.Error != "" {
		return f.result
	}
	return review.Result{OK: true, Message: d.Action + " done", Undo: "1"}
}
func (f *fakeEngine) Undo(bool) (string, error) { f.undos++; return "undid it", nil }
func (f *fakeEngine) UndoDepth() int            { return len(f.decisions) - f.undos }
func (f *fakeEngine) Invalidate()               {}

func queue() []review.Item {
	return []review.Item{
		{ID: "m-1", Kind: review.KindStaleLink, Path: "a.py", Line: 1, Relation: "implements", Targets: []string{".Doer"}, Summary: "a.py:1 implements .Doer (stale)", Gate: "AUTO",
			Actions: []review.Action{{Name: review.ActReconfirm, Gate: "AUTO", AgentMay: true}, {Name: review.ActReject, Gate: "SOFT", AgentMay: true}}},
		{ID: "m-2", Kind: review.KindProposedLink, Path: "b.py", Line: 3, Relation: "implements", Targets: []string{"i:NEW:X"}, Summary: "b.py:3 implements i:NEW:X (proposed)", Gate: "HARD",
			Actions:        []review.Action{{Name: review.ActAccept, Gate: "HARD", Reasons: []string{"new concept"}}, {Name: review.ActReject, Gate: "HARD"}, {Name: review.ActDismiss, Gate: "AUTO", AgentMay: true}},
			Recommendation: &review.Recommendation{Item: "m-2", By: "agent:bot", Decision: review.Decision{Action: review.ActReject, Reason: "no such concept"}}},
		{ID: "finding:untested:i:Doer", Kind: review.KindFinding, Finding: "untested", Concept: "i:Doer", Summary: "untested: i:Doer has no test", Gate: "HARD",
			Actions: []review.Action{{Name: review.ActWaive, Gate: "HARD"}}},
	}
}

func newApp(f *fakeEngine) *App {
	a := New(f, Options{Actor: "human:alice", NoColor: true, Now: func() time.Time { return time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC) }})
	a.Reload()
	return a
}

func typeKeys(a *App, s string) {
	for _, r := range s {
		a.Update(R(r))
	}
}

func TestDecideFromKeys(t *testing.T) {
	f := &fakeEngine{items: queue()}
	a := newApp(f)
	a.Update(R('a')) // reconfirm the stale link
	a.Update(R('j'))
	a.Update(R('r')) // reject the proposal: reason, then remove?
	typeKeys(a, "wrong concept")
	a.Update(N("enter"))
	a.Update(R('y'))
	a.Update(N("enter"))
	a.Update(R('y')) // take the recommendation on it
	a.Update(R('j'))
	a.Update(R('w')) // waive: owner and expires prefilled
	a.Update(N("enter"))
	a.Update(N("enter"))
	typeKeys(a, "next sprint")
	a.Update(N("enter"))
	want := []review.Decision{
		{ID: "m-1", Action: review.ActReconfirm},
		{ID: "m-2", Action: review.ActReject, Reason: "wrong concept", Remove: true},
		{ID: "m-2", Action: review.ActReject, Reason: "no such concept"},
		{ID: "finding:untested:i:Doer", Action: review.ActWaive, Owner: "alice", Expires: "2026-11-21", Reason: "next sprint"},
	}
	if len(f.decisions) != len(want) {
		t.Fatalf("decisions: %+v", f.decisions)
	}
	for i := range want {
		if f.decisions[i] != want[i] {
			t.Errorf("decision %d: %+v, want %+v", i, f.decisions[i], want[i])
		}
	}
}

func TestFormRequiredAndCancel(t *testing.T) {
	f := &fakeEngine{items: queue()}
	a := newApp(f)
	a.Update(R('r'))
	a.Update(N("enter")) // an empty reason is refused
	if a.form == nil || !strings.Contains(a.status, "required") {
		t.Fatalf("empty required field: form %v status %q", a.form, a.status)
	}
	typeKeys(a, "x")
	a.Update(N("backspace"))
	a.Update(N("esc"))
	if a.form != nil || len(f.decisions) != 0 {
		t.Errorf("esc should cancel without deciding: %+v", f.decisions)
	}
	if eff := a.Update(R('q')); !eff.Quit {
		t.Error("q should quit")
	}
}

func TestRenderFrame(t *testing.T) {
	f := &fakeEngine{items: queue()}
	a := newApp(f)
	a.Update(R('j'))
	frame := a.Render(120, 30)
	rows := strings.Split(frame, "\r\n")
	if len(rows) != 30 {
		t.Fatalf("%d rows, want 30", len(rows))
	}
	for i, r := range rows {
		if n := len([]rune(r)); n != 120 {
			t.Errorf("row %d is %d cells: %q", i, n, r)
		}
	}
	for _, want := range []string{"human:alice", "1 proposed · 1 stale · 1 finding", "▸ HARD ★ proposed  b.py:3 implements i:NEW:X",
		"agent:bot recommends reject: no such concept", "i:Doer", "Does the thing.", "+added", "a accept [HARD] human  new concept"} {
		if !strings.Contains(frame, want) {
			t.Errorf("frame lacks %q:\n%s", want, frame)
		}
	}
	// A narrow terminal stacks the list over the detail.
	narrow := a.Render(80, 24)
	if !strings.Contains(narrow, strings.Repeat("─", 80)) {
		t.Errorf("narrow frame should stack:\n%s", narrow)
	}
	// Color is applied unless it is off.
	a.opt.NoColor = false
	if !regexp.MustCompile(`\x1b\[31mHARD`).MatchString(a.Render(120, 30)) {
		t.Error("HARD should render red")
	}
}

func TestFiltersAndUndo(t *testing.T) {
	f := &fakeEngine{items: queue()}
	a := newApp(f)
	a.Update(R('f')) // proposed links only
	if len(a.visible) != 1 || a.Selected().ID != "m-2" {
		t.Fatalf("kind filter: %v", a.visible)
	}
	a.Update(N("esc"))
	a.Update(R('/'))
	typeKeys(a, "untested")
	a.Update(N("enter"))
	if len(a.visible) != 1 || a.Selected().Kind != review.KindFinding {
		t.Fatalf("text filter: %v", a.visible)
	}
	a.Update(R('u'))
	if f.undos != 1 || !strings.Contains(a.status, "undid") {
		t.Errorf("undo: %d %q", f.undos, a.status)
	}
	f.result = review.Result{Error: "HARD: a human decides this"}
	a.Update(R('w'))
	a.Update(N("enter"))
	a.Update(N("enter"))
	a.Update(N("enter"))
	if a.statusOK || !strings.Contains(a.status, "HARD") {
		t.Errorf("a refused decision shows its error: %q", a.status)
	}
	if eff := a.Update(N("enter")); eff.Open != nil {
		t.Error("a finding without a path has nothing to open")
	}
}

func TestDecode(t *testing.T) {
	got := Decode([]byte("j\x1b[Ak\r\x7f\x1b[6~\x03é\x1b"))
	var names []string
	for _, k := range got {
		names = append(names, k.String())
	}
	if want := "j up k enter backspace pgdn ctrl-c é esc"; strings.Join(names, " ") != want {
		t.Errorf("Decode: %q, want %q", strings.Join(names, " "), want)
	}
}
