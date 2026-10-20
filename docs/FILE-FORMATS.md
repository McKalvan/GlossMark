<!-- @gloss documents gm:Namespace, gm:Boundary scope=file -->
# File formats

Three file kinds carry the ontology: the repository file, ontology files per namespace, and per-directory boundary manifests. Views are a fourth, optional file. JSON Schemas for all of them live in `schemas/`; the Go types in `internal/model` are the source of truth.

## `glossmark.yaml` (repository root)

```yaml
grammar: 1                                      # @gloss grammar version this repo is written in (default 1)
owns:
  lib:  { iri: https://onto.acme.dev/library/,     file: ontology/lib.gloss.yaml,  tier: core,           version: 1.0.0, owner: library-services }
  circ: { iri: https://onto.acme.dev/circulation/, file: ontology/circ.gloss.yaml, tier: implementation, version: 0.3.0 }
imports:
  acq: { iri: https://onto.acme.dev/acquisitions/, version: "^2.1", source: ../acquisitions/ontology/ }
repo-iri: https://arch.acme.dev/circulation/    # an undeclared boundary prefix p: expands to <repo-iri>boundaries/p/
prefixes:
  acme: https://arch.acme.dev/boundaries/      # non-ontology prefixes (boundaries); optional with repo-iri
views: ontology/views.yaml                      # default
exclude: ["vendor/**"]
comment-styles: { ".pkl": ["//"] }              # extend the comment table
test-paths: ["**/tests/**", "**/*_test.go"]     # inferrer: implements in a test path also verifies
suggest: { stopwords: [widget] }                # gloss suggest: words that never match a concept on their own (added to handler, manager, ...)
source-url: https://github.com/acme/circulation/blob/main/{path}#L{line}   # viewer links paths to the code host
policy:
  soft-conf-threshold: 0.7                      # proposals at or above are SOFT
  hard-paths: ["ontology/**", "**/.gloss.yaml", "glossmark.yaml"]
  coverage-fail-under: 0                        # a percentage, or current (fail only below the base's)
  criticality-gates: { critical: HARD }         # minimum gate by criticality (docs/GOVERNANCE.md)
  waivers:
    - { finding: "gap:acme:library-platform:publishes:lib:HoldReady", owner: alice, expires: 2026-12-31, reason: "published by holds-worker" }  # expires is inclusive
```

- `owns.<prefix>`: a namespace this repo defines. `file` defaults to `ontology/<prefix>.gloss.yaml`. `tier` is `core` or `implementation`. `version` may repeat the ontology file's `version:`, which is the namespace version; `gloss check` fails when they disagree (GM115) and warns when only this one is set (GM114).
- `imports.<prefix>`: a namespace defined elsewhere. `source` is a local path (file or directory) today; `git+` and `https://` sources are parsed and rejected with a message until the fetcher lands (vendor the directory, or point at a sibling checkout). `version` is a range (`^1.2`, `~1.2.3`, `1.x`, `>=1.2 <2.0`) enforced against the resolved ontology's version; `gloss lock` pins the result in `glossmark.lock`. Only **published** concepts of an imported implementation namespace are referenceable (GM230). `tier` (`core` or `implementation`) applies when the imported file states none; a top-level `tier:` in the imported file wins, and `gloss publish` writes one. With neither, the namespace is read as core, and `gloss check` warns (GM113) when it has `published:` or `realizes:` concepts, because the published seam is then not enforced.
- Authoring uses CURIEs; the graph stores IRIs. Concept IDs are permanent: a rename is `deprecated: true` plus `replaced-by:`.
- Versions are semver: MAJOR removes or retypes a concept or changes is-a/part-of; MINOR adds or deprecates; PATCH touches labels and docs. `gloss diff` classifies the bump and gates a missing one HARD.

## `*.gloss.yaml` (one per namespace)

```yaml
namespace: circ
version: 0.3.0
concepts:
  CirculationService: { kind: system-type, realizes: lib:CirculationDesk }
  MarcParser:         { kind: process, part-of: CatalogIntake, internal: true }
  BookIntake:         { kind: process, realizes: lib:Cataloging, produces: [lib:Book] }
  DefaultLoanPeriod:
    kind: measure
    criticality: low                 # a wrong number confuses patrons; see docs/GOVERNANCE.md
    realizes: lib:LoanPeriod
    published: { as: lib:LoanPeriod }
    value: 21
    value-from: { path: services/circulation/loans/policy.py, symbol: LOAN_PERIOD_DAYS }
relations:
  produces: { lift: range-lcs }
```

