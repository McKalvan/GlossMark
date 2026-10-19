// Package skills ships the Claude Code skills that come with GlossMark, so
// gloss hook install --skill can install them without a network fetch.
package skills

import "embed"

// FS holds every skill directory (adopt-glossmark/SKILL.md, ...).
//
//go:embed adopt-glossmark/SKILL.md
var FS embed.FS

// Names lists the shipped skills.
var Names = []string{"adopt-glossmark"}
