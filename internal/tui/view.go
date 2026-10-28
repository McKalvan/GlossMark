// @gloss implements gmi:ReviewTerminal scope=file

package tui

import (
	"fmt"
	"strings"

	"github.com/mckalvan/glossmark/internal/review"
)

// ANSI styles.
const (
	stReset   = "\x1b[0m"
	stBold    = "\x1b[1m"
	stDim     = "\x1b[2m"
	stReverse = "\x1b[7m"
	stRed     = "\x1b[31m"
	stGreen   = "\x1b[32m"
	stYellow  = "\x1b[33m"
	stBlue    = "\x1b[34m"
	stCyan    = "\x1b[36m"
)

// seg is a run of text in one style.
type seg struct{ text, style string }

// line is one row of the frame.
type line []seg

func plain(s string) line               { return line{{s, ""}} }
func styled(style, s string) line       { return line{{s, style}} }
func (l line) add(style, s string) line { return append(l, seg{s, style}) }

// renderLine fits a line to w cells (truncating with …, padding with
// spaces, a wide rune counting two) and applies its styles unless color
// is off.
func (a *App) renderLine(l line, w int, base string) string {
	var b strings.Builder
	left := w
	color := !a.opt.NoColor
	if color && base != "" {
		b.WriteString(base)
	}
	for _, s := range l {
		t := strings.ReplaceAll(s.text, "\t", "    ")
		n := strWidth(t)
		cut := n > left
		if cut {
			t = truncRight(t, left)
			n = strWidth(t)
		}
		if color && s.style != "" {
			b.WriteString(s.style)
		}
		b.WriteString(t)
		if color && s.style != "" {
			b.WriteString(stReset + base)
		}
		left -= n
		if cut || left == 0 {
			break
		}
	}
	b.WriteString(strings.Repeat(" ", left))
	if color && base != "" {
		b.WriteString(stReset)
	}
	return b.String()
}

// Render returns a whole frame of h rows of w cells, rows joined by \r\n.
func (a *App) Render(w, h int) string {
	a.w, a.h = w, h
	if w < 40 || h < 10 {
		return a.renderLine(plain("gloss review needs at least 40×10; q quits"), w, "")
	}
	rows := make([]string, 0, h)
	rows = append(rows, a.renderLine(a.header(), w, stReverse))
	rows = append(rows, a.renderLine(a.filterLine(), w, ""))
	body := h - 3
	if a.help {
		for _, l := range helpLines(body) {
			rows = append(rows, a.renderLine(l, w, ""))
		}
	} else if a.tab == 1 {
		rows = append(rows, a.browseRows(w, body)...)
	} else {
		rows = append(rows, a.body(w, body)...)
	}
	rows = append(rows, a.renderLine(a.footer(), w, ""))
	return strings.Join(rows, "\r\n")
}

// header counts the queue by group: the decisions by kind, the findings
// in one number, and how many of the opening decisions are decided.
func (a *App) header() line {
	counts := map[string]int{}
	var order []string
	decisions, findings := 0, 0
	for i := range a.items {
		it := &a.items[i]
		if !it.NeedsDecision() {
			findings++
			continue
		}
		decisions++
		k := tag(*it)
		if counts[k] == 0 {
			order = append(order, k)
		}
		counts[k]++
	}
	l := plain(" ").add(stBold, "gloss review").add("", " │ "+a.opt.Actor+" │ ")
	// Progress first: on a narrow terminal the breakdown is what gets cut.
	if done, total, arrived := a.progress(); total > 0 || arrived > 0 {
		p := fmt.Sprintf("%d of %d decided", done, total)
		if arrived > 0 {
			p += fmt.Sprintf(" · %d new", arrived)
		}
		l = l.add(stBold, p).add("", " │ ")
	}
	var parts []string
	if decisions > 0 {
		var ks []string
		for _, k := range order {
			ks = append(ks, fmt.Sprintf("%d %s", counts[k], k))
		}
		parts = append(parts, fmt.Sprintf("%d %s: %s", decisions, plural(decisions, "decision"), strings.Join(ks, " · ")))
	}
	if findings > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", findings, plural(findings, "finding")))
	}
	if len(parts) == 0 {
		parts = []string{"nothing waits on review"}
	}
	l = l.add("", strings.Join(parts, " │ "))
	if n := a.eng.UndoDepth(); n > 0 {
		l = l.add("", fmt.Sprintf(" │ %d to undo", n))
	}
	return l
}

