// @gloss implements gmi:ViewProjector scope=file

package slice

import (
	"fmt"
	"strings"
)

// Render prints a slice grouped by role: text, or Markdown when md is set.
// Each node shows its kind, tier and criticality, the path that brought it
// in, and its drift findings; the summary says what in the slice is untested.
func Render(s *Slice, md bool) string {
	var b strings.Builder
	ctx := "context on"
	if !s.Context {
		ctx = "context off"
	}
	if len(s.Follow) > 0 {
		ctx += ", following " + strings.Join(s.Follow, ", ")
	}
	sum := s.Summary
	head := fmt.Sprintf("Slice of %s (%s)", strings.Join(s.Seeds, ", "), ctx)
	line := fmt.Sprintf("%d members: %d concepts, %d artifacts, %d tests · %d boundaries · %d context", sum.Members, sum.Concepts, sum.Artifacts, sum.Tests, sum.Boundaries, sum.Context)
	if sum.Criticality != "" {
		line += " · highest criticality " + sum.Criticality
	}
	if md {
		fmt.Fprintf(&b, "### %s\n\n%s\n", head, line)
	} else {
		fmt.Fprintf(&b, "%s\n  %s\n", head, line)
	}
	for _, role := range Roles {
		var group []Node
		for _, n := range s.Nodes {
			if n.Role == role {
				group = append(group, n)
			}
		}
		if len(group) == 0 {
			continue
		}
		if md {
			fmt.Fprintf(&b, "\n**%s** (%d)\n\n", role, len(group))
		} else {
			fmt.Fprintf(&b, "\n%s (%d)\n", role, len(group))
		}
		for _, n := range group {
			var attrs []string
			for _, a := range []string{n.Kind, n.Tier, n.Criticality} {
				if a != "" {
					attrs = append(attrs, a)
				}
			}
			if n.Internal {
				attrs = append(attrs, "internal")
			}
			if n.Imported {
				attrs = append(attrs, "imported")
			}
			loc := ""
			if n.Path != "" {
				loc = n.Path
				if n.Line > 0 {
					loc += fmt.Sprintf(":%d", n.Line)
				}
			}
			title := n.ID
			if md {
				title = "`" + n.ID + "`"
			}
			if len(attrs) > 0 {
				title += "  " + strings.Join(attrs, " · ")
			}
			if loc != "" && loc != n.ID {
				title += "  " + loc
			}
			if md {
				fmt.Fprintf(&b, "- %s\n", title)
			} else {
				fmt.Fprintf(&b, "  %s\n", title)
			}
			ind := "      "
			if md {
				ind = "  - "
			}
			if len(n.Via) > 0 {
				fmt.Fprintf(&b, "%svia %s\n", ind, strings.Join(n.Via, " · "))
			}
			for _, f := range n.Findings {
				fmt.Fprintf(&b, "%s%s\n", ind, f)
			}
		}
	}
	if md {
		b.WriteString("\n")
	}
	if len(sum.Untested) > 0 {
		fmt.Fprintf(&b, "\nUntested in this slice: %s\n", strings.Join(sum.Untested, ", "))
	} else {
		b.WriteString("\nNothing in this slice is reported untested.\n")
	}
	return b.String()
}
