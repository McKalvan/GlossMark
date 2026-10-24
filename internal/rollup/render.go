// @gloss implements gmi:DriftComparator scope=file

package rollup

import (
	"fmt"
	"strings"
)

var kindOrder = []string{KindViolation, KindValueDrift, KindOverdue, KindPlanOverdue, KindOutOfRange, KindGap, KindPlannedClosed, KindRogue, KindProposed, KindPlanned, KindDangling, KindStale, KindOrphan, KindPresentedOnly, KindUnguarded, KindUntested, KindDeprecatedUse, KindOverlap}

var kindTitle = map[string]string{
	KindViolation:     "Violations (boundary exceeds its declared capability)",
	KindValueDrift:    "Published values drifted (the bound code changed, the value did not)",
	KindGap:           "Gaps (declared, not implemented)",
	KindRogue:         "Rogue concepts (decision requests)",
	KindProposed:      "Proposed concepts (awaiting acceptance)",
	KindDangling:      "Dangling references",
	KindStale:         "Stale links (anchor changed)",
	KindOrphan:        "Orphan concepts (no artifacts)",
	KindUntested:      "Implemented but untested",
	KindPresentedOnly: "Presented but not implemented (a surface with no mechanism)",
	KindUnguarded:     "Unguarded handlers (a guarded-by boundary's guard decorator missing)",
	KindOverlap:       "Overlaps (one concept, several unrelated boundaries, no declared migration)",
	KindOverdue:       "Overdue retirements (retire-by passed, code still there)",
	KindOutOfRange:    "Imports out of range",
	KindDeprecatedUse: "Deprecated in use",
	KindPlanned:       "Plan items (planned, nothing implements them yet)",
	KindPlannedClosed: "Plan items closed (implemented: drop planned)",
	KindPlanOverdue:   "Plan items overdue (by passed, nothing implements them)",
}

var kindGlyph = map[string]string{KindPlanned: "◌", KindPlannedClosed: "●", KindPlanOverdue: "⌛", KindOverlap: "⇉", KindOverdue: "⌛", KindOutOfRange: "✗", KindDeprecatedUse: "~", KindViolation: "✗", KindGap: "○", KindRogue: "?", KindProposed: "?", KindDangling: "!", KindStale: "~", KindOrphan: "·", KindUntested: "△", KindPresentedOnly: "◇", KindUnguarded: "⊘", KindValueDrift: "≠", KindConsistent: "✓"}

// RenderText renders the report for a terminal.
func RenderText(r *Report, all bool) string {
	return RenderTextOpts(r, TextOptions{All: all})
}

// TextOptions tunes RenderTextOpts.
type TextOptions struct {
	// All also lists consistent pairs and overlaps.
	All bool
	// IDs ends each finding's line with its id, the key a waiver's finding:
	// names (gloss drift --ids).
	IDs bool
}

