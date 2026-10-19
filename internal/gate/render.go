// @gloss implements gmi:Gate scope=file

package gate

import (
	"fmt"
	"sort"
	"strings"
)

// maxRows caps every list in the Markdown comment so it stays under
// GitHub's comment limit on a large PR; the rest is "… N more".
const maxRows = 40

// RenderText renders the PR comment in the design's compact form.
func RenderText(im *Impact) string {
	var sb strings.Builder
	sb.WriteString(header(im))
	if im.Adoption != nil {
		sb.WriteString(renderAdoption(im.Adoption))
	}
	if len(im.Changes) == 0 && im.Adoption == nil {
		sb.WriteString("  no ontology-relevant changes\n")
	}
	sb.WriteString(renderRows(im.Changes, 0))
	sb.WriteString(footer(im))
	return sb.String()
}

func header(im *Impact) string {
	var sb strings.Builder
	where := im.Repo
	if len(im.Boundaries) > 0 && im.Adoption == nil {
		where += " (" + strings.Join(im.Boundaries, ", ") + ")"
	}
	fmt.Fprintf(&sb, "Ontology impact — %s   base %s   gate %s", where, im.Base, im.Gate)
	if im.Criticality != "" {
		fmt.Fprintf(&sb, "   criticality %s", im.Criticality)
	}
	sb.WriteString("\n")
	return sb.String()
}

// renderRows renders changes as aligned rows; limit > 0 caps them.
func renderRows(changes []Change, limit int) string {
	var sb strings.Builder
	w1, w2 := 0, 0
	rows := make([][3]string, 0, len(changes))
	for _, ch := range changes {
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
	more := 0
	if limit > 0 && len(rows) > limit {
		more = len(rows) - limit
		rows = rows[:limit]
	}
	for _, r := range rows {
		fmt.Fprintf(&sb, "%-*s   %-*s   %s\n", w1, r[0], w2, r[1], r[2])
	}
	if more > 0 {
		fmt.Fprintf(&sb, "… %d more\n", more)
	}
	return sb.String()
}

func renderAdoption(a *Adoption) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Adoption: the base has no graph, so every link is new. %d namespaces, %d concepts, %d boundaries, %d files, %d markers (%d links, %d proposed, %d rogue). Counts replace %d per-link rows.\n",
		a.Namespaces, a.Concepts, a.Boundaries, a.Files, a.Markers, a.Links, a.Proposed, a.Rogues, a.Hidden)
	w := len("Boundary")
	for _, b := range a.PerBoundary {
		if len(b.Boundary) > w {
			w = len(b.Boundary)
		}
	}
	fmt.Fprintf(&sb, "  %-*s  %5s  %7s  %5s  %8s  %8s\n", w, "Boundary", "files", "markers", "links", "proposed", "realizes")
	rows := a.PerBoundary
	more := 0
	if len(rows) > maxRows {
		more = len(rows) - maxRows
		rows = rows[:maxRows]
	}
	for _, b := range rows {
		fmt.Fprintf(&sb, "  %-*s  %5d  %7d  %5d  %8d  %8d\n", w, b.Boundary, b.Files, b.Markers, b.Links, b.Proposed, b.Realizes)
	}
	if more > 0 {
		fmt.Fprintf(&sb, "  … %d more boundaries\n", more)
	}
	for _, v := range a.PerView {
		fmt.Fprintf(&sb, "  view %s: %d nodes, %d edges\n", v.View, v.Nodes, v.Edges)
	}
	if len(a.PerView) > 0 {
		sb.WriteString("\n")
	}
	return sb.String()
}

func footer(im *Impact) string {
	var sb strings.Builder
	vc := "none"
	if len(im.ViewsChanged) > 0 {
		vc = strings.Join(im.ViewsChanged, ", ")
	}
	fmt.Fprintf(&sb, "Views changed: %s", vc)
	if len(im.Rollup) > 0 {
		fmt.Fprintf(&sb, "      Rollup: %s", strings.Join(im.Rollup, "; "))
	}
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "Concepts covered %.0f%% → %.0f%%   Files %.0f%% → %.0f%%   Orphans: %s   Rogue: %d   Proposed links: %d   Proposed concepts: %d   Stale: %d\n",
		im.Before.ConceptCoveragePct, im.After.ConceptCoveragePct, im.Before.CoveragePct, im.After.CoveragePct, orphans(im.Orphans), im.After.Rogues, im.After.Proposed, im.After.ProposedConcepts, im.After.Stale)
	if len(im.Critical) > 0 {
		sb.WriteString("Criticality touched: " + criticalList(im.Critical, 6) + "\n")
	}
	for _, v := range im.Versions {
		mark := "✓"
		if !v.OK {
			mark = "✗"
		}
		fmt.Fprintf(&sb, "Version %s: %s → %s (requires %s) %s\n", v.Namespace, orNone(v.From), orNone(v.To), v.Required, mark)
	}
	return sb.String()
}

