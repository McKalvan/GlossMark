// @gloss implements gmi:AgentsSnippet scope=file
// @gloss consumes gm:PlanItem scope=file

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
		head.WriteString(retiring(g, m, opt.Dir))
		if len(allowed.Patterns) > 0 {
			how := ""
			if allowed.Derived {
				how = ", derived from what the manifest declares"
			}
			fmt.Fprintf(&head, "Allowed targets: %s (others are HARD-gated%s)\n", strings.Join(allowed.Patterns, ", "), how)
		}
	}
	head.WriteString(applied(g, rollup.AppliesRelationsIn(ws.Vocab, "context"), opt.Dir))
	head.WriteString(remits(g, opt.Actor))
	head.WriteString(planItems(rep, m, allowed, opt.Dir))

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

// maxPlanItems caps the plan items the briefing lists.
const maxPlanItems = 8

// planItems lists the plan items (ADR 0034) an agent working in dir is
// building towards: the planned declarations of a manifest at or under dir,
// and the planned concepts the directory's manifest declares, plans or
// allows code to link to. A closed one says to drop planned.
func planItems(rep *rollup.Report, m *model.Manifest, allowed rollup.Allowed, dir string) string {
	if rep == nil {
		return ""
	}
	dir = strings.Trim(filepath.ToSlash(dir), "/")
	if dir == "." {
		dir = ""
	}
	declared := map[string]bool{}
	if m != nil {
		for _, ts := range m.DeclaredSets() {
			for _, t := range ts {
				declared[t] = true
			}
		}
	}
	var open, closed []string
	for _, f := range rep.Findings {
		switch f.Kind {
		case rollup.KindPlanned, rollup.KindPlanOverdue, rollup.KindPlannedClosed:
		default:
			continue
		}
		here := f.Path != "" && !strings.HasSuffix(f.Path, ".gloss.yaml") && rollup.Under(f.Path, dir) ||
			strings.HasSuffix(f.Path, "/.gloss.yaml") && rollup.Under(f.Path, dir) ||
			f.Path == ".gloss.yaml" && dir == "" ||
			m != nil && f.Boundary == m.Boundary ||
			f.Concept != "" && (declared[f.Concept] || m != nil && len(allowed.Patterns) > 0 && allowed.Matches(f.Concept))
		if !here {
			continue
		}
		if f.Kind == rollup.KindPlannedClosed {
			closed = append(closed, f.Msg)
		} else {
			open = append(open, f.Msg)
		}
	}
	if len(open)+len(closed) == 0 {
		return ""
	}
	var sb strings.Builder
	list := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		sb.WriteString(title + "\n")
		for i, it := range items {
			if i == maxPlanItems {
				fmt.Fprintf(&sb, "  ... %d more (gloss drift)\n", len(items)-i)
				break
			}
			sb.WriteString("  - " + it + "\n")
		}
	}
	list("Open plan items here (ADR 0034; build against them, and record a departure as a proposed concept, not a silent change):", open)
	list("Closed plan items here (drop planned with gloss accept --planned):", closed)
	return sb.String()
}

