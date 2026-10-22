// @gloss verifies gmi:GraphBrowser scope=file

package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/mckalvan/glossmark/internal/browse"
)

type fakeBrowser struct{ looked []string }

func (f *fakeBrowser) Lookup(ref string) (*browse.Node, error) {
	f.looked = append(f.looked, ref)
	switch ref {
	case "i:Doer":
		return &browse.Node{Ref: "i:Doer", Type: "concept", Kind: "process", Doc: "Does the thing.",
			Out: []browse.Group{{Relation: "realizes", Nodes: []browse.Neighbor{{Ref: "z:Doing", Type: "concept"}}}},
			In:  []browse.Group{{Relation: "implements", Nodes: []browse.Neighbor{{Ref: "a.py", Type: "artifact", Path: "a.py", Line: 1, Proposed: true}}}}}, nil
	case "z:Doing":
		return &browse.Node{Ref: "z:Doing", Type: "concept", Kind: "process"}, nil
	}
	return nil, errors.New("no node " + ref)
}

func (f *fakeBrowser) Index() ([]browse.Neighbor, error) {
	return []browse.Neighbor{{Ref: "o:b", Type: "boundary"}, {Ref: "i:Doer", Type: "concept"}}, nil
}

func TestBrowseTab(t *testing.T) {
	fb := &fakeBrowser{}
	f := &fakeEngine{items: queue()}
	a := New(f, Options{Actor: "human:alice", NoColor: true, Browser: fb})
	a.Reload()
	a.Update(N("tab")) // the stale link's concept: i:Doer, from the detail
	if a.tab != 1 || a.browse.node == nil || a.browse.node.Ref != "i:Doer" {
		t.Fatalf("browse opens at the item's concept: %+v", a.browse)
	}
	frame := a.Render(100, 24)
	for _, want := range []string{"[browse]", "i:Doer", "Does the thing.", "→ realizes (1)", "← implements (1)", "a.py?", "path: i:Doer"} {
		if !strings.Contains(frame, want) {
			t.Errorf("frame lacks %q:\n%s", want, frame)
		}
	}
	a.Update(N("enter")) // follow realizes → z:Doing
	if a.browse.node.Ref != "z:Doing" || len(a.browse.stack) != 2 {
		t.Fatalf("follow: %+v", a.browse.stack)
	}
	a.Update(R('h'))
	a.Update(R('j')) // onto a.py
	if eff := a.Update(R('o')); eff.Open == nil || eff.Open.Path != "a.py" {
		t.Errorf("open a neighbour: %+v", eff)
	}
	a.Update(R('i'))
	if a.browse.node != nil || !strings.Contains(a.Render(100, 24), "o:b") {
		t.Error("i shows the index")
	}
	a.Update(R('r')) // no decisions from the browser
	a.Update(N("tab"))
	if a.tab != 0 || len(f.decisions) != 0 {
		t.Errorf("back to the queue with nothing decided: tab %d, %+v", a.tab, f.decisions)
	}
}

// v opens the HTML viewer on the item's concept, or on the browsed node.
func TestOpenViewer(t *testing.T) {
	var seeds []string
	f := &fakeEngine{items: queue()}
	a := New(f, Options{Actor: "human:alice", NoColor: true, Browser: &fakeBrowser{},
		OpenViewer: func(seed string) (string, error) { seeds = append(seeds, seed); return "opened " + seed, nil }})
	a.Reload()
	a.Update(R('v')) // the stale link: its concept, from the detail
	a.Update(N("tab"))
	a.Update(N("enter")) // browse to z:Doing
	a.Update(R('v'))
	if got := strings.Join(seeds, ","); got != "i:Doer,z:Doing" {
		t.Errorf("seeds: %q", got)
	}
	if !strings.Contains(a.Render(100, 24), "opened z:Doing") {
		t.Error("the browse footer shows the viewer's message")
	}
	b := New(f, Options{Actor: "human:alice", NoColor: true})
	b.Reload()
	b.Update(R('v'))
	if b.statusOK || !strings.Contains(b.status, "gloss view") {
		t.Errorf("without an opener: %q", b.status)
	}
}
