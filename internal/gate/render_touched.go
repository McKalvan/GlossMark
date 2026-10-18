package gate

import (
	"fmt"
	"strings"
)

// RenderTouched renders the impact of changed files for a terminal.
func RenderTouched(ts []FileTouch) string {
	var sb strings.Builder
	for _, t := range ts {
		fmt.Fprintf(&sb, "%s  [%s", t.Path, t.Gate)
		if t.Boundary != "" {
			fmt.Fprintf(&sb, ", %s", t.Boundary)
		}
		sb.WriteString("]\n")
		for _, l := range t.Links {
			q := ""
			if l.Proposed {
				q = "?"
			}
			sym := ""
			if l.Symbol != "" {
				sym = " " + l.Symbol
			}
			fmt.Fprintf(&sb, "  @gloss%s %s %s  (line %d%s)\n", q, l.Relation, l.Target, l.Line, sym)
		}
		for _, c := range t.Concepts {
			fmt.Fprintf(&sb, "  %s (%s, %s)", c.Curie, c.Kind, c.Tier)
			var bits []string
			if c.Published != "" {
				bits = append(bits, "PUBLISHED as "+c.Published)
			}
			if len(c.PublishedBy) > 0 {
				bits = append(bits, "published by "+strings.Join(c.PublishedBy, ", "))
			}
			if len(c.Realizes) > 0 {
				bits = append(bits, "realizes "+strings.Join(c.Realizes, ", "))
			}
			if len(c.Constrains) > 0 {
				bits = append(bits, "constrains "+strings.Join(c.Constrains, ", "))
			}
			if len(c.ConstrainedBy) > 0 {
				bits = append(bits, "constrained by "+strings.Join(c.ConstrainedBy, ", "))
			}
			if c.Proposed {
				bits = append(bits, "PROPOSED")
			}
			if len(bits) > 0 {
				sb.WriteString(": " + strings.Join(bits, "; "))
			}
			sb.WriteString("\n")
			if len(c.Boundaries) > 0 {
				fmt.Fprintf(&sb, "      boundaries: %s\n", strings.Join(c.Boundaries, "; "))
			}
			if len(c.Tests) > 0 {
				fmt.Fprintf(&sb, "      tests: %s\n", strings.Join(c.Tests, ", "))
			}
			if len(c.OtherArtifacts) > 0 {
				fmt.Fprintf(&sb, "      also linked from: %s\n", strings.Join(c.OtherArtifacts, ", "))
			}
			for _, w := range c.Warnings {
				fmt.Fprintf(&sb, "      ! %s\n", w)
			}
		}
		for _, h := range t.Hints {
			fmt.Fprintf(&sb, "  → %s\n", h)
		}
	}
	return sb.String()
}
