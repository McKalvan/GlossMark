package agents

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// Context renders the compact session-start briefing: a few dozen lines an
// agent can afford on every session, scoped to the directory's boundary.
func Context(ws *load.Workspace, g *graph.Graph, m *model.Manifest, rep *rollup.Report, maxConcepts int) string {
	var sb strings.Builder
	sb.WriteString("# GlossMark context\n")
	sb.WriteString("This repository links code to a two-tier ontology with one-line `@gloss` comments. ")
	sb.WriteString("Write `@gloss? <relation> <target>` on declarations you add or change (`?` = proposal). ")
	sb.WriteString("Never invent concept names: use an existing CURIE, or `gloss propose concept ns:Name --kind ...` to add a proposed one. ")
	sb.WriteString("Ask the MCP server (`gloss mcp`) with search_concepts, concepts_for_path, propose_link and impact_of.\n")
	if m != nil {
		fmt.Fprintf(&sb, "\nBoundary: %s (%s)", m.Boundary, m.Level)
		if m.DefaultNamespace != "" {
			fmt.Fprintf(&sb, "; `.Name` = `%s:Name`", m.DefaultNamespace)
		}
		if len(m.Realizes) > 0 {
			fmt.Fprintf(&sb, "; realizes %s", strings.Join(m.Realizes, ", "))
		}
		sb.WriteString("\n")
		if len(m.Allowed) > 0 {
			fmt.Fprintf(&sb, "Allowed targets: %s (others are HARD-gated)\n", strings.Join(m.Allowed, ", "))
		}
	}
	// Agent remits present in the repo.
	var remits []string
	for _, a := range g.NodesOf(model.NodeArtifact) {
		if es := g.Out(a.ID, "may-modify"); len(es) > 0 {
			var ts []string
			for _, e := range es {
				ts = append(ts, g.Curie(e.O))
			}
			remits = append(remits, fmt.Sprintf("%s may modify %s", a.Path, strings.Join(ts, ", ")))
		}
	}
	if len(remits) > 0 {
		sb.WriteString("Agent remits: " + strings.Join(remits, "; ") + "\n")
	}
	// Concepts, grouped by namespace, core first, allowed-only when a list exists.
	type row struct{ ns, line string }
	var rows []row
	for _, n := range g.NodesOf(model.NodeConcept) {
		if n.Deprecated || (n.Imported && n.Tier == model.TierImpl && n.Published == "") {
			continue
		}
		if m != nil && len(m.Allowed) > 0 && !model.MatchAnyCurie(m.Allowed, n.CURIE) {
			continue
		}
		var tags []string
		for _, r := range g.Objects(n.ID, "realizes") {
			tags = append(tags, "realizes "+g.Curie(r))
		}
		for _, r := range g.Objects(n.ID, "is-a") {
			tags = append(tags, "is-a "+g.Curie(r))
		}
		if n.Published != "" {
			tags = append(tags, "published as "+n.Published)
		}
		if n.Internal {
			tags = append(tags, "internal")
		}
		if n.Proposed {
			tags = append(tags, "PROPOSED")
		}
		if len(n.Aliases) > 0 {
			tags = append(tags, "aka "+strings.Join(n.Aliases, "/"))
		}
		line := fmt.Sprintf("- %s (%s)", n.CURIE, n.Kind)
		if len(tags) > 0 {
			line += ": " + strings.Join(tags, ", ")
		}
		if n.Doc != "" {
			d := n.Doc
			if len(d) > 80 {
				d = d[:77] + "..."
			}
			line += " — " + d
		}
		rows = append(rows, row{n.NS, line})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ti, tj := ws.Tier(rows[i].ns), ws.Tier(rows[j].ns)
		if ti != tj {
			return ti == model.TierCore
		}
		return rows[i].ns < rows[j].ns
	})
	fmt.Fprintf(&sb, "\nConcepts (%d)", len(rows))
	if maxConcepts > 0 && len(rows) > maxConcepts {
		fmt.Fprintf(&sb, ", first %d; use search_concepts for the rest", maxConcepts)
		rows = rows[:maxConcepts]
	}
	sb.WriteString(":\n")
	for _, r := range rows {
		sb.WriteString(r.line + "\n")
	}
	if rep != nil {
		mm := rep.Metrics
		fmt.Fprintf(&sb, "\nState: coverage %.0f%%, %d proposed links, %d proposed concepts, %d stale, %d violations, %d gaps, %d untested.\n",
			mm.CoveragePct, mm.Proposed, mm.ProposedConcepts, mm.Stale, mm.Violations, mm.Gaps, mm.Untested)
		var untested []string
		for _, f := range rep.Findings {
			if f.Kind == rollup.KindUntested && (m == nil || len(m.Allowed) == 0 || model.MatchAnyCurie(m.Allowed, f.Concept)) {
				untested = append(untested, f.Concept)
			}
		}
		if len(untested) > 0 {
			if len(untested) > 8 {
				untested = append(untested[:8], "...")
			}
			sb.WriteString("Implemented but untested here: " + strings.Join(untested, ", ") + " (add `@gloss verifies` in tests).\n")
		}
	}
	return sb.String()
}