// criticalList renders "pay:Payment critical (declared), acme:payments-api critical (via pay:Payment), …".
func criticalList(ts []CriticalityTouch, max int) string {
	var parts []string
	for i, t := range ts {
		if i >= max {
			parts = append(parts, fmt.Sprintf("… %d more", len(ts)-max))
			break
		}
		from := t.From
		if from != "declared" && from != "" {
			from = "via " + from
		}
		parts = append(parts, fmt.Sprintf("%s %s (%s)", t.Curie, t.Level, from))
	}
	return strings.Join(parts, ", ")
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
	if ch.Criticality != "" {
		parts = append(parts, strings.ToUpper(ch.Criticality))
	}
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

// RenderMarkdown renders the PR comment for GitHub: the header and footer
// in one block, the changes grouped by boundary in collapsible sections
// (HARD groups open), each capped, then per-view details. An adoption diff
// shows counts instead of links.
func RenderMarkdown(im *Impact) string {
	var sb strings.Builder
	sb.WriteString("<!-- glossmark-impact -->\n")
	fmt.Fprintf(&sb, "## Ontology impact — gate **%s**", im.Gate)
	if im.Criticality != "" {
		fmt.Fprintf(&sb, " · criticality **%s**", im.Criticality)
	}
	sb.WriteString("\n\n```text\n")
	sb.WriteString(header(im))
	if im.Adoption != nil {
		sb.WriteString(renderAdoption(im.Adoption))
	}
	sb.WriteString(footer(im))
	sb.WriteString("```\n")
	if len(im.Critical) > 0 {
		fmt.Fprintf(&sb, "\n**Criticality touched** (highest first): %s. Reviewers of %s-criticality areas should look at this PR.\n", criticalList(im.Critical, 8), im.Criticality)
	}
	if len(im.Changes) == 0 && im.Adoption == nil {
		sb.WriteString("\nNo ontology-relevant changes.\n")
	}
	// Changes grouped by boundary, decision requests first.
	for _, grp := range groupChanges(im.Changes) {
		hard, soft, auto := 0, 0, 0
		for _, ch := range grp.changes {
			switch ch.Gate {
			case HARD:
				hard++
			case SOFT:
				soft++
			default:
				auto++
			}
		}
		open := ""
		if hard > 0 {
			open = " open"
		}
		fmt.Fprintf(&sb, "\n<details%s><summary><b>%s</b>: %d change(s) — %d HARD, %d SOFT, %d AUTO</summary>\n\n```text\n%s```\n</details>\n", open, grp.name, len(grp.changes), hard, soft, auto, renderRows(grp.changes, maxRows))
	}
	for _, d := range im.Views {
		if !d.Changed() {
			continue
		}
		fmt.Fprintf(&sb, "\n<details><summary>View <b>%s</b>: +%d/−%d nodes, +%d/−%d edges, %d changed</summary>\n\n", d.View, len(d.AddedNodes), len(d.RemovedNodes), len(d.AddedEdges), len(d.RemovedEdges), len(d.ChangedNodes))
		if im.Adoption != nil {
			sb.WriteString("Every node and edge is new in this view; open the viewer (`gloss view`) for the picture.\n")
		} else {
			n := 0
			line := func(s string) {
				n++
				if n <= maxRows {
					sb.WriteString(s)
				}
			}
			for _, x := range d.AddedNodes {
				line(fmt.Sprintf("- 🟢 node `%s`\n", x))
			}
			for _, x := range d.RemovedNodes {
				line(fmt.Sprintf("- 🔴 node `%s`\n", x))
			}
			for _, x := range d.ChangedNodes {
				line(fmt.Sprintf("- 🟡 node `%s` changed\n", x))
			}
			for _, e := range d.AddedEdges {
				dash := ""
				if e.Proposed {
					dash = " (proposed)"
				}
				line(fmt.Sprintf("- 🟢 `%s` %s `%s`%s\n", e.S, e.P, e.O, dash))
			}
			for _, e := range d.RemovedEdges {
				line(fmt.Sprintf("- 🔴 `%s` %s `%s`\n", e.S, e.P, e.O))
			}
			if n > maxRows {
				fmt.Fprintf(&sb, "- … %d more\n", n-maxRows)
			}
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

type changeGroup struct {
	name    string
	changes []Change
}

// groupChanges splits changes into decision requests, ontology and manifest
// edits, and one group per boundary (then files under no manifest).
func groupChanges(changes []Change) []changeGroup {
	const decisions, ontology, nowhere = "Decisions requested", "Ontology, manifests and glossmark.yaml", "Files under no manifest"
	byName := map[string][]Change{}
	var order []string
	add := func(name string, ch Change) {
		if _, ok := byName[name]; !ok {
			order = append(order, name)
		}
		byName[name] = append(byName[name], ch)
	}
	for _, ch := range changes {
		switch {
		case ch.Kind == "new-concept" || ch.Kind == "concept-proposed":
			add(decisions, ch)
		case ch.Boundary != "":
			add(ch.Boundary, ch)
		case ch.Kind == "link-added" || ch.Kind == "link-removed" || ch.Kind == "link-accepted" || ch.Kind == "fingerprint-changed" || ch.Kind == "stale":
			add(nowhere, ch)
		default:
			add(ontology, ch)
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		rank := func(n string) int {
			switch n {
			case decisions:
				return 0
			case ontology:
				return 1
			case nowhere:
				return 3
			}
			return 2
		}
		if rank(order[i]) != rank(order[j]) {
			return rank(order[i]) < rank(order[j])
		}
		return order[i] < order[j]
	})
	var out []changeGroup
	for _, n := range order {
		out = append(out, changeGroup{name: n, changes: byName[n]})
	}
	return out
}
