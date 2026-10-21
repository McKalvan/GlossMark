<!-- @gloss documents gm:DriftComparison scope=file -->
# ADR 0010: Deep hierarchies: one coverage relation, findings by cause

Status: accepted, 2026-10-20. Builds on [ADR 0009](0009-hierarchy-direction.md). Closes issues #87 (F-DEEP-5), #88 (F-DEEP-6), #89 (F-DEEP-7), #90 (F-DEEP-8) and #92 (F-DEEP-10).

## Context

The deep-hierarchies lab in glossmark-examples (a product taxonomy seven `is-a` levels deep, an importer taxonomy whose root alone says `realizes:`, two `part-of` chains five levels deep, nine nested manifests) showed that each rule worked one hop at a time. `gloss check` and `gloss drift` used different relations for the same question, provenance kept only the first and last step, and the output grew with the depth of the hierarchy instead of with the mistake.

## Decision

### The derived allowed set is drift's coverage (#87)

1. A concept is in a manifest's derived allowed set when a declared concept covers it in drift: the concept itself, its `is-a` and `part-of` descendants along any mix of the two, and every concept whose realization closure (what it realizes, the wholes it is part of, what its `is-a` ancestors realize) reaches the declared concept or an `is-a` descendant of it. A boundary declaring `cyc:CatalogueIngestion` therefore admits `spk:ShimanoXmlImporter`, which is five `is-a` levels below the `spk:Importer` that realizes it, and a boundary declaring `spk:Importer` admits `spk:CsvRowParser`, a part of one of its subtypes.
2. ADR 0009 still holds: a declared implementation concept adds the cores it realizes, not the wholes it is part of, and a declared part admits neither its whole nor the whole's other parts.
3. Every entry of a derived set records why it is there (`default-namespace`, `declared`, `covered` from a declared concept, `realized` by a declared implementation concept, `listed` next to `derived`, `nested` from a child boundary). `gloss check --explain` prints the set grouped by those reasons, and `--format json` prints every entry (issue #93).

### Violations follow the mistake, not the depth (#88)

4. **One finding per link.** An uncovered derived concept that is only there because another uncovered derived concept lifted it (every derived edge to it names that concept as `via[1]`, apart from edges lifted from a covered part) is not reported on its own; it is listed in the lifting concept's finding as `lifted`. One marker on `spk:ScanVerifier` under a manifest declaring `cyc:Picking` is one violation at `verifier.py:11` that lifts `spk:BinRouter`, `spk:WaveBuilder`, `spk:PickPlanner`, `spk:FulfilmentEngine` and `cyc:OrderFulfilment`. The finding id stays `violation:<boundary>:<relation>:<concept>` for the lifting concept.
5. **One finding at the boundary that omitted.** For `realizes`, walk down from the boundary toward the boundary of the code, passing every boundary that declares `realizes: inherit`. The first one that does not inherit (or the code's own boundary) owns the finding. When that is a nested boundary that declares `realizes`, its own comparison reports the concept and the inheriting ancestors say nothing. When it declares nothing (or only an `inherit` that resolves to nothing), one violation is reported at it, grouped over everything its code derives, naming its manifest and every ancestor that inherits from it. This generalizes the single-level rule of issue #40, which only applied when the code's own boundary was the one that declared nothing.
6. **No repeated sets.** A violation line names the declared set, never the derived one (22 concepts at the root of the lab), and only the first violation of a boundary and relation prints it (issue #40). The full derived set stays in the JSON `derived` field and in `gloss drift --all`. Relations that summarise with `range-lcs` keep their `rollup:` clause ([ADR 0011](0011-range-lcs-summary.md)).

### An abstract supertype is built when anything below it is (#90)

7. A concept is not an orphan when an `is-a` descendant is reached by any route that would make the descendant itself not an orphan: a direct link, a concept that realizes it, a part, or a deeper subtype. Before, only a direct link to a descendant counted, so `cyc:Lighting` was an orphan although `spk:LightListing`, which has code, realizes its subtype `cyc:FrontLight`, and the orphan list changed with unrelated `produces` markers. `orphan` now means "nothing below it is built".

### Provenance names the whole chain (#89)

8. A lifted boundary edge keeps `via` as the list of CURIEs the derivation passed through, now all of them: the artifact, the concept it links, and every concept on the shortest path to the object. A new `steps` list spells the hops as `relation CURIE`. `via[1]` is still the linked concept, so readers of the old two-element form keep working.
9. An inherited criticality carries `criticality-source`, the node at the end of the `criticality-from` chain that declares the level.
10. `collapse: via realizes` folds a concept with no `realizes`, `published-as` or `part-of` where its `is-a` parent folds, so `spk:ShimanoXmlImporter` is folded into `cyc:CatalogueIngestion` with the `spk:Importer` that realizes it.
11. `is-a` ancestors stay out of a boundary's derived set while the wholes above a part are in it. Implementing a step implements part of the whole; implementing a subtype does not implement the supertype in general. The asymmetry is deliberate and now visible in `steps`.

### A whole sits with the code that runs it (#92)

12. In view placement, code implementing a concept whose own `realizes` (or `published-as`) names the target counts as the boundary's own code (ADR 0012 already ranks it so). It beats a manifest that only declares the target, so `cyc:OrderFulfilment` sits in `spw:fulfilment`, whose `engine.py` implements `spk:FulfilmentEngine`, and not in the leaf `spw:scanning` that declares it to cover a deep part.
13. A `part-of` whole is never placed inside the box of one of its parts; it moves to the nearest boundary enclosing its own box and its parts'.

### Domain areas that span boundaries (pfinance adoption, ADR 0025)

pfinance grouped its core tier into seven `part-of` areas, and some areas, such as its bookkeeping, are realized in both `app/` and `src/`. Each area therefore sits in the system box (13). In the engineering view the viewer drew every part inside its area's box, which pulled the parts out of the `app/` and `src/` boxes that hold their code, and the area boxes overlapped the boundary boxes.

14. A part with no boundary of its own follows the root of its `part-of` tree after the root is placed, including the move in 13, not before it. Before, a note with no code followed its product to `app/` and stayed there when the product moved to the system box.
15. In the viewer, a part housed in a boundary that is shown strictly inside its whole's boundary is drawn in that boundary, not in the whole's box. It takes the whole's colour, and the concept panel names the whole and says why. A whole whose housed parts all share one boundary is still a box, inside that boundary. A part that is only near a boundary (flow, shared, unlinked) stays in its whole. Views at system zoom are unchanged: every boundary below the system is folded there, so no part is ever deeper than its whole.

## Alternatives considered

- **Admit the default namespace instead of realizers.** Rejected: `default-namespace: spk` admits every `spk:` concept, including unrelated ones, which is not "the implementation of what I declare".
- **Report an inherit omission at every ancestor, with a pointer to the child.** Rejected: the ancestors' manifests are not wrong, and N copies of one finding is what the lab complained about.
- **A per-boundary header in the text report with the collapsed sets.** Not done: once violation lines stop repeating the derived set they are short, and the set is one `--all` or JSON read away.

## Migration

- Links that were GM503 warnings (and HARD in the gate) because they named an implementation concept realizing a declared core concept now pass. Nothing that passed before fails.
- `gloss drift` reports fewer violations for the same mistakes. A waiver that names a concept now listed under another finding's `lifted` matches nothing, and `gloss check` reports it as an unmatched waiver; waive the lifting concept's finding instead.
- `graph.jsonl` grows: lifted edges carry the full `via` and a `steps` list, inherited levels a `criticality-source`.
- Supertypes reached only by realization drop out of the orphan list; views may place a whole one box higher.
- (14, 15) Engineering-style views of a repository whose `part-of` wholes span boundaries draw the parts in their own boundaries. On GlossMark itself one concept moves (`gmi:HtmlViewer`, whose code is in `internal/`, leaves the box of `gmi:ViewProjector`, which spans the CLI and the library); the library example is unchanged.