Top-level keys: `namespace`, `version`, `tier` (optional; an owned file defaults to `owns.<prefix>.tier`, and an importer reads it before `imports.<prefix>.tier`), `doc`, `concepts`, `relations`.

`relations:` tunes a builtin relation or declares a new concept → concept one (`lift`, `transitive`, `inverse`, `domain`, `range`). `lift` only sets how `gloss drift` prints a boundary's derived set for the relation: `range-lcs`, already the builtin for `produces`, `consumes`, `publishes` and `subscribes`, shows it by its least common subsumer; `none` shows the members. It never changes the comparison: a declared supertype covers a derived subtype for every compared relation ([ADR 0011](adr/0011-range-lcs-summary.md)).

`gloss publish` writes `<out>/<ns>/` with `ontology.jsonld`, a copy of this file and, for an implementation namespace, `graph.jsonl`. The copy states `tier:` and, for an implementation namespace, holds only the published concepts, like `ontology.jsonld`; an importer can point `source:` at the directory.

Concept keys:

| Key | Meaning |
| --- | --- |
| `kind` | `entity`, `process`, `capability`, `event`, `measure`, `constraint`, `resource`, `system-type` (required unless `is-a` is given). A concept without one inherits the kind of its nearest `is-a` ancestor that declares one; the graph node records it with `kind-from`, and every kind-dependent rule (`provisions`/`configures`/`deploys` on a resource or system-type lifting to `realizes`, domain and range) uses it. Parents that give different kinds are an error (GM306), and so is no kind anywhere up the chain (GM300). |
| `is-a`, `part-of`, `has-part` | Taxonomy and partonomy. Bare names are in the file's namespace. |
| `realizes` | Implementation → core. Every impl concept carries this or `internal: true` (GM320). |
| `published: { as: <core> }` | The single seam into the business view and across repos. `as:` must name an existing core concept (GM330). |
| `internal: true` | Engineering-private mechanism; realizes nothing, never crosses repos. Left out of concept coverage and the `untested` finding unless its effective criticality is `high` or `critical`. |
| `deprecated`, `replaced-by` | Renames and successors. `replaced-by` is a CURIE or a full IRI; its namespace need not be imported when its prefix is under `prefixes:` (the successor is often in a new repository): `gloss check` reports it (GM341) only when the namespace is owned or imported, and `gloss landscape` reports a dangling successor across repositories. It is stored as a `replaced-by` edge ([ADR 0020](adr/0020-replacements-across-repos.md)). |
| `value` | A concrete value (used by published measures and constraints). |
| `value-from` | `{ path: pfinance/fire.py, symbol: swr }` or `{ path, line: 42 }`: the code the value comes from (the first line that assigns, annotates or defaults the symbol). In a YAML, TOML or JSON file `symbol` is a dotted key path, `{ path: config/ratelimit.yaml, symbol: policies.claims-write.per_minute }`; a bare key or a tail (`claims-write.per_minute`) works when it ends exactly one path, and GM360 names the candidates when it ends several. A list of such locations binds a value duplicated in several places (the Go API and the TypeScript page); a change to any of them is drift, and so is one of several `publishes-value` markers on the same concept. The first build records the line; a later change to it without a change to `value` is a **published value drifted** finding in `gloss drift` (fails by default) and HARD in `gloss diff`. Changing both together moves the baseline; `gloss accept --value ns:X` confirms the code as is. The marker form is `@gloss publishes-value .X scope=line` above the line; it also counts as `implements .X`, so the boundary holding the line realizes X and can declare it ([ADR 0017](adr/0017-published-values-implement.md)). The viewer shows the live line. |
| `proposed: true`, `proposed-by` | The entry is a proposal, the ontology's `@gloss?`. Links to it resolve; it is HARD-gated until `gloss accept --concept` drops the flag. `gloss propose` writes these entries. |
| `aliases` | Other names people and agents use; `search_concepts` matches them. |
| `criticality` | `low`, `medium`, `high` or `critical`: how much a mistake here matters. Implementation concepts inherit the highest level of what they realize when unset. See [GOVERNANCE.md](GOVERNANCE.md). |
| any other key | A relation: `produces`, `consumes`, `publishes`, `subscribes`, `constrains`, `guards` (alias `applies-to`, [ADR 0015](adr/0015-guards.md)), `reverses`, `attaches-to`, or one declared under `relations:`. |

Write `doc` quoted (`doc: "One account (checking, savings)."`) or in block style. Inside a `{ }` flow mapping an unquoted doc ends at its first comma: the rest of the sentence becomes null-valued keys that add no edge, and nothing downstream notices. `gloss check` reports the cut doc (GM305) and any other null key (GM304); `gloss propose` writes quoted docs.