// retiring is the "Retiring here" line: the boundary (or one enclosing it)
// being retired, and the deprecated concepts the directory's code links to,
// each with its successor and the boundary that realizes the successor, so
// an agent builds on the replacement instead of extending legacy code.
func retiring(g *graph.Graph, m *model.Manifest, dir string) string {
	var parts []string
	if b := g.NodeByCurie(m.Boundary); b != nil {
		for _, id := range append([]string{b.ID}, g.PartOfChain(b.ID)...) {
			n := g.Node(id)
			if n == nil || !n.Deprecated {
				continue
			}
			p := n.CURIE + " is deprecated"
			if n.ReplacedBy != "" {
				p += " → " + n.ReplacedBy
			}
			if n.RetireBy != "" {
				p += ", retire by " + n.RetireBy
			}
			parts = append(parts, p)
			break
		}
	}
	dir = strings.Trim(filepath.ToSlash(dir), "/")
	if dir == "." {
		dir = ""
	}
	realizers := rollup.Realizers(g, "realizes")
	seen := map[string]bool{}
	var concepts []string
	for _, a := range g.NodesOf(model.NodeArtifact) {
		if dir != "" && a.Path != dir && !strings.HasPrefix(a.Path, dir+"/") {
			continue
		}
		for _, e := range g.Out(a.ID, "") {
			o := g.Node(e.O)
			if !graph.LinkSource(e.Source) || o == nil || o.NodeType != model.NodeConcept || !o.Deprecated || seen[o.ID] {
				continue
			}
			seen[o.ID] = true
			p := o.CURIE
			if o.ReplacedBy != "" {
				p += " → " + o.ReplacedBy
				if bs := realizers[o.ReplacedBy]; len(bs) > 0 {
					p += " (" + strings.Join(bs, ", ") + ")"
				}
			}
			concepts = append(concepts, p)
		}
	}
	sort.Strings(concepts)
	parts = append(parts, concepts...)
	if len(parts) == 0 {
		return ""
	}
	return "Retiring here: " + strings.Join(parts, "; ") + ". Do not add links to deprecated concepts or extend retiring code; build on the replacement.\n"
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
			if graph.LinkSource(e.Source) || e.Source == graph.SourceInferred {
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
	if n.Planned {
		tags = append(tags, plannedTag(n))
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

// applied names the mechanisms and rules that apply to the code under dir:
// what guards (ADR 0015) or constrains a concept an artifact there links to,
// or a concept that one stands for, so an agent editing a handler knows the
// authentication and rate limits it sits behind.
func applied(g *graph.Graph, rels []string, dir string) string {
	dir = strings.Trim(filepath.ToSlash(dir), "/")
	if dir == "." {
		dir = ""
	}
	type key struct{ src, rel string }
	on := map[key]map[string]bool{}
	var order []key
	// guarded-by in a manifest at, above or below dir guards code here.
	for _, mg := range rollup.ManifestGuards(g) {
		if !rollup.Under(dir, mg.Dir) && !rollup.Under(mg.Dir, dir) {
			continue
		}
		k := key{mg.Guard, "guards"}
		if on[k] == nil {
			on[k] = map[string]bool{}
			order = append(order, k)
		}
		where := mg.Dir
		if where == "" {
			where = "."
		}
		on[k]["code under "+where+"/"] = true
	}
	for _, a := range g.NodesOf(model.NodeArtifact) {
		if dir != "" && a.Path != dir && !strings.HasPrefix(a.Path, dir+"/") {
			continue
		}
		for _, e := range g.Out(a.ID, "") {
			if e.Source == graph.SourceDerived || !rollup.GuardedLink(e.P) {
				continue
			}
			if o := g.Node(e.O); o == nil || o.NodeType != model.NodeConcept {
				continue
			}
			for _, x := range append([]string{e.O}, rollup.RealizesStar(g, e.O)...) {
				for _, rel := range rels {
					for _, s := range g.Subjects(x, rel) {
						k := key{s, rel}
						if on[k] == nil {
							on[k] = map[string]bool{}
							order = append(order, k)
						}
						on[k][g.Curie(x)] = true
					}
				}
			}
		}
	}
	if len(order) == 0 {
		return ""
	}
	relRank := func(r string) int {
		switch r {
		case "guards":
			return 0
		case "constrains":
			return 1
		}
		return 2
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].rel != order[j].rel {
			if relRank(order[i].rel) != relRank(order[j].rel) {
				return relRank(order[i].rel) < relRank(order[j].rel) // guards, constrains, then custom
			}
			return order[i].rel < order[j].rel
		}
		return g.Curie(order[i].src) < g.Curie(order[j].src)
	})
	var parts []string
	note := ""
	for _, k := range order {
		if k.rel == "guards" {
			note = " (a change to guarded code is reviewed at the guard's level)"
		}
		label := g.Curie(k.src)
		if n := g.Node(k.src); n != nil && n.Criticality != "" {
			label += " (" + n.Criticality + ")"
		}
		var ts []string
		for t := range on[k] {
			ts = append(ts, t)
		}
		sort.Strings(ts)
		parts = append(parts, fmt.Sprintf("%s %s %s", label, k.rel, strings.Join(ts, ", ")))
	}
	return "Applies here: " + strings.Join(parts, "; ") + note + "\n"
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

// plannedTag marks a planned concept for an agent: "PLANNED", with the
// deadline when the plan has one, so the agent building the feature finds
// the owner's name for it (ADR 0025 6.1, ADR 0034).
func plannedTag(n *graph.Node) string {
	if n.PlanBy != "" {
		return "PLANNED (by " + n.PlanBy + ")"
	}
	return "PLANNED"
}
