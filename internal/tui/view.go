// @gloss implements gmi:ReviewTerminal scope=file

package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

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

// render fits a line to w cells (truncating with …, padding with spaces)
// and applies its styles unless color is off.
func (a *App) renderLine(l line, w int, base string) string {
	var b strings.Builder
	left := w
	color := !a.opt.NoColor
	if color && base != "" {
		b.WriteString(base)
	}
	for i, s := range l {
		t := strings.ReplaceAll(s.text, "\t", "    ")
		n := utf8.RuneCountInString(t)
		if n > left {
			r := []rune(t)
			if left > 0 {
				t = string(r[:left-1]) + "…"
			} else {
				t = ""
			}
			n = left
		}
		if color && s.style != "" {
			b.WriteString(s.style)
		}
		b.WriteString(t)
		if color && s.style != "" {
			b.WriteString(stReset + base)
		}
		left -= n
		if left == 0 && i < len(l)-1 {
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

func (a *App) header() line {
	counts := map[string]int{}
	for _, it := range a.items {
		counts[it.Kind]++
	}
	l := plain(" ").add(stBold, "gloss review").add("", " │ "+a.opt.Actor+" │ ")
	var parts []string
	for _, k := range review.Kinds {
		if n := counts[k]; n > 0 {
			s := shortKind(k)
			if n > 1 && k == review.KindFinding {
				s += "s"
			}
			parts = append(parts, fmt.Sprintf("%d %s", n, s))
		}
	}
	if len(parts) == 0 {
		parts = []string{"nothing waits on review"}
	}
	l = l.add("", strings.Join(parts, " · "))
	if n := a.eng.UndoDepth(); n > 0 {
		l = l.add("", fmt.Sprintf(" │ %d to undo", n))
	}
	return l
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
		lw := w * 2 / 5
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

func (a *App) listRows(w, h int) []string {
	if a.cur < a.top {
		a.top = a.cur
	}
	if a.cur >= a.top+h {
		a.top = a.cur - h + 1
	}
	var rows []string
	if len(a.visible) == 0 {
		rows = append(rows, a.renderLine(styled(stDim, " nothing to review here"), w, ""))
	}
	for vi := a.top; vi < len(a.visible) && len(rows) < h; vi++ {
		it := a.items[a.visible[vi]]
		mark, base := "  ", ""
		if vi == a.cur {
			mark, base = "▸ ", stReverse
		}
		rec := " "
		if it.Recommendation != nil {
			rec = "★"
		}
		l := plain(mark).add(gateStyle(it.Gate), fmt.Sprintf("%-4s", it.Gate)).add("", " "+rec+" ").add(stCyan, fmt.Sprintf("%-9s", tag(it))).add("", " "+listText(it))
		rows = append(rows, a.renderLine(l, w, base))
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
	if it := a.Selected(); it != nil {
		return styled(stDim, " "+hints(it)+"  o open  n link  v viewer  u undo  q quit")
	}
	return styled(stDim, " n link  u undo  R reload  q quit")
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
	"An agent's HARD decision waits here as a recommendation (★). Every decision can be undone.",
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

// listText is an item's line in the list: where it is and what it says.
func listText(it review.Item) string {
	switch it.Kind {
	case review.KindProposedLink, review.KindStaleLink:
		return fmt.Sprintf("%s:%d %s %s", it.Path, it.Line, it.Relation, strings.Join(it.Targets, ", "))
	case review.KindProposedConcept, review.KindValueDrift:
		return it.Concept
	}
	return strings.TrimPrefix(it.Summary, it.Finding+": ")
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
			case utf8.RuneCountInString(cur)+1+utf8.RuneCountInString(word) <= w:
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
