package agents

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/rollup"
)

// ProposalSentence is the one proposal workflow, stated verbatim by the
// briefing, the AGENTS.md snippet, the MCP server and QUICKSTART, so an agent
// reading any of them gets the same instruction.
const ProposalSentence = "If no concept fits, write `@gloss? <relation> <ns>:NEW:<Name> note=\"why\"`, then run `gloss propose --from-new --doc \"what it is\"` (MCP: propose_concept) so the concept ships in the same PR with `proposed: true`; a human accepts it with `gloss accept --concept <ns>:<Name>`. Never invent a concept name any other way, and never hand-edit an ontology file to make a link resolve."

// ContextOptions tunes the briefing.
type ContextOptions struct {
	// Dir is the repo-relative directory the agent works in ("" for the root).
	Dir string
	// MaxConcepts caps the concept list (0: no cap beyond the budget).
	MaxConcepts int
	// Budget is the approximate token budget of the whole briefing (0: none).
	// Tokens are estimated at four characters each.
	Budget int
	// Actor is GLOSS_ACTOR. When it names an agent with an agent file
	// (agent:breeding-planner and .claude/agents/breeding-planner.md), only
	// that agent's remit is shown.
	Actor string
	// Changed is a pre-rendered impact of the working tree (gloss context
	// --changed); the briefing opens with it.
	Changed string
}

// Context renders the compact session-start briefing for the boundary m.
// Concepts are ranked by relevance to the directory: linked by artifacts
// under it, then declared by its manifest (and what those realize), then in
// its default namespace, then the rest.
func Context(ws *load.Workspace, g *graph.Graph, m *model.Manifest, rep *rollup.Report, opt ContextOptions) string {
	var head, tail strings.Builder
	head.WriteString("# GlossMark context\n")
	head.WriteString("This repository links code to a two-tier ontology with one-line `@gloss` comments. ")
	head.WriteString("Write `@gloss? <relation> <target>` on declarations you add or change (`?` = proposal). ")
	head.WriteString(ProposalSentence + " ")
	head.WriteString("Ask the MCP server (`gloss mcp`) with search_concepts, concepts_for_path, propose_link and impact_of.\n")
	if opt.Changed != "" {
		head.WriteString("\n## You are touching\n" + opt.Changed)
	}
	allowed := rollup.AllowedFor(g, ws, m)
	if m != nil {
		fmt.Fprintf(&head, "\nBoundary: %s (%s)", m.Boundary, m.Level)
		if m.DefaultNamespace != "" {
			fmt.Fprintf(&head, "; `.Name` = `%s:Name`", m.DefaultNamespace)
		}
		if len(m.Realizes) > 0 {
			fmt.Fprintf(&head, "; realizes %s", strings.Join(m.Realizes, ", "))
		}
		if b := g.NodeByCurie(m.Boundary); b != nil && b.Criticality != "" {
			from := b.CriticalityFrom
			if from != "declared" {
				from = "inherited from " + from
			}
			fmt.Fprintf(&head, "; criticality %s (%s)", b.Criticality, from)
		}
		head.WriteString("\n")
		if len(allowed.Patterns) > 0 {
			how := ""
			if allowed.Derived {
				how = ", derived from what the manifest declares"
			}
			fmt.Fprintf(&head, "Allowed targets: %s (others are HARD-gated%s)\n", strings.Join(allowed.Patterns, ", "), how)
		}
	}
	head.WriteString(remits(g, opt.Actor))

	if rep != nil {
		mm := rep.Metrics
		fmt.Fprintf(&tail, "\nState: %d/%d concepts covered (artifact and verifier), files %.0f%%, %d proposed links, %d proposed concepts, %d stale, %d violations, %d gaps, %d untested.\n",
			mm.ConceptsCovered, mm.ConceptsInScope, mm.CoveragePct, mm.Proposed, mm.ProposedConcepts, mm.Stale, mm.Violations, mm.Gaps, mm.Untested)
		var untested []string
		for _, f := range rep.Findings {
			if f.Kind == rollup.KindUntested && (m == nil || allowed.Matches(f.Concept)) {
				untested = append(untested, f.Concept)
			}
		}
		if len(untested) > 0 {
			if len(untested) > 8 {
				untested = append(untested[:8], "...")
			}
			tail.WriteString("Implemented but untested here: " + strings.Join(untested, ", ") + " (add `@gloss verifies` in tests).\n")
		}
	}

	rows := rankConcepts(ws, g, m, allowed, opt.Dir)
	total := len(rows)
	// Fit the concept list into what the budget leaves.
	limit := total
	if opt.MaxConcepts > 0 && limit > opt.MaxConcepts {
		limit = opt.MaxConcepts
	}
	if opt.Budget > 0 {
		left := opt.Budget*4 - head.Len() - tail.Len() - 80
		n := 0
		for n < limit && left-len(rows[n].line)-1 >= 0 {
			left -= len(rows[n].line) + 1
			n++
		}
		limit = n
	}
	var sb strings.Builder
	sb.WriteString(head.String())
	fmt.Fprintf(&sb, "\nConcepts (%d, most relevant to this directory first)", total)
	if limit < total {
		fmt.Fprintf(&sb, ", first %d; use search_concepts for the rest", limit)
	}
	sb.WriteString(":\n")
	for _, r := range rows[:limit] {
		sb.WriteString(r.line + "\n")
	}
	sb.WriteString(tail.String())
	out := sb.String()
	if opt.Budget > 0 && len(out) > opt.Budget*4 {
		out = out[:opt.Budget*4-4] + "...\n"
	}
	return out
}

