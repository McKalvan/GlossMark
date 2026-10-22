// @gloss implements gmi:ReviewTerminal scope=file

// Package tui is the interactive terminal for gloss review (ADR 0033): the
// review queue beside the selected item's context, history and concepts, one
// key per decision, prompts for what a decision needs, and undo. App is the
// state machine and renders frames as strings; Run drives it on a real
// terminal. Nothing here decides anything: every action goes to the review
// engine, which applies the same rules as the headless mode.
package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/review"
)

// Engine is what the terminal needs from a review session.
type Engine interface {
	Queue() ([]review.Item, error)
	Detail(id string) (*review.Detail, error)
	Decide(review.Decision) review.Result
	Undo(force bool) (string, error)
	UndoDepth() int
	Invalidate()
}

// Options configure an App.
type Options struct {
	Actor   string
	NoColor bool
	Now     func() time.Time
	// Browser, when set, adds the read-only Browse tab (tab switches).
	Browser Browser
	// OpenViewer, when set, builds the HTML viewer and opens it focused on
	// a concept or boundary ("" for the whole graph); v calls it. It
	// returns what to tell the user: where the page opened, or how to.
	OpenViewer func(curie string) (string, error)
}

// Effect is what the caller must do after an Update.
type Effect struct {
	Quit bool
	// Open asks for the editor on Path at Line.
	Open *Location
}

// Location is a file position.
type Location struct {
	Path string
	Line int
}

// field is one input a prompt asks for.
type field struct {
	key, label, value string
	required, yesno   bool
}

type form struct {
	title  string
	fields []field
	at     int
	submit func(map[string]string)
}

// App is the terminal's state.
type App struct {
	eng Engine
	opt Options

	items   []review.Item
	visible []int // indexes into items after the filters
	cur     int   // index into visible
	top     int   // first visible row of the list

	detail    *review.Detail
	detailErr string
	dscroll   int

	kind     string // kind filter: "" for all
	text     string // text filter
	form     *form
	status   string
	statusOK bool
	help     bool
	tab      int // 0 the queue, 1 the graph browser

	browse *browseState
	w, h   int
}

// New returns an App over a review engine; call Reload before the first frame.
func New(eng Engine, opt Options) *App {
	if opt.Now == nil {
		opt.Now = time.Now
	}
	a := &App{eng: eng, opt: opt}
	if opt.Browser != nil {
		a.browse = &browseState{}
	}
	return a
}

// kindFilters are the values f cycles through.
var kindFilters = []string{"", review.KindProposedLink, review.KindStaleLink, review.KindProposedConcept, review.KindValueDrift, review.KindFinding, review.KindRecommendation}

// Reload re-reads the queue and keeps the cursor on the same item when it
// is still there.
func (a *App) Reload() {
	keep := a.selectedID()
	a.eng.Invalidate()
	items, err := a.eng.Queue()
	if err != nil {
		a.setStatus(false, err.Error())
		items = nil
	}
	a.items = items
	a.refilter(keep)
}

func (a *App) refilter(keep string) {
	prev := a.cur
	a.visible = a.visible[:0]
	for i, it := range a.items {
		if a.kind != "" && it.Kind != a.kind {
			continue
		}
		if a.text != "" && !strings.Contains(strings.ToLower(it.Summary+" "+it.ID+" "+it.Path), strings.ToLower(a.text)) {
			continue
		}
		a.visible = append(a.visible, i)
	}
	// The same item when it is still listed; otherwise the one that took
	// its place, so deciding an item moves on to the next.
	a.cur = prev
	for vi, i := range a.visible {
		if a.items[i].ID == keep {
			a.cur = vi
		}
	}
	a.clamp()
	a.loadDetail()
}

func (a *App) clamp() {
	if a.cur >= len(a.visible) {
		a.cur = len(a.visible) - 1
	}
	if a.cur < 0 {
		a.cur = 0
	}
}

// Selected returns the item under the cursor, or nil.
func (a *App) Selected() *review.Item {
	if a.cur < 0 || a.cur >= len(a.visible) {
		return nil
	}
	return &a.items[a.visible[a.cur]]
}

func (a *App) selectedID() string {
	if it := a.Selected(); it != nil {
		return it.ID
	}
	return ""
}

func (a *App) loadDetail() {
	a.dscroll = 0
	a.detail, a.detailErr = nil, ""
	it := a.Selected()
	if it == nil {
		return
	}
	d, err := a.eng.Detail(it.ID)
	if err != nil {
		a.detailErr = err.Error()
		return
	}
	a.detail = d
}

func (a *App) setStatus(ok bool, msg string) {
	a.statusOK, a.status = ok, strings.Join(strings.Fields(msg), " ")
}

func (a *App) move(n int) {
	a.cur += n
	a.clamp()
	a.loadDetail()
}

