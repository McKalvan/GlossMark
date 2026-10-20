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

## Alternatives considered

- **Admit the default namespace instead.** Rejected: `default-namespace: spk` admits every `spk:` concept, including unrelated ones, which is not "the implementation of what I declare".

## Migration

Links that were GM503 warnings (and HARD in the gate) because they named an implementation concept realizing a declared core concept now pass. Nothing that passed before fails.
