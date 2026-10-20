<!-- @gloss documents gmi:Gate scope=file -->
# ADR 0018: what the gate and the semver classification take seriously

Status: accepted, 2026-10-20. Closes issues #97 (F-SPLIT-4), #98 (F-SPLIT-5), #106 (F-VER-1), #112 (F-VER-8) and #113 (F-VER-9).

## Context

The companion repository's decomposition and version-evolution labs found the owner's gate green on changes that break or redefine what importers and business readers rely on:

- A rewritten `doc:` on a core concept was SOFT and changed no view, although the business view shows that definition. A version-only edit to the same file was HARD through `hard-paths`, so the semantic classification lowered the path rule.
- A missing or undersized bump was SOFT. The owner's PR passed, and every importer then failed GM107, or took a breaking change inside its range.
- A deprecation was PATCH, so a `~1.0.0` importer received new GM222 warnings in a fix release.
- Removing a concept the base had already deprecated was told to "prefer deprecated: true".
- A move of a capability from one boundary to another was only visible as per-link rows, and a deleted view was not reported.

## Decision

1. **Definitions.** A label or `doc:` change on a core concept is HARD (`definition of a core concept changed`), and `views.Diff` counts a doc change as a changed node, so every view that shows the concept is listed under "Views changed". On an implementation concept it stays SOFT.
2. **hard-paths is a floor.** Any change whose path matches `policy.hard-paths` is HARD whatever its semantic class; bookkeeping (accepted proposals, stale links, fingerprint-only changes) stays AUTO. Under the default hard-paths an implementation doc edit in `ontology/` is therefore HARD too; a workspace that keeps implementation ontologies elsewhere gets the SOFT class.
3. **Bumps.** Each ontology change carries the bump it requires (`bump` in JSON). A deprecation, an un-deprecation and a `replaced-by` change are MINOR. A missing or undersized bump on an owned namespace is a HARD `version-bump` change.
4. **Removal after deprecation** is still MAJOR, and its line reads `concept X removed (deprecated since base, replaced-by Y)`.
5. **Realization changes.** `gloss diff` lists, per concept, the deepest boundaries that realize or hold it before and after (text, markdown and `realizations` in JSON). A boundary that loses its last implementation of a concept is a `realization-removed` change: SOFT, raised by the concept's criticality gate, and HARD when nothing realizes or holds the concept any more. A view declared at the base and missing from the head is listed as `name (removed)`.

## Alternatives considered

- **A separate gate class for missing bumps** that the Action fails on. Rejected: it adds a class every consumer must learn, and HARD already means "a human decides".
- **Gate a lost realization of a `high` concept HARD by default.** Rejected: the policy's `criticality-gates` is where a team says which levels need review; the default gates only `critical`.

## Migration

Owners whose PRs only edited docs of core concepts, or forgot bumps, now see HARD and must bump or approve. Nothing in any file format changes.
