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
9. **Comparison details the spec leaves open.** A manifest relation that declares nothing is not compared (no claim, no finding). `implements` lifts to `realizes`; `represents` lifts as itself and is reported, not gated, because manifests declare behaviour. Internal concepts and event emitters are exempt from the `realizes` comparison; events are compared through `publishes`/`subscribes`. Intended relations lift through the realized core concept, so `AuthorizeHandler realizes Authorization produces Payment` makes the boundary produce `Payment`.
10. **Landscape before federation.** `gloss landscape <repo>...` unions sibling checkouts by IRI and re-derives rollups, which gives the context-map edge and closes single-repo gaps today; the published-artifact fetcher will feed the same union later.

## 6. Agent-experience additions (beyond the spec)

Built after the first cut, because they are what pays off on an agent's first day:

1. **Proposed ontology entries.** `proposed: true` / `proposed-by:` on a concept is the ontology's `@gloss?`. `gloss propose concept` and `gloss propose --from-new` write them (and rewrite `ns:NEW:Name` markers); they are HARD until `gloss accept --concept` drops the flag, tracked in the ledger as `concept:<curie>` entries, so a new concept and its implementation ship in one PR.
2. **Diff-aware impact.** `gloss impact [--base ref] [paths]` and MCP `impact_of` with `paths`/`base` report, per changed file, the concepts touched, whether they sit on the published seam, what constrains them, which boundaries declare them, their tests and the other files sharing them, plus the AUTO/SOFT/HARD classification when a base is given.
3. **Search by meaning.** `search_concepts` matches aliases (`aliases:` on concepts), CamelCase words, stems and small typos, follows `realizes`/`is-a` one hop, and says why each hit matched.
4. **Session-start briefing.** `gloss context --dir` renders a bounded briefing (boundary, allowed targets, remits, concepts, state); `gloss hook install` registers it as a Claude Code SessionStart hook and registers the MCP server in `.mcp.json`.

## 7. Versioning

Four numbers, documented in `docs/VERSIONING.md`: the tool release (git tag, injected at build time by the release workflow, read from Go build info for `go install`), the grammar (`grammar: N` directive with a written stability and deprecation policy), file formats (`format` headers on `graph.jsonl`, `ledger.jsonl` and `glossmark.lock`, newer formats refused), and namespace semver with enforced import ranges and a `glossmark.lock` written by `gloss lock`.

## 8. Verification

- `go test ./...` runs the conformance corpus, grammar edge cases, rollup rules on synthetic graphs, and end-to-end runs on both examples.
- The payments example test asserts the spec's "What the tooling makes of it" table: one violation (Refunding handled, not declared), one gap (Capture implemented but untested), one landscape edge (CaptureConfirmed across repos), one HARD change (SettlementCutoff value), one consistent rollup after taxonomic lift.
- `gloss build` on either example completes well under the 5 s gate.