type conceptRow struct {
	curie string
	rank  int // 0 linked under dir, 1 declared, 2 namespace, 3 rest
	links int
	core  bool
	line  string
}

func rankConcepts(ws *load.Workspace, g *graph.Graph, m *model.Manifest, allowed rollup.Allowed, dir string) []conceptRow {
	dir = strings.Trim(filepath.ToSlash(dir), "/")
	if dir == "." {
		dir = ""
	}
	// Concepts linked by artifacts under the directory, with link counts.
	linked := map[string]int{}
	for _, a := range g.NodesOf(model.NodeArtifact) {
		if dir != "" && a.Path != dir && !strings.HasPrefix(a.Path, dir+"/") {
			continue
		}
		for _, e := range g.Out(a.ID, "") {
			if e.Source == graph.SourceInline || e.Source == graph.SourceInferred {
				if o := g.Node(e.O); o != nil && o.NodeType == model.NodeConcept {
					linked[o.ID]++
					for _, c := range rollup.RealizesStar(g, o.ID) {
						if _, ok := linked[c]; !ok {
							linked[c] = 0
						}
					}
				}
			}
		}
	}
	declared := map[string]bool{}
	ns := ""
	if m != nil {
		ns = m.DefaultNamespace
		for _, targets := range m.DeclaredSets() {
			for _, t := range targets {
				iri, _ := g.Expand(t)
				declared[iri] = true
				for _, c := range rollup.RealizesStar(g, iri) {
					declared[c] = true
				}
				if ns == "" {
					if i := strings.Index(t, ":"); i > 0 {
						ns = t[:i]
					}
				}
			}
		}
	}
	var rows []conceptRow
	for _, n := range g.NodesOf(model.NodeConcept) {
		if n.Deprecated || (n.Imported && n.Tier == model.TierImpl && n.Published == "") {
			continue
		}
		if m != nil && !allowed.Matches(n.CURIE) {
			continue
		}
		r := conceptRow{curie: n.CURIE, rank: 3, core: n.Tier == model.TierCore, line: conceptLine(g, n)}
		if c, ok := linked[n.ID]; ok {
			r.rank, r.links = 0, c
		} else if declared[n.ID] {
			r.rank = 1
		} else if ns != "" && n.NS == ns {
			r.rank = 2
		}
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if a.links != b.links {
			return a.links > b.links
		}
		if a.core != b.core {
			return a.core
		}
		return a.curie < b.curie
	})
	return rows
}

func conceptLine(g *graph.Graph, n *graph.Node) string {
	var tags []string
	for _, r := range g.Objects(n.ID, "realizes") {
		tags = append(tags, "realizes "+g.Curie(r))
	}
	for _, rel := range []string{"presents", "hosts", "is-a"} {
		for _, r := range g.Objects(n.ID, rel) {
			tags = append(tags, rel+" "+g.Curie(r))
		}
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
	if model.CriticalityRank(n.Criticality) >= model.CriticalityRank("high") {
		tags = append(tags, "criticality "+n.Criticality)
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
	return line
}

// remits lists the agent remits: only the acting agent's when GLOSS_ACTOR
// names an agent with an agent file, otherwise every agent's.
func remits(g *graph.Graph, actor string) string {
	name := ""
	if strings.HasPrefix(actor, "agent:") {
		name = strings.TrimPrefix(actor, "agent:")
		if i := strings.Index(name, "@"); i >= 0 {
			name = name[:i]
		}
	}
	type remit struct{ path, targets string }
	var all, mine []remit
	for _, a := range g.NodesOf(model.NodeArtifact) {
		es := g.Out(a.ID, "may-modify")
		if len(es) == 0 {
			continue
		}
		var ts []string
		for _, e := range es {
			ts = append(ts, g.Curie(e.O))
		}
		r := remit{a.Path, strings.Join(ts, ", ")}
		all = append(all, r)
		base := strings.TrimPrefix(strings.TrimSuffix(filepath.Base(a.Path), filepath.Ext(a.Path)), ".")
		if name != "" && base == name {
			mine = append(mine, r)
		}
	}
	switch {
	case len(mine) > 0:
		var parts []string
		for _, r := range mine {
			parts = append(parts, r.targets+" ("+r.path+")")
		}
		return fmt.Sprintf("Your remit as %s: you may modify %s. Links outside it are HARD-gated.\n", actor, strings.Join(parts, "; "))
	case len(all) > 0:
		var parts []string
		for _, r := range all {
			parts = append(parts, fmt.Sprintf("%s may modify %s", r.path, r.targets))
		}
		return "Agent remits: " + strings.Join(parts, "; ") + "\n"
	}
	return ""
}
