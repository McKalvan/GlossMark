<!-- @gloss documents gm:Relation scope=file -->
# ADR 0025: Mapping sets — cross-namespace equivalences with a review lifecycle

Status: **proposed** (scope), 2026-10-21. Fills the "SSSOM mappings" item that PLAN.md lists as deferred in Phase 3.

## Context

Two teams name the same thing differently: `zoo:Horse` and `farm:Equine`, or `pay:CaptureConfirmed` and `ledger:PaymentCaptured`. The design says the landscape should read "an SSSOM mapping file for cross-team equivalences (`zoo:Horse skos:exactMatch farm:Equine`) rather than forcing one global vocabulary". None of it exists. `vocab.go` reserves a `same-as` relation that nothing emits. The consequences today:

- `gloss landscape` unions graphs by IRI only. A repository that publishes `zoo:FoalBorn` and one that subscribes to `farm:FoalBirth` get no context-map edge, and cross-repo drift cannot see that they agree.
- Nothing records that two concepts mean the same thing, so `gloss propose` and `gloss suggest` happily mint a duplicate of a concept that already exists under another team's name.
- Most important: when an ontology change is proposed on one side (a definition rewritten, a concept deprecated, retyped or removed), nothing tells the people who rely on an equivalence that its meaning moved. A mapping silently goes wrong.

## Decision (proposed)

### 1. Format: standard SSSOM/TSV, no extensions

A mapping set is an SSSOM/TSV file with an embedded metadata block, conventionally `mappings/<set>.sssom.tsv`, listed in `glossmark.yaml`:

```yaml
mappings:
  zoo-farm: { file: mappings/zoo-farm.sssom.tsv, owner: zoo-product }
```

```
# mapping_set_id: https://onto.acme.dev/mappings/zoo-farm
# mapping_set_version: 1.2.0
# license: https://creativecommons.org/publicdomain/zero/1.0/
# creator_id: [team:zoo-product]
# curie_map: { zoo: https://onto.acme.dev/zoo/, farm: https://onto.acme.dev/farm/, skos: http://www.w3.org/2004/02/skos/core#, semapv: https://w3id.org/semapv/vocab/ }
subject_id	predicate_id	predicate_modifier	object_id	mapping_justification	author_id	confidence	reviewer_id	review_date	comment
zoo:Horse	skos:exactMatch		farm:Equine	semapv:ManualMappingCuration	human:alice		human:bob	2026-10-20
zoo:Foal	skos:exactMatch		farm:Colt	semapv:LLMBasedMatching	agent:claude-code@model	0.71			proposed by an agent
zoo:Mare	skos:exactMatch	Not	farm:Equine	semapv:MappingReview	agent:claude-code@model	0.64	human:bob	2026-10-20	a mare is narrower than equine
```

Only standard SSSOM slots are written. SSSOM readers must discard unknown columns, so GlossMark-specific state (fingerprints, gate history) lives in the ledger, as it already does for markers. The external metadata mode (`<set>.sssom.yml` beside the TSV) is read too. `sssom-py` and other SSSOM tools can open every file GlossMark writes.

### 2. Ownership: extend, never mutate

A mapping is a separate statement about two concepts. It never becomes an edge in either concept's ontology file, so the own-subject rule is not bent.

- **Repository sets.** Every row has at least one endpoint in a namespace the repository owns, written as the `subject_id`. The other endpoint is imported or named under `prefixes:` (no import needed, as for `replaced-by`, ADR 0020).
- **Landscape sets.** A set where neither side is owned lives in a registry repository (or any repository that declares it `scope: landscape`). It applies only in `gloss landscape`, never in a single repository's drift.
- The seam still holds: an endpoint that is an unpublished concept of an imported implementation namespace is an error, as GM230 is for any reference.

### 3. Lifecycle: the SSSOM fields carry the state, the ledger tracks it