// Update applies one key and returns what the caller must do.
func (a *App) Update(k Key) Effect {
	if k.Name == "ctrl-c" {
		return Effect{Quit: true}
	}
	if a.form != nil {
		a.updateForm(k)
		return Effect{}
	}
	// A status message answers the last key; the next one clears it.
	a.status = ""
	if a.help {
		a.help = false
		return Effect{}
	}
	if k.Name == "tab" && a.browse != nil {
		a.tab = 1 - a.tab
		if a.tab == 1 {
			a.enterBrowse()
		}
		return Effect{}
	}
	if a.tab == 1 {
		return a.updateBrowse(k)
	}
	switch k.String() {
	case "q":
		return Effect{Quit: true}
	case "?":
		a.help = true
	case "j", "down":
		a.move(1)
	case "k", "up":
		a.move(-1)
	case "g", "home":
		a.move(-len(a.visible))
	case "G", "end":
		a.move(len(a.visible))
	case "pgdn", "ctrl-d", " ":
		a.dscroll += a.pageSize()
	case "pgup", "ctrl-u", "b":
		a.dscroll -= a.pageSize()
		if a.dscroll < 0 {
			a.dscroll = 0
		}
	case "f":
		a.kind = kindFilters[(indexOf(kindFilters, a.kind)+1)%len(kindFilters)]
		a.refilter(a.selectedID())
	case "/":
		a.ask("filter", func(v map[string]string) { a.text = v["text"]; a.refilter(a.selectedID()) },
			field{key: "text", label: "filter (empty clears)", value: a.text})
	case "esc":
		if a.text != "" || a.kind != "" {
			a.text, a.kind = "", ""
			a.refilter(a.selectedID())
		}
	case "R":
		a.Reload()
		a.setStatus(true, "reloaded")
	case "u":
		msg, err := a.eng.Undo(false)
		if err != nil && strings.Contains(err.Error(), "changed since") {
			a.ask("undo", func(v map[string]string) {
				if v["force"] == "y" {
					a.report(a.eng.Undo(true))
				}
			}, field{key: "force", label: err.Error() + "; undo anyway?", yesno: true, value: "n"})
			return Effect{}
		}
		a.report(msg, err)
	case "n":
		a.newLink()
	case "v":
		seed := ""
		if it := a.Selected(); it != nil {
			if c, _ := a.conceptOf(it); c != "" && !strings.Contains(c, ":NEW:") {
				seed = c
			} else {
				seed = it.Boundary
			}
		}
		a.openViewer(seed)
	default:
		return a.itemKey(k)
	}
	return Effect{}
}

// itemKey handles the keys that act on the selected item.
func (a *App) itemKey(k Key) Effect {
	it := a.Selected()
	if it == nil {
		return Effect{}
	}
	switch k.String() {
	case "enter", "o":
		if it.Path != "" {
			return Effect{Open: &Location{Path: it.Path, Line: it.Line}}
		}
	case "a":
		for _, name := range []string{review.ActAccept, review.ActReconfirm, review.ActConfirm, review.ActApply} {
			if it.Action(name) != nil {
				a.decide(review.Decision{ID: it.ID, Action: name})
				return Effect{}
			}
		}
		a.setStatus(false, "nothing to accept here: "+hints(it))
	case "r":
		if it.Action(review.ActReject) == nil {
			a.setStatus(false, "this item cannot be rejected")
			return Effect{}
		}
		a.ask("reject "+it.ID, func(v map[string]string) {
			a.decide(review.Decision{ID: it.ID, Action: review.ActReject, Reason: v["reason"], Remove: v["remove"] == "y"})
		}, field{key: "reason", label: "reason", required: true}, field{key: "remove", label: "also delete the marker line (or concept entry)?", yesno: true, value: "n"})
	case "w":
		if it.Action(review.ActWaive) == nil {
			a.setStatus(false, "only a drift finding can be waived")
			return Effect{}
		}
		owner := a.opt.Actor
		if _, n, ok := strings.Cut(owner, ":"); ok {
			owner = n
		}
		a.ask("waive "+it.ID, func(v map[string]string) {
			a.decide(review.Decision{ID: it.ID, Action: review.ActWaive, Owner: v["owner"], Expires: v["expires"], Reason: v["reason"]})
		}, field{key: "owner", label: "owner", value: owner, required: true},
			field{key: "expires", label: "expires (YYYY-MM-DD)", value: a.opt.Now().AddDate(0, 0, 30).Format("2006-01-02"), required: true},
			field{key: "reason", label: "reason"})
	case "p":
		if it.Action(review.ActPropose) == nil {
			a.setStatus(false, "only a rogue ns:NEW: target can be proposed")
			return Effect{}
		}
		a.ask("propose "+it.Concept, func(v map[string]string) {
			a.decide(review.Decision{ID: it.ID, Action: review.ActPropose, Kind: v["kind"], Doc: v["doc"]})
		}, field{key: "kind", label: "kind (empty: from the relation)"}, field{key: "doc", label: "doc: one sentence"})
	case "d":
		if it.Action(review.ActDismiss) == nil {
			a.setStatus(false, "no recommendation to dismiss")
			return Effect{}
		}
		a.decide(review.Decision{ID: it.ID, Action: review.ActDismiss})
	case "y":
		r := it.Recommendation
		if r == nil {
			a.setStatus(false, "no recommendation on this item")
			return Effect{}
		}
		if it.Kind == review.KindRecommendation {
			if it.Action(review.ActApply) == nil {
				a.setStatus(false, "this recommendation's item is gone; d dismisses it")
				return Effect{}
			}
			a.decide(review.Decision{ID: it.ID, Action: review.ActApply})
			return Effect{}
		}
		d := r.Decision
		d.ID, d.Recommend = it.ID, false
		a.decide(d)
	case "e":
		c, doc := a.conceptOf(it)
		if c == "" {
			a.setStatus(false, "no concept to edit here")
			return Effect{}
		}
		a.ask("edit the doc of "+c, func(v map[string]string) {
			a.decide(review.Decision{Action: review.ActEditDoc, Curie: c, Doc: v["doc"]})
		}, field{key: "doc", label: "doc", value: doc, required: true})
	}
	return Effect{}
}

