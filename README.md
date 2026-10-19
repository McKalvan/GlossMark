<!-- @gloss documents arch:glossmark scope=file -->
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
Concepts covered 17% (1/6 with an artifact and a verifier)   Files 85% (11/13)   Orphans: 3   Rogue: 1   Violations: 1   Gaps: 0   Stale: 0   Untested: 3   Proposed links: 3   Proposed concepts: 0
```

## What is in this repository

| Path | What |
| --- | --- |
| `cmd/gloss` | The CLI: `init`, `suggest`, `import`, `criticality`, `build`, `check`, `drift`, `diff`, `impact`, `coverage`, `accept`, `reject`, `status`, `propose`, `context`, `hook`, `doctor`, `lock`, `view`, `export`, `publish`, `landscape`, `agents`, `mcp`, `version` |
| `internal/marker` | The frozen `@gloss` grammar, comment table, declaration detector |
| `internal/load`, `internal/extract`, `internal/graph` | Workspace loader, pass-1 extractor, canonical graph (`.glossmark/graph.jsonl`) |
| `internal/check`, `internal/rollup`, `internal/gate` | Lint rules, rollup rules and drift comparison, AUTO/SOFT/HARD gating with per-view impact |
| `internal/ledger`, `internal/views`, `internal/viewer` | Review-state ledger, view projections, static HTML viewer |
| `internal/scaffold`, `internal/doctor`, `internal/suggest`, `internal/importers` | `gloss init` templates and plan; `gloss doctor` checks; `gloss suggest` matcher; glossary and enum importers |
| `skills/` | Claude Code skills shipped with the binary: `adopt-glossmark` (installed by `gloss hook install --skill`, or by name with `--skills`) |
| `internal/guard` | `gloss hook guard`: criticality, remit and lint feedback at edit time, a clean-tree check at stop |
| `internal/store`, `internal/mcp`, `internal/agents` | SQLite export, MCP server, AGENTS.md vocabulary snippet |
| `ontology/`, `glossmark.yaml`, `.gloss.yaml` | GlossMark applied to itself: the `gm:` core and `gmi:` implementation tiers this source tree is linked to |
| `corpus/` | Conformance corpus for the grammar across 18 file types |
| `schemas/` | JSON Schemas for every file format, LinkML schema for the graph |
| `examples/horse-factory` | The zoo example from the design: one repo, both tiers |
| `examples/payments` | The worked example: two repos sharing one core, with the published seam, extension, remits and a landscape |
| `docs/` | [Quickstart](docs/QUICKSTART.md) · [When to mark](docs/MARKING.md) · [Advanced](docs/ADVANCED.md) · [Grammar](docs/GRAMMAR.md) · [File formats](docs/FILE-FORMATS.md) · [Graph](docs/GRAPH.md) · [Governance](docs/GOVERNANCE.md) · [Versioning](docs/VERSIONING.md) · [Plan](docs/PLAN.md) |
| `action.yml`, `hooks/`, `.pre-commit-hooks.yaml` | GitHub Action with PR comment, pre-commit hook |

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/mckalvan/glossmark/main/install.sh | sh -s -- v0.1.0   # Linux/macOS binary into ~/.local/bin
go install github.com/mckalvan/glossmark/cmd/gloss@v0.1.0                                          # with a Go toolchain
```

