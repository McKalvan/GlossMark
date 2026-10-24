<!-- @gloss documents gm:Boundary scope=file -->
# ADR 0037: holds is declared ownership; represents is only a shape

Status: proposed, 2026-10-24. Amends [ADR 0013](0013-data-shapes-and-coverage.md); builds on [ADR 0036](0036-flow-concepts-sit-in-the-box-that-encloses-them.md).

## Context

`represents` says an artifact is the data shape of a concept. ADR 0013 made every `represents` derive `holds` on the boundaries above it, lifted through `realizes` to the core concepts, and made `represents` (with `realizes`) what places a concept in a view's box. So every shape of a concept counted as owning it.

Many shapes are not the owner: an OpenAPI schema or a GraphQL type in a gateway, a protobuf message, a cache table, a read model of another service's entity. The rule had three symptoms.

- **Placement.** A copy placed the concept in the copier's box. In a repository that does not contain the owner, a cache of `lib:Item` drew the entity as the cache's.
- **Drift.** A boundary whose manifest declared some `holds` was a violation for every other concept its code represented, and two boundaries representing one entity were an ADR 0019 `overlap`. The workarounds were MARKING.md's advice to mark a gateway schema `presents` (it is not a surface) and the `flowShape` exemption for event payloads (#172).
- **The lift in the wrong place.** `holds` lifted to the core concept but `represents` did not, and only `represents` placed. In the self-host, `arch:ledger` holds `gmi:ReviewLedger`, which realizes `gm:Ledger`, and `gm:Ledger` was drawn `unlinked` in `architecture`.

The relations answer different questions: `represents` is about code (*is this the shape of X?*), `holds` is about a boundary (*does X's data live here?*). A shape is evidence for ownership, not the claim.

## Decision

1. **`holds` is declared only.** The rollup no longer derives `holds` from `represents`. A manifest's `holds:` is the claim that the boundary owns the concept's data (`inherit` as before).
2. **Drift compares a declared `holds` with what code under the boundary represents.** A declared `holds` that no `represents` under it covers (the concept, a subtype or part, or an implementation concept that realizes it) is a gap, as before. Representing a concept the manifest does not hold is no violation and no omission: it is a shape. The `flowShape` exemption goes with the violation it exempted.
3. **Overlap counts declared `holds`.** Two unrelated boundaries that both declare `holds` for one concept are an `overlap`; two that only represent it are not.
4. **The gate's realization changes track `realizes` only.** A `holds` change is a change to a manifest line, already a HARD `manifest-changed`.
5. **Views place by `realizes` and declared `holds`.** A declared `holds` places the concept and the core concepts a held implementation concept realizes. It is as strong as code-and-declaration when code under the boundary represents the concept, and as strong as a declaration otherwise. `represents` alone places nothing.
6. **A new placement, `shape`**: code represents the concept, but no boundary declares `holds` for it (`represented by o:q, which declares no holds for it`). It ranks after `surface` and before `unlinked`.
7. **GM503's hint changes.** A `represents` outside the derived allowed set of a concept another boundary holds suggests `allowed: [derived, X]`, not `presents`.

## Consequences

- The self-host is unchanged in drift, and every concept in every view is housed: `gm:Ledger` sits in `arch:ledger` in `architecture` (78 housed, was 77 housed and 1 unlinked after ADR 0036).
- The library example declares ownership it used to infer: `acme:library-platform` adds `holds: [lib:Loan, lib:Patron]` (its migration is the system of record) and `acme:holds-worker` adds `holds: [lib:Hold]`. Without them the three would be drawn outside their boxes. In the holds-worker repository alone, `lib:Item` and `lib:ItemReturned` now float as inputs from outside (`consumed by`, `subscribed to by`), which they are.
- Adopters whose manifests name no `holds:` lose the placement of concepts only their tables placed; the concept moves to `shape`, and the reason says which boundary to declare it on. Drift reports nothing new for them, and fewer violations for those with partial `holds:` lists.
- Criticality is unchanged: it rolls along `realizes` and `represents` (`rollup.HousingRelations`), never along `holds`.
- `holds` in the vocabulary is no longer `Derived`. ADVANCED.md, MARKING.md and FILE-FORMATS.md say so.
- A graph built before this change carries derived `holds` edges. `gloss landscape` over graphs built by different versions disagrees on `holds` overlaps; rebuild every repository with the same version.

## Alternatives considered

- **A `mirrors` relation for copies, keeping `represents` → `holds`.** Says the same thing with one more word, and every adopter would have to tell `represents` from `mirrors` on each table. The owner is a fact about the boundary, so it belongs in the manifest.
- **Let a lifted `holds` place, as a weaker tier.** Fixes `gm:Ledger`, but a cache of an entity whose owner is out of view still lands in the cache's box.
- **Keep deriving `holds` and only stop it from placing.** Fixes the map, not the false violations and overlaps.