// conceptOf is the concept an item is about, with its current doc.
func (a *App) conceptOf(it *review.Item) (string, string) {
	if a.detail != nil && a.detail.ID == it.ID {
		for _, c := range a.detail.Concepts {
			if c.Exists {
				return c.Curie, c.Doc
			}
		}
	}
	return it.Concept, ""
}

func (a *App) newLink() {
	path, line := "", ""
	if it := a.Selected(); it != nil && it.Path != "" {
		path = it.Path
		if it.Line > 0 {
			line = strconv.Itoa(it.Line)
		}
	}
	a.ask("write a link", func(v map[string]string) {
		n, _ := strconv.Atoi(v["line"])
		a.decide(review.Decision{Action: review.ActWriteLink, Path: v["path"], Line: n, Relation: v["relation"], Target: v["target"], Accepted: !review.IsAgent(a.opt.Actor)})
	}, field{key: "path", label: "file", value: path, required: true}, field{key: "line", label: "above line (0: top of file)", value: line},
		field{key: "relation", label: "relation", value: "implements", required: true}, field{key: "target", label: "concept (ns:Name, .Local or ns:NEW:Name)", required: true})
}

// openViewer asks the caller to open the HTML viewer on a seed.
func (a *App) openViewer(seed string) {
	if a.opt.OpenViewer == nil {
		a.setStatus(false, "no viewer here: run gloss view")
		return
	}
	msg, err := a.opt.OpenViewer(seed)
	if err != nil {
		a.setStatus(false, err.Error())
		return
	}
	a.setStatus(true, msg)
}

func (a *App) decide(d review.Decision) {
	r := a.eng.Decide(d)
	switch {
	case !r.OK:
		a.setStatus(false, r.Error)
	case r.Recommended:
		a.setStatus(true, r.Message)
	default:
		msg := r.Message
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = msg[i+1:]
		}
		a.setStatus(true, msg+"  (u undoes)")
	}
	a.Reload()
}

func (a *App) report(msg string, err error) {
	if err != nil {
		a.setStatus(false, err.Error())
	} else {
		a.setStatus(true, msg)
	}
	a.Reload()
}

func (a *App) ask(title string, submit func(map[string]string), fields ...field) {
	a.form = &form{title: title, fields: fields, submit: submit}
}

func (a *App) updateForm(k Key) {
	f := a.form
	fl := &f.fields[f.at]
	switch {
	case k.Name == "esc":
		a.form = nil
		a.setStatus(false, "cancelled")
	case k.Name == "enter":
		if fl.required && strings.TrimSpace(fl.value) == "" {
			a.setStatus(false, fl.label+" is required")
			return
		}
		a.status = ""
		f.at++
		if f.at == len(f.fields) {
			v := map[string]string{}
			for _, x := range f.fields {
				v[x.key] = strings.TrimSpace(x.value)
			}
			a.form = nil
			f.submit(v)
		}
	case fl.yesno:
		switch k.Rune {
		case 'y', 'Y':
			fl.value = "y"
		case 'n', 'N':
			fl.value = "n"
		}
	case k.Name == "backspace":
		if r := []rune(fl.value); len(r) > 0 {
			fl.value = string(r[:len(r)-1])
		}
	case k.Name == "ctrl-u":
		fl.value = ""
	case k.Rune != 0:
		fl.value += string(k.Rune)
	}
}

// pageSize is half the detail pane.
func (a *App) pageSize() int {
	if n := (a.h - 4) / 2; n > 1 {
		return n
	}
	return 1
}

func indexOf(xs []string, x string) int {
	for i, y := range xs {
		if y == x {
			return i
		}
	}
	return -1
}

// hints lists the keys for an item's actions.
func hints(it *review.Item) string {
	var hs []string
	for _, act := range it.Actions {
		k := map[string]string{review.ActAccept: "a", review.ActReconfirm: "a", review.ActConfirm: "a", review.ActApply: "a",
			review.ActReject: "r", review.ActWaive: "w", review.ActPropose: "p", review.ActDismiss: "d"}[act.Name]
		if k != "" {
			hs = append(hs, fmt.Sprintf("%s %s", k, act.Name))
		}
	}
	if it.Recommendation != nil && it.Kind != review.KindRecommendation {
		hs = append(hs, "y take recommendation")
	}
	return strings.Join(hs, "  ")
}