Tier rules: a core file may not reference any implementation namespace (GM310); core concepts realize nothing (GM312). The own-subject rule: a repo may only add edges whose subject is in a namespace it owns, so `hw:CurbsidePickup is-a lib:HoldFulfillment` is allowed and `lib:HoldFulfillment has-part hw:CurbsidePickup` is rejected (GM400). The hierarchies are acyclic: an `is-a` cycle, a self-loop included, is an error (GM370), because every concept on it would subsume every other and widen every declaration that names one; so is a `part-of`/`has-part` cycle in either tier (GM371). Each error names the members in order with the file and line of every edge. GM320 follows `is-a` and `part-of` to a concept that realizes something (or is core, internal, or presents or hosts something); two concepts that are only part of each other do not satisfy it.

## `.gloss.yaml` (boundary manifest, any directory)

```yaml
boundary: acme:circulation-svc
level: container                 # domain | system | container | component
label: Circulation service
part-of: acme:library-platform
realizes: [circ:CirculationService, lib:Cataloging, lib:Return]   # or [inherit, circ:Extra]: what the nested boundaries declare, plus additions
produces: [lib:Media]
holds: [lib:Loan]                # concepts whose data lives here (tables, files); derived from represents
configures: [lib:LoanPeriod]     # concepts whose value this boundary edits at runtime (an admin page)
default-namespace: circ          # .Name in markers below resolves to circ:Name
allowed: [lib:Media, lib:Book, circ:Catalog*, circ:Return*]   # optional narrowing; see below
generated: ["gen/**"]
owner: circulation-eng
criticality: high                # optional; else the highest of what the code realizes, represents or configures, else the parent's (not for a boundary that only presents or hosts)
criticality-floor: true          # optional: the level above is a floor for every artifact beneath this directory
deprecated: true                 # optional: the boundary is being retired
replaced-by: acme:loans-svc      # optional: the boundary taking over, here or in another repository
retire-by: 2027-03-31            # optional: when it is due to go (YYYY-MM-DD)
```

An unknown key is an error (GM110) reported at its own line, but the rest of the manifest still applies: the boundary exists and its files stay in it, so one misspelt key does not move them to the parent boundary. `deprecated`, `replaced-by` and `retire-by` mark a boundary that is being split out or retired; they are carried on the boundary node, and a link into a deprecated boundary is warned (GM222) and HARD in the gate, as for a deprecated concept ([ADR 0007](adr/0007-retiring-boundaries.md)). `deprecated` with `replaced-by` naming another boundary declares a migration: drift reports two unrelated boundaries realizing or holding the same concept as an `overlap` unless such a migration links them, and a deprecated boundary that still has code after its `retire-by` day is `overdue`, which fails `drift` ([ADR 0019](adr/0019-migration-overlap.md)).

The nearest ancestor manifest applies to every file beneath it, as with CODEOWNERS; manifests are optional at every level. `part-of` may name any boundary, not only the one in the parent directory, so a logical parent (a domain, a platform) can live beside its children; that boundary keeps the level, directory and file of its own manifest. A `part-of` cycle among boundaries is an error (GM510): each would roll up the other's code. A boundary named only by `part-of`, with no manifest, is a stub with no level; views place it one level above its children. `realizes`, `holds`, `presents`, `hosts`, `configures`, `produces`, `consumes`, `publishes` and `subscribes` are the declared intents that `gloss drift` compares with what the code derives: `implements` (and `provisions`, `configures`, `deploys` on a resource or system-type) derive `realizes`; `represents` derives `holds` on any kind of concept (a table that stores a measure holds it, [ADR 0013](adr/0013-data-shapes-and-coverage.md)); `configures` lifts as itself on any kind, as well as to `realizes` on a resource or system-type; the rest lift as themselves. A manifest `publishes` or `subscribes` of a concept that is not an event (often after an upstream retype) is warned by `gloss check` (GM225), and a drift gap over code that links to the declared concept only by relations that do not lift names those links. A relation the manifest does not declare is not compared. An explicit empty list is a declaration of none: `realizes: []` on a boundary that is only a surface makes any code under it that implements something a violation (`hpb:storefront realizes hp:Pricing, declared realizes []`), and a `presents` or `hosts` gap over code that implements the concept says so ([ADR 0012](adr/0012-surfaces.md)). A container that only contains its components writes `realizes: inherit` (optionally with additions, `realizes: [inherit, circ:Gateway]`): its declared set is the union of what the boundaries nested inside it declare, so the list is not repeated at every level; gaps in inherited entries are reported by the child that declared them. A nested boundary that realizes concepts but declares no `realizes` of its own is reported once, on that boundary's manifest (`o:web realizes 5 concepts but declares no realizes; o:clinic inherits from it`), not as one violation per concept at the parent. When a boundary has several violations of one relation, the first lists the declared and derived sets and the others name only their concept. A concept inherits `realizes` through `is-a` and a part realizes its whole, so `CsvImporter: { is-a: Importer }` needs no `realizes:` line of its own and code that implements a part of a pipeline satisfies a manifest that declares the pipeline. The reverse does not hold: a manifest that declares a part is not satisfied by code that implements the whole ([ADR 0009](adr/0009-hierarchy-direction.md)).