Binaries for Linux, macOS and Windows are attached to each [release](https://github.com/mckalvan/glossmark/releases); the script verifies the sha256. In CI, `uses: mckalvan/glossmark@v0.1.0` downloads the same binary.

## Try it

```sh
go build -o gloss ./cmd/gloss
cd examples/horse-factory
../../gloss check          # 0 errors, 1 warning: the Donkey link is outside the boundary's allowed list
../../gloss drift          # the drift report above
../../gloss view           # .glossmark/viewer.html + views.js — open it in a browser: boundaries are boxes, click to drill in, click a concept for its code, tests and findings
cd ../payments
../../gloss landscape payments-api settlement-worker   # cross-repo context map and drift
```

`go test ./...` runs the conformance corpus, graph tests and end-to-end runs that assert the design document's worked-example table.

## For agents

`gloss hook install` registers a SessionStart hook that briefs an agent on the directory it is working in (boundary, allowed targets, remits, concepts, current state) and an MCP server with `search_concepts` (aliases, CamelCase words, stems, typos, one hop through `realizes`), `concepts_for_path`, `propose_link`, `propose_concept` (adds a `proposed: true` ontology entry so a new concept and its implementation ship in one PR), `impact_of` (what changed files touch: published seam, constraints, boundaries, tests, blast radius) and `drift`. `gloss hook install --guards` adds `gloss hook guard` on edits and at stop: before an edit it names the file's criticality, published-seam concepts and whether the file is outside the agent's `may-modify` remit; after an edit it reports that file's `gloss check` findings; at stop it blocks while `gloss check` has errors or the graph is stale (`policy.guard-stop: report` turns blocking off). See [docs/QUICKSTART.md](docs/QUICKSTART.md#agent-experience).

## GlossMark on GlossMark

This repository is its own first user. `glossmark.yaml` at the root owns two namespaces: `gm:` is the core tier (what a comment-native code ontology is about: links, concepts, namespaces, boundaries, extraction, lint, drift, review, gating) and `gmi:` is the implementation tier (the Go packages and commands that realize it). Every package and command carries a marker, the JSON Schemas have sidecars, the docs `documents` what they describe, and `.gloss.yaml` manifests at the root, `cmd/` and `internal/` declare what `gloss drift` compares with the code. The self-description uses the whole syntax: the root manifest says `realizes: [inherit, gm:Onboarding]` instead of repeating its children, the library declares the entities it `holds` and widens its derived allowed set with `allowed: [derived, gm:Confidence, gm:GlossMark]`, the HTML viewer `presents` `gm:View` rather than realizing it, the graph format version is bound to `internal/version/version.go` with `value-from` and the default confidence threshold with a `publishes-value` marker (so bumping either in code without the ontology fails `gloss drift`), and `ontology/views.yaml` adds a curated `constraints` view. `go test ./internal/e2e/` includes a self-host test that fails on any lint warning or error, violation, gap, dangling reference, orphan or unbound value in this tree.

```sh
go build -o gloss ./cmd/gloss
./gloss check     # 0 errors, 0 warnings
./gloss drift     # what is left is honest: proposed concepts awaiting a human, and three mechanisms nothing tests yet
./gloss view      # .glossmark/viewer.html
```

The links were written by an agent (`GLOSS_ACTOR=agent:claude`), so the ledger records them as accepted without a reviewer until a human reviews them. Concepts the agent added after the first pass carry `proposed: true`; `gloss status` lists them and `gloss accept --concept` accepts each one.

## Design in one paragraph

Two tiers plus instances: core is implementation-independent by construction; every implementation concept `realizes` a core concept or is `internal`; instances live outside both. The **namespace** is the unit of modularity, with one repo as the default layout and imports by path (git ref and URL to come). The **published** flag is the only seam between tiers and the only thing importable across repos; touching a published concept is HARD. Importers **extend, never mutate** (own-subject rule). **Views are projections** over tier × zoom; PR impact is "which views changed". Five deterministic rollup rules derive higher-level facts from code and compare them with what manifests declare; the comparison is the drift signal.

## Governance

Not every subsystem carries the same risk. `criticality: low | medium | high | critical` on a concept or a boundary flows through `realizes` to the code that implements it (a boundary's level reaches its files only with `criticality-floor: true`; `gloss criticality suggest` drafts a first classification), tags every change in the PR comment and `gloss impact`, raises the gate by policy (`critical` is HARD by default), colours the viewer and gives agents a reason to flag what they touch. See [docs/GOVERNANCE.md](docs/GOVERNANCE.md).

## Versions

`gloss version` prints the tool release, the grammar version and the file-format versions. Releases are git tags built by the release workflow; pin them in the Action (`uses: mckalvan/glossmark@v0.1.0`), pre-commit (`rev: v0.1.0`), `install.sh v0.1.0` or `go install ...@v0.1.0`. Repositories pin their grammar with `grammar: 1` in `glossmark.yaml`, importers pin ontology versions with ranges and `glossmark.lock`. See [docs/VERSIONING.md](docs/VERSIONING.md).

## Status

Phase 1 (POC) and most of Phase 2 (MVP) of the roadmap are built; see [docs/PLAN.md](docs/PLAN.md) for what is deferred (tree-sitter anchoring, remote import fetch, SSSOM mappings, SCIP join, SHACL instance validation, plugin SDK).
