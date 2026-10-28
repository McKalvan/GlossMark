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
	if a.Selected().ID != "m-1" {
		t.Fatalf("the fake keeps every item, so the cursor stays: %s", a.Selected().ID)
	}
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
	for _, want := range []string{"human:alice", "0 of 2 decided │ 2 decisions: 1 stale · 1 proposed │ 1 finding",
		" Decisions · 2", " Findings · 1", "AUTO/SOFT   stale     implements .Doer  a.py:1", "▸ HARD      ★ proposed  implements i:NEW:X  b.py:3",
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
	a.Update(R('j'))
	if a.status != "" {
		t.Errorf("the next key clears the status: %q", a.status)
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

// Deciding an item that leaves the queue moves the cursor to the next one,
// not back to the top.
func TestCursorMovesOn(t *testing.T) {
	f := &fakeEngine{items: queue()}
	a := newApp(f)
	a.Update(R('j'))
	f.items = append(f.items[:1:1], f.items[2:]...) // m-2 decided
	a.Reload()
	if it := a.Selected(); it == nil || it.ID != "finding:untested:i:Doer" {
		t.Errorf("cursor after the item left: %+v", it)
	}
	f.items = f.items[:1] // the last one decided too
	a.Reload()
	if it := a.Selected(); it == nil || it.ID != "m-1" {
		t.Errorf("cursor clamps to the list: %+v", it)
	}
}

// mixed is a queue whose engine order puts a rogue after a finding.
func mixed() []review.Item {
	waive := review.Action{Name: review.ActWaive, Gate: "HARD"}
	return []review.Item{
		{ID: "m-1", Kind: review.KindProposedLink, Path: "services/circulation/loans/renew.py", Line: 1, Relation: "implements", Targets: []string{"lib:Renewal"}, Gate: "SOFT",
			Actions: []review.Action{{Name: review.ActAccept, Gate: "SOFT", AgentMay: true}, {Name: review.ActReject, Gate: "SOFT", AgentMay: true}}},
		{ID: "finding:orphan:lib:Hold", Kind: review.KindFinding, Finding: "orphan", Concept: "lib:Hold", Path: "ontology/lib.gloss.yaml", Line: 4,
			Summary: "orphan: lib:Hold: orphan concept", Gate: "HARD", Actions: []review.Action{waive}},
		{ID: "finding:rogue:x", Kind: review.KindFinding, Finding: "rogue", Concept: "circ:NEW:DamageCheck", Relation: "implements",
			Path: "services/circulation/loans/returns.py", Line: 10, Summary: "rogue: x", Gate: "SOFT",
			Actions: []review.Action{{Name: review.ActPropose, Gate: "SOFT", AgentMay: true}, waive}},
	}
}

func TestGroupsAndProgress(t *testing.T) {
	f := &fakeEngine{items: mixed()}
	a := newApp(f)
	var ids []string
	for _, i := range a.visible {
		ids = append(ids, a.items[i].ID)
	}
	if got := strings.Join(ids, " "); got != "m-1 finding:rogue:x finding:orphan:lib:Hold" {
		t.Fatalf("decisions first, then findings: %s", got)
	}
	if f.items[1].ID != "finding:orphan:lib:Hold" {
		t.Error("the engine's own order must stay as it was")
	}
	frame := a.Render(120, 20)
	for _, want := range []string{"0 of 2 decided", " Decisions · 2", " Findings · 1", "SOFT/HARD   rogue"} {
		if !strings.Contains(frame, want) {
			t.Errorf("frame lacks %q:\n%s", want, frame)
		}
	}
	// The rogue is proposed: it leaves, and the proposed concept arrives.
	f.items = []review.Item{f.items[0], f.items[1], {ID: "concept:circ:DamageCheck", Kind: review.KindProposedConcept, Concept: "circ:DamageCheck", Gate: "HARD",
		Actions: []review.Action{{Name: review.ActAccept, Gate: "HARD"}}}}
	a.Reload()
	if h := a.renderLine(a.header(), 200, ""); !strings.Contains(h, "1 of 2 decided · 1 new") {
		t.Errorf("after propose: %s", h)
	}
	// Undo puts the rogue back; it counts as open again.
	f.items = mixed()
	a.Reload()
	if h := a.renderLine(a.header(), 200, ""); !strings.Contains(h, "0 of 2 decided │") {
		t.Errorf("after undo: %s", h)
	}
}

// The gate column shows the range of an item's gates when they differ;
// dismissing a recommendation does not count.
func TestGateLabel(t *testing.T) {
	q, m := queue(), mixed()
	for _, c := range []struct {
		it   review.Item
		want string
	}{{q[0], "AUTO/SOFT"}, {q[1], "HARD"}, {q[2], "HARD"}, {m[0], "SOFT"}, {m[2], "SOFT/HARD"}} {
		if got := gateLabel(&c.it); got != c.want {
			t.Errorf("%s: gate %q, want %q", c.it.ID, got, c.want)
		}
	}
}

// The footer lists what the selected item allows, as its actions say,
// then the keys that work anywhere.
func TestFooterFollowsItem(t *testing.T) {
	a := newApp(&fakeEngine{items: mixed()})
	foot := func() string { return strings.Join(a.footerKeys(), "  ") }
	if got := foot(); !strings.HasPrefix(got, "a accept  r reject  ") || strings.Contains(got, "w waive") || strings.Contains(got, "u undo") {
		t.Errorf("proposed link: %s", got)
	}
	a.Update(R('j'))
	if got := foot(); !strings.HasPrefix(got, "p propose  w waive  ") || strings.Contains(got, "a accept") {
		t.Errorf("rogue: %s", got)
	}
	a.Update(R('j'))
	if got := foot(); got != "w waive  e doc  o open  n link  v viewer  ? help  q quit" {
		t.Errorf("orphan: %s", got)
	}
}

func TestHelpPointsToGlossary(t *testing.T) {
	a := newApp(&fakeEngine{items: queue()})
	a.Update(R('?'))
	if frame := a.Render(120, 40); !strings.Contains(frame, "docs/GLOSSARY.md") {
		t.Errorf("help lacks the glossary:\n%s", frame)
	}
}

// Rows that differ only in the file name keep it: the path is cut from
// the left, after the relation and target.
func TestListKeepsFileNames(t *testing.T) {
	for _, c := range []struct {
		head, tail string
		w          int
		want       string
	}{
		{"implements lib:Renewal", "services/circulation/loans/renew.py:1", 61, "implements lib:Renewal  services/circulation/loans/renew.py:1"},
		{"implements lib:Renewal", "services/circulation/loans/renew.py:1", 44, "implements lib:Renewal  …/loans/renew.py:1"},
		{"implements lib:Renewal", "services/circulation/loans/returns.py:10", 32, "implements lib…  …/returns.py:10"},
		{"x", "日本語/ファイル.py:3", 12, "…ァイル.py:3"},
	} {
		got := fitPair(c.head, c.tail, c.w)
		if got != c.want || strWidth(got) > c.w {
			t.Errorf("fitPair(%d) = %q (%d cells), want %q", c.w, got, strWidth(got), c.want)
		}
	}
}

// Wide runes take two cells and combining marks none: a row stays
// exactly w cells.
func TestRenderWideRunes(t *testing.T) {
	a := newApp(&fakeEngine{})
	for _, s := range []string{"日本語のパス/ファイル.py", "cafe\u0301 résumé", "emoji 🎉 ok", "ab日"} {
		for w := 1; w < 30; w++ {
			if got := strWidth(a.renderLine(plain(s), w, "")); got != w {
				t.Errorf("%q at %d: %d cells", s, w, got)
			}
		}
	}
	if got := truncLeft("a/日本/ファイル.py", 14); got != "…/ファイル.py" {
		t.Errorf("truncLeft: %q", got)
	}
}