// RenderTextOpts renders the report for a terminal.
func RenderTextOpts(r *Report, opt TextOptions) string {
	all := opt.All
	var sb strings.Builder
	sb.WriteString("Ontology drift\n")
	for _, k := range kindOrder {
		if k == KindOverlap && !all {
			continue // informational; listed with --all, always in JSON
		}
		var fs []Finding
		for _, f := range r.Findings {
			if f.Kind == k {
				fs = append(fs, f)
			}
		}
		if len(fs) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "\n%s %s\n", kindGlyph[k], kindTitle[k])
		for _, f := range fs {
			loc := ""
			if f.Path != "" {
				loc = f.Path
				if f.Line > 0 {
					loc += fmt.Sprintf(":%d", f.Line)
				}
				loc = "  [" + loc + "]"
			}
			w := ""
			if f.Waived {
				w = "  (waived: " + f.Waiver + ")"
			}
			id := ""
			if opt.IDs && f.ID != "" {
				id = "  (id " + f.ID + ")"
			}
			fmt.Fprintf(&sb, "  %s%s%s%s\n", f.Msg, loc, w, id)
		}
	}
	if all {
		sb.WriteString("\n✓ Consistent\n")
		for _, f := range r.Findings {
			if f.Kind == KindConsistent {
				fmt.Fprintf(&sb, "  %s\n", f.Msg)
			}
		}
	}
	if len(r.Decisions) > 0 {
		sb.WriteString("\nDecision requests\n")
		for _, d := range r.Decisions {
			owners := ""
			if len(d.Owners) > 0 {
				owners = " → " + strings.Join(d.Owners, ", ")
			}
			switch d.Kind {
			case "new-concept":
				fmt.Fprintf(&sb, "  propose %s (%s at %s:%d)%s", d.Concept, d.Relation, d.Path, d.Line, owners)
			case "proposed-concept":
				fmt.Fprintf(&sb, "  accept or reject proposed %s (%s:%d)%s", d.Concept, d.Path, d.Line, owners)
			default:
				fmt.Fprintf(&sb, "  %s (%s:%d)%s", d.Note, d.Path, d.Line, owners)
			}
			if d.Note != "" && d.Kind == "new-concept" {
				fmt.Fprintf(&sb, "  — %s", d.Note)
			}
			sb.WriteString("\n")
		}
	}
	// File coverage stays out of the trailer: every file added or deleted,
	// prose included, moves it, so a committed drift output would change
	// with every commit. gloss coverage and the JSON metrics carry it.
	m := r.Metrics
	fmt.Fprintf(&sb, "\nConcepts covered %s (%d/%d with an artifact and a verifier)   Orphans: %d   Rogue: %d   Violations: %d   Gaps: %d   Stale: %d   Untested: %d   Proposed links: %d   Proposed concepts: %d   Plan items: %d\n",
		m.ConceptCoverage("%.0f%%"), m.ConceptsCovered, m.ConceptsInScope, m.Orphans, m.Rogues, m.Violations, m.Gaps, m.Stale, m.Untested, m.Proposed, m.ProposedConcepts, m.PlanItems)
	return sb.String()
}

// RenderMarkdown renders the report as a GitHub-flavoured Markdown table.
func RenderMarkdown(r *Report, all bool) string {
	var sb strings.Builder
	sb.WriteString("### Ontology drift\n\n| | Derived from code | Declared | Result |\n| --- | --- | --- | --- |\n")
	for _, f := range r.Findings {
		if (f.Kind == KindConsistent || f.Kind == KindOverlap) && !all {
			continue
		}
		derived := strings.Join(f.Derived, ", ")
		if derived == "" && f.Concept != "" {
			derived = f.Concept
		}
		if f.Path != "" {
			derived += fmt.Sprintf(" (`%s", f.Path)
			if f.Line > 0 {
				derived += fmt.Sprintf(":%d", f.Line)
			}
			derived += "`)"
		}
		declared := strings.Join(f.Declared, ", ")
		if declared == "" {
			declared = "—"
		}
		res := strings.Title(f.Kind)
		if f.Waived {
			res += " (waived)"
		}
		fmt.Fprintf(&sb, "| %s | %s | %s | **%s**: %s |\n", kindGlyph[f.Kind], derived, declared, res, f.Msg)
	}
	if len(r.Decisions) > 0 {
		sb.WriteString("\n**Decision requests**\n\n")
		for _, d := range r.Decisions {
			fmt.Fprintf(&sb, "- `%s` — %s (`%s:%d`)\n", d.Concept, d.Note, d.Path, d.Line)
		}
	}
	m := r.Metrics
	fmt.Fprintf(&sb, "\nConcepts covered %s · Orphans %d · Rogue %d · Violations %d · Gaps %d · Stale %d · Untested %d · Proposed links %d · Proposed concepts %d · Plan items %d\n",
		m.ConceptCoverage("%.0f%%"), m.Orphans, m.Rogues, m.Violations, m.Gaps, m.Stale, m.Untested, m.Proposed, m.ProposedConcepts, m.PlanItems)
	return sb.String()
}
