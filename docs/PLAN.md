<!-- @gloss documents arch:glossmark scope=file -->
# GlossMark build plan

Source: "GlossMark — Design v2" (claude.ai doc, revision 140, 2026-10-17). This plan turns that design into a shippable toolchain. It records the analysis of the spec, the architecture chosen, what is built in this repository today, and what is deliberately deferred.

## 1. What the spec asks for

GlossMark is a comment-native grammar (`@gloss`) plus a two-tier ontology (core, implementation) that compile into one canonical graph. Tooling reads only the graph, never the source. Five mechanisms carry the whole design:

| Mechanism | Spec section | What it needs from the tooling |
| --- | --- | --- |
| Two tiers + instances | Two tiers plus instances | Namespace carries `tier`; impl concepts must `realizes:` or be `internal: true`; core files may not reference impl namespaces |
| Namespace modularity | Namespaces are the unit of modularity | `glossmark.yaml` with `owns`/`imports`; CURIE→IRI resolution; permanent IDs; semver classification |
| Published seam | The published seam | `published: { as: core }`; business-view surfacing; cross-repo referenceability; HARD gate |
| Extend, never mutate | Extension rule | Own-subject check on every declared edge |
| Views as projections | Views are projections | `views.yaml`; tier × zoom projection engine; PR impact = views changed |

Everything else (markers, metamodel, rollup, workflow, repository layout, tooling) is the machinery that produces and consumes the graph.

## 2. Architecture

One Go module, one binary (`gloss`), no runtime services. The spec names "Go or Rust single binary"; Go was chosen for fast builds, trivially embeddable assets (the HTML viewer), and pure-Go SQLite.

```
glossmark.yaml, ontology/*.gloss.yaml, **/.gloss.yaml, inline @gloss markers
        │
        ▼  internal/load      (workspace: config, ontologies, imports, manifests, file walk)
        ▼  internal/marker    (grammar: regex, comment table, SPDX sugar, diagnostics)
        ▼  internal/extract   (pass 1: markers → artifact nodes + edges, anchors, fingerprints)
        ▼  internal/graph     (canonical graph: IRIs, closures, deterministic graph.jsonl)
        │
        ├─ internal/check     gloss check     (lint: syntax, refs, domain/range, own-subject, tiers)
        ├─ internal/rollup    gloss drift     (rules 1–4, declared-vs-derived, orphans, rogues, metrics)
        ├─ internal/ledger    gloss accept    (review states, stale detection, provenance)
        ├─ internal/gate      gloss diff      (AUTO/SOFT/HARD, per-view impact, PR comment)
        ├─ internal/views     gloss view      (projections → static HTML viewer)
        ├─ internal/store     gloss export    (SQLite with recursive CTE closures, JSON-LD context)
        ├─ internal/mcp       gloss mcp       (stdio MCP server: 5 tools)
        └─ internal/agents    gloss agents    (AGENTS.md vocabulary snippet)
```

### Canonical graph format

`.glossmark/graph.jsonl`, one JSON object per line, sorted by (`type`, `id`/`s`, `p`, `o`, `path`, `line`) so `git diff` is meaningful.

- Line 1: `{"type":"context","prefixes":{...},"generator":"gloss x.y"}` — doubles as the JSON-LD context seed.
- Node lines: `{"type":"node","node":"concept|boundary|artifact|namespace|actor","id":"<IRI>","curie":"zoo:Mare","ns":"zoo","tier":"core","kind":"entity",...}`
- Edge lines: `{"type":"edge","s":"<IRI>","p":"is-a","o":"<IRI>","source":"declared|inline|derived|inferred","prov":{...}}`

IRIs are stored, as the spec requires; CURIEs are kept beside them for readability. Artifact IRIs are `urn:gloss:artifact:<repo-relative-path>[#<symbol>]`. Boundary prefixes resolve through a `prefixes:` map in `glossmark.yaml`, falling back to `urn:gloss:boundary:<prefix>:<name>` with a warning.

### Marker grammar

