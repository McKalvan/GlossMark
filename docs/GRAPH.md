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

`source` is `declared` (ontology files, manifests), `inline` (markers), `inferred` (inferrers such as test paths) or `derived` (rollup rules). Proposals carry `"proposed":true`; confidences `"conf":0.72`.

Special object IRIs: `urn:gloss:new:impl:NEW:Gelding` (a proposed new concept), `urn:gloss:glob:pa:*` (a `may-modify` glob), `urn:gloss:unresolved:x:Y` (unknown prefix).

## Derivation rules

1. **Containment.** An artifact under a manifest is `part-of` that boundary; boundaries chain upward via `part-of`.
2. **Realization lift.** If artifacts in boundary B `implements` X then B `realizes` X, and everything X stands for: the core concepts it realizes or is published as (transitively), the wholes it is `part-of` and what they realize, and what its `is-a` ancestors realize. `provisions`, `configures` and `deploys` on a `resource` or `system-type` concept lift to `realizes` the same way, so infrastructure, configuration and deployment boundaries can declare what they stand up. `represents` on an `entity` lifts to `holds` (and to the core entities it realizes), so a data boundary can declare `holds: [pf:Transaction]` and be compared. `presents` (a page) and `hosts` (a shell) lift as themselves, directly from markers and through the concept an artifact implements, and never to `realizes` ([ADR 0001](adr/0001-presents-and-hosts.md)). Intended relations declared on X and on the concepts it realizes (`produces`, `consumes`, `publishes`, `subscribes`) lift to B as well; `represents` and `verifies` (with its alias `tests`) lift as themselves; the `test-paths` inferrer turns `implements` and `represents` under a test path into `verifies`.
3. **Taxonomic lift** (`lift: range-lcs`). A set `{Mare, Stallion}` is summarized as its least common subsumer `Horse` in views and in the drift report. Opt-in per relation because it is valid for existential claims and invalid for exclusivity claims.
4. **Implied relationships.** A `publishes`/`subscribes` or `produces`/`consumes` pair on one concept between two boundaries implies `depends-on` (consumer → producer) unless declared. Across repositories this is the context-map edge `gloss landscape` draws.
5. **Generalization.** Views fold implementation into core via `collapse: via realizes`.
6. **Criticality.** Every concept, boundary and artifact carries an effective `criticality` and its provenance (`criticality-from`): declared levels win; implementation concepts inherit from what they realize, boundaries from what their code realizes (else their parent), artifacts from their boundary and the concepts they link to. See `docs/GOVERNANCE.md`.

## Comparison

For each boundary and each of `realizes`, `holds`, `presents`, `hosts`, `produces`, `consumes`, `publishes`, `subscribes` that the manifest declares:

| Declared | Derived from code | Result |
| --- | --- | --- |
| `HorseFactory produces Horse` | boundary produces {Mare, Stallion} ⊑ Horse | Consistent |
| `HorseFactory produces Horse` | an agent adds `@gloss produces zoo:Donkey` | **Violation**: boundary exceeds its declared capability |
| `HorseFactory produces Horse` | nothing produces Stallion | **Gap**: intended capability not implemented, or the core is wrong |
| `impl:X realizes zoo:Y` | X has no artifacts | **Orphan** impl concept |
| none | artifact links `impl:NEW:Gelding` | **Rogue** concept; becomes a decision request |

A declared concept covers a derived one when they are equal, when the declared one subsumes it (`is-a*`), or when the derived one realizes it (impl → core). Internal concepts and event emitters are exempt from the `realizes` comparison.

## Querying

`gloss export --sqlite graph.db` writes tables `nodes`, `edges`, `prefixes` and views `is_a_closure`, `part_of_closure`, `realizes_closure`, `concept_artifacts` (recursive CTEs), so any SQL client can ask "which artifacts implement anything under zoo:Horse?":

```sql
SELECT a.path, a.symbol FROM concept_artifacts a
JOIN is_a_closure c ON c.sub = a.concept
JOIN nodes h ON h.id = c.super WHERE h.curie = 'zoo:Horse';
```

`gloss mcp` serves the same graph to agents over the Model Context Protocol.
