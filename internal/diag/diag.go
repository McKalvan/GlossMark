// @gloss implements gmi:Diagnostics scope=file

// Package diag is the one table of every GM diagnostic code that gloss emits:
// its severity, what it means, how to fix it and the page that explains the
// concept. gloss explain prints it, and docs/DIAGNOSTICS.md is generated from
// it. A test fails when a code is emitted without a row, when a row names a
// code nothing emits, and when docs/DIAGNOSTICS.md is out of date.
package diag

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Code is one row of the table.
type Code struct {
	Code string `json:"code"`
	// Severity is "error", "warning", or "error or warning" for a code emitted
	// at both; Meaning then says which applies when.
	Severity string `json:"severity"`
	Meaning  string `json:"meaning"`
	Fix      string `json:"fix"`
	// Doc is the page under docs/ that explains the concept, with an optional
	// #anchor.
	Doc string `json:"doc"`
}

// DocPath is Doc as a repository-relative path, for terminal output.
func (c Code) DocPath() string { return "docs/" + c.Doc }

// Range is a family of codes sharing a hundreds digit.
type Range struct {
	Prefix string `json:"prefix"` // "GM0" for GM0xx
	Title  string `json:"title"`
	Intro  string `json:"intro"`
}

// Ranges in code order. Every code's prefix has a range.
var Ranges = []Range{
	{"GM0", "Marker grammar", "The scanner reports what it cannot read in a marker. These are warnings, never silent drops: GM001 skips the whole marker; the others keep the link and drop or flag one part of it."},
	{"GM1", "Workspace, imports and targets", "Loading `glossmark.yaml`, owned and imported ontology files, `glossmark.lock` and manifests (GM100–GM112); resolving marker targets (GM120–GM122); and markers that state the same fact twice (GM130–GM131)."},
	{"GM2", "Relations and references", "Each edge, from a marker, an ontology file or a manifest, is checked for a known relation written in the right place, a target that exists, the right types at both ends, and the published seam."},
	{"GM3", "Ontology entries", "Rules for concepts in owned ontology files: kind, naming, criticality, YAML shape, tiers, the published seam, deprecation, proposals and `value-from`."},
	{"GM4", "Own-subject rule", "A repository only adds statements whose subject is a concept it owns: it extends other namespaces, never mutates them."},
	{"GM5", "Manifests and allowed lists", "A boundary manifest's `default-namespace` and `allowed:` list, and the links under it that fall outside its allowed set."},
	{"GM6", "Governance", "Criticality declared on boundaries."},
	{"GM9", "Gate", "Changes the PR gate classifies HARD."},
}

