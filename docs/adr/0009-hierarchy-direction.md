<!-- @gloss documents gm:DriftComparison scope=file -->
# ADR 0009: Both hierarchies point one way: coverage and kind

Status: accepted, 2026-10-20. Closes issues #83 (F-DEEP-1) and #84 (F-DEEP-2).

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

### Kind follows is-a down (issue #84, F-DEEP-2)

`kind` is "required unless `is-a` is given", which says a subtype is the kind of its ancestor, but the graph node of a subtype carried no kind, so every kind-dependent rule skipped it: `represents cyc:HydraulicDiscBrake` (an entity five `is-a` levels down) derived no `holds`, and a manifest declaring it got a false gap. Decision:

7. A concept with no kind of its own takes the kind of its nearest `is-a` ancestor that declares one, resolved at build time and written to the node with `kind-from` (as criticality carries `criticality-from`). Among ancestors at the same distance the first by IRI wins.
8. Every kind-dependent rule uses the effective kind: `represents` on an entity lifts to `holds`, `provisions`/`configures`/`deploys` on a resource or system-type lift to `realizes`, the domain/range warning (GM221), the event exemptions and `gloss impact`.
9. `is-a` parents that give different kinds are an error (GM306): declare `kind:` on the concept. A concept with no kind anywhere up its chain is GM300, as a concept with neither `kind` nor `is-a` already was.

`kind` moves only downwards; `part-of` carries none, since a part of a process can be an entity.

## Alternatives considered

- **Stop lifting wholes.** Rejected: "a part realizes its whole" is what lets a manifest declare a pipeline and have code mark its steps, and it is what views and criticality build on.
- **Require every lifted whole to be declared.** Rejected: every boundary implementing a part would have to list every whole above it, which is the boilerplate `realizes: inherit` (#9) removed.

## Migration

None in the shipped examples' drift results. On an adoption branch, `gloss drift` now reports a violation and a gap where a manifest declares a part and its code implements the whole; declare the whole, or move the marker to the part. Subtypes now carry a kind, so `holds` gaps on entity subtypes close, and GM221 may warn on links it used to skip; `gloss diff` reports a parent's kind change on its subtypes too.
