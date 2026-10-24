<!-- @gloss documents gm:Boundary scope=file -->
# ADR 0035: components under a flat package tree

Status: accepted, 2026-10-23. Closes issue #192; builds on [ADR 0019](0019-migration-overlap.md) and [ADR 0026](0026-rollup-intent-nesting-inherit.md).

## Context

GlossMark's own ontology was written while GlossMark was being defined. Three manifests described it: the system, the `cmd/` container and the `internal/` container. Both containers realized nearly the same set of `gm:` processes, nothing sat below container level although `internal/` had 28 packages, and `gmi:` had three `part-of` edges among some forty concepts. The `engineering` view set `expand: { part-of: 2 }` at container zoom, so the map was two overlapping boxes full of leaves.

Two facts of the tool shaped the second pass. A manifest covers the directory it sits in, and `internal/` is flat: a component made of several packages has no directory to put its manifest in. And a view folds every boundary deeper than its zoom into the nearest shown ancestor, so components are invisible at container zoom whatever the manifests say.

## Decision

1. **A logical component may have no code of its own.** Its manifest sits in a directory that holds nothing else (`internal/reading/.gloss.yaml`), and the packages that belong to it name it with `part-of`. This is the existing rule that a logical parent can live beside its children (FILE-FORMATS.md, [ADR 0026](0026-rollup-intent-nesting-inherit.md) rule 9); nothing in the tool changes. The group manifest writes `inherit` for every relation its packages declare, so its declared set is the union of theirs. GlossMark's library has seven: reading a workspace, the graph and its exports, checks and gates, review, views and slices, onboarding, and the agent surface.
2. **A component may contain components.** `level` has nothing below `component`, and a package inside a logical component is one too. Views draw the nesting from `part-of` and fold both at any zoom above `component`; rollup containment follows `part-of` as it always did. No rule orders the levels of a parent and its child, and this ADR does not add one.
3. **A package manifest declares the implementation concepts its code implements**, not the core concepts they realize. The rollup lifts `realizes` from a declared `gmi:` concept to its `gm:` core, and that lift is the part's, not a claim the manifest must repeat ([ADR 0009](0009-hierarchy-direction.md)); `internal: true` concepts are never a manifest's to claim. `holds`, `produces` and `publishes` are declared where the code carries the marker. Rule 11 of ADR 0026 enforces completeness: a package that derives a relation its ancestors inherit while declaring nothing is reported on its own manifest.
4. **Containers write `inherit`.** `internal/.gloss.yaml` no longer repeats what its components declare; `cmd/.gloss.yaml` is one package and keeps its list; the root keeps `[inherit, gm:Onboarding]` for the skill shipped in `skills/`.
5. **The command line and the library still overlap, by design.** Every `gloss` command realizes what the library mechanism it calls realizes. ADR 0019's deepest-wins rule now reports each overlap as `arch:gloss-cli` against the package, not the library. It stays info-level and outside CI's fail-on list; marking `cmd/` differently would hide that the command is the mechanism's surface.
6. **A `gmi:` concept is nested under the mechanism it is a step of or a parameter of.** `MarkerGrammar`, `DeclarationAnchor`, `WorkspaceLoader` and `ValueBinder` are part of `Extractor`, `ImportLock` of `WorkspaceLoader`; `FormatVersions` of `FileFormats` and `GraphFormatVersion` of `FormatVersions`; `CriticalityStamper` of `RollupEngine`; `TextDiff` of `OntologyEditor`; `SuggestCalibration` of `MarkerSuggester` and `DefaultSoftThreshold` of `SuggestCalibration`. Each part keeps its own `realizes:` line, so the file reads without following the chain. No concept was added: coverage needs a direct `implements` and a verifier for every non-internal concept, and an umbrella with neither would be an orphan.
7. **Views are named for who reads them.** `business` and `engineering` are `gloss init`'s generic presets and said nothing about this repository. `concepts` (the core tier at system zoom, for adopters) and `architecture` (both tiers at component zoom, two levels of parts, for maintainers) replace them; `drift` and `constraints` keep their names. Every view's `doc` says who it is for and what it shows at its zoom. The gate classifies a view by its content (`coreOnly`), not its name, so nothing in the tool reads these names.

## Consequences

- `gmi` is `1.0.0`: adding `part-of` to an existing concept is MAJOR ([VERSIONING.md](../VERSIONING.md)). It has no published concepts and no importers, so the bump costs nothing beyond the HARD gate this pull request already carries.
- The self-host test is unchanged and passes; the derived allowed set of every package is its `default-namespace` plus what it declares, so the links beneath it are covered without an `allowed:` list. The library keeps `allowed: [derived, gm:GlossMark]` for the e2e tests that verify the system.
- `gloss drift --all` lists each cli-versus-package overlap in place of the cli-versus-library ones.
- Two marker lines were added: `internal/mcp/review.go` and `internal/review/review.go` publish `gm:LinkAccepted` and `gm:LinkRejected`, as `cmd/gloss/accept.go` already did. The review engine intends them (`gm:Review` publishes both), and once the package declared `publishes` for `gm:LinkProposed`, the intent was compared and found undeclared (ADR 0026 rule 3). The decide tool does accept and reject links, so the marker is true.

## Alternatives considered

- **One manifest per package and no groups.** Honest and minimal, but the map would be 27 boxes beside each other inside the library, which is the flat shape the issue reports one level down.
- **Move packages into group directories.** The cleanest model, with the directory tree saying what the manifests say. Rejected for now: a Go import-path refactor across the repository, every doc path and the `value-from` binding, for a result the manifests express on their own. It stays open if the groups prove stable.
- **A stub parent named only by `part-of`.** A stub takes its level from its children, so one over components is a container beside the library, not a component inside it.
