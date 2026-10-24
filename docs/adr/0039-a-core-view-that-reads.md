<!-- @gloss documents gm:View scope=file -->
# ADR 0039: a core view that reads

Status: proposed, 2026-10-24. Builds on [ADR 0010](0010-deep-hierarchies.md), [ADR 0019](0019-migration-overlap.md) and [ADR 0035](0035-components-under-a-flat-package-tree.md).

## Context

The self-host's `concepts` view is the core tier at system zoom, with `gmi:` folded into what it realizes. It drew 42 concepts and 79 edges in one box, and the clusters a reader can see by eye were not in it. Four causes, in order of the edges they cost:

- **The fold drew itself.** `gmi:Linter` realizes `gm:Lint` and four rules, so the view shows it as all five; its own `realizes` edges were then redrawn between every pair of them (`gm:AllowedList realizes gm:Lint`, and back). 22 edges, none in the ontology.
- **Ports on a box with no neighbours.** A box's interface edges (the box produces what it houses) face the boxes beside it. The one system at system zoom has none, so its 19 ports repeated the edges inside it.
- **No structure to lay out.** `gm:` had one `is-a` and no `part-of`: 41 peers.
- **A hub.** `gm:GlossMark produces [Graph, View, Drift, Impact, Landscape]` repeated, from one node, what five processes produce.

## Decision

1. **A fold absorbs the edges it folds along.** When a concept is collapsed (`collapse: via realizes`), its `realizes`, `published-as`, `part-of` and `is-a` edges to the concepts it is shown as are not drawn between them. Its other edges attach to each concept it folds into, as before.
2. **A lone box draws no ports, and opens.** The viewer skips the interface edges of a box with no other box beside it (in its container, or at the top), and a view whose top is one such box opens with it open. Boxes with neighbours keep their ports and start closed.
3. **The core tier has areas.** Six capabilities group every `gm:` concept by `part-of`, named after the ADR 0035 components where they match: `Modelling`, `Reading`, `Checking`, `Reviewing`, `Projecting`, `Adopting`. Entities, events, measures and constraints are parts of the area that produces or enforces them (`Drift part-of Checking`, `LinkAccepted part-of Reviewing`). That is looser than "a piece of"; it reads "belongs to the area of", which is what a reader opening the map wants. `concepts` opens on the six areas; each area expands on click. `architecture` excludes the areas, so each concept stays in the box that builds it.
4. **`gm:GlossMark` produces nothing of its own.** What the system produces is what its processes produce. The areas are not `part-of` it: with one root above them, the view would open on a single node. `concepts` excludes it, since the system box already stands for it. Adding `part-of` to existing concepts is a MAJOR change (VERSIONING.md), so `gm` goes to 1.0.0.

## Consequences

- `concepts` opens on 8 nodes and 10 edges (was 43 and 79). Expanded fully, it draws 33 edges among 48 concepts.
- Drift is unchanged. A part realizes its whole (ADR 0009), so code under a boundary that implements `gm:Lint` also realizes `gm:Checking`; no manifest declares the areas and none needs to.
- A curated view or a slice that reaches a part shows its area as a box around it (the `drift` slice draws `gm:Drift` inside `gm:Checking`).
- `architecture` now opens its one system box, on 31 nodes and 84 edges where it used to open on one closed box. The clutter was there behind the box: 26 core concepts sit at system level, because the command line and the library realize the same ones (an `overlap`, ADR 0019). That is a separate issue.

## Alternatives considered

- **Areas for processes only.** Honest about `part-of`, but 31 of the 42 concepts are not processes; the view would open on about 36 nodes.
- **Group by boundary (component zoom).** The command line and the library both realize most core concepts, so they resolve to the system box; at component zoom 26 concepts stayed there and the edges rose to 202.
- **A view-only grouping key in views.yaml.** Would group the map without saying anything in the ontology; the areas are worth stating once, where agents and other views can read them.