func plural(n int, s string) string {
	if n == 1 {
		return s
	}
	return s + "s"
}

func (a *App) filterLine() line {
	if a.browse != nil {
		if a.tab == 1 {
			return styled(stDim, " queue ").add(stBold, "[browse]").add(stDim, "  read-only · enter follows · h back · i index · o open · tab queue")
		}
	}
	k := "all"
	if a.kind != "" {
		k = shortKind(a.kind)
	}
	l := styled(stBold, " [queue]").add(stDim, fmt.Sprintf(" %d/%d  f kind: %s", len(a.visible), len(a.items), k))
	if a.text != "" {
		l = l.add(stDim, "  / "+a.text)
	}
	if a.browse != nil {
		l = l.add(stDim, "  tab browse")
	}
	return l.add(stDim, "  ? help")
}

func (a *App) body(w, h int) []string {
	var rows []string
	if w >= 110 {
		lw := w / 2
		dw := w - lw - 1
		list := a.listRows(lw, h)
		det := a.detailRows(dw, h)
		for i := 0; i < h; i++ {
			sep := "│"
			if !a.opt.NoColor {
				sep = stDim + "│" + stReset
			}
			rows = append(rows, list[i]+sep+det[i])
		}
		return rows
	}
	lh := h * 2 / 5
	if lh < 3 {
		lh = 3
	}
	rows = append(rows, a.listRows(w, lh)...)
	rows = append(rows, a.renderLine(styled(stDim, strings.Repeat("─", w)), w, ""))
	return append(rows, a.detailRows(w, h-lh-1)...)
}

// listRow is a row of the list: an item (an index into visible) or the
// header of a group.
type listRow struct {
	vi     int // -1 for a header
	header line
}

// listLayout puts a header above each group of visible items: Decisions,
// then Findings.
func (a *App) listLayout() []listRow {
	var rows []listRow
	for vi, i := range a.visible {
		dec := a.items[i].NeedsDecision()
		if vi == 0 || dec != a.items[a.visible[vi-1]].NeedsDecision() {
			n := 0
			for _, j := range a.visible[vi:] {
				if a.items[j].NeedsDecision() == dec {
					n++
				}
			}
			h := styled(stBold, " Findings").add(stDim, fmt.Sprintf(" · %d: fix in code, or w waive", n))
			if dec {
				h = styled(stBold, " Decisions").add(stDim, fmt.Sprintf(" · %d waiting on a reviewer", n))
			}
			rows = append(rows, listRow{vi: -1, header: h})
		}
		rows = append(rows, listRow{vi: vi})
	}
	return rows
}

func (a *App) listRows(w, h int) []string {
	layout := a.listLayout()
	at := 0
	for r, lr := range layout {
		if lr.vi == a.cur {
			at = r
		}
	}
	if at < a.top {
		a.top = at
		// The first item of a group brings its header into view.
		if at > 0 && layout[at-1].vi < 0 {
			a.top = at - 1
		}
	}
	if at >= a.top+h {
		a.top = at - h + 1
	}
	if a.top > max(0, len(layout)-h) {
		a.top = max(0, len(layout)-h)
	}
	gw := 4
	for _, i := range a.visible {
		gw = max(gw, strWidth(gateLabel(&a.items[i])))
	}
	var rows []string
	if len(a.visible) == 0 {
		rows = append(rows, a.renderLine(styled(stDim, " nothing to review here"), w, ""))
	}
	for r := a.top; r < len(layout) && len(rows) < h; r++ {
		if layout[r].vi < 0 {
			rows = append(rows, a.renderLine(layout[r].header, w, ""))
			continue
		}
		vi := layout[r].vi
		it := &a.items[a.visible[vi]]
		mark, base := "  ", ""
		if vi == a.cur {
			mark, base = "▸ ", stReverse
		}
		rec := " "
		if it.Recommendation != nil {
			rec = "★"
		}
		l := append(plain(mark), gateSegs(it, gw)...)
		l = l.add("", " "+rec+" ").add(stCyan, fmt.Sprintf("%-9s", tag(*it))).add("", " ")
		used := 0
		for _, sg := range l {
			used += strWidth(sg.text)
		}
		head, where := listText(*it)
		rows = append(rows, a.renderLine(l.add("", fitPair(head, where, w-used)), w, base))
	}
	for len(rows) < h {
		rows = append(rows, strings.Repeat(" ", w))
	}
	return rows
}

