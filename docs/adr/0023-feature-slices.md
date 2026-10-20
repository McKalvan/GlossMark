<!-- @gloss documents gm:Projection scope=file -->
# ADR 0023: Feature slices follow typed relations in fixed directions

Status: accepted, 2026-10-20. Closes issue #34.

## Context

Someone working on one feature (checkout, cataloging) needs the part of the ontology that belongs to it: the core concept, its kinds and parts, the implementation concepts behind it, their code, tests and docs, the boundaries that hold them, and the events and constraints around them. Viewer focus works only on boundaries, curated views list concepts by hand and keep one hop, search only highlights, and `impact_of`, `concepts_for_path` and `gloss context` go from files to concepts, never from a concept to everything about it.

Following every edge for N hops does not work: hub concepts such as `lib:Item`, or a shared event, reach most of the graph in two hops.

## Decision

A **slice** is computed from one or more seeds by one function, `internal/slice.Compute`, over the canonical graph. Each relation is followed in one direction for one reason. Every node records a **role** and a **via** path: the edges, as they read, from a seed to the node (`circ:CheckoutHandler realizes lib:Checkout`, then `services/circulation/loans/checkout.py#check_out implements circ:CheckoutHandler`). Rounds are breadth-first and sorted, so the first way in is the shortest and the result is deterministic.

**Seeds.** A concept or boundary CURIE; an artifact (`path#symbol`), which stands for the concepts it links to; or a path, whose artifacts' linked concepts become the seeds (what `concepts_for_path` returns). A seed boundary brings in the boundaries inside it (`part-of`, transitively), every concept they realize, hold, represent, present or host (role `housed`) and the artifacts under them.

**Stages.** Stages 1 and 2 repeat until nothing new is added. Stages 3 to 5 run once each, on the result.

| Stage | Adds | Direction | Role |
| --- | --- | --- | --- |
| 1. Core | concepts that `is-a` a member | subtype → member | `kind` |
| | concepts that are `part-of` a member (or that a member `has-part`) | part → whole | `part` |
| 2. Across tiers | concepts `published-as` a member, and what a member is `published-as` | both ways | `published` |
| | concepts that `realizes` a member | realizer → member | `realization` |
| | with `--follow`: what a member `produces` or `publishes`; who `consumes` or `subscribes` to a member | downstream | `flow` |
| 3. Artifacts | artifacts linked to a member by `implements`, `represents`, `verifies`, `documents`, `instructs`, `presents`, `hosts`, `provisions`, `configures`, `deploys`, `may-modify` or `publishes-value` | artifact → member | `artifact`, or `test` when its only links to members are `verifies` |
| 4. Boundaries | boundaries that realize, hold, represent, present, host, produce, consume, publish or subscribe to a member; the boundary of each member artifact; and the boundaries those sit in (`part-of`) | boundary → member | `boundary` |
| 5. Context | see below; never expanded further | one hop | `context` |

Members are every node whose role is not `context`. Boundaries are members drawn as boxes.

**Context** (stage 5, on by default, `--context 0` drops it) explains the feature without belonging to it:

- what a member concept or artifact produces, consumes, publishes or subscribes to, and the producer or consumer on the other side of that flow (`produces` faces `consumes`, `publishes` faces `subscribes`);
- concepts that produce, consume, publish or subscribe to a member;
- constraints and measures that `constrains` a member, and what a member constrains;
- what a member `realizes` and the wholes it is `part-of`: the upward links stages 1 and 2 never follow, so an implementation seed shows the core concept it stands for without pulling in that concept's other realizers;
- `is-a` ancestors, transitively, to show where the feature sits in the taxonomy;
- any other concept-to-concept relation (`reverses`, `attaches-to`, a domain relation) in either direction.

Other repositories' boundaries and concepts reach a slice through the same rules when the slice runs on a union graph: `gloss slice --repo a --repo b` slices the landscape `gloss landscape` builds, so the subscriber of a published event in another repository is context.

**Edges.** The slice keeps every graph edge whose two ends are in it, one per (subject, relation, object), with its source (`mixed` when sources differ).

**Directions are the guard against hubs.** `realizes` is followed only downward (from a member to what realizes it), `is-a` only to subtypes, `part-of` only to parts, and flows only as one hop of context unless `--follow` names them. A member never pulls in a sibling through a shared supertype, a shared whole, a shared flow or a shared boundary.

**Not rules.** Two artifacts in one file, or two concepts in one boundary, are not related by that alone: a router file or a service boundary would merge every feature it touches. A fact a slice should follow belongs in the ontology. The library example states one such fact: `circ:ReturnNotifier part-of circ:ReturnPipeline`, the notifier being the step of checking in that publishes the return.

## Surfaces

All of them call `Compute`, so they return the same nodes for the same seed; `cmd/gloss/slice_test.go` compares the JSON of each.

- `gloss slice <curie|path>... [--context 0|1] [--follow rels] [--format text|json|md] [--repo root]...` prints the slice grouped by role, with each node's kind, tier, criticality, via path and drift findings, and a summary that lists the untested members.
- MCP `slice_of` returns the same JSON. `gloss context --concept <curie>` renders the slice as a budgeted briefing for an agent working on a feature.
- **Saved slices.** A view in `views.yaml` may give `slice: [lib:Cataloging]` (CURIEs or globs) instead of `include:`; the projection shows the slice with context, recomputed on every build. `gloss impact` and MCP `impact_of` name the saved slices a change touches: a changed file whose artifacts or linked concepts are in the slice (sharing a boundary is not enough). `gloss diff` already reports per-view impact, so a PR that changes the slice is reported as changing that view.
- **Viewer.** `gloss view` precomputes the slice (context on) of every concept and boundary into `views.json` as interned index lists (`slices`: `ids`, `roles`, `steps`, and per seed `[node, role, step...]`), so the page filters without a second implementation in JavaScript. "Slice on this" in the panel of a concept, boundary or artifact, or Alt+click on a node, shows only the slice; context is faded; a chip in the breadcrumb shows the active slice with a clear button; `#slice=lib:Cataloging` in the URL makes it shareable. A folded member maps onto the node it folds into; an artifact slices on the concepts it links to. The slice combines with the filters and with Shift+click.

## Size limit

Measured on GlossMark's own map (76 concepts and boundaries, 201 distinct nodes in slices): the index holds about 4,200 integers and 418 via steps, 46 KB of JSON, against 230 KB for the whole `views.json`. The largest slices are the root boundaries (`arch:glossmark`, 199 nodes); a concept slice is 6 to 35 nodes, and `gm:Drift` with context on is 6 nodes (the measure, the three boundaries that produce it, `gm:DriftComparison` and `gm:GlossMark`), which fits on one screen.

`viewer.SliceBudget` caps the index at 300,000 integers, about seventy times GlossMark's own map and roughly 2 MB of JSON. Seeds are indexed in criticality order, highest first, so a capped index keeps the slices that matter most; the page counts the rest as omitted and tells the reader to run `gloss slice` for a seed it lacks. A page served by a process could compute slices on demand instead; the static viewer cannot, so the cap is the fallback.

## Alternatives considered

- **N-hop neighbourhood.** Rejected: see Context.
- **Port the rules to JavaScript.** Rejected: two implementations drift, and the done-when asks that the MCP tool, the saved view and the viewer agree.
- **Co-location as a rule** (artifacts in one file, concepts in one boundary). Rejected, see Not rules.
- **A slice as a new graph record type.** Rejected: a slice is a projection, computed from the graph like a view; nothing about it needs storing.
