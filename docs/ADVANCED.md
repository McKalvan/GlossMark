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
| `publishes-value` | ✓ | | | the line below holds a measure's or constraint's published value (`scope=line`); same as `value-from:` in the ontology, and lifts as `implements` (the line is the code that holds the setting) |
| `may-modify` | ✓ | | | an agent's change remit; targets may be globs (`circ:*`); links outside it by that agent are HARD |
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
| `replaced-by` | | ✓ | ✓ | written as the `replaced-by:` key on a deprecated concept or boundary; stored as an edge so views can select and draw it; the successor may be in a namespace known only under `prefixes:` ([ADR 0020](adr/0020-replacements-across-repos.md)) |
| `guards` (`applies-to`) | | ✓ | | a cross-cutting mechanism (authentication, a rate limit, a flag) applies to concepts and the code implementing them; `impact` and `context` show it on both ends, `diff` reviews guarded code at the guard's level, no criticality flows ([ADR 0015](adr/0015-guards.md)) |
| `constrains` | | ✓ | | a rule applies to a concept; `impact` lists the constrained code, `context` names it |
| `reverses`, `attaches-to` | | ✓ | | domain relations between concepts; declare your own under `relations:` |
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

`conf=0.72` (an agent's confidence, drives SOFT vs HARD; on a `gloss suggest` proposal it is the precision that kind of evidence measured on the calibration set, [ADR 0021](adr/0021-suggest-conf-is-measured-precision.md)), `note="why"` (goes to the ledger as the rationale), `id=` (a stable id when the derived one would change), `scope=`. Nothing else goes inline; provenance lives in the ledger.

## Finding kinds (`gloss drift`)

| Kind | Meaning | Fails by default |
| --- | --- | :-: |
| violation | a boundary's code realizes, holds, produces, consumes, publishes or subscribes something its manifest does not declare | ✓ |
| gap | a manifest declares something nothing under it implements | ✓ |
| value-drift | the code a published value is bound to (`value-from`, `publishes-value`) changed and the ontology `value` did not | ✓ |
| dangling | a marker, a manifest relation key or `allowed:` entry, or a relation in the workspace's own ontology files targets a concept that does not exist (in a marker, write `ns:NEW:Name` with `@gloss?` to propose it) | ✓ |
| overdue | a deprecated boundary whose `retire-by` date has passed still has code under it ([ADR 0019](adr/0019-migration-overlap.md)) | ✓ |
| overlap | two boundaries, neither nested in the other, realize or hold the same concept through their own code (not only through a part of it), and no manifest declares one `deprecated` with `replaced-by` the other (listed with `--all`, like consistent pairs; always in JSON) | |
| rogue | a `ns:NEW:Name` proposal: a decision request for the namespace owner | |
| orphan | a concept nothing is built for: no code links to it (a `documents`, `instructs` or `may-modify` link does not count), to a concept that realizes it or is part of it, or to anything below it in `is-a` by any of those routes (an abstract supertype whose subtype is realized by code is not an orphan); a concept that a reached concept `produces`, `publishes`, `consumes` or `subscribes` to in the ontology is reached too, as a marker on that code would make it ([ADR 0028](adr/0028-adoption-diagnostics.md)) | |
| untested | an implemented concept nothing verifies (a test of a core concept counts for an implementation concept realizing it only when that is the sole realizer or the test sits in a boundary holding one of its artifacts). An `internal: true` concept is left out unless its effective criticality is `high` or `critical`; then it is reported, and counted in concept coverage, like any other | |
| presented-only | a page presents the concept, or a shell hosts a concept that is not a system-type, but nothing implements it | |
| stale | an accepted link whose anchored signature changed since acceptance | |
| proposed | an ontology entry with `proposed: true` awaiting `gloss accept --concept` | |
| out-of-range | `gloss landscape` only: an importer resolves a namespace version outside its range (GM106 in that repository's `check`) | |
| deprecated-use | a marker, manifest or concept references a deprecated concept; the line names the `replaced-by` (GM222 in `check`; `gloss export --status` marks the concept `deprecated: true`). It does not fail by default: the migration is under way, and `overdue` enforces a boundary's deadline | |

`gloss landscape` also prints an **Imports** section, one row per importer and namespace: range, locked version, resolved version and a status (`ok`, `not locked`, `lock behind` with the version available and whether it is inside the range, `out of range`, `content changed without bump`). In `--format json`, `repos` holds one object per repository (`path`, `name`, `owns` with each namespace's version, `imports` with those rows), and `imports` lists every row.

`gloss drift --fail-on` and `gloss landscape --fail-on` take a comma-separated list of these kind names, spelled as in the first column (`value-drift`, `presented-only`), or `none`; any of them fails the command when listed, `stale`, `rogue` and `proposed` included, and an unknown kind exits 2 with the list of valid ones. The defaults are `violation,value-drift,gap,dangling,overdue` for `drift` (the ticks above) and `violation,dangling,overdue` for `landscape`. The text trailer leaves file coverage out, so a committed drift output does not change with every file added; `gloss coverage` and `--format json` carry it. `gloss diff --fail-on` takes a gate class instead (`HARD`, `SOFT` or `none`). `policy.waivers` in `glossmark.yaml` silences one finding until a date; its `finding:` is the finding's id, which `gloss drift --ids` prints at the end of each line (and `--format json` as `id`), or a concept CURIE for every finding on it. `gloss check` fails a waiver whose `expires:` is missing or not a `YYYY-MM-DD` date (GM610; such a waiver waives nothing) and warns about one whose `finding:` matches no finding id or concept (GM611) and about one that has expired (GM612). `expires` is inclusive: the waiver holds through that day. `gloss doctor` lists waivers that expired or expire within 30 days. A waiver silences a drift finding only; a retirement deadline belongs in the manifest's `retire-by`, which drift turns into `overdue` (ADR 0019).

## Gates (`gloss diff`)

- **AUTO**: fingerprint-only changes, accepted proposals, derived edges. A changed decorator stack on an unchanged signature (`decorators changed: removed @requires_role("approver")`, [ADR 0014](adr/0014-decorator-stack-in-fingerprint.md)) is AUTO too, but `criticality-gates` raises it.
- **SOFT**: a link to an existing concept, valid, inside the allowed set, at or above the auto-accept threshold, changing only engineering views. The threshold is derived from `suggest.auto-accept-precision` (default 0.9): it is the lowest conf whose proposals at or above it were at least that precise on the calibration data, the repository's own (`suggest.calibration`) when measured, else the shipped one. `policy.soft-conf-threshold` still fixes it when no precision is set. When no conf meets the target, nothing is SOFT on conf ([ADR 0024](adr/0024-auto-accept-precision.md)). The same threshold applies to a `conf=` an agent writes by hand, so an agent's conf is read as its estimated precision.
- **HARD**: a new or proposed concept, a link outside an agent's remit or the allowed set, any ontology or manifest edit, anything touching a published concept, a criticality the policy gates, a MAJOR version bump, a missing or undersized version bump, a cross-repo import, removing the last implementation of a concept anywhere.

A label or `doc:` change on a core concept is HARD and changes every view that shows the concept: the definition is what the business view says. On an implementation concept it is SOFT, unless its file is under `policy.hard-paths` (default `ontology/**`, `**/.gloss.yaml`, `glossmark.yaml`): a change in a file under hard-paths is HARD whatever its semantic class, so the path rule is never lowered ([ADR 0018](adr/0018-gate-classification.md)).

`gloss diff` also prints a **Realization changes** block (`realizations` in JSON): one line per concept whose deepest realizing or holding boundaries changed, such as `hire:Invoicing realized by {bw:hirehq-invoicing} → {bw:hirehq-invoicing, bw:invoicing-service}`. A boundary that loses its last implementation of a concept is a `realization-removed` change, gated at the concept's criticality, and HARD when no boundary realizes or holds the concept any more. A view the change deletes is listed under "Views changed" as `name (removed)`.

## Review states (the ledger)

`proposed` → `accepted` (the `?` was removed, or `gloss accept`) or `rejected` (`gloss reject`); `accepted` → `stale` when the anchor's signature or decorator stack changes (`gloss accept <id>` re-confirms it); any → `removed` when the marker disappears, except `rejected`, which stays the decision. A rejected `@gloss?` line still in the source adds no edge: it does not lift, gate, drift or count toward coverage, and `gloss suggest` does not propose the link again. A `value-from` baseline (`value:<curie>`) is `accepted` or `drifted` (the bound code changed, the published value did not) until `gloss accept --value`; it is never `stale`, so clearing stale links cannot hide a value drift. The ledger belongs in git.

## Concept kinds

`entity`, `process`, `capability`, `event`, `measure`, `constraint`, `resource`, `system-type`. `implements` expects a process, capability, system-type, event, constraint or measure; `represents` an entity, event, measure, resource or constraint; `publishes` and `subscribes` an event (GM221 warns otherwise, on a proposed `@gloss?` link too, and names `represents` when `implements` targets an entity). `hosts` expects a system-type, or a page concept that itself presents or hosts something (GM223 warns otherwise: a CLI command or a router that does the work implements it), and `presents` on a constraint warns (GM224) because a page that edits a limit `configures` it ([ADR 0012](adr/0012-surfaces.md)).

## Feature slices (`gloss slice`)

`gloss slice lib:Cataloging` prints everything relevant to one feature: the seed, its kinds (`is-a`) and parts (`part-of`), the implementation concepts that realize it or are published as it, the code, tests, docs, pages and infrastructure linked to any of those, the boundaries around them, and a faded ring of context (what they produce, consume, publish or subscribe to with the party on the other side, the constraints on them, what they stand for and their `is-a` ancestors). Each node carries its role and the path of edges that brought it in, its criticality and its drift findings; the summary lists what in the slice is untested, so "what is untested in cataloging" is one command.

| Flag | Effect |
| --- | --- |
| *seeds* | concept or boundary CURIEs, artifacts (`path#symbol`), or paths (the concepts their markers link) |
| `--context 0` | drop the context ring |
| `--follow produces,consumes` | follow those flows end to end as members (what members produce or publish; who consumes or subscribes to them) |
| `--format text\|json\|md` | output; `json` is what MCP `slice_of` returns |
| `--repo <root>` (repeatable) | slice the landscape of several repositories, so a subscriber in another repo appears |

Every relation is followed in one direction for one reason, never as a blind N-hop neighbourhood, so a hub concept does not pull in the graph; [ADR 0023](adr/0023-feature-slices.md) has the rules. The same slice is MCP `slice_of`, `gloss context --concept <curie>` (a briefing on a feature rather than a directory), a saved slice in `views.yaml` (`slice: [lib:Cataloging]`, see [FILE-FORMATS.md](FILE-FORMATS.md#viewsyaml)) and the viewer's "Slice on this" (or Alt+click a node; `#slice=lib:Cataloging` in the URL shares it; Esc or the chip's × clears it).

## Files and version numbers

Three kinds of file: `glossmark.yaml` (namespaces, policy), `*.gloss.yaml` (one per namespace, the concepts) and `.gloss.yaml` (one per boundary directory). Four numbers: the tool release, the grammar (`grammar: 1`), the file formats (`"format"` headers) and each namespace's semver. [FILE-FORMATS.md](FILE-FORMATS.md) and [VERSIONING.md](VERSIONING.md) have the details.
