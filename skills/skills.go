// @gloss implements gmi:AdoptionSkill scope=file
// @gloss implements gmi:IncrementCurator scope=file

// Package skills ships the Claude Code skills that come with GlossMark, so
// gloss hook install --skill can install them without a network fetch.
package skills

import (
	"embed"
	"strings"
)

// FS holds every skill directory (adopt-glossmark/SKILL.md, ...).
//
//go:embed adopt-glossmark/SKILL.md derive-ontology/SKILL.md review-glossmark/SKILL.md curate-increment/SKILL.md
var FS embed.FS

// Names lists the shipped skills.
var Names = []string{"adopt-glossmark", "derive-ontology", "review-glossmark", "curate-increment"}

// Installable returns a skill's SKILL.md as it is installed into another
// repository: without GlossMark's own self-hosting markers, which name the
// gm: namespace that repository does not have (GM210 there).
func Installable(name string) ([]byte, error) {
	b, err := FS.ReadFile(name + "/SKILL.md")
	if err != nil {
		return nil, err
	}
	lines := strings.SplitAfter(string(b), "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "<!-- @gloss") && strings.HasSuffix(t, "-->") {
			// Drop the blank line after it too, so no gap is left behind.
			if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "" {
				i++
			}
			continue
		}
		out = append(out, lines[i])
	}
	return []byte(strings.Join(out, "")), nil
}
