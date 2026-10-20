<!-- @gloss documents gmi:ReviewLedger scope=file -->
# ADR 0005: Review decisions in the ledger keep their own fields and states

Status: accepted, 2026-10-20. Closes issues #119, #132 and #62.

## Context

The ledger (`.glossmark/ledger.jsonl`) is the only store of review decisions. Adoption showed it losing them: `gloss reject --reason` wrote the reason into `rationale`, and the next build put the marker's `note=` back over it, so a rejected link with a note kept no reason at all. And a `value-from` baseline whose code drifted was recorded as `stale`, the status of a link whose anchor changed, so the documented way to clear stale links (drop those lines and rebuild) silently made the drifted code the new baseline. Finally, a rejection did not stick: the rejected `@gloss?` line kept lifting (a drift violation, GM503, coverage), deleting it turned the entry into `removed`, and `gloss suggest` proposed the same link on the next run.

## Decision

1. **`review_reason`** is a new optional entry field: the reviewer's reason for a rejection. `rationale` stays the author's `note=` (or a concept's doc). `gloss export --prov` exports it as `gm:reviewReason`.

2. **`drifted`** is a new status, used only by `value:<curie>` entries: the bound code changed and the published value did not. `gloss drift` reports it as `value-drift` until `gloss accept --value` or a matching ontology edit. Links never take it, and value entries never take `stale`; a value entry an older release left `stale` is read as drifted and rewritten on the next build.

3. **`rejected` sticks.** A rejected marker's edges are dropped from the graph after the ledger sync, so a rejected line still in the source states nothing, like a comment. When the line disappears the entry stays `rejected` (deleting a rejected proposal is the expected cleanup), and `gloss suggest` skips a link the ledger records as rejected, by marker id or by anchor fingerprint, relation and target, and reports how many it skipped. Writing the marker again without `?` is still a human decision that accepts it.

The changes are additive: older ledgers still read, the ledger `format` stays 1, and a reason that an older release stored in `rationale` stays there.

## Alternatives considered

- **Let the reason win over `note=` in `rationale`.** Rejected: the note is why the author wrote the link, which a reviewer reading a rejection wants beside the reason, not instead of it.
