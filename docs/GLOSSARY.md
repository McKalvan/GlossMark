<!-- @gloss documents arch:glossmark scope=file -->
# Glossary

GlossMark's own terms, as `gloss drift`, `gloss check`, `gloss review`, the viewer and the PR comment use them. Each entry is one line of definition and what to do about it. The full rules are in [ADVANCED.md](ADVANCED.md), [GRAPH.md](GRAPH.md) and [GOVERNANCE.md](GOVERNANCE.md).

Contents: [finding kinds](#finding-kinds) · [gates](#gates) · [tiers](#tiers) · [placement](#placement) · [edge provenance](#edge-provenance) · [review states](#review-states)

## Finding kinds

What `gloss drift` reports, under one heading each. `gloss drift --explain` prints each finding with the declared and derived sets, `--format json` carries every field, and `--fail-on` lists the kinds that fail the command ([defaults](ADVANCED.md#finding-kinds-gloss-drift)). A waiver in `policy.waivers` silences one finding until a date; `gloss drift --ids` prints the id it names.

### violation

A boundary's code does something its manifest does not declare: it realizes, produces, consumes, publishes or subscribes to a concept outside its declared list, or a concept it implements intends such a flow.
**What to do:** add the concept to that list in the boundary's `.gloss.yaml`, or move the code to a boundary that declares it. To accept it for now, waive it in `gloss review`.

### gap

A manifest declares something no code under the boundary does.
**What to do:** implement it and mark the code, or remove the declaration. A manifest's `planned:` block declares capability that is not built yet without a gap.

### orphan

A concept nothing is built for: no code links to it (a `documents`, `instructs` or `may-modify` link does not count).
**What to do:** link the code that implements or represents it with a `@gloss` marker (`gloss suggest` proposes them), or delete the concept. Mark it `planned` if it is specified and not built yet.

### rogue

A marker names a concept that does not exist yet (`ns:NEW:Name`): a decision request for the namespace owner.
**What to do:** `gloss propose --from-new` adds a proposed concept for every such marker and rewrites them; or change the marker to an existing concept.

### untested

A concept the code implements (or otherwise reaches, in concept coverage) that no test verifies.
**What to do:** add a `verifies` marker on a test that exercises it.

### presented-only

A page presents a concept, or a shell hosts it, but no code implements it: a surface with no mechanism behind it.
**What to do:** mark the code that implements it, or remove the `presents` link if the page does not really show it.

### stale

An accepted link whose anchored code (its signature or decorator stack) changed since it was accepted.
**What to do:** check that the link still holds, then re-confirm it with `gloss accept <id>`, `gloss accept --stale`, or in `gloss review`.

### value-drift

The code a published value is bound to (`value-from`, `publishes-value`) changed and the ontology's `value:` did not.
**What to do:** update `value:` in the ontology, or confirm the code as is with `gloss accept --value <concept>`.

### dangling

A marker, manifest or ontology entry names a concept that does not exist.
**What to do:** fix the name, or define the concept. In a marker, write `ns:NEW:Name` with `@gloss?` to propose it.

### overdue

A deprecated boundary's `retire-by:` date has passed and code is still under it.
**What to do:** move the remaining code to the boundary named in `replaced-by:`, or extend `retire-by:` in its manifest.

### overlap

Two unrelated boundaries realize or hold the same concept and neither is declared the replacement of the other. Listed with `gloss drift --all`.
**What to do:** usually nothing (a command line and the library it calls realize the same concepts). If one replaces the other, mark the old manifest `deprecated: true` with `replaced-by:` and `retire-by:`.

### unguarded

A handler under a manifest's `guarded-by` lacks the guard decorator the other handlers there carry.
**What to do:** add the guard, or move the handler out of the boundary.

### proposed

An ontology entry with `proposed: true`, awaiting a person's decision.
**What to do:** accept it with `gloss accept --concept <concept>`, or reject it in `gloss review`.

### planned

A plan item: a `planned` concept nothing implements yet, or a manifest `planned:` declaration no code under the boundary does yet. It replaces the orphan or gap the entry would otherwise be.
**What to do:** nothing until the code lands; then it closes.

### planned-closed

A plan item the code now implements, while the entry still says `planned`.
**What to do:** drop the flag with `gloss accept --planned <concept>`, or move the manifest entry from `planned:` to its plain list.

### plan-overdue

A plan item whose `by:` date has passed with nothing implementing it.
**What to do:** implement it, or move the date (and tell whoever the ticket names).

### deprecated-use

A marker, manifest or concept still references a deprecated concept.
**What to do:** move to the concept its `replaced-by` names, then remove the reference.

### out-of-range

`gloss landscape` only: an importer resolves a namespace version outside the range it imports.
**What to do:** widen the range in the importer's `glossmark.yaml`, or lock a version inside it (`gloss lock`).

### consistent

A boundary and relation where what the code derives fits what the manifest declares. Listed with `gloss drift --all`.
**What to do:** nothing.

## Gates

How `gloss diff` and `gloss review` classify a change: who may decide it. See [GOVERNANCE.md](GOVERNANCE.md) and [the gate rules](ADVANCED.md#gates-gloss-diff).

### Gate: AUTO

A change with no decision in it: a fingerprint-only change, an accepted proposal, a derived edge.
**What to do:** nothing; it passes.

### Gate: SOFT

A link to an existing concept, valid, inside the allowed set, at or above the auto-accept threshold, changing only engineering views.
**What to do:** an agent or any reviewer may accept it (`gloss accept --policy` accepts every SOFT proposal).

### Gate: HARD

A change that alters meaning: a new concept, any ontology or manifest edit, a link outside the allowed set or an agent's remit, anything touching a published concept or a gated criticality.
**What to do:** a person decides, usually the namespace or boundary owner. An agent may leave a recommendation in `gloss review`.

## Tiers

### Tier: core

A concept that is implementation-independent by construction: what the business means (`lib:Book`, `clinic:Booking`).
**What to do:** keep implementation detail out of it; changing a core definition is HARD.

### Tier: implementation

A concept for a mechanism (a handler, pipeline, queue) that `realizes` a core concept, or is `internal` and realizes nothing.
**What to do:** give it a `realizes` link, or mark it `internal: true`.

### Tier: instance

A catalogue value (a plan, a region, an enum value) rather than a concept. Instances live outside both tiers; their file format is not built yet.
**What to do:** keep them out of the core: `gloss import --from enums --instances` imports such enums without their values.

### Published

The `published` flag on a concept: the only seam between the tiers and the only thing importable across repositories.
**What to do:** treat a change to it as an API change; it is HARD and may need a version bump.

## Placement

Where the viewer draws a concept. A concept inside a boundary box is housed; the others are drawn outside the boxes, grouped by the first reason that applies, in this order.

### Placement: housed

Inside a boundary box: a boundary's code realizes it, or its manifest declares that it holds it.
**What to do:** nothing.

### Placement: shared

Several unrelated boundaries realize or hold it and no boundary encloses them all.
**What to do:** if they belong together, add a parent manifest that contains them; if one replaces another, declare it.

### Placement: flow

Nothing shown produces or publishes it, or what does shares no enclosing boundary with what uses it.
**What to do:** usually nothing: expected for inputs from outside and for what passes between unrelated systems.

### Placement: bound

Its value is bound to a line of code with `value-from` or `publishes-value`.
**What to do:** nothing.

### Placement: surface

Only pages present it or shells host it.
**What to do:** mark the code that implements it, if any (see [presented-only](#presented-only)).

### Placement: shape

Code represents it, but no boundary declares that it holds it: a copy, a cache or a payload of data owned elsewhere.
**What to do:** if a boundary owns that data, add the concept to its manifest's `holds:`.

### Placement: planned

The concept is planned and no manifest plans it yet.
**What to do:** add it to the `planned:` block of the boundary that will build it.

### Placement: unlinked

No boundary or artifact places it or flows it; usually nothing links to it.
**What to do:** see [orphan](#orphan).

## Edge provenance

Where an edge in the graph comes from. The viewer draws each kind with its own line.

### Edge: declared

Written in an ontology file or a boundary manifest.
**What to do:** edit it there; the change is HARD.

### Edge: inline

From a `@gloss` marker in the code.
**What to do:** edit the marker; its review state lives in the ledger.

### Edge: derived

Written by the rollup rules (for example a boundary realizes what its code implements), never by hand.
**What to do:** change the markers or manifests it comes from; `gloss build` rewrites it.

### Edge: inferred

Added by an inferrer, such as a test path that implies `verifies`.
**What to do:** write a marker if the inference is wrong or missing.

### Edge: lifted

Several links from one boundary to concepts with a common parent, shown as one edge to that parent (its least common subsumer in `is-a`). The originals are listed under `via`.
**What to do:** nothing; open the edge to see the originals.

### Edge: proposed

A proposal (`@gloss?` or a `proposed: true` entry) that no one has accepted yet.
**What to do:** accept or reject it in `gloss review`, or with `gloss accept` and `gloss reject`.

### Edge: planned

Declared only in a manifest's `planned:` block: capability the boundary will have.
**What to do:** nothing until the code lands; then move it to the plain list.

## Review states

The state of each marker in the ledger (`.glossmark/ledger.jsonl`). See [ADVANCED.md](ADVANCED.md#review-states-the-ledger).

### Review: proposed

Written with `@gloss?`, or by an agent or `gloss suggest`, and not decided yet.
**What to do:** `gloss review`, or `gloss accept` / `gloss reject`.

### Review: accepted

A person (or the policy, for SOFT) confirmed the link; its anchor's fingerprint is the baseline.
**What to do:** nothing.

### Review: stale

An accepted link whose anchored code changed since it was accepted.
**What to do:** see [stale](#stale).

### Review: rejected

A person decided the link is wrong. The decision stays even if the marker is removed.
**What to do:** remove the marker (`gloss reject --remove` does both).

### Review: removed

The marker disappeared from the tree without a decision.
**What to do:** nothing.

### Review: drifted

A published value's baseline whose bound code changed while the value did not.
**What to do:** see [value-drift](#value-drift).