`allowed` is a deliberate narrowing, not a requirement. When it is absent the allowed set is derived: the `default-namespace`, every concept the manifest declares (`realizes`, `produces`, `consumes`, `publishes`, `subscribes`, `holds`, `presents`, `hosts`, `configures`), every concept a declared one covers in `gloss drift` (its `is-a` and `part-of` descendants, followed along mixed chains, and the implementation concepts that realize it or one of those descendants, so a boundary declaring `cyc:CatalogueIngestion` admits `spk:ShimanoXmlImporter`, which is-a `spk:Importer`, which realizes it), the core concepts a declared implementation concept realizes (not the wholes it is part of), and whatever the boundaries nested inside it allow (a container holds its components). A link outside the derived set is a warning in `gloss check` (GM503) and HARD in the PR gate; outside a written list it is an error (GM502) and HARD. `gloss check --explain` prints the effective set per manifest, grouped by source: a header with the number of entries, then one line each for the default namespace, the declared concepts (each with the number of concepts it covers, `cyc:Product (+17 below)`), the cores realized by a declared implementation concept, the `listed` extras and the nested boundaries (each with the entries it adds). `--format json` prints one record per manifest with every entry and its reason (`default-namespace`, `declared`, `covered`, `realized`, `listed`, `nested`, or `allowed` for a written list) and the concept or boundary it comes `from`. A manifest that declares nothing and names no default namespace restricts nothing. Write `allowed:` only to permit less than the derived set (a list of its own), or to add a concept the boundary uses without declaring it: `allowed: [derived, gm:Confidence]` is the derived set plus `gm:Confidence`, the way `realizes: [inherit, ...]` extends what children declare. `criticality` classifies the boundary for review (see [GOVERNANCE.md](GOVERNANCE.md)); `gloss check` warns when it is declared below a concept the code realizes.

## `views.yaml`

```yaml
views:
  business:
    include: [lib:*]
    collapse: { "circ:*": via realizes }     # circ nodes fold into what they realize
    show-published: true
    relations: [produces, consumes, constrains]
    lift: range-lcs                           # {Book, DVD} shows as Media
    zoom: system
  engineering:
    include: [lib:*, circ:*]
    expand: { part-of: 2 }                    # two levels of parts
    zoom: container
  loan-policy:                                # curated: it names its concepts
    include: [lib:LoanPeriod, lib:Loan, circ:DefaultLoanPeriod]
    zoom: code
  checkout:                                   # a saved slice
    slice: [lib:Checkout]
    zoom: code
```

Two presets ship (business, engineering) and are generated when the file is absent. An unknown key, at the top or in a view, is an error (GM118) at its own line; the rest of the file still applies. Zoom (`landscape`, `system`, `container`, `component`, `code`) decides which boundary levels appear; artifacts appear only at `code`.

A view whose `include:` names at least one concept (not only `ns:*` globs) is **curated**: the viewer opens it with every box open, and it keeps only the artifacts that link to a named concept and the edges with at least one named end.

A view with `slice:` (CURIEs or globs of concepts and boundaries) is a **saved slice**: instead of an `include:` list it shows the feature slice of those seeds, with context, computed by the same rules as `gloss slice` ([ADR 0023](adr/0023-feature-slices.md)) on every build, so it tracks the code instead of a hand-kept list. Its concepts, boundaries and artifacts are the slice's nodes; it is curated, and the viewer opens it with the slice active. `slice:` and `include:` together are an error. The projection repeats the seeds as `slice`, `gloss impact` and MCP `impact_of` name the saved slices a change touches, and `gloss diff` reports a change to the slice as a change to that view.

