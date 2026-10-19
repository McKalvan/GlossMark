// @gloss implements gmi:AdoptionSkill scope=file

// Package skills ships the Claude Code skills that come with GlossMark, so
// gloss hook install --skill can install them without a network fetch. The
// same files are the skills of the GlossMark Claude Code plugin, which finds
// them under skills/ at the repository root.
package skills

import "embed"

// FS holds every skill directory (adopt-glossmark/SKILL.md, ...).
//
//go:embed */SKILL.md
var FS embed.FS

// Names lists the shipped skills: adoption first, then the day-2 workflows
// in the order an adopter meets them. The contract test checks that it names
// every directory under skills/ and nothing else.
var Names = []string{
	"adopt-glossmark",
	"gloss-link",
	"gloss-resolve-drift",
	"gloss-review",
	"gloss-criticality",
	"gloss-upgrade-import",
}
