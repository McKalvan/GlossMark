<!-- @gloss documents gm:DriftComparison scope=file -->
# ADR 0013: Data shapes hold what they store; coverage counts what code implements

Status: accepted, 2026-10-20. Closes issues #54 (F-SURF-8) and #63 (F-POOR-6).

## Context

`represents` is the relation for a data shape: a model class, a table, a schema. On an entity it derived `holds`, so a data boundary could declare `holds: [pf:Transaction]` and be compared by `gloss drift`. On any other kind it derived nothing. Legacy schemas are full of tables that store measures (counts, scores, balances, KPIs), event logs and configured limits. A boundary whose table `represents wh:CountVariance` could not state it: declaring `holds: [wh:CountVariance]` was a gap that failed by default, and leaving it out put the marker outside the derived allowed set (GM503, HARD in the gate). The only clean manifest was `allowed: [derived, wh:CountVariance]`, which turns the check off rather than stating the intent. GlossMark's own library had the same line for `gm:Confidence`.

A GraphQL type, an OpenAPI schema or a protobuf message in a gateway looks like a data shape, and `represents` is the relation for one. On an entity it lifts to `holds`, so the gateway was recorded as storing orders, and the entity's placement moved from the order service to the root. GM503 fired, but following it by declaring `presents` cleared the warning and left the placement where it was.

## Decision

1. `represents` lifts to `holds` on a concept of any kind, and to the core concepts the target realizes, as it did for entities. A boundary whose code stores a concept's values holds them.
2. `holds` is still compared only when the manifest declares it, a declared `holds` entry is in the derived allowed set, and placement and criticality still use the boundary's `represents` edges.
3. In placement, a boundary whose code represents a concept and whose manifest declares `holds` for it beats one whose code only represents it ([ADR 0012](0012-surfaces.md) rule 7), so a gateway schema no longer takes the entity from the service that stores it.
4. API and wire schemas of an entity stored elsewhere are `presents` (or unmarked), not `represents` (MARKING.md). GM503 on a `represents` says so when another boundary declares `holds` for the concept.

## Consequences

- A manifest that declares `holds:` and whose code represents a measure, event, resource or constraint it does not list gets a `holds` violation; add the concept to `holds:`. GlossMark's own `internal/.gloss.yaml` now lists `gm:Graph`, `gm:Ledger` and `gm:Confidence`, and drops `gm:Confidence` from `allowed:`.
- `allowed: [derived, X]` written only because a table stores measure `X` can become `holds: [..., X]`.