In a projection every concept also carries the boundary it sits in (`boundary`). Candidates are the boundaries whose code realizes or represents it. A boundary whose own code links to the concept, and whose manifest declares it (`realizes` or `holds` it, or an implementation concept that realizes it), is strongest; then one whose own code links to it without the declaration; then one whose manifest only declares it; then one that reaches it by lifting through other concepts. "Own code" is a marker that names the concept, or names an implementation concept that realizes it in one hop, so `implements hp:OrderPlacement` and `implements .OrderIntake` (which realizes it) count the same, and the CURIE style of a marker never decides the box ([ADR 0012](adr/0012-surfaces.md)). A service that declares `holds: [hp:Order]` keeps the entity when a gateway schema also represents it ([ADR 0013](adr/0013-data-shapes-and-coverage.md)). Among the strongest candidates the deepest wins, or the nearest boundary enclosing all of them when several unrelated ones remain. A `part-of` whole is never placed deeper than one of its parts: when its box lies inside a part's box it moves to the nearest boundary enclosing its own and all its parts' boxes ([ADR 0010](adr/0010-deep-hierarchies.md)), so `cyc:OrderFulfilment` sits in the fulfilment container whose `engine.py` implements `spk:FulfilmentEngine`, not in the leaf component that declares it. Pages that `presents` a concept never place it. The viewer draws a boundary as a box around those concepts, closed at first; the reader opens boxes to drill in. Projections also carry each node's effective `criticality` (the highest of its own and of everything folded into it), and `deprecated`, `replacedBy` (and `retireBy` on boundaries) for what is being retired; the viewer draws deprecated nodes dotted and names the successor in the concept pane. A migration view reads old next to new with `relations: [realizes, holds, replaced-by]`.

A concept no boundary realizes or represents stays **unplaced**: the viewer draws it outside every box, in a dashed group named for its `placement`. Every concept in a projection carries one, with the reason in `placementReason` (such as `produced by acme:circulation-svc`); the first value that applies wins:

- `housed`: inside a boundary box (`boundary` is set).
- `shared`: unrelated boundaries realize or represent it (itself or its parts) and no boundary encloses them all; give them a common parent manifest, or let the concept stay shared.
- `flow`: a boundary `produces`, `consumes`, `publishes` or `subscribes` to it, the expected case for outputs, inputs and events that pass between boundaries.
- `bound`: its value is bound to code with `value-from` or `publishes-value`.
- `surface`: only pages `presents` it, shells `hosts` it or value editors `configures` it.
- `unlinked`: none of the above. Usually nothing links to it at all, which `gloss drift` reports as an orphan; otherwise the reason names the links it has, such as `documents` from code.

Boundaries and artifacts carry no `placement`. The viewer draws a part under its whole when the whole is shown, so only concepts without a shown whole go in a group.

## Tool-maintained files under `.glossmark/`

- `graph.jsonl` — the canonical graph, one JSON record per line, deterministically sorted so `git diff` is meaningful (see `docs/GRAPH.md`). Its first line carries `"format"`; a newer format is refused, not misread.
- `glossmark.lock` (repo root, committed) — resolved version, range, source and content hash per import, written by `gloss lock`.
- `imports/<prefix>@<version>.gloss.yaml` (committed) — the copy of each locked import that `gloss lock` vendors; commands resolve the import from it while its hash matches the lock ([ADR 0002](adr/0002-lock-pins-imports.md)).
- `import-record.jsonl` (committed) — what `gloss import` last read from its source for each concept it wrote: `{ns, name, doc, aliases}` after a `{"type":"header","format":1}` line. A re-run updates a doc or an alias list only when the source changed since, and does not re-add a concept deleted or renamed after an import ([ADR 0016](adr/0016-adoption-scaffold-and-imports.md)). Delete a line to let the importer treat that concept as new again.
- `context.jsonld` — the JSON-LD context that makes `graph.jsonl` RDF-loadable.
- `viewer.html`, `views.js`, `views.json` (not committed) — `gloss view` writes the page, the data script it loads (a plain `<script src>`, so opening the file from disk works) and the same data as JSON for tools; `--inline` embeds the data in one page instead. The data includes `slices`, the precomputed feature slice of every concept and boundary as interned index lists (`ids`, `roles`, `steps`, and `of`: seed → `[node, role, via step...]`), capped in size ([ADR 0023](adr/0023-feature-slices.md)).
- `ledger.jsonl` (committed) — provenance and review state per marker: `{marker_id, path, relation, targets, actor, model, conf, created, status, reviewed_by, reviewed_at, anchor_fingerprint, rationale, review_reason}`. `rationale` is the author's `note=`; `review_reason` is the reviewer's `gloss reject --reason`. It is the only store of accepted, rejected and stale decisions, so it belongs in git like the graph. `actor` is fixed at the marker's first appearance (see `docs/QUICKSTART.md`). Exportable as PROV-O with `gloss export --prov`.
