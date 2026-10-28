// @gloss implements gmi:ComplianceReporter scope=file

package report

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"strings"
)

// Formats the report renders in. Markdown is the text form; HTML is a
// self-contained page a browser prints to PDF; CSV is one row per piece of
// evidence; JSON is the Report itself.
var Formats = []string{"markdown", "html", "csv", "json"}

// Render writes the report in format to w.
func Render(w io.Writer, r *Report, format string) error {
	switch format {
	case "markdown", "md", "":
		_, err := io.WriteString(w, Markdown(r))
		return err
	case "html":
		_, err := w.Write(HTML(r))
		return err
	case "csv":
		return CSV(w, r)
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	}
	return fmt.Errorf("unknown format %q; want %s", format, strings.Join(Formats, ", "))
}

// eventText says what an event means, for a reader outside engineering.
func eventText(ev Event) string {
	who := ev.Actor
	if who == "" {
		who = "unknown"
	}
	link := fmt.Sprintf("%s → %s in %s", ev.Relation, ev.Target, ev.Path)
	var s string
	switch ev.Kind {
	case "linked":
		s = fmt.Sprintf("%s recorded the link %s", who, link)
	case "proposed":
		s = fmt.Sprintf("%s proposed the link %s", who, link)
	case "accepted":
		s = fmt.Sprintf("%s accepted the link %s", who, link)
	case "rejected":
		s = fmt.Sprintf("%s rejected the link %s", who, link)
	case "stale":
		s = fmt.Sprintf("the code behind %s changed since it was accepted", link)
	case "removed":
		s = fmt.Sprintf("the link %s was removed from the code", link)
	case "value-bound":
		s = fmt.Sprintf("%s recorded the published value of %s from %s", who, ev.Target, ev.Path)
	case "value-drifted":
		s = fmt.Sprintf("the code the published value of %s is bound to changed (%s)", ev.Target, ev.Path)
	default:
		s = fmt.Sprintf("%s: %s", ev.Kind, link)
	}
	switch ev.Kind {
	case "linked", "proposed", "accepted", "rejected":
		if ev.Gate != "" {
			s += " [" + ev.Gate + "]"
		}
		if ev.Conf > 0 && ev.Kind != "accepted" && ev.Kind != "rejected" {
			s += fmt.Sprintf(" (confidence %.2f)", ev.Conf)
		}
	}
	if ev.Reason != "" {
		s += ": " + ev.Reason
	}
	return s
}

