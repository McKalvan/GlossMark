<!-- @gloss documents arch:glossmark scope=file -->
# Beyond the starter five

[MARKING.md](MARKING.md) covers `implements`, `represents`, `verifies`, `documents`, `instructs` and `scope=file`, which is most of what a repository writes. This page is the rest, for when a marker needs it.

## Relation profiles

A repository picks how much of the vocabulary it shows with `relations:` in `glossmark.yaml` ([ADR 0041](adr/0041-relation-profiles.md)). `gloss init` writes `relations: starter`: `implements`, `represents`, `verifies`, `produces`, `consumes`, `part-of` and, in ontology files, `is-a` (plus `realizes`, `replaced-by` and `published-as`, which the tier seam itself asks for, and `owns`, `same-as` and `derived-from`, which the tool writes from CODEOWNERS or an import). Under `starter`, the AGENTS.md snippet and the MCP server list only those, and `gloss check` hints at any other relation it finds in a marker, a manifest or an ontology file:

```
web/page.py:3: hint: presents is outside the starter relation profile (implements, represents, verifies, produces, consumes, part-of, is-a); set relations: full in glossmark.yaml to use it, or write one of the starter relations [GM250]
```

A hint is not a warning: `gloss check --strict` ignores it, the edge is built as written, and `gloss build`, `gloss drift` and the gate behave the same under either profile. Set `relations: full` when the repository needs a page (`presents`), a shell (`hosts`), configuration or IaC (`configures`, `provisions`, `deploys`), events (`publishes`, `subscribes`), a rule (`constrains`, `guards`) or the rest of the table below. A repository without the key is `full`.

## Every relation

Inline relations go in `@gloss` markers (artifact → concept or boundary). Ontology relations go in `*.gloss.yaml` files (concept → concept). Manifest relations go in `.gloss.yaml` boundary files (boundary → concept or boundary). Derived relations are written by the tool, never by hand.

| Relation | Inline | Ontology | Manifest | Meaning |
| --- | :-: | :-: | :-: | --- |
| `implements` | ✓ | | | artifact is the mechanism behind a concept; lifts to the boundary as `realizes` |
| `represents` | ✓ | | | artifact is the data shape of a concept (the system of record, a copy, a wire schema); it satisfies a declared `holds` but never derives one ([ADR 0037](adr/0037-holds-is-declared-ownership.md)) |
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
| `produces`, `consumes` | ✓ | ✓ | ✓ | intended (concept, manifest) or actual (artifact) flow of a concept; a concept's lifts to the boundaries implementing it as intent, compared with the manifest but never closing a gap; summarized with `range-lcs`, one lift per subset sharing a subsumer ([ADR 0026](adr/0026-rollup-intent-nesting-inherit.md)) |
| `publishes`, `subscribes` | ✓ | ✓ | ✓ | same for events |
| `realizes` | | ✓ | ✓ | implementation concept → core concept; a boundary → what its code stands for |
| `holds` | | | ✓ | boundary owns a concept's data (an entity's records, a measure's stored values); declared only, and `gloss drift` reports a gap when no code under it represents the concept |
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