The reference regex from the spec is frozen, with one documented extension: a target may carry a `NEW:` infix (`impl:NEW:Gelding`) so proposals of new concepts parse as one token instead of leaving `:Gelding` as trailing text. Malformed markers are warnings at `file:line`, never silent drops. Positions are physical columns.

Anchoring without tree-sitter (POC): a per-language declaration detector (regex) finds the next declaration after the marker and fingerprints its signature line. This gives `scope=symbol` and stale detection in Phase 1; tree-sitter replaces the detector in Phase 2 without changing the graph.

### Comparison and gating

Rollup derives, per boundary, `realizes`/`produces`/`consumes`/`publishes`/`subscribes` sets from the artifacts under it (rule 1–2), lifts impl → core through `realizes`, and compares against the manifest's declared sets under the subsumption rule (rule 3, `lift: range-lcs`): derived ⊑ declared is consistent; derived not covered is a violation; declared not reached is a gap. Orphans are concepts no artifact reaches; rogues are `NEW:` targets or dangling references.

Gates are a policy table with spec defaults, overridable in `glossmark.yaml` under `policy:`.

## 3. Phasing

| Phase | Spec gate | Status in this repo |
| --- | --- | --- |
| 1 POC | Drift report on one repo; extractor < 5 s; corpus parses with zero false markers | Built: grammar + corpus, file formats + schemas, extractor, graph.jsonl, check, drift (rules 1–4) |
| 2 MVP | Second team onboards in a day; ≥80 % auto-accept at ≥90 % precision | Built: ledger, accept/reject, gates, diff with per-view impact and PR comment, coverage, views + static viewer, pre-commit hook, GitHub Action, AGENTS.md snippet, SQLite store, MCP server. Deferred: tree-sitter anchoring (regex detector stands in), confidence calibration tooling |
| 3 v1.0 | Three repos federate; published change caught across repos; outside plugin | Partially built: `imports` by local path; own-subject rule; published-as; `gloss publish` (`ontology.jsonld` + `graph.jsonl` per namespace); `gloss landscape` (union by IRI, context-map edges, cross-repo drift and viewer); MCP server. Deferred: git/URL import fetch with pinning, SSSOM mappings, Backstage ownership join, SCIP join, SHACL runtime validation, plugin SDK, OWL/SHACL export |

## 4. Repository layout of this project

```
cmd/gloss/                CLI entry point (subcommands: build, check, drift, diff, impact, coverage, accept, reject, status, propose, context, hook, view, export, publish, landscape, mcp, agents, version)
internal/ontoedit/        Formatting-preserving edits to *.gloss.yaml (propose, accept, remove)
internal/model/           File formats as Go types + validation; relation vocabulary
internal/marker/          Grammar, comment table, declaration detector
internal/load/            Workspace loader and import resolver
internal/extract/         Pass 1 extractor
internal/graph/           Graph, closures, LCS, JSONL I/O
internal/check/           Lint rules
internal/rollup/          Rollup rules, drift classification, metrics
internal/ledger/          Ledger I/O, review states
internal/gate/            Gate policy, diff, PR comment rendering
internal/views/           Projection engine
internal/viewer/          Embedded static HTML viewer
internal/store/           SQLite export
internal/mcp/             MCP stdio server
internal/agents/          AGENTS.md snippet
schemas/                  JSON Schemas for glossmark.yaml, *.gloss.yaml, .gloss.yaml, views.yaml; LinkML schema for the graph
corpus/                   Conformance corpus: one file per supported extension + expected.json
examples/horse-factory/   The zoo example from the spec (one repo, both tiers)
examples/payments/        The worked example: payments-api (owns pay:, pa:) and settlement-worker (imports pay:, owns sw:)
docs/                     PLAN.md (this file), GRAMMAR.md, FILE-FORMATS.md, QUICKSTART.md, GRAPH.md
hooks/                    pre-commit hook; .pre-commit-hooks.yaml at root
action.yml                Composite GitHub Action: build, check, drift, diff, PR comment
```

## 5. Decisions taken while building

