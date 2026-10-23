<!-- @gloss documents gm:DriftComparison scope=file -->
# ADR 0013: Data shapes hold what they store; coverage counts what code implements

Status: accepted, 2026-10-20. Closes issues #53 (F-SURF-7), #54 (F-SURF-8) and #63 (F-POOR-6).

## Context

`represents` is the relation for a data shape: a model class, a table, a schema. On an entity it derived `holds`, so a data boundary could declare `holds: [pf:Transaction]` and be compared by `gloss drift`. On any other kind it derived nothing. Legacy schemas are full of tables that store measures (counts, scores, balances, KPIs), event logs and configured limits. A boundary whose table `represents wh:CountVariance` could not state it: declaring `holds: [wh:CountVariance]` was a gap that failed by default, and leaving it out put the marker outside the derived allowed set (GM503, HARD in the gate). The only clean manifest was `allowed: [derived, wh:CountVariance]`, which turns the check off rather than stating the intent. GlossMark's own library had the same line for `gm:Confidence`.

A GraphQL type, an OpenAPI schema or a protobuf message in a gateway looks like a data shape, and `represents` is the relation for one. On an entity it lifts to `holds`, so the gateway was recorded as storing orders, and the entity's placement moved from the order service to the root. GM503 fired, but following it by declaring `presents` cleared the warning and left the placement where it was.

Concept coverage counted implementation-tier concepts only. Nothing discourages `implements hp:X` on a core concept, and a server-rendered page, a CLI or a BFF that implements a core process directly is a natural reading, so such a workspace showed `0 of 0` and `0%`, and `--fail-under` failed a workspace whose every concept was implemented and tested.

## Decision

1. `represents` lifts to `holds` on a concept of any kind, and to the core concepts the target realizes, as it did for entities. A boundary whose code stores a concept's values holds them.
2. `holds` is still compared only when the manifest declares it, a declared `holds` entry is in the derived allowed set, and placement and criticality still use the boundary's `represents` edges.
3. In placement, a boundary whose code represents a concept and whose manifest declares `holds` for it beats one whose code only represents it ([ADR 0012](0012-surfaces.md) rule 7), so a gateway schema no longer takes the entity from the service that stores it.
4. API and wire schemas of an entity stored elsewhere are `presents` (or unmarked), not `represents` (MARKING.md). GM503 on a `represents` says so when another boundary declares `holds` for the concept.
5. Concept coverage puts in scope every owned implementation concept, as before, and every owned core concept that a marker `implements` directly; either is covered with an artifact and a verifier. With nothing in scope coverage is `n/a` in every report, and `--fail-under` (or `coverage-fail-under`) never fails on it.

## Consequences

- A manifest that declares `holds:` and whose code represents a measure, event, resource or constraint it does not list gets a `holds` violation; add the concept to `holds:`. GlossMark's own `internal/.gloss.yaml` now lists `gm:Graph`, `gm:Ledger` and `gm:Confidence`, and drops `gm:Confidence` from `allowed:`.
- `allowed: [derived, X]` written only because a table stores measure `X` can become `holds: [..., X]`.
- Coverage numbers change where code implements core concepts directly: they enter the denominator, usually already verified. A workspace with no implementation concepts and no direct `implements` reports `n/a`.
- The payload type of an event the same boundary publishes, subscribes, produces or consumes is the flow's shape, not stored data (#172): its derived `holds` is not a violation when the manifest leaves it out. It still covers a declared `holds:` entry, so an event log can be declared.
