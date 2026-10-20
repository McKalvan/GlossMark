<!-- @gloss documents gm:DriftComparison scope=file -->
# ADR 0010: Deep hierarchies: one coverage relation, findings by cause

Status: accepted, 2026-10-20. Builds on [ADR 0009](0009-hierarchy-direction.md).

## Context

The deep-hierarchies lab in glossmark-examples (a product taxonomy seven `is-a` levels deep, an importer taxonomy whose root alone says `realizes:`, two `part-of` chains five levels deep, nine nested manifests) showed that each rule worked one hop at a time. `gloss check` and `gloss drift` used different relations for the same question, and the output grew with the depth of the hierarchy instead of with the mistake.

## Decision

### The derived allowed set is drift's coverage (issue #87, F-DEEP-5)

1. A concept is in a manifest's derived allowed set when a declared concept covers it in drift: the concept itself, its `is-a` and `part-of` descendants along any mix of the two, and every concept whose realization closure (what it realizes, the wholes it is part of, what its `is-a` ancestors realize) reaches the declared concept or an `is-a` descendant of it. A boundary declaring `cyc:CatalogueIngestion` therefore admits `spk:ShimanoXmlImporter`, which is five `is-a` levels below the `spk:Importer` that realizes it, and a boundary declaring `spk:Importer` admits `spk:CsvRowParser`, a part of one of its subtypes.
2. ADR 0009 still holds: a declared implementation concept adds the cores it realizes, not the wholes it is part of, and a declared part admits neither its whole nor the whole's other parts.
3. Every entry of a derived set records why it is there (`default-namespace`, `declared`, `covered` from a declared concept, `realized` by a declared implementation concept, `listed` next to `derived`, `nested` from a child boundary). `gloss check --explain` prints the reasons.

### An abstract supertype is built when anything below it is (issue #90, F-DEEP-8)

4. A concept is not an orphan when an `is-a` descendant is reached by any route that would make the descendant itself not an orphan: a direct link, a concept that realizes it, a part, or a deeper subtype. Before, only a direct link to a descendant counted, so `cyc:Lighting` was an orphan although `spk:LightListing`, which has code, realizes its subtype `cyc:FrontLight`, and the orphan list changed with unrelated `produces` markers. `orphan` now means "nothing below it is built".

### Provenance names the whole chain (issue #89, F-DEEP-7)

5. A lifted boundary edge keeps `via` as the list of CURIEs the derivation passed through, now all of them: the artifact, the concept it links, and every concept on the shortest path to the object. A new `steps` list spells the hops as `relation CURIE`. `via[1]` is still the linked concept, so readers of the old two-element form keep working.
6. An inherited criticality carries `criticality-source`, the node at the end of the `criticality-from` chain that declares the level.
7. `collapse: via realizes` folds a concept with no `realizes`, `published-as` or `part-of` where its `is-a` parent folds, so `spk:ShimanoXmlImporter` is folded into `cyc:CatalogueIngestion` with the `spk:Importer` that realizes it.
8. `is-a` ancestors stay out of a boundary's derived set while the wholes above a part are in it. Implementing a step implements part of the whole; implementing a subtype does not implement the supertype in general. The asymmetry is deliberate and now visible in `steps`.

### A whole sits with the code that runs it (issue #92, F-DEEP-10)

9. In view placement, code implementing a concept whose own `realizes` (or `published-as`) names the target counts as a direct link, read from the edge's `steps`. It beats a manifest that only declares the target, so `cyc:OrderFulfilment` sits in `spw:fulfilment`, whose `engine.py` implements `spk:FulfilmentEngine`, and not in the leaf `spw:scanning` that declares it to cover a deep part.
10. A `part-of` whole is never placed inside the box of one of its parts; it moves to the nearest boundary enclosing its own box and its parts'.

## Alternatives considered

- **Admit the default namespace instead.** Rejected: `default-namespace: spk` admits every `spk:` concept, including unrelated ones, which is not "the implementation of what I declare".

## Migration

Links that were GM503 warnings (and HARD in the gate) because they named an implementation concept realizing a declared core concept now pass. Nothing that passed before fails.
