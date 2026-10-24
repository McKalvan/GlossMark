<!-- @gloss documents gm:Projection scope=file -->
# ADR 0036: Flow concepts sit in the box that encloses their parties

Status: proposed, 2026-10-24. Refines the placement rules of [ADR 0012](0012-surfaces.md) and [ADR 0013](0013-data-shapes-and-coverage.md); assumes issue #192 lands as ADR 0035.

## Context

A view draws a concept inside a boundary box only when the boundary houses it: its code implements or represents the concept. Every other concept is drawn outside all boxes, in a dashed group named for its `placement`. `flow` was the group for concepts a boundary only produces, consumes, publishes or subscribes to, on the reasoning that outputs, inputs and events pass between boundaries and drawing one in the producer's box would hide its consumers.

That holds between unrelated systems, but not inside one. On GlossMark's own graph (with the #192 manifests), every `flow` concept was produced and consumed by boundaries under `arch:glossmark`: `gm:Graph`, `gm:Drift`, `gm:Coverage`, `gm:Impact`, `gm:DecisionRequest` and the `Link*` events, 11 nodes in `architecture` and 10 in `concepts`. In `concepts`, where the system is the only box, all 10 were produced by that one box and still drawn outside it. A first-time reader saw the system's central outputs set apart from the system, with a label ("unplaced · flow") that gave the algorithm's reason, not the domain's. `shared` already handled the same shape for housing candidates by taking their nearest common enclosing boundary; flow links never became candidates, so that rule never reached them.

## Decision

1. **Flow is a placement tier below housing.** A concept no boundary houses is placed by its flows: it goes to the nearest boundary enclosing every shown boundary that produces, consumes, publishes or subscribes to it, by the same `resolveBoundary` rule as housing candidates. A concept with any housing candidate is never placed by its flows, so a gateway that consumes an entity cannot pull it out of the service that represents it.
2. **Its placement is `housed`, and the reason names the flows**: `inside o:sys; published by o:p; subscribed to by o:q`. The box is the answer to "where does this live"; the reason keeps the evidence.
3. **A producer is required; one is enough.** An output only one boundary produces sits inside that boundary; the `produces` edge is still drawn, so the output stays visible when the box is open. A concept that boundaries only consume or subscribe to is never placed by its flows: it comes from outside the view, and housing it in its consumer would draw a cache or an inbound event as the consumer's own.
4. **`flow` now means a seam**: nothing shown produces the concept (an input from outside, `consumed by o:q`), or its parties share no enclosing boundary, and the reason says so (`produced by o:a; consumed by o:b, and no boundary encloses them all`). A party that is not shown in the view (excluded, or hidden by the view's folds) is not counted, as for housing.
5. **Part-of trees, `notBelowParts` and followers apply unchanged.** A concept placed by its flows takes part in them like any housed concept.

## Consequences

- On the self-host, every `flow` concept is housed: `architecture` goes from 66 housed, 11 flow, 1 unlinked to 77 housed, 1 unlinked; `concepts` from 30 housed, 10 flow to 40 housed; `drift` puts `gm:Drift` inside the system.
- In a repository with one root manifest, `flow` essentially never appears. It appears only where the parties have no common boundary: several top-level boundaries in one repository, or a `gloss landscape` of repositories that are not part of one system. That is the intent: it marks the edges a reader should look at across systems. The library landscape is not such a case: `acme:holds-worker` is part of `acme:library-platform`, so `lib:HoldReady` is housed in the platform at system zoom and in the worker at container zoom.
- In the library example, `lib:Laptop` (produced by `acme:circulation-svc`) is housed in the service and `lib:HoldReady` (published by `acme:library-platform`, whose publisher is the holds-worker repository) is housed in the platform. `TestPlacement` covers `flow` with a two-root fixture instead.
- The viewer's legend text for `flow` and FILE-FORMATS.md change with it. `PlacementFlow` keeps its name and JSON value.
- Not addressed: `gm:Ledger` stays `unlinked` in the self-host's `architecture` view although `arch:ledger` holds `gmi:ReviewLedger`, which realizes it. The derived `holds` reaches the core concept but no `represents` does, and only `realizes` and `represents` house. Whether a lifted `holds` should house is a separate question about `rollup.HousingRelations`, which criticality also rolls along.

## Alternatives considered

- **Flow parties as weak housing candidates in the same tier set.** Simpler code, but a concept whose parties share no boundary would then report `shared` with "realized or represented by", which is false.
- **Let any party place it, consumers included.** The first draft did. It houses an inbound event or a cache of another system's entity in its consumer, as if the consumer owned it.
- **Place only when two or more parties share a box; leave a single producer's output in `flow`.** Keeps the original argument for outputs, but the self-host's `concepts` view would still draw ten outputs of the only box outside it, which is the case this ADR exists for.
- **Rename the groups and leave placement alone.** Better wording ("passes between ...") helps the label but not the picture: the system's central concepts would still sit outside the system.
