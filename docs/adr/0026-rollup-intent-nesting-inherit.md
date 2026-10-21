<!-- @gloss documents gm:DriftComparison scope=file -->
# ADR 0026: Rollup semantics: intent apart from code, partial lifts, nesting is containment, inherit on every flow

Status: accepted, 2026-10-21. Builds on [ADR 0010](0010-deep-hierarchies.md) and [ADR 0011](0011-range-lcs-summary.md). Closes issue #159 (E16, E17, FF11, L19).

## Context

Adopting v0.1.0 in the glossmark-examples showcases found four places where the rollup did what GRAPH.md said, and the result still surprised the adopter or forced a workaround:

- **E16, concept-level flows.** Rule 2 lifts the `produces`, `consumes`, `publishes` and `subscribes` a concept declares to every boundary whose code implements it, as ordinary derived edges. Two consequences: a container that implements `ClaimFiling` (which `publishes: [ClaimFiled]`) could not declare only the `MotorClaimFiled` its code publishes (`x:a publishes cl:ClaimFiled but declares publishes {cl:MotorClaimFiled}`), and the concept's intent satisfied the manifest by itself. Adding `PayoutFailed` to `PayoutNotification.subscribes` made the documented gap on the payout notifier vanish (`Gaps: 0`) although no code subscribes to it. A declaration in the ontology was checking a declaration in the manifest, and the code was never asked.
- **E17, `lift: range-lcs`.** The lift was all or nothing per boundary and relation. The claims root produces `{Claim, MotorClaim, FloodClaim, FireClaim, Assessment, Payout}`, which has no single subsumer, so nothing was lifted and the three claim subtypes stayed three edges instead of folding into `Claim`.
- **FF11, nesting vs `part-of`.** `gloss init` writes `part-of: <root>` into child manifests, and the rollup depended on it: deleting that line from `shipment-core/service/.gloss.yaml` gave the platform a gap (`declares publishes frt:ShipmentBooked but nothing under it publishes it`) although `service/` sits in the platform's directory. FILE-FORMATS.md says the nearest ancestor manifest applies to every file beneath it, and `inherit` and the derived allowed set already treated a manifest in a subdirectory as nested.
- **L19, `holds: inherit`.** Only `realizes` took `inherit`. With code tables declared at the root, every entity a child held was a violation at the root (seven lines), and `holds: [inherit, lib:LoanStatus]` was `unresolved target "inherit"` (GM211). The adopter moved the tables into a `config/` boundary to get around it.

## Decision

### Intent is compared apart from code (E16)

1. A flow a boundary has because a concept its code implements declares it (on the implemented concept or on what it realizes) is derived with `"rule":"intended-lift"` instead of `realization-lift`. The code's own flows (markers such as `@gloss publishes cl:MotorClaimFiled`, and what they lift to) keep `realization-lift`. `presents` and `hosts` reached through a concept stay facts (ADR 0001); only the four flows are intent.
2. Drift keeps two sets per boundary and flow: `derived`, from the code, and `intended`, from the concepts (the JSON `intended` field). Only `derived` closes a gap. A declared flow that only intent reaches is a gap that says so: `o:payout declares subscribes cl:PayoutFailed and cl:PayoutNotification intends it, but no code under it subscribes to it: mark the code that does, or the declaration is wrong`.
3. The intended set is compared with the manifest on its own. An intended concept is consistent when a declared concept covers it, or when the manifest declares a refinement of it: a concept it covers (a subtype, a part, a concept that realizes it). So a container may declare `publishes: [MotorClaimFiled]` under a concept that publishes `ClaimFiled`. An intended concept the manifest neither covers nor refines is a violation that names the intent: `o:payout subscribes cl:PayoutFailed by intent (cl:PayoutNotification intends it), but declares subscribes {cl:PayoutAuthorized}`. It is printed with its sets and not grouped with the code's violations. Under `inherit`, intent from a nested boundary's code is that boundary's to compare, as code is (rule 11).
4. Nothing else changes for intent: views, slices, `gloss landscape` and the implied `depends-on` (rule 4) still read every derived flow edge, intended ones included, since they draw what the system is meant to do. Only drift tells the two apart.

### `range-lcs` lifts each subset that shares a subsumer (E17)

