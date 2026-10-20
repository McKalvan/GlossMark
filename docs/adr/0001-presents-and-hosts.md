<!-- @gloss documents gm:Relation scope=file -->
# ADR 0001: `presents` and `hosts` take the UI and the shell out of `realizes`

Status: accepted, 2026-10-19. Closes issue #8 (review findings 3.2 and 3.11).

## Context

Until now `realizes` was the only way an implementation concept, and the code that implements it, could stand for a core concept. The first two adoptions used it for four different facts:

| Fact | pfinance | AgentArena |
| --- | --- | --- |
| computes it | `pfi:CashFlowEngine realizes pf:CashFlowAnalysis` | grading pipeline |
| shows it | `pfi:CashFlowPage realizes pf:CashFlowAnalysis` | every `*Page.tsx` |
| contains it | three boundaries realize the same concept | containers listing their parts |
| is the shell of it | | every `.tsx` `implements .WebApp`, `aa:WebApp realizes arena:AgentArena` |

Consequences: the business view cannot tell a page from an engine; `untested` cannot say the engine is tested and the page is not; in the viewer almost every concept has several realizers and falls back to the root box (137 of 168 AgentArena concepts); and criticality inherited through `realizes` makes a page as critical as the engine it shows.

"Contains" is solved separately by containment and `realizes: inherit` (#9). This ADR handles "shows" and "is the shell of".

## Decision

Two relations, usable inline, in ontology files and in manifests:

- **`presents`**: a UI surface (page, screen, view, report) for a concept. `pfi:FirePage: { kind: process, presents: pf:FirePlanning }`, or inline `// @gloss presents pf:FirePlanning` on the component.
- **`hosts`**: a shell or container for a concept (the app frame, a router, a plugin host). `aa:WebApp: { kind: system-type, hosts: arena:AgentArena }`.

Rules:

1. Both lift to the boundary as themselves, through the implemented concept and directly from markers. Neither ever lifts to `realizes`, and `RealizesStar` does not follow them.
2. A manifest may declare `presents:` and `hosts:`; `gloss drift` compares them like `realizes`.
3. Criticality does not flow through them. A concept inherits only through `realizes`, `published-as`, `part-of` and `is-a`; an artifact takes no level from a concept it presents or hosts; a boundary is classified by what it realizes or represents.
4. A concept something `presents` but nothing `implements` is a new drift finding, `presented-only`: a surface with no mechanism behind it. `hosts` is excluded, because a shell hosting a whole system-type is expected to have no single implementation. ([ADR 0012](0012-surfaces.md) narrows the exclusion to system-types and page concepts.)
5. An implementation concept that `presents` or `hosts` a core concept satisfies GM320 like one that realizes it.
6. View containment uses `realizes` and `represents` only, so a concept is drawn inside the boundary that computes it, not every boundary that shows it.

## Alternatives considered

- **Drive it from the concept kind** (a `page` kind realizes by presentation). Rejected: it adds a ninth kind for something that is a relation, and a file can be both an engine and a page.
- **A `role:` key on `realizes`.** Rejected: every consumer of `realizes` would have to filter it, and the bug class this ADR removes would come back the first time one forgets.
- **Only an inline marker, no ontology form.** Rejected: pfinance models its pages as implementation concepts with their own docs and tests; the relation belongs where `realizes` is written.

## Migration

`gloss` does not rewrite anything; review these by hand. On an adoption branch:

```sh
grep -rn "@gloss implements .*\(Page\|Screen\|View\|WebApp\|App\)\b" --include=*.tsx --include=*.ts --include=*.vue .
```

- An implementation concept that is a page: replace `realizes: pf:X` with `presents: pf:X`. The `.tsx` marker stays `implements .XPage`.
- The app shell: replace `aa:WebApp: { realizes: arena:AgentArena }` with `hosts: arena:AgentArena`. Markers stay `implements .WebApp`, or become `hosts arena:AgentArena` if the shell concept is removed.
- A component that shows a core concept directly: `// @gloss presents pf:X` instead of `implements pf:X`.
- A manifest that declared `realizes: [pf:X]` only because its pages realized it: move the entry to `presents:`. `gloss drift` reports the old entry as a gap once the pages stop realizing it, which is the reminder.

After migration `untested` lists the pages that have no test (usually acceptable, and visible instead of hidden behind the engine's tests), and `presented-only` lists pages for which no engine exists yet.