func (a *App) detailRows(w, h int) []string {
	lines := a.detailLines(w - 1)
	if a.dscroll > len(lines)-1 {
		a.dscroll = max(0, len(lines)-1)
	}
	lines = lines[a.dscroll:]
	var rows []string
	for i := 0; i < h; i++ {
		l := line{}
		if i < len(lines) {
			l = lines[i]
		}
		rows = append(rows, a.renderLine(append(line{{" ", ""}}, l...), w, ""))
	}
	return rows
}

func (a *App) detailLines(w int) []line {
	it := a.Selected()
	if it == nil {
		return []line{styled(stDim, "Nothing selected. n writes a link, R reloads, q quits.")}
	}
	var out []line
	out = append(out, wrap(it.Summary, w, stBold)...)
	out = append(out, plain(""))
	for _, act := range it.Actions {
		l := plain("  ").add(stBold, keyFor(act.Name)+" ").add("", act.Name+" ").add(gateStyle(act.Gate), "["+act.Gate+"]")
		if !act.AgentMay {
			l = l.add(stDim, " human")
		}
		if len(act.Reasons) > 0 {
			l = l.add(stDim, "  "+strings.Join(act.Reasons, "; "))
		}
		out = append(out, l)
	}
	if r := it.Recommendation; r != nil {
		msg := fmt.Sprintf("★ %s recommends %s", r.By, r.Decision.Action)
		if r.Decision.Reason != "" {
			msg += ": " + r.Decision.Reason
		}
		if r.Decision.Doc != "" {
			msg += fmt.Sprintf(" (doc %q)", r.Decision.Doc)
		}
		if r.Outdated {
			msg += "  [outdated: the item changed since]"
		}
		out = append(out, plain(""))
		out = append(out, wrap(msg, w, stYellow)...)
		if it.Kind != review.KindRecommendation {
			out = append(out, styled(stDim, "  y takes it, d dismisses it"))
		}
	}
	if a.detailErr != "" {
		out = append(out, plain(""), styled(stRed, a.detailErr))
	}
	d := a.detail
	if d == nil || d.ID != it.ID {
		return out
	}
	for _, c := range d.Concepts {
		out = append(out, plain(""))
		if !c.Exists {
			out = append(out, styled(stRed, c.Curie+": no such concept"))
			continue
		}
		out = append(out, plain("").add(stBold, c.Curie).add(stDim, fmt.Sprintf("  %s, %s", c.Kind, c.Tier)))
		out = append(out, wrap(firstNonEmpty(c.Doc, "(no doc: e writes one)"), w, "")...)
	}
	if len(d.Context) > 0 {
		out = append(out, plain(""), styled(stDim, it.Path))
		for _, l := range d.Context {
			out = append(out, plain(l))
		}
	}
	if d.History != "" {
		out = append(out, plain(""), styled(stBold, "changed since "+d.Since))
		for _, l := range strings.Split(d.History, "\n") {
			out = append(out, styled(diffStyle(l), l))
		}
	}
	return out
}

func (a *App) footer() line {
	if f := a.form; f != nil {
		fl := f.fields[f.at]
		if fl.yesno {
			v := "y/N"
			if fl.value == "y" {
				v = "Y/n"
			}
			return plain(" ").add(stBold, f.title+": ").add("", fl.label+" ["+v+"] ").add(stDim, " enter ok, esc cancel")
		}
		return plain(" ").add(stBold, f.title+": ").add("", fl.label+": ").add(stReverse, fl.value+" ").add(stDim, "  enter ok, esc cancel")
	}
	if a.tab == 1 && a.status == "" {
		return styled(stDim, " enter follow  h back  i index  o open  v viewer  tab queue  ? help  q quit")
	}
	if a.status != "" {
		st := stGreen
		if !a.statusOK {
			st = stRed
		}
		return styled(st, " "+a.status)
	}
	return styled(stDim, " "+strings.Join(a.footerKeys(), "  "))
}

