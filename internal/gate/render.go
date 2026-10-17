package gate

import (
	"fmt"
	"strings"
)

// RenderText renders the PR comment in the design's compact form.
func RenderText(im *Impact) string {
	var sb strings.Builder
	where := im.Repo
	if len(im.Boundaries) > 0 {
		where += " (" + strings.Join(im.Boundaries, ", ") + ")"
	}
	fmt.Fprintf(&sb, "Ontology impact — %s   base %s   gate %s\n", where, im.Base, im.Gate)
	if len(im.Changes) == 0 {
		sb.WriteString("  no ontology-relevant changes\n")
	}
	// Column widths.
	w1, w2 := 0, 0
	rows := make([][3]string, 0, len(im.Changes))
	for _, ch := range im.Changes {
		what := ch.Msg
		loc := ch.Path
		if ch.Symbol != "" {
			loc += ":" + ch.Symbol
		} else if ch.Line > 0 {
			loc += fmt.Sprintf(":%d", ch.Line)
		}
		tag := tagFor(ch)
		rows = append(rows, [3]string{ch.Glyph + " " + what, loc, tag})
		if len(rows[len(rows)-1][0]) > w1 {
			w1 = len(rows[len(rows)-1][0])
		}
		if len(loc) > w2 {
			w2 = len(loc)
		}
	}
	if w1 > 60 {
		w1 = 60
	}
	if w2 > 60 {
		w2 = 60
	}
	for _, r := range rows {
		fmt.Fprintf(&sb, "%-*s   %-*s   %s\n", w1, r[0], w2, r[1], r[2])
	}
	vc := "none"
	if len(im.ViewsChanged) > 0 {
		vc = strings.Join(im.ViewsChanged, ", ")
	}
	fmt.Fprintf(&sb, "Views changed: %s", vc)
	if len(im.Rollup) > 0 {
		fmt.Fprintf(&sb, "      Rollup: %s", strings.Join(im.Rollup, "; "))
	}
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "Coverage %.0f%% → %.0f%%   Orphans: %s   Rogue: %d   Proposed: %d   Stale: %d\n", im.Before.CoveragePct, im.After.CoveragePct, orphans(im.Orphans), im.After.Rogues, im.After.Proposed, im.After.Stale)
	for _, v := range im.Versions {
		mark := "✓"
		if !v.OK {
			mark = "✗"
		}
		fmt.Fprintf(&sb, "Version %s: %s → %s (requires %s) %s\n", v.Namespace, orNone(v.From), orNone(v.To), v.Required, mark)
	}
	return sb.String()
}

func orphans(xs []string) string {
	if len(xs) == 0 {
		return "0"
	}
	if len(xs) > 3 {
		return fmt.Sprintf("%d (%s, …)", len(xs), strings.Join(xs[:3], ", "))
	}
	return strings.Join(xs, ", ")
}

func tagFor(ch Change) string {
	var parts []string
	if ch.Actor != "" {
		if strings.HasPrefix(ch.Actor, "agent:") {
			parts = append(parts, "agent")
		} else {
			parts = append(parts, "human")
		}
	}
	if ch.Conf > 0 {
		parts = append(parts, fmt.Sprintf("conf %.2f", ch.Conf))
	}
	switch ch.Gate {
	case AUTO:
		parts = append(parts, "AUTO")
	case SOFT:
		if ch.Proposed {
			parts = append(parts, "SOFT: auto-accept eligible")
		} else {
			parts = append(parts, "SOFT")
		}
	default:
		parts = append(parts, "HARD: "+strings.Join(ch.Reasons, "; "))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// RenderMarkdown renders the PR comment for GitHub: the compact block plus
// per-view details.
func RenderMarkdown(im *Impact) string {
	var sb strings.Builder
	sb.WriteString("<!-- glossmark-impact -->\n")
	fmt.Fprintf(&sb, "## Ontology impact — gate **%s**\n\n```text\n%s```\n", im.Gate, RenderText(im))
	for _, d := range im.Views {
		if !d.Changed() {
			continue
		}
		fmt.Fprintf(&sb, "\n<details><summary>View <b>%s</b>: +%d/−%d nodes, +%d/−%d edges, %d changed</summary>\n\n", d.View, len(d.AddedNodes), len(d.RemovedNodes), len(d.AddedEdges), len(d.RemovedEdges), len(d.ChangedNodes))
		for _, n := range d.AddedNodes {
			fmt.Fprintf(&sb, "- 🟢 node `%s`\n", n)
		}
		for _, n := range d.RemovedNodes {
			fmt.Fprintf(&sb, "- 🔴 node `%s`\n", n)
		}
		for _, n := range d.ChangedNodes {
			fmt.Fprintf(&sb, "- 🟡 node `%s` changed\n", n)
		}
		for _, e := range d.AddedEdges {
			dash := ""
			if e.Proposed {
				dash = " (proposed)"
			}
			fmt.Fprintf(&sb, "- 🟢 `%s` %s `%s`%s\n", e.S, e.P, e.O, dash)
		}
		for _, e := range d.RemovedEdges {
			fmt.Fprintf(&sb, "- 🔴 `%s` %s `%s`\n", e.S, e.P, e.O)
		}
		sb.WriteString("\n</details>\n")
	}
	if im.Gate == HARD {
		sb.WriteString("\n**HARD** changes need a human decision in this PR. ")
	} else if im.Gate == SOFT {
		sb.WriteString("\n**SOFT** changes alter only the engineering view and clear by policy (`gloss accept --policy`). ")
	} else {
		sb.WriteString("\nOnly **AUTO** changes: nothing to review. ")
	}
	sb.WriteString("Accept a proposal with `gloss accept <marker-id>`.\n")
	return sb.String()
}