// loc renders path:line.
func loc(path string, line int) string {
	if line > 0 {
		return fmt.Sprintf("%s:%d", path, line)
	}
	return path
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func day(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

// ---------------------------------------------------------------------------
// Markdown

// Markdown renders the report as Markdown.
func Markdown(r *Report) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Compliance report\n\nGenerated %s · %s", r.Generated, r.Selection)
	if r.Since != "" {
		fmt.Fprintf(&sb, " · history since %s", r.Since)
	}
	sb.WriteString("\n\n")
	if len(r.Concepts) == 0 {
		sb.WriteString("No concepts selected.\n")
		return sb.String()
	}
	sb.WriteString("| Rule | Criticality | Status | Enforced by | Tests | Open findings |\n| --- | --- | --- | --- | --- | --- |\n")
	for _, c := range r.Concepts {
		open := 0
		for _, f := range c.Findings {
			if !f.Waived {
				open++
			}
		}
		fmt.Fprintf(&sb, "| `%s` | %s | %s | %d | %d | %d |\n", c.CURIE, orDash(c.Criticality), c.Status, len(c.Artifacts), len(c.Tests), open)
	}
	for _, c := range r.Concepts {
		fmt.Fprintf(&sb, "\n## %s (`%s`)\n\n", c.Label, c.CURIE)
		if c.Doc != "" {
			fmt.Fprintf(&sb, "%s\n\n", c.Doc)
		}
		fmt.Fprintf(&sb, "- Kind: %s (%s tier)\n", orDash(c.Kind), c.Tier)
		crit := orDash(c.Criticality)
		if c.CriticalityFrom != "" && c.CriticalityFrom != "declared" {
			crit += " (inherited from " + c.CriticalityFrom + ")"
		}
		fmt.Fprintf(&sb, "- Criticality: %s\n", crit)
		fmt.Fprintf(&sb, "- Status: %s\n", c.Status)
		if c.NamespaceOwner != "" {
			fmt.Fprintf(&sb, "- Definition owned by: %s\n", c.NamespaceOwner)
		}
		for _, b := range c.Boundaries {
			fmt.Fprintf(&sb, "- Implemented in: %s (`%s`), owner %s\n", orDash(b.Label), b.CURIE, orDash(b.Owner))
		}
		for _, x := range c.Realizers {
			fmt.Fprintf(&sb, "- In code as: `%s` — %s\n", x.CURIE, x.Doc)
		}

		sb.WriteString("\n### Where it is enforced\n\n")
		if len(c.Artifacts) == 0 {
			sb.WriteString("No code is linked to this rule.\n")
		} else {
			sb.WriteString("| Code | Relation | Review state | Linked by |\n| --- | --- | --- | --- |\n")
			for _, l := range c.Artifacts {
				fmt.Fprintf(&sb, "| %s | %s | %s | %s |\n", mdLink(l.Path, l.Line, l.Symbol, l.URL), relText(l), orDash(l.Status), orDash(l.Actor))
			}
		}

		sb.WriteString("\n### How it is verified\n\n")
		if len(c.Tests) == 0 {
			sb.WriteString("No test is linked to this rule.\n")
		} else {
			sb.WriteString("| Test | Review state | Linked by |\n| --- | --- | --- |\n")
			for _, l := range c.Tests {
				fmt.Fprintf(&sb, "| %s | %s | %s |\n", mdLink(l.Path, l.Line, l.Symbol, l.URL), orDash(l.Status), orDash(l.Actor))
			}
		}

		if len(c.Values) > 0 {
			sb.WriteString("\n### Published value\n\n| Concept | Value | Bound to | Code reads | State |\n| --- | --- | --- | --- | --- |\n")
			for _, v := range c.Values {
				bound := "—"
				if v.BoundTo != "" {
					path, line := splitLoc(v.BoundTo)
					bound = mdLink(path, line, "", v.URL)
				}
				fmt.Fprintf(&sb, "| `%s` | %s | %s | `%s` | %s |\n", v.Concept, v.Value, bound, orDash(v.Live), v.Status)
			}
		}

		sb.WriteString("\n### Decision history\n\n")
		if len(c.History) == 0 {
			sb.WriteString("No ledger entries.\n")
		} else {
			sb.WriteString("| When | What happened | Record |\n| --- | --- | --- |\n")
			for _, ev := range c.History {
				fmt.Fprintf(&sb, "| %s | %s | `%s` |\n", ev.When, eventText(ev), ev.MarkerID)
			}
		}

		sb.WriteString("\n### Open findings and waivers\n\n")
		if len(c.Findings) == 0 && len(c.Waivers) == 0 {
			sb.WriteString("None.\n")
		}
		for _, f := range c.Findings {
			w := ""
			if f.Waived {
				w = " (waived: " + f.Waiver + ")"
			}
			fmt.Fprintf(&sb, "- **%s** (%s): %s%s\n", f.Kind, f.Severity, f.Msg, w)
		}
		for _, w := range c.Waivers {
			state := "expires " + w.Expires
			if w.Expired {
				state = "expired " + w.Expires
			}
			fmt.Fprintf(&sb, "- Waiver on `%s` by %s, %s: %s\n", w.Finding, w.Owner, state, orDash(w.Reason))
		}
	}
	return sb.String()
}

func relText(l Link) string {
	if l.Via != "" {
		return l.Relation + " (via " + l.Via + ")"
	}
	return l.Relation
}

func mdLink(path string, line int, symbol, url string) string {
	text := loc(path, line)
	if symbol != "" {
		text += " (" + symbol + ")"
	}
	if url == "" {
		return "`" + text + "`"
	}
	return fmt.Sprintf("[`%s`](%s)", text, url)
}

// ---------------------------------------------------------------------------
// CSV

// CSVHeader is the column list of the CSV form. Each row is one piece of
// evidence about one concept: the concept itself, a boundary, a linked
// artifact or test, a published value, a ledger decision, a drift finding
// or a waiver.
var CSVHeader = []string{"concept", "label", "kind", "criticality", "status", "owner", "row", "relation", "path", "line", "url", "state", "actor", "when", "record", "reason", "detail"}

// CSV writes one row per (concept, evidence item).
func CSV(w io.Writer, r *Report) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(CSVHeader); err != nil {
		return err
	}
	for _, c := range r.Concepts {
		owner := c.NamespaceOwner
		if len(c.Boundaries) > 0 {
			var os []string
			for _, b := range c.Boundaries {
				if b.Owner != "" {
					os = append(os, b.Owner)
				}
			}
			if len(os) > 0 {
				owner = strings.Join(os, "; ")
			}
		}
		row := func(kind string, cols ...string) error {
			base := []string{c.CURIE, c.Label, c.Kind, c.Criticality, c.Status, owner, kind}
			all := append(base, cols...)
			for len(all) < len(CSVHeader) {
				all = append(all, "")
			}
			return cw.Write(all)
		}
		if err := row("concept", "", "", "", "", "", "", "", "", "", c.Doc); err != nil {
			return err
		}
		for _, b := range c.Boundaries {
			if err := row("boundary", "realizes", b.Manifest, "", "", "", b.Owner, "", b.CURIE, "", b.Label); err != nil {
				return err
			}
		}
		for _, l := range c.Artifacts {
			if err := row("artifact", l.Relation, l.Path, lineStr(l.Line), l.URL, l.Status, l.Actor, "", l.MarkerID, "", symbolVia(l)); err != nil {
				return err
			}
		}
		for _, l := range c.Tests {
			if err := row("test", l.Relation, l.Path, lineStr(l.Line), l.URL, l.Status, l.Actor, "", l.MarkerID, "", symbolVia(l)); err != nil {
				return err
			}
		}
		for _, v := range c.Values {
			path, line := splitLoc(v.BoundTo)
			if err := row("value", "publishes-value", path, lineStr(line), v.URL, v.Status, "", "", v.Concept, "", fmt.Sprintf("value %s; code reads %s", v.Value, orDash(v.Live))); err != nil {
				return err
			}
		}
		for _, ev := range c.History {
			if err := row("decision", ev.Relation, ev.Path, "", ev.URL, ev.Kind, ev.Actor, ev.When, ev.MarkerID, ev.Reason, eventText(ev)); err != nil {
				return err
			}
		}
		for _, f := range c.Findings {
			state := "open"
			if f.Waived {
				state = "waived"
			}
			if err := row("finding", f.Kind, f.Path, lineStr(f.Line), f.URL, state, "", "", f.ID, f.Waiver, f.Msg); err != nil {
				return err
			}
		}
		for _, wv := range c.Waivers {
			state := "active"
			if wv.Expired {
				state = "expired"
			}
			if err := row("waiver", "", "", "", "", state, wv.Owner, wv.Expires, wv.Finding, wv.Reason, ""); err != nil {
				return err
			}
		}
	}
	cw.Flush()
	return cw.Error()
}