// footerKeys are the keys that do something now: the selected item's
// actions (those gloss review --format json lists), the keys that read
// it, then the global ones.
func (a *App) footerKeys() []string {
	var ks []string
	if it := a.Selected(); it != nil {
		if h := hints(it); h != "" {
			ks = append(ks, h)
		}
		if c, _ := a.conceptOf(it); c != "" && !strings.Contains(c, ":NEW:") {
			ks = append(ks, "e doc")
		}
		if it.Path != "" {
			ks = append(ks, "o open")
		}
	}
	ks = append(ks, "n link", "v viewer")
	if a.eng.UndoDepth() > 0 {
		ks = append(ks, "u undo")
	}
	return append(ks, "? help", "q quit")
}

var helpText = []string{
	"Keys",
	"  j/k ↑/↓       move through the queue      g/G   first / last",
	"  space/b       scroll the detail           f     cycle the kind filter   / filter text   esc clear",
	"  a             accept, re-confirm, confirm or apply, as the item allows",
	"  r             reject (asks a reason, and whether to delete the marker)",
	"  w             waive a drift finding until a date",
	"  p             propose the concept a ns:NEW: marker names",
	"  e             edit the doc of the item's concept",
	"  y / d         take / dismiss the recommendation on the item",
	"  n             write a link (above a line of a file)",
	"  v             open the HTML viewer in a browser, focused on the item's concept",
	"  o, enter      open the file in $EDITOR at the item's line",
	"  u             undo the last decision (files, ledger and graph)",
	"  tab           the read-only graph browser: enter follows an edge, h goes back, i the index",
	"  R             reload           q, ctrl-c   quit",
	"",
	"Gates: AUTO and SOFT actions are open to agents; HARD ones (\"human\") are a person's decision.",
	"The list shows one gate when all an item's actions carry it, else the range (SOFT/HARD);",
	"the detail shows each action's. An agent's HARD decision waits here as a recommendation (★).",
	"",
	"Decisions (proposed links and concepts, stale links, drifted values, rogue ns:NEW: targets,",
	"recommendations) come first; \"2 of 4 decided\" counts those of the opening queue that have left",
	"it, and undo puts one back. Findings (orphan, untested, violation, gap, ...) are fixed in code",
	"or waived; docs/GLOSSARY.md says what each kind means. Every decision can be undone.",
	"",
	"Any key closes this help.",
}

func helpLines(h int) []line {
	var out []line
	for i, t := range helpText {
		if i == 0 {
			out = append(out, styled(stBold, " "+t))
			continue
		}
		out = append(out, plain(" "+t))
	}
	for len(out) < h {
		out = append(out, plain(""))
	}
	return out[:h]
}

func keyFor(action string) string {
	switch action {
	case review.ActAccept, review.ActReconfirm, review.ActConfirm, review.ActApply:
		return "a"
	case review.ActReject:
		return "r"
	case review.ActWaive:
		return "w"
	case review.ActPropose:
		return "p"
	case review.ActDismiss:
		return "d"
	}
	return " "
}

func gateStyle(g string) string {
	switch g {
	case "HARD":
		return stRed
	case "SOFT":
		return stYellow
	case "AUTO":
		return stGreen
	}
	return ""
}

var gateRank = map[string]int{"AUTO": 1, "SOFT": 2, "HARD": 3}

