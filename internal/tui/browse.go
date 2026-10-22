// @gloss implements gmi:GraphBrowser scope=file

package tui

import (
	"fmt"
	"strings"

	"github.com/mckalvan/glossmark/internal/browse"
)

// Browser is the read-only graph the Browse tab walks (internal/browse).
type Browser interface {
	Lookup(ref string) (*browse.Node, error)
	Index() ([]browse.Neighbor, error)
}

type browseRow struct {
	header string // a relation heading; nb is nil
	nb     *browse.Neighbor
}

type browseState struct {
	stack []string // refs browsed, "" for the index; the last is shown
	node  *browse.Node
	rows  []browseRow
	cur   int
	top   int
	err   string
}

// enterBrowse opens the Browse tab, the first time at the selected item's
// concept, or its file, or the index.
func (a *App) enterBrowse() {
	b := a.browse
	if len(b.stack) > 0 {
		a.browseTo(b.stack[len(b.stack)-1], false)
		return
	}
	ref := ""
	if it := a.Selected(); it != nil {
		c, _ := a.conceptOf(it)
		ref = firstNonEmpty(c, it.Path)
	}
	if ref != "" {
		if _, err := a.opt.Browser.Lookup(ref); err != nil {
			ref = ""
		}
	}
	a.browseTo(ref, true)
}

func (a *App) browseTo(ref string, push bool) {
	b := a.browse
	if push {
		b.stack = append(b.stack, ref)
	}
	b.node, b.rows, b.cur, b.top, b.err = nil, nil, 0, 0, ""
	if ref == "" {
		idx, err := a.opt.Browser.Index()
		if err != nil {
			b.err = err.Error()
		}
		kind := ""
		for i := range idx {
			if idx[i].Type != kind {
				kind = idx[i].Type
				b.rows = append(b.rows, browseRow{header: map[string]string{"boundary": "boundaries", "concept": "concepts"}[kind]})
			}
			b.rows = append(b.rows, browseRow{nb: &idx[i]})
		}
	} else {
		n, err := a.opt.Browser.Lookup(ref)
		if err != nil {
			b.err = err.Error()
			return
		}
		b.node = n
		for _, dir := range []struct {
			arrow string
			gs    []browse.Group
		}{{"→", n.Out}, {"←", n.In}} {
			for gi := range dir.gs {
				g := &dir.gs[gi]
				b.rows = append(b.rows, browseRow{header: fmt.Sprintf("%s %s (%d)", dir.arrow, g.Relation, len(g.Nodes))})
				for i := range g.Nodes {
					b.rows = append(b.rows, browseRow{nb: &g.Nodes[i]})
				}
			}
		}
	}
	b.moveTo(0, 1)
}

// moveTo puts the cursor on row i, or the next neighbour row in direction d.
func (b *browseState) moveTo(i, d int) {
	for i >= 0 && i < len(b.rows) && b.rows[i].nb == nil {
		i += d
	}
	if i >= 0 && i < len(b.rows) {
		b.cur = i
	}
}

func (b *browseState) selected() *browse.Neighbor {
	if b.cur >= 0 && b.cur < len(b.rows) {
		return b.rows[b.cur].nb
	}
	return nil
}

func (a *App) updateBrowse(k Key) Effect {
	b := a.browse
	switch k.String() {
	case "q":
		return Effect{Quit: true}
	case "?":
		a.help = true
	case "j", "down":
		b.moveTo(b.cur+1, 1)
	case "k", "up":
		b.moveTo(b.cur-1, -1)
	case "g", "home":
		b.moveTo(0, 1)
	case "G", "end":
		b.moveTo(len(b.rows)-1, -1)
	case "enter", "l", "right":
		if nb := b.selected(); nb != nil {
			a.browseTo(nb.Ref, true)
		}
	case "backspace", "h", "left":
		if len(b.stack) > 1 {
			b.stack = b.stack[:len(b.stack)-1]
			a.browseTo(b.stack[len(b.stack)-1], false)
		}
	case "i":
		a.browseTo("", true)
	case "o":
		if nb := b.selected(); nb != nil && nb.Path != "" {
			return Effect{Open: &Location{Path: nb.Path, Line: nb.Line}}
		}
		if b.node != nil && b.node.Path != "" {
			return Effect{Open: &Location{Path: b.node.Path, Line: b.node.Line}}
		}
	}
	return Effect{}
}

func (a *App) browseRows(w, h int) []string {
	b := a.browse
	var head []line
	switch {
	case b.err != "":
		head = append(head, styled(stRed, b.err))
	case b.node == nil:
		head = append(head, styled(stBold, "Index: every boundary and owned concept"))
	default:
		n := b.node
		head = append(head, plain("").add(stBold, n.Ref).add(stDim, "  "+strings.TrimSpace(strings.Join([]string{n.Type, n.Kind, n.Tier, n.Criticality}, " "))))
		if n.Doc != "" {
			head = append(head, wrap(n.Doc, w-2, "")...)
		}
		if n.Path != "" {
			head = append(head, styled(stDim, fmt.Sprintf("%s:%d", n.Path, n.Line)))
		}
		for _, f := range n.Findings {
			head = append(head, wrap("! "+f.Kind+": "+f.Msg, w-2, stRed)...)
		}
	}
	crumbs := make([]string, len(b.stack))
	for i, r := range b.stack {
		crumbs[i] = firstNonEmpty(r, "index")
	}
	head = append(head, styled(stDim, "path: "+strings.Join(crumbs, " › ")), plain(""))
	if len(head) > h/2 {
		head = head[:h/2]
	}
	var rows []string
	for _, l := range head {
		rows = append(rows, a.renderLine(append(line{{" ", ""}}, l...), w, ""))
	}
	lh := h - len(rows)
	if b.cur < b.top {
		b.top = b.cur
	}
	if b.cur >= b.top+lh {
		b.top = b.cur - lh + 1
	}
	for i := b.top; i < len(b.rows) && len(rows) < h; i++ {
		r := b.rows[i]
		if r.nb == nil {
			rows = append(rows, a.renderLine(styled(stBold, " "+r.header), w, ""))
			continue
		}
		mark, base := "   ", ""
		if i == b.cur {
			mark, base = " ▸ ", stReverse
		}
		l := plain(mark).add(stCyan, r.nb.Ref)
		if r.nb.Proposed {
			l = l.add(stYellow, "?")
		}
		l = l.add(stDim, "  "+strings.TrimSpace(r.nb.Type+" "+r.nb.Kind))
		if r.nb.Source == "derived" {
			l = l.add(stDim, " derived")
		}
		rows = append(rows, a.renderLine(l, w, base))
	}
	for len(rows) < h {
		rows = append(rows, strings.Repeat(" ", w))
	}
	return rows
}