| State | In the file | In the ledger (`mapping:<subject>\|<predicate>\|<object>`, or the row's `record_id`) |
| --- | --- | --- |
| proposed | no `reviewer_id` | `proposed`, author, justification, confidence |
| accepted | `reviewer_id` and `review_date` set | `accepted`, reviewer, both endpoint fingerprints at review time |
| rejected | `predicate_modifier: Not` with a `reviewer_id` (a reviewed negative mapping) | `rejected`, `review_reason`; sticks as in ADR 0005 |
| stale | unchanged | `stale`: an endpoint's fingerprint changed since review (section 4) |
| removed | row deleted | `removed`; a rejected row stays `rejected` |

A rejection is written as a negative mapping rather than deleted. It then travels with the file, other SSSOM tools understand it, and `gloss map suggest` never proposes the pair again. Re-accepting a stale mapping rewrites `review_date` and the ledger fingerprints.

### 4. Tracking ontology changes through mappings

This is the core of the feature. Each endpoint has a **concept fingerprint**: a hash of its kind, label, `doc`, `is-a` parents, `published-as`, and `deprecated`/`replaced-by`. Accepting a mapping records both fingerprints. Then:

| Change to a mapped concept | Effect |
| --- | --- |
| definition, label, kind or `is-a` changes | The mapping goes `stale`. `gloss diff` on the PR making the change adds a `mapping-affected` row naming the mapping, its set and the set's owner, so the author sees the impact **before** the change merges. |
| deprecated with `replaced-by` | `stale`, plus a re-point suggestion: `zoo:Horse exactMatch farm:Equine` → `zoo:Equid exactMatch farm:Equine`. |
| removed | The mapping dangles. Error in the set's repository; HARD in the diff. |
| `proposed: true` concept | May appear only in proposed mappings. Accepting the concept does not accept its mappings. |
| import moved by `gloss lock --update` | Compares the locked copy (`.glossmark/imports/`, ADR 0002) with the new one and lists every mapping whose foreign endpoint changed between the two versions. |

`subject_source_version` and `object_source_version` are written at review time for readers of the file. The fingerprint, not the version number, decides staleness, so a namespace bump that does not touch a mapped concept stales nothing.

### 5. Gates

| Change | Gate |
| --- | --- |
| proposed row, `skos:exactMatch`, `owl:equivalentClass`, `skos:broadMatch`, `skos:narrowMatch` | HARD: these change what the landscape derives |
| proposed row, `skos:closeMatch`, `skos:relatedMatch` | SOFT: search, viewer and briefing only |
| row accepted or rejected (reviewer added) | AUTO, bookkeeping |
| `mapping-affected` (an endpoint changed) | SOFT; HARD for a semantic predicate when either endpoint's criticality gate says so |
| endpoint removed, mapping dangles | HARD |

`criticality-gates` apply to the higher level of the two endpoints, so a mapping touching a critical concept always reaches a human. Mapping files sit under `mappings/`, outside the default `hard-paths`, so they are classified by meaning rather than by path (ADR 0018).

### 6. What a mapping changes downstream

| Predicate | Effect |
| --- | --- |
| `skos:exactMatch`, `owl:equivalentClass` | In `gloss landscape`: publishes/subscribes and produces/consumes pairs join across the mapping, giving context-map edges; cross-repo drift treats the two as the same concept. Everywhere: `search_concepts` returns the counterpart, the viewer draws an equivalence edge, the briefing lists it. |
| `skos:broadMatch`, `skos:narrowMatch` | Act as `is-a` for taxonomic lift in the landscape only when `mappings.lift: true`; otherwise informational. |
| `skos:closeMatch`, `skos:relatedMatch` | Informational: search, viewer, briefing. |
| any predicate with `Not` | Suppresses suggestions; a check warns where another set asserts the negated pair. |

Mappings never merge nodes, never change a single repository's own drift, and never cross the published seam. In `graph.jsonl` an accepted mapping is a `same-as` (or `broader`/`narrower`/`related`) edge with `source: "mapping"` and provenance naming the set and row. That is an additive value, so the graph format stays 1 unless consumers turn out to depend on a closed set of `source` values.

### 7. Checks (GM7xx, unused today)

| Code | Finding |
| --- | --- |
| GM700 | a set file fails SSSOM/TSV parsing, or lacks `mapping_set_id`, `license` or a `curie_map` entry it uses |
| GM701 | an endpoint does not exist (dangling) |
| GM702 | a repository set row with no owned endpoint, or with the owned endpoint as object |
| GM703 | an endpoint is an unpublished concept of an imported implementation namespace |
| GM704 | the same pair asserted both positively and with `Not` |
| GM705 | an `exactMatch` between concepts of one namespace (a duplicate: use `replaced-by`) |
| GM706 | an `exactMatch` whose endpoints have different kinds (warning) |
| GM707 | a concept with `exactMatch` to two concepts in one namespace (cardinality warning) |
| GM708 | a stale mapping (warning; `--strict` fails) |

### 8. Commands and agent tools

```sh
gloss map propose zoo:Foal skos:exactMatch farm:Colt --set zoo-farm --justification llm --conf 0.71
gloss map accept  zoo:Foal skos:exactMatch farm:Colt        # writes reviewer_id and review_date
gloss map reject  zoo:Mare skos:exactMatch farm:Equine --reason "a mare is narrower"   # writes predicate_modifier Not
gloss map status                                             # proposed, stale and rejected rows
gloss map suggest --against farm                             # lexical, alias and doc matching, calibrated
gloss map export --format rdf|json                           # for sssom-py and RDF tools
```

`gloss map suggest` reuses the calibration harness of ADR 0021 and 0024, so its `confidence` is measured precision like every other suggestion. MCP gains `propose_mapping`. `search_concepts` returns mapped counterparts. `propose_concept` warns when the new name matches an existing concept in another namespace, by name, alias or mapping, and offers a mapping instead of a duplicate.

### 9. Versioning

`mapping_set_version` is semver and `gloss diff` classifies it: a row added is MINOR, a row removed or its predicate changed is MAJOR, a comment or label is PATCH. A repository can import another repository's mapping set by version range and lock it, reusing ADR 0002 (copy at `.glossmark/imports/<set>@<version>.sssom.tsv`). The ledger format stays 1: `mapping:` entries are additive, like `concept:` and `value:`.

## Phasing

Estimates are judgement for one engineer, not measurements.

| Milestone | Delivers | Size |
| --- | --- | --- |
| M1 Format and lifecycle | SSSOM/TSV reader and writer, `mappings:` in `glossmark.yaml`, ownership orientation, GM700–GM707, ledger `mapping:` entries, `gloss map propose/accept/reject/status` | about 1 week |
| M2 Change tracking | concept fingerprints, `stale` (GM708), `mapping-affected` diff rows naming owners, re-point on `replaced-by`, the `lock --update` report | about 1 week |
| M3 Semantics | landscape context-map and drift through `exactMatch`, opt-in lift through broad/narrow, viewer edges, search and briefing | about 1 week |
| M4 Agents and interop | `gloss map suggest` with calibration, MCP `propose_mapping`, duplicate warning on `propose_concept`, RDF/JSON export, a round-trip test against `sssom-py` | 1 to 1.5 weeks |

M1 and M2 together answer the original need: a proposed ontology change shows which mappings it affects, and those mappings cannot silently stay accepted.

## Out of scope

Reading SSSOM/JSON or RDF (TSV only; export covers the other direction), inferring new mappings by chaining (`semapv:MappingChaining`), merging nodes, instance-level mappings, and the Backstage ownership join.

## Open questions

1. **Where cross-team sets live.** In one of the two repositories, oriented by the own-subject rule; in a registry repository for landscape-only sets; or both. Recommended: both, as in section 2.
2. **How far `exactMatch` reaches in M3.** Context map and search only, or also cross-repo drift. Recommended: both in the landscape, never in a single repository's drift.
3. **Who approves.** The set owner alone, or the owners of both namespaces. Recommended: the set owner approves; the diff names the other namespace's owner and CODEOWNERS on the set file can require them.
4. **Gate for `closeMatch` and `relatedMatch` proposals.** SOFT, so `accept --policy` may accept them; or HARD like the rest.

## Alternatives considered

- **Mapping keys in `*.gloss.yaml`** (`exact-match: farm:Equine` on a concept). Rejected: it loses SSSOM interop, cannot express a negative mapping, a justification or a confidence, puts review state into the ontology file, and cannot hold a landscape set where neither side is owned.
- **A GlossMark YAML mapping format.** Rejected for the same interop reason; SSSOM/TSV is the established exchange format and already has the review fields.
- **Aliases only.** `aliases:` already serve search inside one namespace; they say nothing about another team's concept and carry no review state.
- **Merge mapped nodes in the landscape.** Rejected: it erases ownership, and a wrong mapping would silently corrupt every downstream view.
