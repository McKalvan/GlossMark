// @gloss implements gmi:DriftComparator scope=file

package rollup

import (
	"fmt"
	"strings"
)

var kindOrder = []string{KindViolation, KindGap, KindRogue, KindDangling, KindStale, KindOrphan, KindUntested}

var kindTitle = map[string]string{
	KindViolation: "Violations (boundary exceeds its declared capability)",
	KindGap:       "Gaps (declared, not implemented)",
	KindRogue:     "Rogue concepts (decision requests)",
	KindDangling:  "Dangling references",
	KindStale:     "Stale links (anchor changed)",
	KindOrphan:    "Orphan concepts (no artifacts)",
	KindUntested:  "Implemented but untested",
}

var kindGlyph = map[string]string{KindViolation: "✗", KindGap: "○", KindRogue: "?", KindDangling: "!", KindStale: "~", KindOrphan: "·", KindUntested: "△", KindConsistent: "✓"}

// RenderText renders the report for a terminal.
func RenderText(r *Report, all bool) string {
	var sb strings.Builder
	sb.WriteString("Ontology drift\n")
	for _, k := range kindOrder {
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
			fmt.Fprintf(&sb, "  %s%s%s\n", f.Msg, loc, w)
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
			default:
				fmt.Fprintf(&sb, "  %s (%s:%d)%s", d.Note, d.Path, d.Line, owners)
			}
			if d.Note != "" && d.Kind == "new-concept" {
				fmt.Fprintf(&sb, "  — %s", d.Note)
			}
			sb.WriteString("\n")
		}
	}
	m := r.Metrics
	fmt.Fprintf(&sb, "\nCoverage %.0f%% (%d/%d files)   Orphans: %d   Rogue: %d   Violations: %d   Gaps: %d   Stale: %d   Untested: %d   Proposed: %d\n",
		m.CoveragePct, m.CoveredFiles, m.InScopeFiles, m.Orphans, m.Rogues, m.Violations, m.Gaps, m.Stale, m.Untested, m.Proposed)
	return sb.String()
}

// RenderMarkdown renders the report as a GitHub-flavoured Markdown table.
func RenderMarkdown(r *Report, all bool) string {
	var sb strings.Builder
	sb.WriteString("### Ontology drift\n\n| | Derived from code | Declared | Result |\n| --- | --- | --- | --- |\n")
	for _, f := range r.Findings {
		if f.Kind == KindConsistent && !all {
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
	fmt.Fprintf(&sb, "\nCoverage %.0f%% · Orphans %d · Rogue %d · Violations %d · Gaps %d · Stale %d · Untested %d · Proposed %d\n",
		m.CoveragePct, m.Orphans, m.Rogues, m.Violations, m.Gaps, m.Stale, m.Untested, m.Proposed)
	return sb.String()
}
