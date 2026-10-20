<!-- @gloss documents gm:DeclaredCapability scope=file -->
# ADR 0012: Surfaces say what they do: configures, placement, empty declarations and levels

Status: accepted, 2026-10-20. Builds on [ADR 0001](0001-presents-and-hosts.md). Closes issue #51 (F-SURF-5).

## Context

ADR 0001 took pages and shells out of `realizes` with `presents` and `hosts`. The ambiguous-surfaces experiment (glossmark-examples, `lab/ambiguous-surfaces`) marked the same code five ways in six kinds of surface (a server-rendered page, a CLI, a BFF, an admin console, a GraphQL gateway, a PDF renderer) and found the places where the outcome depended on how a marker was spelled rather than on what the code does.

An admin page that edits a constraint's value is neither the mechanism nor only a view of it. `configures` says exactly that, and of the relations tried it is the only one that keeps the constraint in its engine's box while giving the page the constraint's level. But a manifest could not declare it (GM110), so the page's link and its test were outside the derived allowed set (GM503, HARD in the gate), and on a constraint it lifted nothing to the boundary, so `drift` never compared it.

## Decision

1. **`configures` is a manifest relation.** A manifest may declare `configures: [hp:CreditLimit]`. It is in the derived allowed set like every declared relation, and `gloss drift` compares it like `presents`.
2. **`configures` lifts as itself on any concept kind.** Code that `configures` a concept makes its boundary `configure` it. On a resource or system-type it still also lifts to `realizes`, as before.
3. **`configures` carries criticality and never places.** The artifact takes the level of the concept it configures (a mistake on the page changes the rule); the concept is still drawn in the box of the boundary that realizes or represents it. A concept only configured, presented or hosted is placed `surface`.
