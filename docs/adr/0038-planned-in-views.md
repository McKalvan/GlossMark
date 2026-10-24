<!-- @gloss documents gm:View scope=file -->
# ADR 0038: views show what is planned

Status: proposed, 2026-10-24. Builds on [ADR 0034](0034-ontology-first-increments.md), [ADR 0036](0036-flow-concepts-sit-in-the-box-that-encloses-them.md) and [ADR 0037](0037-holds-is-declared-ownership.md).

## Context

ADR 0034 made `planned` the first thing an increment writes: a planned concept in a tier file, and a manifest's `planned:` block naming the boundary that will build it. Drift reports both as plan items. Views do not see them at all. A projection carries `deprecated` and `proposed`, not `planned`, so the map draws a concept that is planned exactly like one that is built.

The self-host shows the cost. `gm:Increment` is `planned: { ticket: PR 188 }`, and no manifest plans it yet, so it lands in the `unlinked` group with the reason `no boundary realizes, represents, produces, ...`. That reason reads as an orphan. The one fact that explains it, that nothing is meant to implement it yet, is only in the concept's tier file and in drift. The same is true one step later: once a manifest plans the concept, it is housed in that box and looks built.

Ontology-first engineering means the map is read before the code exists. It has to tell the reader which parts are intent.

## Decision

1. **Projections carry the plan.** A concept node carries `planned`, `planBy` and `planTicket` from its tier file. An edge carries `planned` when every graph edge behind it comes from a manifest's `planned:` block (`Prov.Rule == planned`); one plain declaration or one marker beside it makes the edge built.
2. **A new placement, `planned`**: a planned concept that nothing places, flows, binds, presents or represents (`planned (https://.../pull/188); no manifest plans it: add it to the planned: block of the boundary that will build it`). It ranks after `shape` and before `unlinked`, so it replaces only the `unlinked` reason, which was wrong for it. A planned concept that a manifest plans is housed in that box, as any declared concept ([ADR 0034](0034-ontology-first-increments.md) section 1: a planned declaration is an ordinary declaration everywhere but drift).
3. **The viewer draws a planned concept faint**: its fill at a quarter of its usual strength, its outline as usual. A dashed outline means implementation tier and a dotted one deprecated, so the plan is a change of fill, not another line style; a diff ghost stays fully hollow. A plan whose `planBy` has passed gets a dashed ring in the HARD colour, outside the outline: the outline keeps the concept's own colour, which can be red already. The viewer compares `planBy` with the reader's date, so the projection stays a function of the graph and `plan-overdue` keeps its home in drift. A planned edge is drawn at reduced opacity.
4. **The tooltip and the concept pane say so**: a `planned` row with the date, the ticket (a link only when it is an `http(s)` URL), and `overdue` when it is. The legend lists the style, and a **Plan** filter group (`planned`, `overdue`) hides intent to show only what exists, or shows only intent.
5. **View diffs ignore the plan.** `planned` is not compared on nodes or edges, and an edge's key does not change when an entry moves from `planned:` to the plain list. Dropping `planned` stays bookkeeping, AUTO in the gate ([ADR 0034](0034-ontology-first-increments.md) section 2).

## Consequences

- In the self-host, `gm:Increment` moves from `unlinked` to `planned`, a group of its own, with a reason that says what to do. When P2 is curated and a manifest plans it, it moves into that box, drawn faint until `gloss accept --planned` drops the flag.
- A closed plan (code reaches it, `planned-closed` in drift) is drawn faint until the flag is dropped. That state is short: `planned-closed` fails `gloss drift` by default.
- A manifest's `planned:` block has its own `by`; planned edges do not carry it. The concept pane shows the concept's date, and drift remains the place for the block's.
- FILE-FORMATS.md lists the new placement and the projection fields.

## Alternatives considered

- **A dedicated plan view in views.yaml.** Shows intent apart from the boxes that will build it, which is the context a reader needs.
- **Overlay drift's plan items in the viewer.** Drift needs the date and the workspace; the viewer reads only the projection. The flag and date are already on the node.
- **Rank `planned` first among the unplaced values.** A planned concept that another boundary consumes is better explained by its flow, which names the parties; `planned` is for the case where nothing else explains it.