1. **Go, stdlib-first.** Dependencies: `gopkg.in/yaml.v3`, `modernc.org/sqlite`. No CLI framework; subcommands are hand-rolled on `flag`.
2. **`NEW:` infix in targets** (see grammar). The spec's regex is otherwise unchanged and shipped verbatim in `docs/GRAMMAR.md`.
3. **Marker identity.** `id=` when given; else a short hash of (path, relation, targets, anchor symbol or scope). Line moves do not change the ID; renames do, which is the correct conservative behaviour.
4. **Actor attribution.** `GLOSS_ACTOR` env var or `--actor` flag (`human:alice`, `agent:claude-code@claude-fable-5-1`); default `human:<git user.name>`. Git blame is the second trail and is not re-derived by the tool.
5. **Coverage unit.** File-level by default (share of in-scope, comment-bearing, non-generated files with at least one accepted link). `--unit symbol` uses the declaration detector and is labelled approximate until tree-sitter lands.
6. **Base graph for `gloss diff`.** Reads `<base>:.glossmark/graph.jsonl` from git when committed, else extracts `<base>` into a temp dir via `git archive` and builds it.
7. **Imports.** Local paths resolve now (`source: ../payments-api/ontology/`); `git+` and `https://` sources are parsed and rejected with a clear message until the fetcher lands. A local directory containing a published `ontology.jsonld` is also accepted.
8. **Instances and fixtures.** Out of scope for this build; the schema reserves `shapes:` on impl concepts so the runtime check can land without a format change.
9. **Comparison details the spec leaves open.** A manifest relation that declares nothing is not compared (no claim, no finding). `implements` lifts to `realizes`; `represents` lifts as itself and is reported, not gated, because manifests declare behaviour. Internal concepts and event emitters are exempt from the `realizes` comparison; events are compared through `publishes`/`subscribes`. Intended relations lift through the realized core concept, so `AuthorizeHandler realizes Authorization produces Payment` makes the boundary produce `Payment`. `is-a` and `part-of` cover one way: a declared supertype or whole covers a derived subtype or part, never the reverse, and a concept with no kind takes its nearest `is-a` ancestor's ([ADR 0009](adr/0009-hierarchy-direction.md)).
10. **Quoting the grammar.** Applying GlossMark to itself surfaced the quine problem: a comment that documents `@gloss` is itself a marker. The scanner, not the frozen regex, now treats text between a pair of backticks inside a comment as quoted prose, and skips the body of a Markdown fenced code block. The corpus files `quoted.md` and `quoted.go` pin the rule.
11. **Self-hosting.** The repository owns `gm:` (core) and `gmi:` (implementation) and links its own packages, commands, schemas and docs; `internal/e2e/selfhost_test.go` fails on any lint error or failing drift finding. The example repositories and the corpus test files are excluded from the root workspace because they are workspaces of their own.
12. **Landscape before federation.** `gloss landscape <repo>...` unions sibling checkouts by IRI and re-derives rollups, which gives the context-map edge and closes single-repo gaps today; the published-artifact fetcher will feed the same union later.

## 6. Agent-experience additions (beyond the spec)

Built after the first cut, because they are what pays off on an agent's first day:

1. **Proposed ontology entries.** `proposed: true` / `proposed-by:` on a concept is the ontology's `@gloss?`. `gloss propose concept` and `gloss propose --from-new` write them (and rewrite `ns:NEW:Name` markers); they are HARD until `gloss accept --concept` drops the flag, tracked in the ledger as `concept:<curie>` entries, so a new concept and its implementation ship in one PR.
2. **Diff-aware impact.** `gloss impact [--base ref] [paths]` and MCP `impact_of` with `paths`/`base` report, per changed file, the concepts touched, whether they sit on the published seam, what constrains them, which boundaries declare them, their tests and the other files sharing them, plus the AUTO/SOFT/HARD classification when a base is given.
3. **Search by meaning.** `search_concepts` matches aliases (`aliases:` on concepts), CamelCase words, stems and small typos, follows `realizes`/`is-a` one hop, and says why each hit matched.
4. **Session-start briefing.** `gloss context --dir` renders a bounded briefing (boundary, allowed targets, remits, concepts, state); `gloss hook install` registers it as a Claude Code SessionStart hook and registers the MCP server in `.mcp.json`.

