<!-- @gloss documents gmi:DeclarationAnchor scope=file -->
# ADR 0014: A declaration's fingerprint covers its decorator stack

Status: accepted, 2026-10-20. Closes issue #72 (F-XCUT-1).

## Context

Cross-cutting concerns live in decorators and annotations: `@requires_role("approver")`, `@rate_limit("claims-write")`, `@feature("bulk_export")`, `@PreAuthorize`, `#[guard]`. The declaration detector skips that stack to find the declaration a marker anchors to, and the fingerprint covered only the signature line. Deleting a guard therefore changed no fingerprint, staled nothing, and `gloss diff` reported `no ontology-relevant changes`, even on a `high` handler. Changing the `def` line of the same handler was caught at once. The riskiest one-line change in a web service was the one GlossMark could not see.

## Decision

1. A symbol or block anchor records the decorator or annotation stack between the marker and the declaration as `decorators` on the artifact's anchor in `graph.jsonl`: one whitespace-normalized entry per decorator, in source order, multi-line arguments joined. Blank lines and comments inside the stack are not part of it.
2. The fingerprint is the hash of the stack followed by the signature line. A declaration without decorators fingerprints exactly as before. So adding, deleting, reordering or changing the arguments of a decorator stales the link, as a signature change does; reformatting does not.
3. `gloss diff` reports a link whose signature is unchanged but whose stack changed as `decorators-changed`, naming the decorators removed and added, at the criticality of what the link touches. It is AUTO by default but not bookkeeping: `policy.criticality-gates` applies to it, and on guarded code it is at least SOFT ([ADR 0015](0015-guards.md)).
4. Migration: an accepted ledger entry whose fingerprint equals the signature-only fingerprint of a decorated declaration moves to the new fingerprint on the next build instead of going stale, as ADR 0006 migrated file fingerprints.
5. `gloss diff` re-extracts the base tree when the base's committed `graph.jsonl` was written by another release (its `generator` differs from this binary's), instead of comparing fingerprints the two releases computed differently.

## Consequences

- The first build after the upgrade rewrites the fingerprint of every decorated declaration in `graph.jsonl`; the ledger follows without staling.
- A decorator that is not a guard (`@dataclass`, `@Override`, a route) is part of the stack too: adding `@Override` stales the link. That is the conservative reading; the alternative needs a list of which decorators matter in which framework.
- Point 5 makes a diff against a graph committed by an older (or newer) release as slow as one against an uncommitted graph, once; CI that builds with the release the graph was committed with keeps reading it.

## Alternatives considered

- **Fingerprint the decorators separately and only report them.** Rejected: the link would stay accepted after the guard went, so `gloss drift` and the ledger would disagree with `gloss diff`.
- **Fingerprint the whole body.** Rejected: every edit inside a function would stale its links, which is the noise symbol anchors exist to avoid.
- **A list of guard-like decorator names per framework.** Rejected for now: it needs maintenance per framework and still misses in-house decorators, which are the usual case.
