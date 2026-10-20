<!-- @gloss documents arch:glossmark scope=file -->
# Beyond the starter five

[MARKING.md](MARKING.md) covers `implements`, `represents`, `verifies`, `documents`, `instructs` and `scope=file`, which is most of what a repository writes. This page is the rest, for when a marker needs it.

## Every relation

Inline relations go in `@gloss` markers (artifact → concept or boundary). Ontology relations go in `*.gloss.yaml` files (concept → concept). Manifest relations go in `.gloss.yaml` boundary files (boundary → concept or boundary). Derived relations are written by the tool, never by hand.

| Relation | Inline | Ontology | Manifest | Meaning |
| --- | :-: | :-: | :-: | --- |
| `implements` | ✓ | | | artifact is the mechanism behind a concept; lifts to the boundary as `realizes` |
| `represents` | ✓ | | | artifact is the data shape of a concept; it lifts to `holds` |
| `presents` | ✓ | ✓ | ✓ | a UI surface (page, screen) for a concept; lifts as itself, never to `realizes`, carries no criticality ([ADR 0001](adr/0001-presents-and-hosts.md)) |
| `hosts` | ✓ | ✓ | ✓ | a shell or container for a concept (the app frame, a router); lifts as itself, never to `realizes` |
| `verifies` (`tests`) | ✓ | | | test artifact checks a concept; `tests` is an accepted alias (GM240 asks for `verifies`) |
| `documents` | ✓ | | | Markdown explains a concept or boundary |
| `instructs` | ✓ | | | an agent file's instructions concern a concept or boundary |
| `publishes-value` | ✓ | | | the line below holds a measure's or constraint's published value (`scope=line`); same as `value-from:` in the ontology |
| `may-modify` | ✓ | | | an agent's change remit; targets may be globs (`pa:*`); links outside it by that agent are HARD |
| `provisions` | ✓ | | | IaC artifact provisions a resource or boundary; on a resource or system-type it lifts to `realizes` |
| `configures` | ✓ | | ✓ | a config file, or a page that edits a concept's value at runtime; lifts to the boundary as itself on any kind (and to `realizes` on a resource or system-type), carries the concept's criticality, never places it ([ADR 0012](adr/0012-surfaces.md)) |
| `deploys` | ✓ | | | CI artifact deploys a system-type or boundary; same lift |
| `produces`, `consumes` | ✓ | ✓ | ✓ | intended (concept, manifest) or actual (artifact) flow of a concept; lifted to boundaries with `range-lcs` |
| `publishes`, `subscribes` | ✓ | ✓ | ✓ | same for events |
| `realizes` | | ✓ | ✓ | implementation concept → core concept; a boundary → what its code stands for |
| `holds` | | | ✓ | boundary holds a concept's data (an entity's records, a measure's stored values); derived from `represents` |
| `is-a` | | ✓ | | taxonomy; the closure is computed, a concept inherits `realizes` through it |
| `part-of` | | ✓ | ✓ | partonomy for concepts and boundaries; a part realizes what its whole realizes |
| `has-part` | | ✓ | | inverse of `part-of`, for concepts; a manifest names its whole with `part-of` instead |
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
| untested | an implemented concept nothing verifies (a test of a core concept counts for an implementation concept realizing it only when that is the sole realizer or the test sits in a boundary holding one of its artifacts) | |
| presented-only | a page presents the concept, or a shell hosts a concept that is not a system-type, but nothing implements it | |
| stale | an accepted link whose anchored signature changed since acceptance | |
| proposed | an ontology entry with `proposed: true` awaiting `gloss accept --concept` | |

`gloss drift --fail-on` and `gloss landscape --fail-on` take a comma-separated list of these kind names, spelled as in the first column (`value-drift`, `presented-only`), or `none`; any of them fails the command when listed, `stale`, `rogue` and `proposed` included, and an unknown kind exits 2 with the list of valid ones. The defaults are `violation,value-drift,gap,dangling` for `drift` (the ticks above) and `violation,dangling` for `landscape`. `gloss diff --fail-on` takes a gate class instead (`HARD`, `SOFT` or `none`). `policy.waivers` in `glossmark.yaml` silences one finding until a date. `gloss check` fails a waiver whose `expires:` is missing or not a `YYYY-MM-DD` date (GM610; such a waiver waives nothing) and warns about one whose `finding:` matches no finding id or concept (GM611).