func lineStr(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprint(n)
}

func symbolVia(l Link) string {
	var parts []string
	if l.Symbol != "" {
		parts = append(parts, l.Symbol)
	}
	if l.Via != "" {
		parts = append(parts, "via "+l.Via)
	}
	if l.Conf > 0 {
		parts = append(parts, fmt.Sprintf("confidence %.2f", l.Conf))
	}
	return strings.Join(parts, "; ")
}

// ---------------------------------------------------------------------------
// HTML

const htmlHead = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Compliance report</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
:root{--ink:#1b1b1f;--muted:#5d5d66;--line:#d9d9e0;--bg:#fff;--soft:#f5f5f8;--high:#b4501f;--critical:#a4162a;--ok:#2d6a3e;--warn:#9a6700;--bad:#a4162a}
html{background:var(--bg);color:var(--ink);font:15px/1.5 -apple-system,"Segoe UI",Helvetica,Arial,sans-serif}
body{margin:0 auto;max-width:960px;padding:24px 16px}
h1{font-size:26px;margin:0 0 4px}h2{font-size:21px;margin:36px 0 4px;padding-top:16px;border-top:2px solid var(--line)}h3{font-size:15px;margin:20px 0 6px;color:var(--muted);text-transform:uppercase;letter-spacing:.04em}
.meta{color:var(--muted);margin-bottom:20px}
table{border-collapse:collapse;width:100%;margin:6px 0 10px;font-size:14px}th,td{border:1px solid var(--line);padding:6px 8px;text-align:left;vertical-align:top}th{background:var(--soft);font-weight:600}
code{font:13px/1.4 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;background:var(--soft);padding:1px 4px;border-radius:3px}
.pill{display:inline-block;padding:1px 8px;border-radius:10px;font-size:12px;font-weight:600;border:1px solid var(--line)}
.crit-high{color:var(--high);border-color:var(--high)}.crit-critical{color:var(--critical);border-color:var(--critical)}
.st-accepted,.st-implemented,.st-ok{color:var(--ok)}.st-proposed,.st-stale,.st-untested,.st-waived{color:var(--warn)}.st-rejected,.st-drifted,.st-open,.st-error,.st-removed{color:var(--bad)}
dl{display:grid;grid-template-columns:max-content 1fr;gap:4px 16px;margin:8px 0}dt{color:var(--muted)}dd{margin:0}
.none{color:var(--muted);font-style:italic}
.doc{font-size:16px;margin:4px 0 10px}
.howto{background:var(--soft);border:1px solid var(--line);padding:10px 14px;border-radius:6px;font-size:14px;margin:14px 0 20px}
.toc{columns:2;font-size:14px}
@media print{body{max-width:none;padding:0}h2{break-before:page;border-top:0}h2:first-of-type{break-before:auto}.howto,.toc{display:none}a{color:inherit;text-decoration:none}}
</style></head><body>
`

// HTML renders a self-contained page. It carries no scripts; print it from
// the browser to get a PDF.
func HTML(r *Report) []byte {
	var b bytes.Buffer
	b.WriteString(htmlHead)
	fmt.Fprintf(&b, "<h1>Compliance report</h1>\n<div class=\"meta\">Generated %s · %s", esc(r.Generated), esc(r.Selection))
	if r.Since != "" {
		fmt.Fprintf(&b, " · history since %s", esc(r.Since))
	}
	if r.Generator != "" {
		fmt.Fprintf(&b, " · %s", esc(r.Generator))
	}
	b.WriteString("</div>\n")
	b.WriteString(`<div class="howto">For each rule this report shows what it means and who owns it, the code that enforces it, the tests that check it, the value it is published with, every recorded decision about those links (who linked, proposed, accepted or rejected them, when, and why), and what is still open or waived. Links open the code on the repository host. To file it as a PDF, print this page from the browser.</div>` + "\n")
	if len(r.Concepts) == 0 {
		b.WriteString("<p class=\"none\">No concepts selected.</p>\n</body></html>\n")
		return b.Bytes()
	}
	b.WriteString("<table><tr><th>Rule</th><th>Criticality</th><th>Status</th><th>Enforced by</th><th>Tests</th><th>Open findings</th></tr>\n")
	for _, c := range r.Concepts {
		open := 0
		for _, f := range c.Findings {
			if !f.Waived {
				open++
			}
		}
		fmt.Fprintf(&b, "<tr><td><a href=\"#%s\">%s</a> <code>%s</code></td><td>%s</td><td>%s</td><td>%d</td><td>%d</td><td>%d</td></tr>\n",
			esc(anchor(c.CURIE)), esc(c.Label), esc(c.CURIE), critPill(c.Criticality), statePill(c.Status), len(c.Artifacts), len(c.Tests), open)
	}
	b.WriteString("</table>\n")
	for _, c := range r.Concepts {
		fmt.Fprintf(&b, "<h2 id=\"%s\">%s <code>%s</code></h2>\n", esc(anchor(c.CURIE)), esc(c.Label), esc(c.CURIE))
		if c.Doc != "" {
			fmt.Fprintf(&b, "<p class=\"doc\">%s</p>\n", esc(c.Doc))
		}
		b.WriteString("<dl>")
		fmt.Fprintf(&b, "<dt>Kind</dt><dd>%s (%s tier)</dd>", esc(orDash(c.Kind)), esc(c.Tier))
		crit := critPill(c.Criticality)
		if c.CriticalityFrom != "" && c.CriticalityFrom != "declared" {
			crit += " inherited from <code>" + esc(c.CriticalityFrom) + "</code>"
		}
		fmt.Fprintf(&b, "<dt>Criticality</dt><dd>%s</dd>", crit)
		fmt.Fprintf(&b, "<dt>Status</dt><dd>%s</dd>", statePill(c.Status))
		if c.NamespaceOwner != "" {
			fmt.Fprintf(&b, "<dt>Definition owned by</dt><dd>%s</dd>", esc(c.NamespaceOwner))
		}
		for _, bd := range c.Boundaries {
			fmt.Fprintf(&b, "<dt>Implemented in</dt><dd>%s <code>%s</code>, owner %s</dd>", esc(orDash(bd.Label)), esc(bd.CURIE), esc(orDash(bd.Owner)))
		}
		for _, x := range c.Realizers {
			fmt.Fprintf(&b, "<dt>In code as</dt><dd><code>%s</code> %s</dd>", esc(x.CURIE), esc(x.Doc))
		}
		b.WriteString("</dl>\n")

		b.WriteString("<h3>Where it is enforced</h3>\n")
		if len(c.Artifacts) == 0 {
			b.WriteString("<p class=\"none\">No code is linked to this rule.</p>\n")
		} else {
			b.WriteString("<table><tr><th>Code</th><th>Relation</th><th>Review state</th><th>Linked by</th></tr>\n")
			for _, l := range c.Artifacts {
				fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n", htmlLink(l.Path, l.Line, l.Symbol, l.URL), esc(relText(l)), statePill(l.Status), esc(orDash(l.Actor)))
			}
			b.WriteString("</table>\n")
		}

		b.WriteString("<h3>How it is verified</h3>\n")
		if len(c.Tests) == 0 {
			b.WriteString("<p class=\"none\">No test is linked to this rule.</p>\n")
		} else {
			b.WriteString("<table><tr><th>Test</th><th>Review state</th><th>Linked by</th></tr>\n")
			for _, l := range c.Tests {
				fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%s</td></tr>\n", htmlLink(l.Path, l.Line, l.Symbol, l.URL), statePill(l.Status), esc(orDash(l.Actor)))
			}
			b.WriteString("</table>\n")
		}

		if len(c.Values) > 0 {
			b.WriteString("<h3>Published value</h3>\n<table><tr><th>Concept</th><th>Value</th><th>Bound to</th><th>Code reads</th><th>State</th></tr>\n")
			for _, v := range c.Values {
				bound := "—"
				if v.BoundTo != "" {
					path, line := splitLoc(v.BoundTo)
					bound = htmlLink(path, line, "", v.URL)
				}
				fmt.Fprintf(&b, "<tr><td><code>%s</code></td><td><strong>%s</strong></td><td>%s</td><td><code>%s</code></td><td>%s</td></tr>\n", esc(v.Concept), esc(v.Value), bound, esc(orDash(v.Live)), statePill(v.Status))
			}
			b.WriteString("</table>\n")
		}

		b.WriteString("<h3>Decision history</h3>\n")
		if len(c.History) == 0 {
			b.WriteString("<p class=\"none\">No ledger entries.</p>\n")
		} else {
			b.WriteString("<table><tr><th>When</th><th>What happened</th><th>Record</th></tr>\n")
			for _, ev := range c.History {
				fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td><code>%s</code></td></tr>\n", esc(ev.When), esc(eventText(ev)), esc(ev.MarkerID))
			}
			b.WriteString("</table>\n")
		}

		b.WriteString("<h3>Open findings and waivers</h3>\n")
		if len(c.Findings) == 0 && len(c.Waivers) == 0 {
			b.WriteString("<p class=\"none\">None.</p>\n")
		} else {
			b.WriteString("<ul>\n")
			for _, f := range c.Findings {
				state := statePill("open")
				if f.Waived {
					state = statePill("waived") + " " + esc(f.Waiver)
				}
				fmt.Fprintf(&b, "<li><strong>%s</strong> (%s) %s: %s</li>\n", esc(f.Kind), esc(f.Severity), state, esc(f.Msg))
			}
			for _, w := range c.Waivers {
				state := "expires " + esc(w.Expires)
				if w.Expired {
					state = "<span class=\"st-open\">expired " + esc(w.Expires) + "</span>"
				}
				fmt.Fprintf(&b, "<li>Waiver on <code>%s</code> by %s, %s: %s</li>\n", esc(w.Finding), esc(w.Owner), state, esc(orDash(w.Reason)))
			}
			b.WriteString("</ul>\n")
		}
	}
	b.WriteString("</body></html>\n")
	return b.Bytes()
}

func esc(s string) string { return html.EscapeString(s) }

func anchor(cur string) string {
	return strings.NewReplacer(":", "-", "/", "-", " ", "-").Replace(cur)
}

func critPill(level string) string {
	if level == "" {
		return "<span class=\"pill\">unclassified</span>"
	}
	return fmt.Sprintf("<span class=\"pill crit-%s\">%s</span>", esc(level), esc(level))
}

func statePill(s string) string {
	if s == "" {
		return "—"
	}
	return fmt.Sprintf("<span class=\"st-%s\">%s</span>", esc(s), esc(s))
}

func htmlLink(path string, line int, symbol, url string) string {
	text := "<code>" + esc(loc(path, line)) + "</code>"
	if symbol != "" {
		text += " " + esc(symbol)
	}
	if url == "" {
		return text
	}
	return fmt.Sprintf("<a href=\"%s\">%s</a>", esc(url), text)
}