// Codes is the table, in code order.
var Codes = []Code{
	// GM0xx: marker grammar (internal/marker).
	{"GM001", "warning", "A comment contains `@gloss` but not a marker the grammar can parse, so no link is made.", "Rewrite it as `@gloss[?] <relation> <target>[, <target>...] [key=value ...]`; to mention a marker in prose, put it in backticks.", "GRAMMAR.md#rules"},
	{"GM002", "warning", "A marker carries a key other than `scope`, `conf`, `id` or `note`; the key is ignored.", "Remove the key; provenance such as author and date lives in the ledger, and free text goes in `note=\"...\"`.", "ADVANCED.md#marker-keys"},
	{"GM003", "warning", "A marker repeats a key; the last value wins.", "Keep one `key=value` per key.", "ADVANCED.md#marker-keys"},
	{"GM004", "warning", "Text follows the marker that is neither a `key=value` pair nor a comment closer; the marker itself still parses.", "Move the text to `note=\"...\"` (quote values that contain spaces) or to its own comment line.", "GRAMMAR.md#rules"},
	{"GM005", "warning", "`scope=` is not one of file, symbol, block, line or range; the key is dropped and default anchoring applies.", "Write `scope=file`, `scope=symbol`, `scope=block`, `scope=line` or `scope=range`.", "ADVANCED.md#scopes"},
	{"GM006", "warning", "`conf=` is not a number between 0 and 1; the key is dropped.", "Write the confidence as a decimal in [0,1], such as `conf=0.8`.", "ADVANCED.md#marker-keys"},
	{"GM007", "warning", "A marker target contains `*` on a relation other than `may-modify`.", "Name concrete concepts; globs such as `pa:*` only express a `may-modify` remit.", "ADVANCED.md#every-relation"},

	// GM1xx: workspace loading and imports (internal/load).
	{"GM100", "error", "The ontology file of a namespace listed under `owns:` does not exist, so the namespace loads empty.", "Create the file at `owns.<prefix>.file` (default `ontology/<prefix>.gloss.yaml`) or correct that path in `glossmark.yaml`.", "FILE-FORMATS.md#glossmarkyaml-repository-root"},
	{"GM101", "warning", "An import's `source:` is a git or https URL, which is not fetched yet; the vendored copy `ontology/<prefix>.gloss.yaml` is used instead.", "Set `source:` to the vendored file's path, and refresh that copy when you upgrade the import.", "FILE-FORMATS.md#glossmarkyaml-repository-root"},
	{"GM102", "error", "An import's `source:` is a git or https URL and no vendored copy exists, so the namespace loads empty.", "Vendor the upstream ontology file, or point at a sibling checkout, and set `source:` to that local path.", "FILE-FORMATS.md#glossmarkyaml-repository-root"},
	{"GM103", "error", "An import's `source:` path does not exist, or it is a directory with no `<prefix>.gloss.yaml` in it or in its `ontology/`.", "Correct `source:` under `imports:` in `glossmark.yaml` to the upstream ontology file or its directory.", "FILE-FORMATS.md#glossmarkyaml-repository-root"},
	{"GM104", "error", "An imported ontology file exists but cannot be parsed; the message carries the parse error.", "Fix the YAML in the upstream file, or refresh the vendored copy from the upstream.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM105", "warning", "An import states a version range, but the imported ontology declares no `version:` or a non-semver one, so the range cannot be checked.", "Ask the upstream to set `version: MAJOR.MINOR.PATCH` in its ontology file, or drop the range from `imports:`.", "VERSIONING.md#namespaces"},
	{"GM106", "error", "The resolved version of an imported ontology is outside the range declared under `imports:`.", "Point `source:` at a version inside the range, or review the upstream changes, widen the range and run `gloss lock`.", "VERSIONING.md#namespaces"},
	{"GM107", "error or warning", "`glossmark.lock` disagrees with the resolved imports: a different version, or changed content under the same version (error); an import missing from the lock, or a pin for an import that is gone (warning).", "Review the upstream change, then run `gloss lock` and commit `glossmark.lock`.", "VERSIONING.md#namespaces"},
	{"GM108", "warning", "The repository imports namespaces but has no `glossmark.lock`.", "Run `gloss lock` and commit `glossmark.lock`.", "VERSIONING.md#namespaces"},
	{"GM109", "error", "`glossmark.lock` cannot be parsed, or its `format:` was written by a newer gloss.", "Upgrade gloss when the format is newer; otherwise delete the broken lock and run `gloss lock`.", "VERSIONING.md#file-formats"},

	// GM11x: manifests and boundaries (internal/load, internal/extract).
	{"GM110", "error", "A `.gloss.yaml` manifest cannot be loaded: invalid YAML, no `boundary:`, or a `level:` or `criticality:` outside the allowed values.", "Fix the manifest as the message says: levels are domain, system, container and component; criticalities are low, medium, high and critical.", "FILE-FORMATS.md#glossyaml-boundary-manifest-any-directory"},
	{"GM111", "error", "A manifest's `boundary:` or `part-of:` value has no prefix.", "Write the boundary as `prefix:name`, such as `acme:payments-api`.", "FILE-FORMATS.md#glossyaml-boundary-manifest-any-directory"},
	{"GM112", "warning", "A boundary's prefix is not declared under `prefixes:` and `glossmark.yaml` has no `repo-iri:`, so it expands to a placeholder `urn:gloss:boundary:` IRI.", "Declare the prefix under `prefixes:` in `glossmark.yaml`, or set `repo-iri:`.", "FILE-FORMATS.md#glossmarkyaml-repository-root"},

	// GM12x: marker target resolution and anchoring (internal/extract).
	{"GM120", "error", "A `.Local` target is used where no enclosing manifest sets `default-namespace:`; GM211 follows for the same target.", "Write the full `prefix:Name`, or add `default-namespace:` to the enclosing `.gloss.yaml`.", "GRAMMAR.md"},
	{"GM121", "warning", "A sidecar file (`foo.json.gloss`) annotates a file that does not exist.", "Move or rename the sidecar with its file, or delete it with the file.", "GRAMMAR.md#rules"},
	{"GM122", "warning", "A marker says `scope=symbol`, but no declaration was detected below it; it is anchored to the next block instead.", "Put the marker directly above the declaration and its decorators, or use `scope=block` or `scope=file`.", "ADVANCED.md#scopes"},

	// GM13x: redundant markers (internal/check).
	{"GM130", "warning", "A symbol-level marker repeats the relation and target of a file-level marker in the same file: the same fact, two edges.", "Keep the file marker or the symbol markers, not both.", "MARKING.md#a-file-marker-or-symbol-markers-not-both"},
	{"GM131", "warning", "The same symbol-level marker is on three or more consecutive declarations.", "Replace the run with one `scope=block` marker above it, or one marker on the public entry point that calls them.", "MARKING.md#one-marker-per-public-surface"},

	// GM2xx: relations and references (internal/check).
	{"GM200", "error", "A marker, ontology entry or manifest uses a relation that is not in the vocabulary.", "Use a relation from the relation table, or declare the new one under `relations:` in an ontology file.", "ADVANCED.md#every-relation"},
	{"GM201", "error", "An inline marker uses a relation that only ontology files or manifests may state, such as `realizes`, `is-a` or `part-of`.", "Use an inline relation (`implements`, `represents`, `verifies`, ...), or move the statement to the ontology file or manifest.", "ADVANCED.md#every-relation"},
	{"GM202", "error", "An ontology concept declares a relation that concepts may not carry, such as `implements` or `holds`.", "Remove the key, or state the link where that relation belongs: an inline marker or a manifest.", "ADVANCED.md#every-relation"},
	{"GM203", "error", "A manifest declares a relation the vocabulary does not allow in manifests, which happens when an ontology's `relations:` overrides a built-in relation.", "Move the statement to an ontology file or an inline marker, or revisit the `relations:` override.", "ADVANCED.md#every-relation"},
	{"GM210", "error", "A target's prefix is not an owned, imported or declared namespace.", "Fix the prefix, or declare it under `owns:`, `imports:` or `prefixes:` in `glossmark.yaml`.", "FILE-FORMATS.md#glossmarkyaml-repository-root"},
	{"GM211", "error", "A target has no `prefix:` and could not be resolved (a `.Local` target without a default namespace also reports GM120).", "Write the target as `prefix:Name`, or as `.Name` under a manifest that sets `default-namespace:`.", "GRAMMAR.md"},
	{"GM212", "error", "A `ns:NEW:Name` target is written in an accepted `@gloss` marker; only a proposal may name a concept that does not exist.", "Write the marker as `@gloss?`, then run `gloss propose --from-new --doc \"<one sentence>\"`.", "MARKING.md#proposals"},
	{"GM213", "error", "A `ns:NEW:Name` proposal targets a namespace this repository does not own.", "Propose the concept in a namespace you own, or ask the upstream owner to add it and link to it once it exists.", "MARKING.md#proposals"},
	{"GM214", "error", "A glob target (`ns:*`) is used with a relation other than `may-modify`.", "Name concrete concepts; globs only express a `may-modify` remit.", "ADVANCED.md#every-relation"},
	{"GM215", "error", "A target names a concept that does not exist in its namespace: a dangling reference.", "Correct the name (MCP `search_concepts` finds it), or propose it with `@gloss? <relation> <ns>:NEW:<Name>` and `gloss propose --from-new`.", "ADVANCED.md#finding-kinds-gloss-drift"},
	{"GM220", "error", "A relation connects the wrong types of node, such as a marker that `implements` a boundary instead of a concept.", "Point the relation at a node of the type the message expects, or use the relation that fits the target.", "ADVANCED.md#every-relation"},
	{"GM221", "warning", "An inline relation targets a concept of a kind it does not usually take, such as `implements` on an entity.", "Use the relation that fits the kind (`represents` for entities and resources, `implements` for processes and capabilities), or a better target.", "ADVANCED.md#concept-kinds"},
	{"GM222", "warning", "A link targets a deprecated concept.", "Retarget it to the concept the message names from `replaced-by:`.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM230", "error", "A link targets an imported implementation concept that is not published; only published concepts cross repositories.", "Link to the core concept instead, or ask the upstream to publish the concept with `published: { as: <core> }`.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM240", "warning", "A marker uses `tests`, the accepted alias of `verifies`.", "Write `verifies`.", "ADVANCED.md#every-relation"},

	// GM3xx: ontology entries (internal/check, internal/extract).
	{"GM300", "error", "A concept has no `kind:` and no `is-a` to inherit one from.", "Add `kind:` (entity, process, capability, event, measure, constraint, resource or system-type) or `is-a: <parent>`.", "ADVANCED.md#concept-kinds"},
	{"GM301", "error", "A concept's `kind:` is not one of the eight concept kinds.", "Use entity, process, capability, event, measure, constraint, resource or system-type.", "ADVANCED.md#concept-kinds"},
	{"GM302", "warning", "A concept name is not PascalCase: it starts lower-case or contains `_`, `-` or a space.", "Rename it in PascalCase; for a concept already in use, add the new name and deprecate the old one with `replaced-by:`.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM303", "error", "A concept's `criticality:` is not low, medium, high or critical.", "Use one of the four levels, or remove the key so the concept inherits its level.", "GOVERNANCE.md#one-attribute-four-levels"},
	{"GM304", "warning", "An ontology entry has a key with no value, so it adds no edge.", "Give the key its targets, or remove it.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM305", "error", "An unquoted `doc:` inside a `{ }` flow mapping was cut at a comma; the rest of the sentence became null keys.", "Quote the doc (`doc: \"...\"`) or write the entry in block style.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM310", "error", "A core concept references an implementation concept, by `is-a` or any other declared relation; core must not depend on implementation.", "Remove the reference from the core entry and state it on the implementation side, such as `realizes:` on the implementation concept.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM311", "error", "A core concept carries `published:`, which only applies to implementation concepts.", "Remove `published:` from the core concept; publish an implementation concept `as:` it instead.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM312", "error", "A core concept declares `realizes` or `published-as`; the dependency runs from implementation to core.", "Remove it from the core concept and declare `realizes:` on the implementation concept.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM320", "error or warning", "An implementation concept stands for nothing: no `realizes:`, `presents:` or `hosts:`, no `is-a` or `part-of` a concept that realizes, and not `internal: true` (a warning while the concept is `proposed: true`, an error once accepted).", "Add `realizes: <core concept>`, or `internal: true` for an engineering-only mechanism.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM321", "error", "An implementation concept `realizes:` a concept that is not core.", "Point `realizes:` at a core concept; relate implementation concepts to each other with `part-of` or `is-a`.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM330", "error", "`published: { as: X }` names a core concept that does not exist; publishing never creates a core name.", "Name an existing core concept, or get the core concept accepted first.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM331", "error", "`published: { as: X }` names a concept that is not core.", "Publish the concept as a core concept.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM332", "error", "A concept is both `internal: true` and `published:`.", "Remove one of them: internal concepts never cross repositories.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM340", "warning", "A deprecated concept has no `replaced-by:`.", "Add `replaced-by: <concept>` so links to it can be retargeted.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM341", "error", "`replaced-by:` names a concept that does not exist.", "Point it at an existing concept, or add that concept first.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM350", "error", "An imported ontology contains a concept marked `proposed: true`; proposals live only in the repository that owns the namespace.", "Ask the upstream owner to accept or remove the proposal and release a new version, then refresh the import and run `gloss lock`.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM360", "error", "A concept's `value-from:` names a file, line or symbol that does not exist, or gives neither `symbol:` nor `line:`.", "Correct `value-from: { path, symbol }` or `{ path, line }` so it points at the line that holds the value.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},

	// GM4xx: the own-subject rule (internal/check).
	{"GM400", "error", "An owned concept declares `has-part` on a concept this repository does not own, which would make the imported concept the subject of a statement.", "State the inverse on your own concept: `<yours> part-of <theirs>`.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},
	{"GM401", "error", "An owned ontology file defines a concept under another namespace's prefix.", "Define it in its own namespace's file, or extend it with a concept of yours that `is-a` it.", "FILE-FORMATS.md#glossyaml-one-per-namespace"},

	// GM5xx: manifests and allowed lists (internal/check).
	{"GM500", "error", "A manifest's `default-namespace:` is not an owned or imported namespace.", "Set it to a prefix under `owns:` or `imports:` in `glossmark.yaml`.", "FILE-FORMATS.md#glossyaml-boundary-manifest-any-directory"},
	{"GM501", "warning", "A manifest's `allowed:` lists a concept that does not exist.", "Correct or remove the entry; `derived` and globs such as `ns:*` are always accepted.", "FILE-FORMATS.md#glossyaml-boundary-manifest-any-directory"},
	{"GM502", "error or warning", "An inline link targets a concept outside the manifest's written `allowed:` list, which is HARD in the gate (a warning on a `@gloss?` proposal, an error on an accepted link).", "Retarget the link to an allowed concept; if the boundary really uses this one, add it to `allowed:` and say it is a HARD manifest edit.", "FILE-FORMATS.md#glossyaml-boundary-manifest-any-directory"},
	{"GM503", "warning", "An inline link targets a concept outside the allowed set derived from what the manifest declares, which is HARD in the gate.", "Retarget the link, or declare the concept in the manifest, or write `allowed: [derived, <concept>]`; `gloss check --explain` prints the set.", "FILE-FORMATS.md#glossyaml-boundary-manifest-any-directory"},

	// GM6xx: governance (internal/check).
	{"GM600", "warning", "A boundary declares a `criticality:` below that of a concept its code realizes; reviews use the higher level.", "Raise the manifest's `criticality:` to the concept's level, or remove it so the boundary inherits.", "GOVERNANCE.md#inheritance"},

	// GM9xx: the gate (internal/gate).
	{"GM900", "error", "A change the PR gate classifies HARD, rendered as a diagnostic with its reasons; no command prints this code yet, and `gloss diff` reports HARD changes in its own output.", "Have a human review the change in the PR; `gloss diff --base <ref>` lists each HARD change with its reasons.", "ADVANCED.md#gates-gloss-diff"},
}

// Lookup finds a code, ignoring case; "305" means GM305.
func Lookup(code string) (Code, bool) {
	code = Normalize(code)
	for _, c := range Codes {
		if c.Code == code {
			return c, true
		}
	}
	return Code{}, false
}

// Family returns the codes a pattern such as GM12x or GM3xx names, in order.
// It returns nil for anything that is not such a pattern.
func Family(pattern string) []Code {
	p := Normalize(pattern)
	stem := strings.TrimRight(p, "X")
	if stem == p || len(p) != 5 || len(stem) < 3 {
		return nil
	}
	var out []Code
	for _, c := range Codes {
		if strings.HasPrefix(c.Code, stem) {
			out = append(out, c)
		}
	}
	return out
}

// Normalize upper-cases a code and adds the GM prefix when it is missing.
func Normalize(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !strings.HasPrefix(code, "GM") {
		code = "GM" + code
	}
	return code
}

// RangeOf returns the range a code belongs to.
func RangeOf(code string) Range {
	for _, r := range Ranges {
		if strings.HasPrefix(code, r.Prefix) {
			return r
		}
	}
	return Range{}
}

// Text renders codes for the terminal, one block each.
func Text(cs []Code) string {
	var sb strings.Builder
	for i, c := range cs {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "%s  %s  (%s)\n", c.Code, c.Severity, RangeOf(c.Code).Title)
		fmt.Fprintf(&sb, "  Meaning: %s\n", c.Meaning)
		fmt.Fprintf(&sb, "  Fix:     %s\n", c.Fix)
		fmt.Fprintf(&sb, "  Docs:    %s\n", c.DocPath())
	}
	return sb.String()
}

// JSON renders codes as an indented JSON array.
func JSON(cs []Code) string {
	b, _ := json.MarshalIndent(cs, "", "  ")
	return string(b) + "\n"
}

// Table renders codes as one Markdown table, with doc links relative to docs/.
func Table(cs []Code) string {
	var sb strings.Builder
	sb.WriteString("| Code | Severity | Meaning | Fix | Docs |\n| --- | --- | --- | --- | --- |\n")
	for _, c := range cs {
		page := c.Doc
		if i := strings.Index(page, "#"); i >= 0 {
			page = page[:i]
		}
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | [%s](%s) |\n", c.Code, c.Severity, cell(c.Meaning), cell(c.Fix), page, c.Doc)
	}
	return sb.String()
}

func cell(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

// RegenerateCommand is how docs/DIAGNOSTICS.md is written from the table.
const RegenerateCommand = "go run ./cmd/gloss explain --all --format markdown > docs/DIAGNOSTICS.md"

// Markdown renders docs/DIAGNOSTICS.md: a header, then one table per range.
func Markdown() string {
	var sb strings.Builder
	// Split so this source line is not itself read as a marker.
	sb.WriteString("<!-- @" + "gloss documents gm:Lint scope=file -->\n")
	sb.WriteString("<!-- Generated from internal/diag by `gloss explain --all --format markdown`; do not edit by hand. Regenerate: " + RegenerateCommand + " -->\n")
	sb.WriteString("# Diagnostics\n\n")
	sb.WriteString("`gloss check` reports problems as `path:line[:col]: severity: message [GMnnn]`; `gloss build` prints the ones found while loading and scanning (marker grammar, imports, manifests, targets, `value-from`). An error fails `gloss check`; a warning does not, unless you pass `--strict`. `gloss explain GMnnn` prints one row of this page in the terminal, `gloss explain GM3xx` a whole range, and `gloss explain --all --format json` the table for tools.\n\n")
	sb.WriteString("Every code `gloss` emits has exactly one row here, and a test fails when a code is added without one. Fixes are written for whoever hits the code, human or agent; where a fix edits an ontology file or a manifest, that edit is HARD in the PR gate.\n\n")
	groups := map[string][]Code{}
	for _, c := range Codes {
		r := RangeOf(c.Code)
		groups[r.Prefix] = append(groups[r.Prefix], c)
	}
	for _, r := range Ranges {
		cs := groups[r.Prefix]
		if len(cs) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "## %sxx: %s\n\n%s\n\n%s\n", r.Prefix, r.Title, r.Intro, Table(cs))
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}
