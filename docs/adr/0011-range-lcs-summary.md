<!-- @gloss documents gm:DriftComparison scope=file -->
# ADR 0011: `range-lcs` is a summary, coverage always subsumes

Status: accepted, 2026-10-20. Closes issues #91 (F-DEEP-9) and #133.

## Context

GRAPH.md called the taxonomic lift "opt-in per relation because it is valid for existential claims and invalid for exclusivity claims". The tool never behaved that way. `produces`, `consumes`, `publishes` and `subscribes` carry `lift: range-lcs` in the builtin vocabulary, so writing it in an ontology changed nothing, and the comparison used `is-a` subsumption for every relation whatever `lift` said: removing the four `lift:` entries in the claims-events showcase left `graph.jsonl` and the drift output byte-identical.

The lift itself had two rough edges in deep taxonomies (deep-hierarchies lab):

- `{ElectronicRearDerailleur, ElectronicFrontDerailleur}` has two incomparable minimal common subsumers (`Derailleur`, `PoweredDevice`). Drift printed the raw set with no sign that a lift was tried.
- A curated view keeps only edges with a named end. With `lift: range-lcs` it first replaced the edges to its three named concepts with one to `DrivetrainComponent`, which it does not name, and then dropped that one: a view written to show what produces the drivetrain parts showed no `produces` edge.

## Decision

1. **Coverage always subsumes.** Every relation drift compares (`realizes`, `holds`, `presents`, `hosts`, `produces`, `consumes`, `publishes`, `subscribes`) states what a boundary may do, an existential claim: declaring `produces: [zoo:Horse]` permits producing some horse, so a derived `Mare` is covered. No compared relation is an exclusivity claim, so there is nothing to opt out of. The "opt-in" wording is removed.
2. **`lift` is the summary.** A relation's `lift` only sets how drift prints a boundary's derived set: `range-lcs` (the builtin for the four flow relations) as its least common subsumer, `none` (new) as its members. A view's own `lift:` sets the same for that view.
3. **A unique subsumer or none.** The lift replaces a set only by its unique least common subsumer. When the members share several incomparable minimal subsumers nothing is lifted; drift appends `(no unique subsumer: cyc:Derailleur, cyc:PoweredDevice)` and the projection's edges carry `noUniqueSubsumer`. Before, three members with two such subsumers were printed as the two.
4. **Curated views keep what they name.** A curated view lifts only edges whose concept it does not name, and a lift whose subsumer is not in the view replaces nothing.
5. **A violation names the members.** `rollup: produces {cyc:PoweredDevice} (from cyc:ElectronicRearDerailleur, cyc:FrontLight) ⊄ declared`, so the line does not read as if `PoweredDevice` were produced. Consistent lines keep the short form.

## Alternatives considered

- **Honour the opt-in: no subsumption without `lift: range-lcs`.** Rejected: a manifest declaring `realizes: [cyc:Product]` would stop covering code for a `HydraulicDiscBrake`, and the four flow relations already default to `range-lcs`, so removing the entries would still change nothing.
- **Pick one subsumer when several exist** (the deepest, or the first by IRI). Rejected: either choice hides the other line of the taxonomy, which is exactly what the reader needs to see.

## Migration

None for the comparison. Drift lines change: an ambiguous set shows the note, a violation on a summarizing relation names its members, and three members with several minimal subsumers are no longer printed as those subsumers. Curated views with `lift: range-lcs` show the edges to the concepts they name again. `lift: none` is new in `schemas/ontology.schema.json`.
