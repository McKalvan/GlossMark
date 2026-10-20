<!-- @gloss documents gm:DriftComparison scope=file -->
# ADR 0009: Both hierarchies point one way in drift comparison

Status: accepted, 2026-10-20. Closes issue #83 (F-DEEP-1).

## Context

Drift compares what a boundary's code derives with what its manifest declares. A declared concept covers a derived one when they are equal, when the declared one subsumes it through `is-a*`, or when the derived one realizes it. `part-of` enters through the realization lift: a part realizes its whole, so code implementing `ScanConfirm` makes the boundary realize `ScanConfirm`, `BinPick`, `PickWave`, `Picking` and `OrderFulfilment`, and satisfies a manifest that declares any of them.

`is-a` coverage is one-directional: a declared `Component` covers code for a `Cassette`; a declared `Cassette` does not cover code for `Component`, which is a violation plus a gap. `part-of` coverage was not. `Covers` also asked whether the declared concept stood for the derived one, so a declared part covered its whole: a component declaring the scan-confirmation step could implement all of order fulfilment, with no violation and no gap. The allowed set derived from a manifest made the same mistake, so the link to the whole passed `gloss check` too.

## Decision

`part-of` coverage points the same way as `is-a`:

1. A declared whole covers a derived part. A declared supertype covers a derived subtype.
2. A declared part never covers a derived whole. A declared subtype never covers a derived supertype.
3. A gap on a declared part is closed only by code that implements that part or one of its parts.
4. The wholes lifted from a covered part are not violations. Code implementing a declared `ScanConfirm` still makes the boundary realize `OrderFulfilment`; that derived edge came through `ScanConfirm`, which the manifest declares, so it is consistent. Code that names a whole itself, or a part the manifest does not cover, is judged on its own, and so are the wholes it lifts.
5. A declared implementation concept still covers the core concepts it realizes, directly or through its `is-a` ancestors, but not the wholes it is part of.
6. The derived allowed set adds a declared concept's `part-of` and `is-a` descendants and what it realizes, not the wholes it is part of. Declaring a part does not admit a link to the whole (GM503).

Rule 4 is read from provenance: a lifted derived edge records the concept it came through in `via`, and the comparison checks that concept against the declarations.

## Alternatives considered

- **Stop lifting wholes.** Rejected: "a part realizes its whole" is what lets a manifest declare a pipeline and have code mark its steps, and it is what views and criticality build on.
- **Require every lifted whole to be declared.** Rejected: every boundary implementing a part would have to list every whole above it, which is the boilerplate `realizes: inherit` (#9) removed.

## Migration

None in the shipped examples. On an adoption branch, `gloss drift` now reports a violation and a gap where a manifest declares a part and its code implements the whole; declare the whole, or move the marker to the part.
