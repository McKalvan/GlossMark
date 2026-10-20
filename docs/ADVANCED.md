<!-- @gloss documents arch:glossmark scope=file -->
# Beyond the starter five

[MARKING.md](MARKING.md) covers `implements`, `represents`, `verifies`, `documents`, `instructs` and `scope=file`, which is most of what a repository writes. This page is the rest, for when a marker needs it.

## Every relation

Inline relations go in `@gloss` markers (artifact → concept or boundary). Ontology relations go in `*.gloss.yaml` files (concept → concept). Manifest relations go in `.gloss.yaml` boundary files (boundary → concept or boundary). Derived relations are written by the tool, never by hand.

| Relation | Inline | Ontology | Manifest | Meaning |
| --- | :-: | :-: | :-: | --- |
| `implements` | ✓ | | | artifact is the mechanism behind a concept; lifts to the boundary as `realizes` |
| `represents` | ✓ | | | artifact is the data shape of a concept; on an entity it lifts to `holds` |
| `presents` | ✓ | ✓ | ✓ | a UI surface (page, screen) for a concept; lifts as itself, never to `realizes`, carries no criticality ([ADR 0001](adr/0001-presents-and-hosts.md)) |
| `hosts` | ✓ | ✓ | ✓ | a shell or container for a concept (the app frame, a router); lifts as itself, never to `realizes` |
| `verifies` (`tests`) | ✓ | | | test artifact checks a concept; `tests` is an accepted alias (GM240 asks for `verifies`) |
| `documents` | ✓ | | | Markdown explains a concept or boundary |
| `instructs` | ✓ | | | an agent file's instructions concern a concept or boundary |
| `publishes-value` | ✓ | | | the line below holds a measure's or constraint's published value (`scope=line`); same as `value-from:` in the ontology |
| `may-modify` | ✓ | | | an agent's change remit; targets may be globs (`pa:*`); links outside it by that agent are HARD |
| `provisions` | ✓ | | | IaC artifact provisions a resource or boundary; on a resource or system-type it lifts to `realizes` |
| `configures` | ✓ | | | config artifact configures a concept or boundary; same lift |
| `deploys` | ✓ | | | CI artifact deploys a system-type or boundary; same lift |
| `produces`, `consumes` | ✓ | ✓ | ✓ | intended (concept, manifest) or actual (artifact) flow of a concept; lifted to boundaries with `range-lcs` |
| `publishes`, `subscribes` | ✓ | ✓ | ✓ | same for events |
| `realizes` | | ✓ | ✓ | implementation concept → core concept; a boundary → what its code stands for |
| `holds` | | | ✓ | boundary holds an entity's data; derived from `represents` on entities |
| `is-a` | | ✓ | | taxonomy; the closure is computed, a concept inherits `realizes` through it |
| `part-of`, `has-part` | | ✓ | ✓ | partonomy for concepts and boundaries; a part realizes what its whole realizes |
| `published-as` | | ✓ | | written as `published: { as: core:X }`: the only seam between tiers and across repositories |
| `constrains`, `reverses`, `attaches-to` | | ✓ | | domain relations between concepts; declare your own under `relations:` |
| `depends-on`, `calls`, `derived-from`, `same-as` | | | | derived or reserved (imports, SCIP, SSSOM) |

## Scopes

| `scope=` | Anchors to |
| --- | --- |
| *(none)* | the next declaration after the marker; the file when the marker sits in the header comment; else the next block |
| `symbol` | the next declaration, and a warning (GM122) when none is detected |
| `file` | the whole file (fingerprint of its content, leaving out lines that carry `@gloss` and the `gloss:begin`…`gloss:end` block, ADR 0006) |
| `block` | the next indented or braced block, whatever it is |
| `line` | the next line |
| `range` | a range named in `note=` (sidecars for JSON use JSON Pointers) |

## Marker keys

`conf=0.72` (an agent's confidence, drives SOFT vs HARD), `note="why"` (goes to the ledger as the rationale), `id=` (a stable id when the derived one would change), `scope=`. Nothing else goes inline; provenance lives in the ledger.

## Finding kinds (`gloss drift`)

| Kind | Meaning | Fails by default |
| --- | --- | :-: |
| violation | a boundary's code realizes, holds, produces, consumes, publishes or subscribes something its manifest does not declare | ✓ |
| gap | a manifest declares something nothing under it implements | ✓ |
| value-drift | the code a published value is bound to (`value-from`, `publishes-value`) changed and the ontology `value` did not | ✓ |
| dangling | a marker targets a concept that does not exist (write `ns:NEW:Name` with `@gloss?` to propose it) | ✓ |
| rogue | a `ns:NEW:Name` proposal: a decision request for the namespace owner | |
| orphan | a concept no artifact links to | |
| untested | an implemented concept nothing verifies | |
| presented-only | a page presents the concept but nothing implements it | |
| stale | an accepted link whose anchored signature changed since acceptance | |
| proposed | an ontology entry with `proposed: true` awaiting `gloss accept --concept` | |

`--fail-on` chooses which kinds fail the command; `policy.waivers` in `glossmark.yaml` silences one finding until a date.

## Gates (`gloss diff`)

- **AUTO**: fingerprint-only changes, accepted proposals, derived edges.
- **SOFT**: a link to an existing concept, valid, inside the allowed set, at or above `soft-conf-threshold`, changing only engineering views.
- **HARD**: a new or proposed concept, a link outside an agent's remit or the allowed set, any ontology or manifest edit, anything touching a published concept, a criticality the policy gates, a MAJOR version bump, a cross-repo import.

## Review states (the ledger)

`proposed` → `accepted` (the `?` was removed, or `gloss accept`) or `rejected` (`gloss reject`); `accepted` → `stale` when the anchor's signature changes; any → `removed` when the marker disappears. A `value-from` baseline (`value:<curie>`) is `accepted` or `drifted` (the bound code changed, the published value did not) until `gloss accept --value`; it is never `stale`, so clearing stale links cannot hide a value drift. The ledger belongs in git.

## Concept kinds

`entity`, `process`, `capability`, `event`, `measure`, `constraint`, `resource`, `system-type`. `implements` expects a process, capability, system-type, event, constraint or measure; `represents` an entity, event, measure, resource or constraint; `publishes` and `subscribes` an event (GM221 warns otherwise).

## Files and version numbers

Three kinds of file: `glossmark.yaml` (namespaces, policy), `*.gloss.yaml` (one per namespace, the concepts) and `.gloss.yaml` (one per boundary directory). Four numbers: the tool release, the grammar (`grammar: 1`), the file formats (`"format"` headers) and each namespace's semver. [FILE-FORMATS.md](FILE-FORMATS.md) and [VERSIONING.md](VERSIONING.md) have the details.
