<!-- @gloss documents gmi:ReviewLedger scope=file -->
# ADR 0005: Review decisions in the ledger keep their own fields and states

Status: accepted, 2026-10-20. Closes issue #119.

## Context

The ledger (`.glossmark/ledger.jsonl`) is the only store of review decisions. Adoption showed it losing them: `gloss reject --reason` wrote the reason into `rationale`, and the next build put the marker's `note=` back over it, so a rejected link with a note kept no reason at all.

## Decision

1. **`review_reason`** is a new optional entry field: the reviewer's reason for a rejection. `rationale` stays the author's `note=` (or a concept's doc). `gloss export --prov` exports it as `gm:reviewReason`.

The change is additive: older ledgers still read, the ledger `format` stays 1, and a reason that an older release stored in `rationale` stays there.

## Alternatives considered

- **Let the reason win over `note=` in `rationale`.** Rejected: the note is why the author wrote the link, which a reviewer reading a rejection wants beside the reason, not instead of it.
