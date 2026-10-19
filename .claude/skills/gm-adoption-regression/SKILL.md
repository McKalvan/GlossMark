---
name: gm-adoption-regression
description: Measure what a GlossMark branch changes on real adoptions - build the branch and main binaries, rebuild the in-repo examples and the pfinance and AgentArena adoption checkouts with both, and diff graphs, check, drift, coverage and build time. Use before merging a modelling, grammar, rollup or performance change, and when an ADR's consequences need numbers.
---

<!-- @gloss instructs gm:Onboarding scope=file -->

# Adoption regression

The first two adoptions (pfinance and AgentArena, branch `claude/glossmark-ontology` in each; see `docs/reviews/2026-10-18-adoption-review.md`) are the evidence base. A branch that changes their graphs, findings or timings must say so.

1. **Two binaries.** Build the branch: `go build -o "$TMPDIR/gloss-branch" ./cmd/gloss`. Build `main` from a separate checkout so your working tree is untouched: `git worktree add "$TMPDIR/gm-main" main`, then `go build -o "$TMPDIR/gloss-main" ./cmd/gloss` inside it, and `git worktree remove "$TMPDIR/gm-main"` when done.
2. **Targets.** Always the in-repo fixtures: `examples/horse-factory`, `examples/payments/payments-api`, `examples/payments/settlement-worker`. When sibling checkouts of the adopter repositories exist (ask the user for their paths; never clone without being asked), add them too. Never write into an adopter checkout: copy it to a temporary directory first.
3. **Run both binaries on each target**, from the target's root, writing the graph outside the tree:
   ```sh
   for b in main branch; do
     /usr/bin/time -p "$TMPDIR/gloss-$b" build -q --no-ledger -o "$TMPDIR/$b.graph.jsonl"
     "$TMPDIR/gloss-$b" check  > "$TMPDIR/$b.check.txt" 2>&1
     "$TMPDIR/gloss-$b" drift  > "$TMPDIR/$b.drift.txt" 2>&1
     "$TMPDIR/gloss-$b" coverage > "$TMPDIR/$b.coverage.txt" 2>&1
   done
   ```
4. **Diff.** Graph lines changed (`diff "$TMPDIR/main.graph.jsonl" "$TMPDIR/branch.graph.jsonl" | grep -c '^[<>]'`, ignoring the header line), check and drift summaries, coverage percentages and build time. For the payments pair also compare `gloss landscape examples/payments/payments-api examples/payments/settlement-worker` with each binary.
5. **Explain every difference.** Each changed count is either intended by the branch (name the ADR or issue) or a regression to fix before merging. A PATCH-level change must produce no difference at all (`docs/VERSIONING.md`).
6. **Record it.** Append a row per adoption to the "Regeneration check" table in `docs/reviews/2026-10-18-adoption-review.md` (tool versions, findings before and after, changed graph lines, what changed), or put the numbers in the ADR's Consequences for a modelling change.
