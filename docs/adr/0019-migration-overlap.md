<!-- @gloss documents gm:Boundary scope=file -->
# ADR 0019: overlapping implementations, declared migrations and overdue retirements

Status: accepted, 2026-10-20. Closes issue #95 (F-SPLIT-2); builds on [ADR 0007](0007-retiring-boundaries.md).

## Context

During a strangler migration two boundaries realize the same core process and hold the same entity. Drift reported both as consistent and landscape the same, so the most important fact about the state did not appear. ADR 0007 gave boundaries `deprecated`, `replaced-by` and `retire-by` but left drift unchanged: nothing used them, and a passed `retire-by` raised nothing.

## Decision

1. **`overlap`** (info, not failing by default, listed by `drift --all` and `landscape --all` beside the consistent pairs and always in JSON). For `realizes` and `holds`, take the boundaries whose code derives the relation for a concept and keep the deepest (a boundary is dropped when one nested in it is in the set, so a system does not overlap its own component). Two or more left is an overlap: `hire:Invoicing is realized by bw:hirehq-invoicing and bw:invoicing-service`. It is computed on the graph, so `gloss landscape` reports overlaps across repositories with the same rule.
2. **A declared migration silences it.** Two boundaries are migrating when one is `deprecated: true` with `replaced-by:` naming the other. An overlap is not reported when the declared migrations connect every boundary in the set; a third, undeclared boundary keeps it reported.
3. **`overdue`** (error, failing by default in `drift`, `landscape` and the Action). A deprecated boundary whose `retire-by` day has ended and which still has artifacts under it is overdue; the finding names the date, the replacement and what it still realizes or holds. `retire-by` is inclusive, like a waiver's `expires`.

A `switch:` key naming the feature flag that routes between the two sides (the issue's optional item 4) is not added; `configures` on the flag file already links it, and the viewer change can come later without a format change.

## Alternatives considered

- **Report overlaps only between boundaries in different repositories.** Rejected: the strangler often starts inside the monolith's repository.
- **Make overlap failing.** Rejected: two implementations of a core capability are legitimate in some architectures (regional deployments, read and write paths); the finding is the information, and `--fail-on overlap` makes it a gate where a team wants one.
- **Use waivers for the deadline.** Rejected: a waiver silences one finding, and it is the manifest that knows which boundary retires and what replaces it.
- **List overlaps in the default text output.** Rejected: a command-line boundary and the library it calls both realize the same core concepts (GlossMark itself has 33 such pairs), so the default output would bury the failing findings. A migration is visible with `--all`, in JSON, and as soon as it is declared, through `overdue`.
