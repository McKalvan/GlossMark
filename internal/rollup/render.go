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

// kindHint is the next step under each heading of gloss drift's text
// output; docs/GLOSSARY.md has the long form.
var kindHint = map[string]string{
	KindViolation:     "Widen the manifest or move the code; gloss review to waive.",
	KindValueDrift:    "Update value: in the ontology to match the code, or confirm the code with gloss accept --value.",
	KindOverdue:       "Move the remaining code to the replacement, or extend retire-by: in the manifest.",
	KindPlanOverdue:   "Implement it, or move the plan's by: date.",
	KindOutOfRange:    "Widen the import's version range, or lock a version inside it.",
	KindGap:           "Implement it, or remove the declaration.",
	KindPlannedClosed: "The plan is done: drop planned (gloss accept --planned), or move the manifest entry out of planned:.",
	KindRogue:         "Add the concept with gloss propose --from-new, or rename the marker to an existing concept.",
	KindProposed:      "Accept with gloss accept --concept, or reject it in gloss review.",
	KindPlanned:       "Nothing to do yet: the item closes when code implements it.",
	KindDangling:      "Fix the name, or define the concept in the ontology.",
	KindStale:         "Check that the link still holds, then re-confirm it with gloss accept or gloss review.",
	KindOrphan:        "Link code to it with a marker, or delete the concept.",
	KindPresentedOnly: "Mark the code that implements it, or remove the presents link.",
	KindUnguarded:     "Add the guard decorator, or move the handler out of the boundary.",
	KindUntested:      "Add a verifies marker on a test that exercises it.",
	KindDeprecatedUse: "Move to the replacement (replaced-by), then remove the reference.",
	KindOverlap:       "If one replaces the other, mark the old manifest deprecated: true with replaced-by: and retire-by:.",
}

// kindNoun names a kind in a count, singular and plural: "2 violations".
var kindNoun = map[string][2]string{
	KindViolation:     {"violation", "violations"},
	KindValueDrift:    {"drifted value", "drifted values"},
	KindGap:           {"gap", "gaps"},
	KindDangling:      {"dangling reference", "dangling references"},
	KindOverdue:       {"overdue retirement", "overdue retirements"},
	KindPlannedClosed: {"closed plan item", "closed plan items"},
	KindPlanOverdue:   {"overdue plan item", "overdue plan items"},
	KindOutOfRange:    {"import out of range", "imports out of range"},
	KindDeprecatedUse: {"deprecated use", "deprecated uses"},
	KindRogue:         {"rogue concept", "rogue concepts"},
	KindOrphan:        {"orphan", "orphans"},
	KindUntested:      {"untested concept", "untested concepts"},
	KindPresentedOnly: {"presented-only concept", "presented-only concepts"},
	KindUnguarded:     {"unguarded handler", "unguarded handlers"},
	KindOverlap:       {"overlap", "overlaps"},
	KindStale:         {"stale link", "stale links"},
	KindProposed:      {"proposed concept", "proposed concepts"},
	KindPlanned:       {"plan item", "plan items"},
}

// GlossaryURL is where GlossMark's own terms are defined: finding kinds,
// gates, tiers, placement, edge provenance and review states.
const GlossaryURL = "https://github.com/McKalvan/GlossMark/blob/main/docs/GLOSSARY.md"

// FailSummary counts the failing findings by kind, in report order, and
// names the kinds that fail the command: "failed: 2 violations, 1 gap
// (fail-on: violation, gap)".
func FailSummary(failing []Finding, failOn []string) string {
	n := map[string]int{}
	for _, f := range failing {
		n[f.Kind]++
	}
	var parts []string
	for _, k := range kindOrder {
		switch {
		case n[k] == 1:
			parts = append(parts, "1 "+kindNoun[k][0])
		case n[k] > 1:
			parts = append(parts, fmt.Sprintf("%d %s", n[k], kindNoun[k][1]))
		}
	}
	return fmt.Sprintf("failed: %s (fail-on: %s)", strings.Join(parts, ", "), strings.Join(failOn, ", "))
}

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
	// Guide puts a next step under each heading and a link to the glossary
	// at the end, and writes each finding as consequence and action (gloss
	// drift). Without it, as with Explain, a finding prints its precise
	// message, set notation included, the one JSON carries.
	Guide   bool
	Explain bool
	// Manifests maps a boundary's CURIE to its manifest file, which the
	// guided messages name.
	Manifests map[string]string
	// Color marks headings by severity and dims hints and locations.
	Color bool
}

// ANSI styles for RenderTextOpts with Color.
const (
	ansiRed   = "\x1b[1;31m"
	ansiAmber = "\x1b[1;33m"
	ansiDim   = "\x1b[2m"
	ansiReset = "\x1b[0m"
)

// paint wraps s in style when on.
func paint(on bool, style, s string) string {
	if !on || s == "" {
		return s
	}
	return style + s + ansiReset
}

// severityStyle colours a heading: errors red, warnings amber, the rest dim.
func severityStyle(sev string) string {
	switch sev {
	case "error":
		return ansiRed
	case "warning":
		return ansiAmber
	}
	return ansiDim
}