5. A set is split into its maximal subsets of two or more members that sit below a common concept (no larger such subset contains them). Each is lifted to its unique least common subsumer; every other member stays. `{Claim, MotorClaim, FloodClaim, FireClaim, Assessment, Payout}` reads `{Assessment, Claim, Payout}`. A set with one common subsumer is one subset, as before.
6. ADR 0011's refusal to pick stands, per subset: members that share several incomparable minimal subsumers stay as they are with `(no unique subsumer: ...)`, and so do two maximal subsets that share a member (B is below both X and Y: either lift would hide the other line).
7. A violation names what each lift summarizes: `rollup: produces {cl:Assessment, cl:Claim, cl:Payout} (cl:Claim from cl:Claim, cl:FloodClaim, cl:MotorClaim) ⊄ declared`. A whole-set lift keeps `(from ...)`. Views lift each subset to its own edge with its own `via` and `count`; a curated view still never lifts what it names, and a subsumer the view does not include replaces nothing.

### Nesting is containment, `part-of` overrides it (FF11)

8. A manifest that names no `part-of` is part of the boundary of the nearest manifest in an enclosing directory. The extractor writes the edge as declared, with `"rule":"nearest-manifest"` in its provenance, so it shows in the graph and every reader of `part-of` (rollup containment, criticality, views, slices, `gloss landscape`) sees it.
9. An explicit `part-of` wins, whichever boundary it names. `part-of: []` (an explicit empty list) makes the boundary part of nothing, for the rare directory that only happens to sit inside another boundary's tree; such a manifest is no longer a nested child for `inherit` or the derived allowed set either.

We chose the behaviour change over documenting that containment is `part-of` only. Every reader already behaved as if nesting were containment except the rollup: `inherit`, the derived allowed set (the `nested` reason), `gloss landscape` (which leaves out the context-map edge between a boundary and one in an enclosing directory) and FILE-FORMATS.md's own sentence. The change is safe for the shipped trees: every nested manifest in this repository, the library example and the glossmark-examples showcases at the time already names its parent with `part-of`, so their graphs gain no edge.

### `inherit` on `holds` and the four flows (L19)

10. `holds`, `produces`, `consumes`, `publishes` and `subscribes` take `inherit` as `realizes` does: the declared set is the union of what the nested boundaries declare under the same relation (following their own `inherit`), plus the entries listed beside it. Inherited entries only cover; their gaps are the children's.
11. The rest of ADR 0010's rule 5 applies per relation: a concept derived from a nested boundary's code is judged where the inheritance ends, and a nested boundary that declares nothing under the relation while an ancestor inherits it is reported once, on its own manifest (`o:assess holds 1 concept but declares no holds; o:platform inherits from it: declare it under holds in services/assess/.gloss.yaml: {cl:Assessment}`).
12. `presents`, `hosts` and `configures` do not take `inherit`: they describe the boundary's own surface, and `inherit` there stays an unresolved target (GM211).

## Alternatives considered

- **Let intent close a gap when no code under the boundary marks that relation at all.** It would have kept repositories that model flows only on concepts quiet, GlossMark's own manifests among them. Rejected: it is the masking the issue reports, made conditional on an unrelated marker, and a reader cannot tell from a consistent line which mode was used.
- **Drop concept-level flows from the comparison entirely.** Rejected: the intended set is what lets drift say *which* concept promised the missing flow, and a manifest that contradicts its concepts' intent is still worth a finding.
- **Document that containment is `part-of` only (FF11).** Rejected, see 9: it would make the rollup the one reader that disagrees with the directory tree.
- **Lift greedily from the deepest subsumer down (E17).** Rejected: the result would depend on traversal order where two lines of the taxonomy compete, which is what ADR 0011 refused to do.

## Migration

- **Gaps on flows declared only through concepts.** A manifest whose `produces`, `consumes`, `publishes` or `subscribes` was satisfied only by what its concepts intend now has gaps. Mark the code that does it (`@gloss publishes cl:ClaimFiled` on the publisher) or drop the declaration. GlossMark's own `cmd/` and `internal/` gained seventeen such marker lines, its root now writes `produces: [inherit]` and `publishes: [inherit]`, and the library example marks `announce_return` (publishes `lib:ItemReturned`) and the holds queue (publishes `lib:HoldReady`).
- **Fewer violations for refined intent.** A manifest that declares a subtype of what its concept intends is consistent; a waiver written for that violation matches nothing (GM611).
- **Drift lines.** Partial lifts change the `rollup:` clause and consistent lines of a flow whose set has no single subsumer; a concept that only a concept's intent reached moves from `derived` to `intended` (the library example's `rollup: produces {lib:Item}` no longer lists `lib:Media`, which only `lib:Cataloging` intends).
- **Graph.** Intended flow edges carry `"rule":"intended-lift"`; nested manifests without `part-of` gain a declared `part-of` edge with `"rule":"nearest-manifest"`. `graph.jsonl` changes accordingly; the format number does not, since no record gains a field.
- **Manifests.** `inherit` on the five relations was GM211 before and is valid now. `part-of: []` is new.
