<!-- @gloss documents gm:Graph scope=file -->
# The canonical graph

Everything downstream reads `.glossmark/graph.jsonl`, never the source. Adding a consumer never touches the extractor.

## Records

Line 1 is the context: `{"type":"context","format":1,"generator":"gloss 0.2.0","prefixes":{...},"@context":"./context.jsonld"}`. `format` is the record format version; a reader refuses a newer one (see `docs/VERSIONING.md`).

Node records (`"type":"node"`), sorted by node type then IRI:

```json
{"type":"node","node":"concept","id":"https://onto.acme.dev/zoo/Mare","curie":"zoo:Mare","ns":"zoo","tier":"core","kind":"entity","doc":"...","file":"ontology/zoo.gloss.yaml","line":6}
{"type":"node","node":"boundary","id":"https://arch.acme.dev/boundaries/horse-factory-svc","curie":"acme:horse-factory-svc","kind":"container","dir":"services/horse-factory","owner":"horse-platform","criticality":"medium","criticality-from":"zoo:Breeding"}
{"type":"node","node":"artifact","id":"urn:gloss:artifact:services/horse-factory/breeding/engine.py#foal_male","kind":"function","path":"services/horse-factory/breeding/engine.py","anchor":{"scope":"symbol","symbol":"foal_male","start":7,"end":8,"fingerprint":"sha256:..."},"boundary":"https://arch.acme.dev/boundaries/horse-factory-svc"}
```

Edge records (`"type":"edge"`), sorted by subject, predicate, object, source, path, line:

```json
{"type":"edge","s":"urn:gloss:artifact:...#foal_male","p":"implements","o":"https://onto.acme.dev/horse-factory/StallionProduction","source":"inline","prov":{"path":"services/horse-factory/breeding/engine.py","line":5,"col":3,"scope":"symbol","symbol":"foal_male","fingerprint":"sha256:...","id":"m-9e2d...","actor":"human:alice","raw":".StallionProduction"}}
{"type":"edge","s":"https://arch.acme.dev/boundaries/horse-factory-svc","p":"realizes","o":"https://onto.acme.dev/zoo/Breeding","source":"derived","prov":{"rule":"realization-lift","via":["services/horse-factory/breeding/engine.py#foal_male","impl:StallionProduction"]}}
```

`source` is `declared` (ontology files, manifests), `inline` (markers), `inferred` (inferrers such as test paths) or `derived` (rollup rules). Proposals carry `"proposed":true`; confidences `"conf":0.72`. A marker written with the alias `tests` is stored as `verifies` with `"alias":"tests"` in its provenance, so every test link is a `verifies` edge.

Every `file`, `path` and `prov.path` is relative to the repository root, so the graph does not depend on where it is checked out. An imported ontology outside the root keeps its `../` form (`"file":"../payments-api/ontology/pay.gloss.yaml"`).

Special object IRIs: `urn:gloss:new:impl:NEW:Gelding` (a proposed new concept), `urn:gloss:glob:pa:*` (a `may-modify` glob), `urn:gloss:unresolved:x:Y` (unknown prefix).

## Derivation rules

1. **Containment.** An artifact under a manifest is `part-of` that boundary; boundaries chain upward via `part-of`.
2. **Realization lift.** If artifacts in boundary B `implements` X then B `realizes` X, and everything X stands for: the core concepts it realizes or is published as (transitively), the wholes it is `part-of` and what they realize, and what its `is-a` ancestors realize. `provisions`, `configures` and `deploys` on a `resource` or `system-type` concept lift to `realizes` the same way, so infrastructure, configuration and deployment boundaries can declare what they stand up. Kinds are effective kinds: a concept with none inherits its nearest `is-a` ancestor's, recorded as `kind-from` on the node ([ADR 0009](adr/0009-hierarchy-direction.md)). `represents` lifts to `holds` (and to the core concepts the target realizes) whatever the kind: a table that stores an entity's records or a measure's computed values means the boundary holds that data, so a data boundary can declare `holds: [pf:Transaction, wh:CountVariance]` and be compared ([ADR 0013](adr/0013-data-shapes-and-coverage.md)). `presents` (a page) and `hosts` (a shell) lift as themselves, directly from markers and through the concept an artifact implements, and never to `realizes` ([ADR 0001](adr/0001-presents-and-hosts.md)). `configures` lifts as itself on a concept of any kind, so a page that edits a limit's value can be declared and compared ([ADR 0012](adr/0012-surfaces.md)). Intended relations declared on X and on the concepts it realizes (`produces`, `consumes`, `publishes`, `subscribes`) lift to B as well; `represents` and `verifies` (with its alias `tests`) lift as themselves; the `test-paths` inferrer turns `implements` and `represents` under a test path into `verifies`. Every lifted edge records its whole derivation ([ADR 0010](adr/0010-deep-hierarchies.md)): `via` lists the artifact, the concept it links and every concept passed through, and `steps` spells out the hops, so `spw:feeds realizes cyc:CatalogueIngestion` from code implementing `spk:ShimanoXmlImporter` has `steps` `is-a spk:XmlFeedImporter`, `is-a spk:FeedImporter`, `is-a spk:SupplierImporter`, `is-a spk:Importer`, `realizes cyc:CatalogueIngestion`. The two hierarchies lift differently on purpose: the wholes above a part become derived facts of the boundary (implementing a step implements part of the whole), while `is-a` ancestors do not (implementing `ShimanoXmlImporter` is not implementing every `Importer`); an ancestor only passes on what it realizes, and shows in `via` and `steps`.
3. **Taxonomic lift** (`lift: range-lcs`). A set `{Mare, Stallion}` is summarized as its least common subsumer `Horse` in views and in the drift report ([ADR 0011](adr/0011-range-lcs-summary.md)). It is a summary only. The comparison always subsumes: every compared relation is an existential claim ("this boundary produces some `Horse`"), so a declared supertype covers a derived subtype whatever `lift` says. In drift the summary follows the relation's `lift`, `range-lcs` by default for `produces`, `consumes`, `publishes` and `subscribes`, `none` to print members; a violation names the members it summarizes (`rollup: produces {cyc:PoweredDevice} (from cyc:ElectronicRearDerailleur, cyc:FrontLight)`). In a view it follows the view's `lift:`. The lift needs a unique subsumer: when the members share several incomparable minimal subsumers (`ElectronicRearDerailleur` and `ElectronicFrontDerailleur` are both a `Derailleur` and a `PoweredDevice`) nothing is lifted, drift appends `(no unique subsumer: cyc:Derailleur, cyc:PoweredDevice)` to the set, and the projection's edges carry `noUniqueSubsumer`. A curated view never lifts the concepts its `include:` names, since those edges are what it was written to show.
4. **Implied relationships.** A `publishes`/`subscribes` or `produces`/`consumes` pair on one concept between two boundaries implies `depends-on` (consumer → producer) unless declared. Across repositories this is the context-map edge `gloss landscape` draws, in the same direction; it leaves out an edge between a boundary and one that contains it (through `part-of` or a manifest in an enclosing directory).
5. **Generalization.** Views fold implementation into core via `collapse: via realizes`: a concept folds into what it realizes or is published as, else where the whole it is part of folds, else where its `is-a` parent folds, so every concept whose realization closure reaches a core concept is listed in that node's `folded`. A feature slice (`gloss slice`, a `slice:` view) is the other projection: from a seed it follows each relation in one fixed direction ([ADR 0023](adr/0023-feature-slices.md)). Each concept in a projection (`views.json`) carries the `boundary` it sits in, its `placement` (`housed`, `shared`, `flow`, `bound`, `surface` or `unlinked`) and a `placementReason`; see [FILE-FORMATS.md](FILE-FORMATS.md#viewsyaml).
6. **Criticality.** Every concept, boundary and artifact carries an effective `criticality` and its provenance (`criticality-from`, the node it was inherited from, and `criticality-source`, the node at the end of that chain that declares it): declared levels win; implementation concepts inherit from what they realize, boundaries from what their code realizes (else their parent), artifacts from their boundary and the concepts they link to. See `docs/GOVERNANCE.md`.