// RenderTextOpts renders the report for a terminal.
func RenderTextOpts(r *Report, opt TextOptions) string {
	all, col := opt.All, opt.Color
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
		fmt.Fprintf(&sb, "\n%s\n", paint(col, severityStyle(fs[0].Severity), kindGlyph[k]+" "+kindTitle[k]))
		if opt.Guide && kindHint[k] != "" {
			fmt.Fprintf(&sb, "  %s\n", paint(col, ansiDim, "→ "+kindHint[k]))
		}
		for _, f := range fs {
			loc := ""
			if f.Path != "" {
				loc = f.Path
				if f.Line > 0 {
					loc += fmt.Sprintf(":%d", f.Line)
				}
				loc = "  " + paint(col, ansiDim, "["+loc+"]")
			}
			w := ""
			if f.Waived {
				w = "  (waived: " + f.Waiver + ")"
			}
			id := ""
			if opt.IDs && f.ID != "" {
				id = "  " + paint(col, ansiDim, "(id "+f.ID+")")
			}
			msg := f.Msg
			if opt.Guide && !opt.Explain {
				msg = plainMsg(f, opt.Manifests)
			}
			fmt.Fprintf(&sb, "  %s%s%s%s\n", msg, loc, w, id)
		}
	}
	if all {
		fmt.Fprintf(&sb, "\n%s\n", paint(col, ansiDim, "✓ Consistent"))
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
	if opt.Guide {
		sb.WriteString(paint(col, ansiDim, "What these mean: "+GlossaryURL+" (--explain prints the declared and derived sets)") + "\n")
	}
	return sb.String()
}

// gapUnimplemented ends the message of a gap no code and no intent reaches,
// the one gap the guided text rewrites; the others already name the fix.
const gapUnimplemented = ": intended capability not implemented, or the declaration is wrong"

// plainMsg is a finding's message in gloss drift's guided text: what
// happened and what to do, without set notation, the heading's noun or the
// location the line ends with anyway. Kinds without a plain form keep
// their message.
func plainMsg(f Finding, manifests map[string]string) string {
	msg, waiver := f.Msg, ""
	if i := strings.Index(msg, " [waiver by "); i >= 0 {
		msg, waiver = msg[:i], msg[i:]
	}
	if f.Path != "" {
		msg = strings.TrimPrefix(msg, fmt.Sprintf("%s:%d ", f.Path, f.Line))
		msg = strings.TrimPrefix(msg, f.Path+": ")
	}
	switch f.Kind {
	case KindViolation:
		msg = plainViolation(f, manifestFile(f, manifests))
	case KindGap:
		if strings.HasSuffix(msg, gapUnimplemented) {
			msg = fmt.Sprintf("%s declares %s %s, but no code under it %s it. Implement it, or remove it from %s: in %s.", f.Boundary, f.Relation, f.Concept, verb(f.Relation), f.Relation, manifestFile(f, manifests))
		}
	case KindOrphan:
		msg = strings.Replace(msg, ": orphan concept, implementation concept has no artifacts", ": no code implements this implementation concept", 1)
		msg = strings.Replace(msg, ": orphan concept, ", ": ", 1)
	case KindRogue:
		msg = strings.Replace(msg, ": rogue concept; becomes a decision request", ": not in the ontology; its namespace owner decides (see Decision requests)", 1)
	case KindPresentedOnly:
		msg = strings.TrimSuffix(msg, ": a surface with no mechanism behind it")
	case KindValueDrift:
		msg = strings.TrimPrefix(msg, "published value drifted: ")
	case KindStale:
		msg = strings.Replace(msg, " is stale, the anchored signature changed since it was accepted; re-confirm with ", ": the code changed since the link was accepted; if it still holds, re-confirm with ", 1)
	case KindUntested:
		msg = strings.Replace(msg, " is implemented but nothing tests", ": nothing tests", 1)
		msg = strings.Replace(msg, " has code but nothing tests", ": has code, but nothing tests", 1)
	case KindPlanned, KindPlanOverdue:
		msg = strings.TrimPrefix(msg, "planned concept ")
	}
	return msg + waiver
}

// plainViolation writes a violation as what the boundary now does that its
// manifest does not allow, and the two ways out.
func plainViolation(f Finding, file string) string {
	proposed := ""
	if f.Proposed {
		proposed = " (proposed link)"
	}
	// A concept the code implements intends the flow; the code does not.
	if !has(f.Derived, f.Concept) && has(f.Intended, f.Concept) {
		return fmt.Sprintf("a concept %s's code implements %s %s%s, which its manifest does not allow. Add it, or a refinement of it, to %s: in %s, or change the concept.", f.Boundary, f.Relation, f.Concept, proposed, f.Relation, file)
	}
	it := "it"
	if strings.Contains(f.Concept, ", ") {
		it = "them"
	}
	lifted := ""
	switch {
	case len(f.Lifted) > 3:
		lifted = fmt.Sprintf(" (and through it %s and %d more)", strings.Join(f.Lifted[:3], ", "), len(f.Lifted)-3)
	case len(f.Lifted) > 0:
		lifted = " (and through it " + strings.Join(f.Lifted, ", ") + ")"
	}
	return fmt.Sprintf("%s now %s %s%s%s, which its manifest does not allow. Add %s to %s: in %s, or move the code.", f.Boundary, verb(f.Relation), f.Concept, lifted, proposed, it, f.Relation, file)
}

// manifestFile names the manifest of a finding's boundary for a message.
func manifestFile(f Finding, manifests map[string]string) string {
	if p := manifests[f.Boundary]; p != "" {
		return p
	}
	return "its manifest"
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