## Gates (`gloss diff`)

- **AUTO**: fingerprint-only changes, accepted proposals, derived edges.
- **SOFT**: a link to an existing concept, valid, inside the allowed set, at or above `soft-conf-threshold`, changing only engineering views.
- **HARD**: a new or proposed concept, a link outside an agent's remit or the allowed set, any ontology or manifest edit, anything touching a published concept, a criticality the policy gates, a MAJOR version bump, a cross-repo import.

## Review states (the ledger)

`proposed` → `accepted` (the `?` was removed, or `gloss accept`) or `rejected` (`gloss reject`); `accepted` → `stale` when the anchor's signature changes (`gloss accept <id>` re-confirms it); any → `removed` when the marker disappears, except `rejected`, which stays the decision. A rejected `@gloss?` line still in the source adds no edge: it does not lift, gate, drift or count toward coverage, and `gloss suggest` does not propose the link again. A `value-from` baseline (`value:<curie>`) is `accepted` or `drifted` (the bound code changed, the published value did not) until `gloss accept --value`; it is never `stale`, so clearing stale links cannot hide a value drift. The ledger belongs in git.

## Concept kinds

`entity`, `process`, `capability`, `event`, `measure`, `constraint`, `resource`, `system-type`. `implements` expects a process, capability, system-type, event, constraint or measure; `represents` an entity, event, measure, resource or constraint; `publishes` and `subscribes` an event (GM221 warns otherwise). `hosts` expects a system-type, or a page concept that itself presents or hosts something (GM223 warns otherwise: a CLI command or a router that does the work implements it), and `presents` on a constraint warns (GM224) because a page that edits a limit `configures` it ([ADR 0012](adr/0012-surfaces.md)).

## Feature slices (`gloss slice`)

`gloss slice pay:Withdrawal` prints everything relevant to one feature: the seed, its kinds (`is-a`) and parts (`part-of`), the implementation concepts that realize it or are published as it, the code, tests, docs, pages and infrastructure linked to any of those, the boundaries around them, and a faded ring of context (what they produce, consume, publish or subscribe to with the party on the other side, the constraints on them, what they stand for and their `is-a` ancestors). Each node carries its role and the path of edges that brought it in, its criticality and its drift findings; the summary lists what in the slice is untested, so "what is untested in withdrawals" is one command.

| Flag | Effect |
| --- | --- |
| *seeds* | concept or boundary CURIEs, artifacts (`path#symbol`), or paths (the concepts their markers link) |
| `--context 0` | drop the context ring |
| `--follow produces,consumes` | follow those flows end to end as members (what members produce or publish; who consumes or subscribes to them) |
| `--format text\|json\|md` | output; `json` is what MCP `slice_of` returns |
| `--repo <root>` (repeatable) | slice the landscape of several repositories, so a subscriber in another repo appears |

Every relation is followed in one direction for one reason, never as a blind N-hop neighbourhood, so a hub concept does not pull in the graph; [ADR 0023](adr/0023-feature-slices.md) has the rules. The same slice is MCP `slice_of`, `gloss context --concept <curie>` (a briefing on a feature rather than a directory), a saved slice in `views.yaml` (`slice: [pay:Withdrawal]`, see [FILE-FORMATS.md](FILE-FORMATS.md#viewsyaml)) and the viewer's "Slice on this" (or Alt+click a node; `#slice=pay:Withdrawal` in the URL shares it; Esc or the chip's × clears it).

## Files and version numbers

Three kinds of file: `glossmark.yaml` (namespaces, policy), `*.gloss.yaml` (one per namespace, the concepts) and `.gloss.yaml` (one per boundary directory). Four numbers: the tool release, the grammar (`grammar: 1`), the file formats (`"format"` headers) and each namespace's semver. [FILE-FORMATS.md](FILE-FORMATS.md) and [VERSIONING.md](VERSIONING.md) have the details.