## 7. Governance and the viewer

Built from the first round of user feedback:

1. **Criticality.** `criticality: low | medium | high | critical` on concepts (ontology) and boundaries (manifests); effective levels and provenance stamped on every concept, boundary and artifact by the rollup (declared wins; impl inherits from what it realizes; boundaries from what their code realizes, else their parent; artifacts from both). Surfaced in `gloss diff` (per-change tag, `criticality-gates` policy, PR comment summary), `gloss impact` / MCP `impact_of`, `gloss context`, the SQLite export and the viewer. `gloss check` GM600 flags a boundary that understates a concept it realizes. See `docs/GOVERNANCE.md`.
2. **Boundaries as containers.** A projection places every concept inside the deepest boundary whose code realizes or represents it (`PNode.Boundary`; several unrelated boundaries resolve to their common enclosing boundary, or to none). The viewer draws boundaries as boxes around their members instead of as a hub node; housing edges and boundary `part-of` become containment; the boundary's interface (`produces`, `consumes`, `publishes`, `subscribes`) stays drawn from the box edge.
3. **Top-down discovery.** The viewer opens with every boundary closed: the system's top level plus the concepts nothing realizes. A click opens a box one level, a double-click focuses on a boundary (its contents plus what they connect to, with a breadcrumb), Esc steps back up.
4. **Filters.** Type, tier, namespace, kind, boundary level, criticality (including unclassified), relation and internal mechanisms, each a toggle chip with counts.

## 8. Versioning

Four numbers, documented in `docs/VERSIONING.md`: the tool release (git tag, injected at build time by the release workflow, read from Go build info for `go install`), the grammar (`grammar: N` directive with a written stability and deprecation policy), file formats (`format` headers on `graph.jsonl`, `ledger.jsonl` and `glossmark.lock`, newer formats refused), and namespace semver with enforced import ranges and a `glossmark.lock` written by `gloss lock`, which vendors each import under `.glossmark/imports/` and resolves from that copy, so the lock pins ([ADR 0002](adr/0002-lock-pins-imports.md)).

## 9. Verification

- `go test ./...` runs the conformance corpus, grammar edge cases, rollup rules on synthetic graphs, and end-to-end runs on both examples.
- The payments example test asserts the spec's "What the tooling makes of it" table: one violation (Refunding handled, not declared), one gap (Capture implemented but untested), one landscape edge (CaptureConfirmed across repos), one HARD change (SettlementCutoff value), one consistent rollup after taxonomic lift.
- `gloss build` on either example completes well under the 5 s gate.

## 10. Roadmap from the first two adoptions

Source: the review of the pfinance and AgentArena adoption branches against `main` at 681517a (`docs/reviews/2026-10-18-adoption-review.md`). Every item is a GitHub issue with what ships, what it touches, the finding it closes and a done-when that is testable on one of the two adoption branches. Sizes: S = a day, M = a few days, L = a week plus an ADR.

### Phase A: ship-blockers

