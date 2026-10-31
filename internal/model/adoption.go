// @gloss implements gmi:AdoptionLadder scope=file

package model

import "fmt"

// Adoption levels (docs/QUICKSTART.md, issue #258). A repository adopts
// GlossMark one rung at a time, and each rung pays off on its own; the
// `adoption:` key in glossmark.yaml sets how hard check, drift and diff bite,
// so a repository that has only written a glossary is not failed for the
// manifests and markers it has not written yet. Unset means AdoptionFull: an
// existing repository changes nothing by upgrading.
const (
	AdoptionVocabulary  = 0 // a glossary; agents get the vocabulary, nothing is enforced
	AdoptionManifests   = 1 // one manifest per service, file-level links; drift warns only
	AdoptionCriticality = 2 // criticality on key concepts; the gate covers critical and high only
	AdoptionFull        = 3 // markers on declarations, value binding, imports; everything enforced
)

// AdoptionLevel describes one rung of the ladder: what a team writes to reach
// it and what the tooling gives back.
type AdoptionLevel struct {
	Level int
	Name  string
	// Write is what the level asks the team to write.
	Write string
	// Get is what the tooling gives at this level.
	Get string
	// Enforces is what fails a command at this level, in one phrase.
	Enforces string
}

// AdoptionLevels in order, index = level.
var AdoptionLevels = []AdoptionLevel{
	{Level: AdoptionVocabulary, Name: "vocabulary",
		Write:    "the core glossary (ontology/<core>.gloss.yaml) and glossmark.yaml",
		Get:      "agents get the vocabulary over MCP, gloss context and AGENTS.md; gloss check and drift report, nothing fails",
		Enforces: "nothing (check, drift and diff report and exit 0)"},
	{Level: AdoptionManifests, Name: "manifests",
		Write:    "one .gloss.yaml manifest per service directory and scope=file links in its entry points",
		Get:      "gloss drift reports violations and gaps per boundary (warn only), the viewer draws the map, gloss check fails on broken references",
		Enforces: "gloss check errors (drift and the PR gate still report only)"},
	{Level: AdoptionCriticality, Name: "criticality",
		Write:    "criticality: on the concepts and boundaries that matter",
		Get:      "the PR gate and drift fail for critical and high concepts only; everything else stays advisory",
		Enforces: "check errors, and drift findings and HARD changes on critical or high concepts"},
	{Level: AdoptionFull, Name: "full",
		Write:    "markers on individual declarations, value-from bindings, imports of sibling repositories",
		Get:      "full coverage numbers, published values watched for drift, the cross-repository seam; every finding and gate as --fail-on asks",
		Enforces: "everything --fail-on names"},
}

// AdoptionLevel returns the configured level, AdoptionFull when unset.
func (c *Config) AdoptionLevel() int {
	if c == nil || c.Adoption == nil {
		return AdoptionFull
	}
	return *c.Adoption
}

// AdoptionSet reports whether glossmark.yaml names a level.
func (c *Config) AdoptionSet() bool { return c != nil && c.Adoption != nil }

// ValidAdoption reports whether level is a rung of the ladder.
func ValidAdoption(level int) bool { return level >= AdoptionVocabulary && level <= AdoptionFull }

// AdoptionName is "level N (name)".
func AdoptionName(level int) string {
	if !ValidAdoption(level) {
		return fmt.Sprintf("level %d", level)
	}
	return fmt.Sprintf("level %d (%s)", level, AdoptionLevels[level].Name)
}

// AdoptionChecksFail reports whether gloss check's errors fail the command
// at this level: at level 0 nothing fails.
func AdoptionChecksFail(level int) bool { return level > AdoptionVocabulary }

// AdoptionDriftFails reports whether gloss drift findings can fail the
// command at this level (levels 0 and 1 warn only).
func AdoptionDriftFails(level int) bool { return level >= AdoptionCriticality }

// AdoptionGateFails reports whether gloss diff's gate can fail the command
// at this level (levels 0 and 1 report only).
func AdoptionGateFails(level int) bool { return level >= AdoptionCriticality }

// AdoptionMinCriticality is the lowest criticality a finding or change must
// carry to fail at this level: "high" at level 2, "" (any) at level 3. At a
// level where nothing fails it is irrelevant.
func AdoptionMinCriticality(level int) string {
	if level == AdoptionCriticality {
		return "high"
	}
	return ""
}

// AdoptionNote is the one-line reason a command gives when the level kept
// it from failing: "adoption level 1 (manifests): drift reports only; set
// adoption: 2 in glossmark.yaml to fail on check errors, and drift findings
// and HARD changes on critical or high concepts".
func AdoptionNote(level int, what string) string {
	if level >= AdoptionFull {
		return ""
	}
	next := AdoptionLevels[level+1]
	return fmt.Sprintf("adoption %s: %s; set adoption: %d in glossmark.yaml to fail on %s", AdoptionName(level), what, next.Level, next.Enforces)
}
