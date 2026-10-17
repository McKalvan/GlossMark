# GlossMark

GlossMark is a comment-native code ontology. One grammar, `@gloss`, links any artifact (code, tests, Terraform, configs, CI, Markdown, agent files) to concepts in two ontology tiers: a platonic **core** the business owns and a concrete **implementation** engineering owns. Agents write the links while they write the code; humans own the core and review only novelty. Everything compiles into one graph that tools diff, gate, project into views and serve to agents.

```python
# @gloss implements impl:StallionProduction
def breed_stallion(sire: Horse, dam: Horse) -> Stallion: ...
```
```ts
// @gloss? represents zoo:Mare conf=0.72        <- proposed by an agent; a human accepts it
export interface Mare { id: string; foalingDate?: Date }
```

```
$ gloss drift
✗ Violations (boundary exceeds its declared capability)
  acme:horse-factory-svc produces zoo:Donkey but declares produces {zoo:Horse}; rollup: produces {zoo:Animal} ⊄ declared [proposed link]
? Rogue concepts (decision requests)
  services/horse-factory/breeding/engine.py:22 implements impl:NEW:Gelding: rogue concept; becomes a decision request
· Orphan concepts (no artifacts)
  impl:TailQueue: orphan concept, implementation concept has no artifacts
Coverage 85% (11/13 files)   Orphans: 3   Rogue: 1   Violations: 1   Gaps: 0   Stale: 0   Untested: 6   Proposed: 3
```

## What is in this repository

| Path | What |
| --- | --- |
| `cmd/gloss` | The CLI: `build`, `check`, `drift`, `diff`, `coverage`, `accept`, `reject`, `status`, `view`, `export`, `publish`, `landscape`, `agents`, `mcp` |
| `internal/marker` | The frozen `@gloss` grammar, comment table, declaration detector |
| `internal/load`, `internal/extract`, `internal/graph` | Workspace loader, pass-1 extractor, canonical graph (`.glossmark/graph.jsonl`) |
| `internal/check`, `internal/rollup`, `internal/gate` | Lint rules, rollup rules and drift comparison, AUTO/SOFT/HARD gating with per-view impact |
| `internal/ledger`, `internal/views`, `internal/viewer` | Review-state ledger, view projections, static HTML viewer |
| `internal/store`, `internal/mcp`, `internal/agents` | SQLite export, MCP server, AGENTS.md vocabulary snippet |
| `corpus/` | Conformance corpus for the grammar across 18 file types |
| `schemas/` | JSON Schemas for every file format, LinkML schema for the graph |
| `examples/horse-factory` | The zoo example from the design: one repo, both tiers |
| `examples/payments` | The worked example: two repos sharing one core, with the published seam, extension, remits and a landscape |
| `docs/` | [Plan](docs/PLAN.md) · [Quickstart](docs/QUICKSTART.md) · [Grammar](docs/GRAMMAR.md) · [File formats](docs/FILE-FORMATS.md) · [Graph](docs/GRAPH.md) |
| `action.yml`, `hooks/`, `.pre-commit-hooks.yaml` | GitHub Action with PR comment, pre-commit hook |

## Try it

```sh
go build -o gloss ./cmd/gloss
cd examples/horse-factory
../../gloss check          # 0 errors, 1 warning: the Donkey link is outside the boundary's allowed list
../../gloss drift          # the drift report above
../../gloss view           # .glossmark/viewer.html — open it in a browser
cd ../payments
../../gloss landscape payments-api settlement-worker   # cross-repo context map and drift
```

`go test ./...` runs the conformance corpus, graph tests and end-to-end runs that assert the design document's worked-example table.

## Design in one paragraph

Two tiers plus instances: core is implementation-independent by construction; every implementation concept `realizes` a core concept or is `internal`; instances live outside both. The **namespace** is the unit of modularity, with one repo as the default layout and imports by path (git ref and URL to come). The **published** flag is the only seam between tiers and the only thing importable across repos; touching a published concept is HARD. Importers **extend, never mutate** (own-subject rule). **Views are projections** over tier × zoom; PR impact is "which views changed". Five deterministic rollup rules derive higher-level facts from code and compare them with what manifests declare; the comparison is the drift signal.

## Status

Phase 1 (POC) and most of Phase 2 (MVP) of the roadmap are built; see [docs/PLAN.md](docs/PLAN.md) for what is deferred (tree-sitter anchoring, remote import fetch, SSSOM mappings, SCIP join, SHACL instance validation, plugin SDK).