| Issue | Deliverable | Closes | Size |
| --- | --- | --- | --- |
| [#4](https://github.com/McKalvan/GlossMark/issues/4) | Release v0.1.0: tag, binaries, install script, Action pinned | no releases; `@latest` is `0.0.0-dev` | S |
| [#5](https://github.com/McKalvan/GlossMark/issues/5) | Ontology-file lint: null keys (GM304), docs truncated by flow-style commas (GM305), proposal and rogue counts in `check` | silent doc truncation (3.1), silent proposals (4.4) | S |
| [#6](https://github.com/McKalvan/GlossMark/issues/6) | Durable provenance: actor from ledger or blame, hook sets `GLOSS_ACTOR`, ledger belongs in git | actor rewritten on every rebuild (4.1) | S |
| [#7](https://github.com/McKalvan/GlossMark/issues/7) | Adoption-mode diff, grouped PR comment, corrected gate reasons, coverage ratchet | unreadable first PR (5.1) | M |

### Phase B: modelling

| Issue | Deliverable | Closes | Size |
| --- | --- | --- | --- |
| [#8](https://github.com/McKalvan/GlossMark/issues/8) | `presents` and `hosts` relations; criticality inherits through `implements` only (ADR) | overloaded `realizes` (3.2, 3.11) | L |
| [#9](https://github.com/McKalvan/GlossMark/issues/9) | `realizes` inherits through `is-a` and rolls up through `part-of`; `realizes: inherit` in manifests | manifest boilerplate (3.3) | M |
| [#10](https://github.com/McKalvan/GlossMark/issues/10) | Derived `allowed` default | unusable `allowed` lists (3.4) | S |
| [#11](https://github.com/McKalvan/GlossMark/issues/11) | `provisions`, `deploys`, `configures`, `represents` satisfy boundary declarations | undeclarable data and infra boundaries (3.5) | S |
| [#12](https://github.com/McKalvan/GlossMark/issues/12) | `verifies` canonical, `tests` an alias | two test relations (3.6) | S |
| [#13](https://github.com/McKalvan/GlossMark/issues/13) | `value-from` binding for published values | values drift from code (3.7) | M |
| [#14](https://github.com/McKalvan/GlossMark/issues/14) | Criticality: artifact level rule, `gloss criticality suggest`, empty states | 3.11 | M |

### Phase C: the agent loop

| Issue | Deliverable | Closes | Size |
| --- | --- | --- | --- |
| [#15](https://github.com/McKalvan/GlossMark/issues/15) | Briefing v2: relevance-ranked, budgeted, role-scoped, one proposal sentence everywhere | root-only alphabetical briefing (4.2), three proposal workflows (4.3) | M |
| [#16](https://github.com/McKalvan/GlossMark/issues/16) | Search ranking v2 with docs in hits | flat score-50 results (3.10) | S |
| [#17](https://github.com/McKalvan/GlossMark/issues/17) | Format-preserving `propose`, proposal docs separate from notes, decorator anchoring | reflowed ontology, lost anchors (3.10, 4.5) | S |
| [#18](https://github.com/McKalvan/GlossMark/issues/18) | Redundant-marker lint, concept-side coverage, honest counters | coverage rewarding noise (3.8, 5.3) | S |

### Phase D: onboarding

| Issue | Deliverable | Closes | Size |
| --- | --- | --- | --- |
| [#19](https://github.com/McKalvan/GlossMark/issues/19) | `gloss init` | hand-built scaffolding (6.2) | M |
| [#20](https://github.com/McKalvan/GlossMark/issues/20) | `gloss suggest` and a shipped adoption skill | big-bang hand-written adoption (6.3) | M |
| [#21](https://github.com/McKalvan/GlossMark/issues/21) | Vocabulary importers (markdown glossary, enums) and `export --status` | three sources of truth in AgentArena (3.9) | M |
| [#22](https://github.com/McKalvan/GlossMark/issues/22) | `gloss doctor` | silent misconfiguration (6.9) | S |

### Phase E: viewer and docs

| Issue | Deliverable | Closes | Size |
| --- | --- | --- | --- |
| [#23](https://github.com/McKalvan/GlossMark/issues/23) | Viewer: containment tie-break, curated views open expanded, concept pane, split data file | concepts collapse to the root box (5.2) | M |
| [#24](https://github.com/McKalvan/GlossMark/issues/24) | Docs: starter five relations and a "when to mark" page | vocabulary load (5.3) | S |

### Order

| Week | Issues | Why |
| --- | --- | --- |
| 1 | #4, #5, #6, #17, #18 | small, independent, and they stop silent damage |
| 2 | #7, #10, #12, #16, #22 | the next PR on either adopter becomes readable and the manifests shrink |
| 3 | #8 (ADR), #9, #11 | the modelling fixes; the viewer and criticality depend on them |
| 4 | #15, #23, #24 | agent briefing and viewer on top of the fixed model |
| 5 | #19, #20, #21, #13, #14 | onboarding and the two value-adds |

Each done-when is testable on the pfinance or AgentArena adoption branch, so each issue can carry an end-to-end test in `internal/e2e` against a fixture copied from those repos.