// gateRange is the loosest and the strictest gate of an item's actions.
// Dismissing a recommendation leaves the item as it was, so it counts only
// on an item it is the only action of.
func gateRange(it *review.Item) (string, string) {
	lo, hi := "", ""
	for _, act := range it.Actions {
		if act.Name == review.ActDismiss && len(it.Actions) > 1 || gateRank[act.Gate] == 0 {
			continue
		}
		if lo == "" || gateRank[act.Gate] < gateRank[lo] {
			lo = act.Gate
		}
		if hi == "" || gateRank[act.Gate] > gateRank[hi] {
			hi = act.Gate
		}
	}
	if lo == "" {
		return it.Gate, it.Gate
	}
	return lo, hi
}

// gateLabel is the list's gate column: the gate when all the item's
// actions carry one, else the range: SOFT/HARD for a rogue, whose propose
// is SOFT and waive HARD.
func gateLabel(it *review.Item) string {
	lo, hi := gateRange(it)
	if lo == hi {
		return lo
	}
	return lo + "/" + hi
}

// gateSegs is gateLabel in color, padded to w cells.
func gateSegs(it *review.Item, w int) line {
	lo, hi := gateRange(it)
	l := line{{lo, gateStyle(lo)}}
	if lo != hi {
		l = l.add("", "/").add(gateStyle(hi), hi)
	}
	return l.add("", strings.Repeat(" ", max(0, w-strWidth(gateLabel(it)))))
}

func diffStyle(l string) string {
	switch {
	case strings.HasPrefix(l, "commit "):
		return stYellow
	case strings.HasPrefix(l, "@@"):
		return stCyan
	case strings.HasPrefix(l, "+++") || strings.HasPrefix(l, "---"):
		return stBold
	case strings.HasPrefix(l, "+"):
		return stGreen
	case strings.HasPrefix(l, "-"):
		return stRed
	}
	return stDim
}

func shortKind(k string) string {
	switch k {
	case review.KindProposedLink:
		return "proposed"
	case review.KindStaleLink:
		return "stale"
	case review.KindProposedConcept:
		return "concept"
	case review.KindValueDrift:
		return "value"
	case review.KindFinding:
		return "finding"
	case review.KindRecommendation:
		return "recommended"
	}
	return k
}

func tag(it review.Item) string {
	if it.Finding != "" {
		return it.Finding
	}
	return shortKind(it.Kind)
}

// listText is an item's line in the list: what it says, then where it is
// (path:line) when that is what tells it from its neighbours.
func listText(it review.Item) (string, string) {
	where := ""
	if it.Path != "" {
		where = fmt.Sprintf("%s:%d", it.Path, it.Line)
	}
	switch {
	case it.Kind == review.KindProposedLink || it.Kind == review.KindStaleLink:
		return it.Relation + " " + strings.Join(it.Targets, ", "), where
	case it.Finding == "rogue" && it.Relation != "":
		return it.Relation + " " + it.Concept, where
	case it.Kind == review.KindProposedConcept || it.Kind == review.KindValueDrift:
		return it.Concept, ""
	}
	return strings.TrimPrefix(it.Summary, it.Finding+": "), ""
}

// fitPair fits head and tail into w cells, two spaces apart. When they do
// not fit, the tail (a path) is cut from the left, down to …/file:line,
// and then the head is cut from the right.
func fitPair(head, tail string, w int) string {
	if tail == "" {
		return head
	}
	hw, tw := strWidth(head), strWidth(tail)
	if hw+2+tw <= w {
		return head + "  " + tail
	}
	floor := tw
	if i := strings.LastIndex(tail, "/"); i >= 0 {
		floor = min(tw, 1+strWidth(tail[i:]))
	}
	tb := max(w-hw-2, floor)
	if w-2-tb < 1 {
		return truncLeft(tail, w)
	}
	return truncRight(head, w-2-tb) + "  " + truncLeft(tail, tb)
}

// wrap breaks text into lines of at most w cells at spaces.
func wrap(text string, w int, style string) []line {
	if w < 10 {
		w = 10
	}
	var out []line
	for _, para := range strings.Split(text, "\n") {
		cur := ""
		for _, word := range strings.Fields(para) {
			switch {
			case cur == "":
				cur = word
			case strWidth(cur)+1+strWidth(word) <= w:
				cur += " " + word
			default:
				out = append(out, styled(style, cur))
				cur = word
			}
		}
		out = append(out, styled(style, cur))
	}
	return out
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}