## Comparison

For each boundary and each of `realizes`, `holds`, `presents`, `hosts`, `configures`, `produces`, `consumes`, `publishes`, `subscribes` that the manifest declares (an explicit empty list such as `realizes: []` included: it declares none, so everything derived is a violation):

| Declared | Derived from code | Result |
| --- | --- | --- |
| `HorseFactory produces Horse` | boundary produces {Mare, Stallion} ⊑ Horse | Consistent |
| `HorseFactory produces Horse` | an agent adds `@gloss produces zoo:Donkey` | **Violation**: boundary exceeds its declared capability |
| `HorseFactory produces Horse` | nothing produces Stallion | **Gap**: intended capability not implemented, or the core is wrong |
| `impl:X realizes zoo:Y` | X has no artifacts | **Orphan** impl concept |
| none | artifact links `impl:NEW:Gelding` | **Rogue** concept; becomes a decision request |

A declared concept covers a derived one when they are equal, when the declared one subsumes it (`is-a*`), or when the derived one realizes it (impl → core, and a part realizes its whole). Both hierarchies point one way ([ADR 0009](adr/0009-hierarchy-direction.md)): a declared whole covers a derived part, as a declared supertype covers a derived subtype, but a declared part never covers a derived whole, so code implementing `OrderFulfilment` under a manifest that declares only its step `ScanConfirm` is a violation, and `ScanConfirm` stays a gap until code implements it or one of its parts. The wholes lifted from a declared part's own code are consistent. Internal concepts and event emitters are exempt from the `realizes` comparison.

Violations are reported by cause ([ADR 0010](adr/0010-deep-hierarchies.md)). A concept that is only derived because another uncovered concept lifted it (the wholes above a part, the cores a subtype realizes) is listed under that concept's finding (`lifted`), so one marker on a part five levels down is one violation, not five. Under `realizes: inherit`, a concept derived from code in a nested boundary is judged where the inheritance ends: walking down from the boundary toward the code, the first boundary that does not inherit. If it declares `realizes`, its own comparison reports the concept and its ancestors stay quiet; if it declares nothing, the omission is one violation at that boundary, naming its manifest and the ancestors that inherit from it. The first violation of a boundary and relation names the declared set and the others refer to it; the full derived set is in `--all` and in the JSON `derived` field.

## Querying

`gloss export --sqlite graph.db` writes tables `nodes`, `edges`, `prefixes` and views `is_a_closure`, `part_of_closure`, `realizes_closure`, `concept_artifacts` (recursive CTEs). Besides the graph's nodes, `nodes` holds a `node='file'` row (id `urn:gloss:file:<path>`) for every in-scope file with no marker beneath a `criticality-floor` boundary, carrying the floor's `criticality` and the floor boundary in `criticality_from`; `graph.jsonl` has no such records. Any SQL client can ask "which artifacts implement anything under zoo:Horse?":

```sql
SELECT a.path, a.symbol FROM concept_artifacts a
JOIN is_a_closure c ON c.sub = a.concept
JOIN nodes h ON h.id = c.super WHERE h.curie = 'zoo:Horse';
```

`gloss mcp` serves the same graph to agents over the Model Context Protocol.
