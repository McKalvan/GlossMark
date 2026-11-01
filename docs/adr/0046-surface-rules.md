<!-- @gloss documents gm:Link scope=file -->
# ADR 0046: One rule per surface: lint hints and suggest defaults from the ambiguous-surfaces lab

Status: accepted, 2026-10-28. Builds on [ADR 0001](0001-presents-and-hosts.md) and [ADR 0012](0012-surfaces.md). Closes issue #262.

## Context

[ADR 0012](0012-surfaces.md) fixed what the ambiguous-surfaces lab (glossmark-examples, `lab/ambiguous-surfaces`) found broken in the tools: `configures` in manifests, `hosts` restricted to system-types, placement that ignores CURIE style, empty declarations, a surface's level. What it did not do is tell an adopter which relation to write. The lab modelled six ordinary surfaces in four or five ways each and wrote a decision rule in its findings; the project needed a lab to decide how to mark an admin page, a CLI command or a PDF renderer, so every adopter meets the same questions and today decides alone. The two general warnings ADR 0012 added (GM223 on `hosts` of a non-system-type, GM224 on `presents` of a constraint) catch two of the twenty departures the lab tabulated and do not say which surface the file is or which marking the lab recommends for it.

## Decision

1. **One rule per surface, written down.** MARKING.md "Surfaces" states the lab's conclusion for each case as a rule: a server-rendered view that computes `implements` (plus `presents` when it also shows); an admin page that edits a constraint `configures` it, and as a page concept declares its criticality; a CLI command `implements`, its group `hosts` a system-type; a BFF route `presents` the backend's process and implements only the rule it re-applies, on the declaration that applies it; a GraphQL resolver `presents`, the server `hosts` a system-type, a schema type `represents` the entity; a document renderer `produces` the entity. The rules live in one package, `internal/surface`, that `check`, `suggest` and the MCP server share.

2. **Lint recognises the surface and hints.** `internal/surface` detects the surface from the path, the file type and framework signals read from the file (a `pages/` or `views/` directory, `views.py`, `render(request`; `@click.command`, `cobra.Command`, `argparse`; a `bff/` or `gateway/` directory; an import of `graphql` or `@apollo/*`, `.graphql`; an import of `reportlab`, `pdfkit`, `jspdf`, `page.pdf(`). Library signals are read on import lines only, so a file that mentions a library in prose or a pattern is not the surface. A marker that departs from the surface's rule gets one diagnostic, GM255 to GM259, at a new severity, `hint`: it names the surface, the rule (`[surface rule: <lab case>]`) and the marker to write. A hint never fails `gloss check`, not under `--strict` either; it is advice about a choice the lab showed to be costly, not a defect. Where a surface hint fires, the general GM223 and GM224 stay silent, so a marker gets one diagnostic.

3. **The hints follow drift's reading.** GM255 (a surface presents what nothing implements) uses the same satisfaction as `presented-only`: an entity or a concept something `represents` has its mechanism. GM257 (a route or resolver implements the backend's process) fires only on BFF and GraphQL files that present nothing, so a split marking (`presents` on the route, `implements` of the re-applied rule on the helper) is left alone, and only when code under another manifest already implements the process, directly or through an implementation concept. GM258 on `implements` of a constraint fires only when an engine elsewhere enforces it; a page that is the only enforcement is the mechanism.

4. **`gloss suggest` defaults to the rule.** On a recognised surface the role's relation is overridden by the rule: `produces` on a document renderer naming an entity, `configures` on an admin page naming a constraint, `presents` on a page, command, route or resolver naming a core process, capability, constraint or measure that code under another manifest implements. For a concept nothing implements yet the rule has no preference, since only a reader can run the lab's delete test, and the role's relation stands. A surface also prefers the core concept it shows over the engine's implementation concept in a tie, where other code prefers the implementation tier. `configures` and `produces`, which [ADR 0022](0022-what-suggest-proposes.md) left to hand, are proposed on those two surfaces only.

5. **`propose_link` defaults to the rule and explains a departure.** The MCP tool reports `surface` and `surface_rule` for a recognised file, takes the rule's default when `relation` is omitted (`relation_defaulted: true`; an error asks for one when no rule applies), and returns `hint` and `suggested_relation` when the given relation departs from the rule, so an agent can correct itself before writing the marker.

## Verification

For every variant of the lab, the variant that departs from its surface's rule gets exactly one hint naming the rule and the rewrite, and the recommended variant gets none (`internal/e2e/surface_rules_test.go` reproduces each case in miniature; `gloss check` on the lab itself gives the same table). The showcases keep their counts: one hint in clinic-booking, on a page concept that presents a process nothing implements, which `gloss drift` already reports as presented-only.

## Consequences

- `gloss check` has a third severity. Its summary says `N hints` only when there are any; the JSON diagnostic carries `"severity": "hint"`.
- Adopters whose surfaces are marked the lab's losing way see hints on the next check and a rewrite in each.
- Detection is heuristic and conservative: a Go package named `views` is not a page, a file that names a PDF library in a comment is not a renderer. A surface it misses keeps the general GM223 and GM224.