[GLOSSARY.md](GLOSSARY.md#finding-kinds) gives each kind in one line with what to do about it. The text output writes each finding that way, with a next step under each heading; `gloss drift --explain` prints the precise message instead, with the declared and derived sets (`rollup: produces {lib:Item} (from ...) ⊄ declared`), which `--format json` always carries as `msg`.

| Kind | Meaning | Fails by default |
| --- | --- | :-: |
| violation | a boundary's code realizes, produces, consumes, publishes or subscribes something its manifest does not declare, or a concept its code implements intends a flow the manifest neither covers nor refines | ✓ |
| gap | a manifest declares something no code under it implements (a concept's intended flow does not count) | ✓ |
| value-drift | the code a published value is bound to (`value-from`, `publishes-value`) changed and the ontology `value` did not | ✓ |
| dangling | a marker, a manifest relation key or `allowed:` entry, or a relation in the workspace's own ontology files targets a concept that does not exist (in a marker, write `ns:NEW:Name` with `@gloss?` to propose it). `gloss check` already rejects every one of these but an `allowed:` entry (GM215, or GM210/GM211 for a target that does not resolve; GM501 only warns about `allowed:`), so a workspace whose `check` passes sees `dangling` only there; drift reports them all for a workspace that skips `check` | ✓ |
| overdue | a deprecated boundary whose `retire-by` date has passed still has code under it ([ADR 0019](adr/0019-migration-overlap.md)) | ✓ |
| planned-closed | a `planned` concept that an `implements` or `represents` link now reaches (directly, through a concept realizing it or through a subtype), or a manifest's `planned:` entry that code under the boundary now derives: drop the key (`gloss accept --planned ns:X`, or move the manifest entry to its plain list) so the flag does not go stale ([ADR 0034](adr/0034-ontology-first-increments.md)) | ✓ |
| plan-overdue | a plan item whose inclusive `by` date has passed with nothing implementing it; the line names the ticket. A warning, not a failure, in `gloss landscape`, where the owning repository is the one to act | ✓ |
| overlap | two boundaries, neither nested in the other, realize or hold the same concept through their own code (not only through a part of it), and no manifest declares one `deprecated` with `replaced-by` the other (listed with `--all`, like consistent pairs; always in JSON) | |
| rogue | a `ns:NEW:Name` proposal: a decision request for the namespace owner | |
| orphan | a concept nothing is built for: no code links to it (a `documents`, `instructs` or `may-modify` link does not count), to a concept that realizes it or is part of it, or to anything below it in `is-a` by any of those routes (an abstract supertype whose subtype is realized by code is not an orphan); a flow a reached concept declares in the ontology (`produces`, `publishes`, `consumes`, `subscribes`) is intent and reaches nothing, as it closes no gap; a marker on the code does ([ADR 0028](adr/0028-adoption-diagnostics.md), [ADR 0026](adr/0026-rollup-intent-nesting-inherit.md)) | |
| untested | an implemented concept nothing verifies, and any other concept in concept coverage that code reaches but nothing verifies (a system-type only configured or deployed, an entity only represented), so every concept coverage counts as uncovered is an orphan, untested, or an implementation concept held back for the untested core concept it realizes ([ADR 0025](adr/0025-deriving-the-ontology.md)) (a test of a core concept counts for an implementation concept realizing it only when that is the sole realizer or the test sits in a boundary holding one of its artifacts). An `internal: true` concept is left out unless its effective criticality is `high` or `critical`; then it is reported, and counted in concept coverage, like any other | |
| presented-only | a page presents the concept, or a shell hosts a concept that is not a system-type, but nothing implements it | |
| unguarded | a handler under a manifest's `guarded-by` lacks the guard's decorator that other handlers there carry ([ADR 0015](adr/0015-guards.md)) | |
| stale | an accepted link whose anchored signature changed since acceptance | |
| proposed | an ontology entry with `proposed: true` awaiting `gloss accept --concept` | |
| planned | a plan item under its own heading, **Plan items**: a `planned` concept nothing implements yet (a `verifies` link alone keeps it open, listed as *verified, not implemented*), or a manifest's `planned:` declaration no code under the boundary derives. It replaces the orphan or gap the entry would otherwise be; `gloss coverage` leaves planned concepts out of the denominator and counts open plan items beside it | |
| out-of-range | `gloss landscape` only: an importer resolves a namespace version outside its range (GM106 in that repository's `check`) | |
| deprecated-use | a marker, manifest or concept references a deprecated concept; the line names the `replaced-by` (GM222 in `check`; `gloss export --status` marks the concept `deprecated: true`). It does not fail by default: the migration is under way, and `overdue` enforces a boundary's deadline | |

`gloss landscape` also prints an **Imports** section, one row per importer and namespace: range, locked version, resolved version and a status (`ok`, `not locked`, `lock behind` with the version available and whether it is inside the range, `out of range`, `content changed without bump`). In `--format json`, `repos` holds one object per repository (`path`, `name`, `owns` with each namespace's version, `imports` with those rows), and `imports` lists every row.

`gloss drift --fail-on` and `gloss landscape --fail-on` take a comma-separated list of these kind names, spelled as in the first column (`value-drift`, `presented-only`), or `none`; any of them fails the command when listed, `stale`, `rogue` and `proposed` included, and an unknown kind exits 2 with the list of valid ones. The defaults are `violation,value-drift,gap,dangling,overdue,planned-closed,plan-overdue` for `drift` (the ticks above) and `violation,dangling,overdue` for `landscape`. The text trailer leaves file coverage out, so a committed drift output does not change with every file added; `gloss coverage` and `--format json` carry it. `gloss diff --fail-on` takes a gate class instead (`HARD`, `SOFT` or `none`). `policy.waivers` in `glossmark.yaml` silences one finding until a date; its `finding:` is the finding's id, which `gloss drift --ids` prints at the end of each line (and `--format json` as `id`), or a concept CURIE for every finding on it. `gloss check` fails a waiver whose `expires:` is missing or not a `YYYY-MM-DD` date (GM610; such a waiver waives nothing) and warns about one whose `finding:` matches no finding id or concept (GM611) and about one that has expired (GM612). `expires` is inclusive: the waiver holds through that day. `gloss doctor` lists waivers that expired or expire within 30 days. A waiver silences a drift finding only; a retirement deadline belongs in the manifest's `retire-by`, which drift turns into `overdue` (ADR 0019).

## Gates (`gloss diff`)

- **AUTO**: fingerprint-only changes, accepted proposals, derived edges. A changed decorator stack on an unchanged signature (`decorators changed: removed @requires_role("approver")`, [ADR 0014](adr/0014-decorator-stack-in-fingerprint.md)) is AUTO too, but `criticality-gates` raises it.
- **SOFT**: a link to an existing concept, valid, inside the allowed set, at or above the auto-accept threshold, changing only engineering views. The threshold is derived from `suggest.auto-accept-precision` (default 0.9): it is the lowest conf whose proposals at or above it were at least that precise on the calibration data, the repository's own (`suggest.calibration`) when measured, else the shipped one. `policy.soft-conf-threshold` still fixes it when no precision is set. When no conf meets the target, nothing is SOFT on conf ([ADR 0024](adr/0024-auto-accept-precision.md)). The same threshold applies to a `conf=` an agent writes by hand, so an agent's conf is read as its estimated precision.
- **HARD**: a new or proposed concept, a link outside an agent's remit or the allowed set, a new or changed `instructs` or `may-modify` link (an agent file's instructions or remit), any ontology or manifest edit, anything touching a published concept, a criticality the policy gates, a MAJOR version bump, a missing or undersized version bump, a cross-repo import, removing the last implementation of a concept anywhere.

A label or `doc:` change on a core concept is HARD and changes every view that shows the concept: the definition is what the business view says. On an implementation concept it is SOFT, unless its file is under `policy.hard-paths` (default `ontology/**`, `**/.gloss.yaml`, `glossmark.yaml`): a change in a file under hard-paths is HARD whatever its semantic class, so the path rule is never lowered ([ADR 0018](adr/0018-gate-classification.md)).

`gloss diff` also prints a **Realization changes** block (`realizations` in JSON): one line per concept whose deepest realizing boundaries changed (a `holds` change is a manifest change, HARD), such as `hire:Invoicing realized by {bw:hirehq-invoicing} → {bw:hirehq-invoicing, bw:invoicing-service}`. A boundary that loses its last implementation of a concept is a `realization-removed` change, gated at the concept's criticality, and HARD when no boundary realizes the concept any more. A view the change deletes is listed under "Views changed" as `name (removed)`.

## Review states (the ledger)

`proposed` → `accepted` (the `?` was removed, or `gloss accept`) or `rejected` (`gloss reject`); `accepted` → `stale` when the anchor's signature or decorator stack changes (`gloss accept <id>` re-confirms it, `gloss accept --stale [path...]` every stale link, or those under the given files or directories); any → `removed` when the marker disappears without a decision, except `rejected`, which stays the decision (`gloss reject --remove` records `rejected`). A review command that rewrites a marker's target (`gloss propose --from-new` turns `ns:NEW:Name` into `ns:Name`, `gloss reject --concept --remove` turns it back) changes the marker's derived id, so it moves the row to the new id and lists the old one in `previous_ids`: one proposal keeps one row ([ADR 0027](adr/0027-review-trail-across-rewrites.md)). A rejected `@gloss?` line still in the source adds no edge: it does not lift, gate, drift or count toward coverage, and `gloss suggest` does not propose the link again. A `value-from` baseline (`value:<curie>`) is `accepted` or `drifted` (the bound code changed, the published value did not) until `gloss accept --value`; it is never `stale`, so clearing stale links cannot hide a value drift. The ledger belongs in git.

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

## History (`gloss history`)

`gloss history` is the gate folded over commits: each step is what `gloss diff` would say about a commit against its parent, so it answers when a concept, boundary or link was added, changed, deprecated or removed, by whom, and at which gate ([ADR 0040](adr/0040-ontology-history.md)). It reads the `graph.jsonl` each commit committed and loads that commit's ontology files, manifests, `glossmark.yaml` and views without scanning code, so each step is classified under the policy of its own commit. Only commits that change the graph, the ledger or those files are steps; the rest change nothing in the ontology. Steps print newest first; a step whose changes are all AUTO is hidden unless `--all`.

| Flag | Effect |
| --- | --- |
| *subjects* | concept or boundary CURIEs to follow; changes, realization moves and version bumps that name them, or a predecessor or successor `replaced-by` links them to |
| `--range <a>..<b>` | the revision range to walk (default `HEAD`, all of its history) |
| `--step commit\|first-parent\|version` | every commit, merges left out (their changes are on the commits they merge; default, or `history.step` in `glossmark.yaml`); the main line with merges; or one step per namespace version bump, with the commits after the last bump as an open step |
| `--all` | also show AUTO-only steps |
| `--verify` | rebuild each step's graphs from their trees, as `gloss diff` does, instead of trusting the committed graphs |
| `--format text\|json` | output; `json` lists each step with its commit, author, status and the impact in the shape of `gloss diff --format json` |

A step is `built` (read from the committed graphs), `verified` (`--verify`), `unbuilt` (ontology files changed and `graph.jsonl` did not: the graph was not rebuilt, or the edit changes nothing in it; its changes otherwise appear on the next commit that rebuilds the graph) or `skipped` with the reason (a graph format this release does not read, or no `glossmark.yaml`). A repository that runs `gloss build --check` in CI has no unbuilt step that hides a change. A shallow clone is reported as truncated; fetch the history (`fetch-depth: 0`) first.

MCP `history_of` (`curies`, `since`, `step`, `all`, `limit`) is the same history for an agent, compacted: each step keeps its commit, author, status, gate, version bumps, realization moves, views changed and its SOFT and HARD changes (AUTO ones too with `all`), at most `limit` steps (default 20, newest first, `more` counts the rest). Without `since` it starts at the merge base with the default branch when HEAD is ahead of it, so on a branch it is the branch's own history; otherwise just before the last commit that changed the `version:` of a namespace the subjects are in (every owned namespace without subjects), so the bump is included. `since` in the response says which.

The PR comment (`gloss diff --format markdown`) ends with **Recent history**: for each concept and boundary the change touches (those of HARD changes first, at most 15), its last 3 SOFT and HARD steps on `--base`, lineage followed, AUTO rows left out. `--history N` sets the count; `--history 0` leaves the section out. The Action fetches the base without `--depth` when the checkout has full history, so check out with `fetch-depth: 0` for all of it; a shallow checkout gets a line saying the history is cut short.

`gloss view --history` embeds the same steps, AUTO ones included, in the viewer (`--history-range` bounds the walk). Its History panel lists every commit newest first with its gate and how many nodes and edges it added, removed and changed across the whole graph. Clicking one overlays that commit on the map as `--diff` does, with the commit's summary above the list: the counts per view, the change kinds, the graph's metrics before and after, version bumps and realization moves. *Map as of this commit* (on by default) hides what newer commits added, so the map is the graph at that commit; what a newer commit removed is no longer in the projection and is only counted. Shift+clicking a second commit overlays the net of the steps between them (decision 8: what is added and removed again inside the range is left out). The toolbar's commit selector and `[` `]` step older and newer; `#commit=<sha>` or `#commit=<older>..<newer>` links to one.

## Files and version numbers

Three kinds of file: `glossmark.yaml` (namespaces, policy), `*.gloss.yaml` (one per namespace, the concepts) and `.gloss.yaml` (one per boundary directory). Four numbers: the tool release, the grammar (`grammar: 1`), the file formats (`"format"` headers) and each namespace's semver. [FILE-FORMATS.md](FILE-FORMATS.md) and [VERSIONING.md](VERSIONING.md) have the details.
