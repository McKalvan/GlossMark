<!-- @gloss documents arch:glossmark scope=file -->
# GlossMark

GlossMark is a comment-native code ontology. One grammar, `@gloss`, links any artifact (code, tests, Terraform, configs, CI, Markdown, agent files) to concepts in two ontology tiers: a platonic **core** the business owns and a concrete **implementation** engineering owns. Agents write the links while they write the code; humans own the core and review only novelty. Everything compiles into one graph that tools diff, gate, project into views and serve to agents.

```python
# @gloss implements circ:BookIntake
# @gloss produces lib:Book
def catalog_book(isbn: str, copies: int) -> Book: ...
```
```ts
// @gloss? represents lib:Book conf=0.72        <- proposed by an agent; a human accepts it
export interface Book { isbn: string; dueOn?: Date }
```

```
$ gloss drift
✗ Violations (boundary exceeds its declared capability)
  acme:circulation-svc produces lib:Laptop but declares produces {lib:Media}; rollup: produces {lib:Item} (from lib:Book, lib:DVD, lib:Laptop, lib:Media) ⊄ declared [proposed link]
  acme:circulation-svc realizes lib:Renewal but declares realizes {circ:CirculationService, lib:Cataloging, lib:Checkout, lib:LoanPeriod, lib:Return} [proposed link]
○ Gaps (declared, not implemented)
  acme:library-platform declares publishes lib:HoldReady but nothing under it publishes it: intended capability not implemented, or the declaration is wrong
? Rogue concepts (decision requests)
  services/circulation/loans/returns.py:10 implements circ:NEW:DamageCheck: rogue concept; becomes a decision request
◇ Presented but not implemented (a surface with no mechanism)
  lib:Hold is presented (apps/opac/src/PlaceHold.tsx#PlaceHold) but nothing implements it: a surface with no mechanism behind it
Concepts covered 22% (2/9 with an artifact and a verifier)   Orphans: 6   Rogue: 1   Violations: 2   Gaps: 1   Stale: 0   Untested: 4   Proposed links: 4   Proposed concepts: 0
```

## What is in this repository

| Path | What |
| --- | --- |
| `cmd/gloss` | The CLI: `init`, `harvest`, `suggest`, `import`, `criticality`, `build`, `check`, `drift`, `diff`, `impact`, `coverage`, `accept`, `reject`, `status`, `propose`, `context`, `hook`, `doctor`, `lock`, `view`, `export`, `publish`, `landscape`, `agents`, `mcp`, `version` |
| `internal/marker` | The frozen `@gloss` grammar, comment table, declaration detector |
| `internal/load`, `internal/extract`, `internal/graph` | Workspace loader, pass-1 extractor, canonical graph (`.glossmark/graph.jsonl`) |
| `internal/check`, `internal/rollup`, `internal/gate` | Lint rules, rollup rules and drift comparison, AUTO/SOFT/HARD gating with per-view impact |
| `internal/ledger`, `internal/views`, `internal/viewer` | Review-state ledger, view projections, static HTML viewer |
| `internal/scaffold`, `internal/doctor`, `internal/suggest`, `internal/importers`, `internal/harvest` | `gloss init` templates and plan; `gloss doctor` checks; `gloss suggest` matcher; glossary and enum importers; `gloss harvest` evidence for deriving an ontology ([ADR 0025](docs/adr/0025-deriving-the-ontology.md), experimental) |
| `skills/` | Claude Code skills shipped with the binary: `adopt-glossmark` and `derive-ontology` (deriving the core and implementation tiers from an existing application, [ADR 0025](docs/adr/0025-deriving-the-ontology.md)), installed by `gloss hook install --skill`, before `gloss init` too |
| `internal/store`, `internal/mcp`, `internal/agents` | SQLite export, MCP server, AGENTS.md vocabulary snippet |
| `ontology/`, `glossmark.yaml`, `.gloss.yaml` | GlossMark applied to itself: the `gm:` core and `gmi:` implementation tiers this source tree is linked to |
| `corpus/` | Conformance corpus for the grammar across 18 file types |
| `schemas/` | JSON Schemas for every file format, LinkML schema for the graph |
| `examples/library` | The worked example: a public library in two repos sharing one core (`circulation` owns both tiers, `holds-worker` imports and extends the core), covering taxonomic lift, the published seam, `value-from`, surfaces, criticality, extension, remits and a landscape; see its [README](examples/library/README.md) |
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
cd examples/library/circulation
../../../gloss check       # 0 errors, 1 warning: the Laptop link is outside the boundary's allowed list
../../../gloss drift       # the drift report above
../../../gloss view        # .glossmark/viewer.html + views.js — open it in a browser: boundaries are boxes, click to drill in, click a concept for its code, tests and findings
cd ..
../../gloss landscape circulation holds-worker   # cross-repo context map; the HoldReady gap closes
```

`go test ./...` runs the conformance corpus, graph tests and end-to-end runs that assert the design document's worked-example table.

## For agents

`gloss hook install` registers a SessionStart hook that briefs an agent on the directory it is working in (boundary, allowed targets, remits, concepts, current state) and an MCP server with `search_concepts` (aliases, CamelCase words, stems, typos, one hop through `realizes`), `concepts_for_path`, `propose_link`, `propose_concept` (adds a `proposed: true` ontology entry so a new concept and its implementation ship in one PR), `impact_of` (what changed files touch: published seam, constraints, boundaries, tests, blast radius), `slice_of` (everything relevant to one feature, like `gloss slice`) and `drift`. See [docs/QUICKSTART.md](docs/QUICKSTART.md#agent-experience).

## GlossMark on GlossMark

This repository is its own first user. `glossmark.yaml` at the root owns two namespaces: `gm:` is the core tier (what a comment-native code ontology is about: links, concepts, namespaces, boundaries, extraction, lint, drift, review, gating) and `gmi:` is the implementation tier (the Go packages and commands that realize it). Every package and command carries a marker, the JSON Schemas have sidecars, the docs `documents` what they describe, and `.gloss.yaml` manifests at the root, `cmd/` and `internal/` declare what `gloss drift` compares with the code. The self-description uses the whole syntax: the root manifest says `realizes: [inherit, gm:Onboarding]`, `produces: [inherit]` and `publishes: [inherit]` instead of repeating its children, the library declares the entities it `holds` and widens its derived allowed set with `allowed: [derived, gm:Confidence, gm:GlossMark]`, the HTML viewer `presents` `gm:View` rather than realizing it, the graph format version is bound to `internal/version/version.go` with `value-from` and the default confidence threshold with a `publishes-value` marker (so bumping either in code without the ontology fails `gloss drift`), and `ontology/views.yaml` adds a curated `constraints` view. `go test ./internal/e2e/` includes a self-host test that fails on any lint warning or error, violation, gap, dangling reference, orphan, untested mechanism, proposed concept or unbound value in this tree.

```sh
go build -o gloss ./cmd/gloss
./gloss check     # 0 errors, 0 warnings
./gloss drift     # 0 violations, gaps, orphans, untested, proposed concepts or links; stale only for files edited since their last review
./gloss view      # .glossmark/viewer.html
```

The links were written by an agent (`GLOSS_ACTOR=agent:claude`), so the ledger records them as accepted without a reviewer until a human reviews them. Most markers here are `scope=file`, and a file-scope link goes stale whenever its file's code or prose changes ([ADR 0006](docs/adr/0006-file-fingerprint-excludes-markers.md)). So `gloss drift` lists stale links for the files a change touched: check that each link still holds, then `gloss accept --stale <paths>` (or `gloss accept <id>`) re-confirms them. CI holds this repository to the strictest gate the tool offers ([.github/workflows/ci.yml](.github/workflows/ci.yml)): the committed graph must match the tree (`gloss build --check`); `gloss drift` fails on every finding kind but `overlap` (the command line and the library it calls realize the same concepts by design, [ADR 0019](docs/adr/0019-migration-overlap.md)), stale links included; concept coverage must stay at 100%; and every concept must carry a doc. The workflow runs the Action from this checkout (`uses: ./`, `version: source`), so every PR also exercises `action.yml` and posts its own impact comment; a HARD change needs a reviewer to label the PR `ontology-approved`.

## Design in one paragraph

Two tiers plus instances: core is implementation-independent by construction; every implementation concept `realizes` a core concept or is `internal`; instances live outside both. The **namespace** is the unit of modularity, with one repo as the default layout and imports by path (git ref and URL to come). The **published** flag is the only seam between tiers and the only thing importable across repos; touching a published concept is HARD. Importers **extend, never mutate** (own-subject rule). **Views are projections** over tier × zoom; PR impact is "which views changed". Five deterministic rollup rules derive higher-level facts from code and compare them with what manifests declare; the comparison is the drift signal.

## Governance

Not every subsystem carries the same risk. `criticality: low | medium | high | critical` on a concept or a boundary flows through `realizes` to the code that implements it (a boundary's level reaches its files only with `criticality-floor: true`; `gloss criticality suggest` drafts a first classification), tags every change in the PR comment and `gloss impact`, raises the gate by policy (`critical` is HARD by default), colours the viewer and gives agents a reason to flag what they touch. See [docs/GOVERNANCE.md](docs/GOVERNANCE.md).

## Versions

`gloss version` prints the tool release, the grammar version and the file-format versions. Releases are git tags built by the release workflow; pin them in the Action (`uses: mckalvan/glossmark@v0.1.0`), pre-commit (`rev: v0.1.0`), `install.sh v0.1.0` or `go install ...@v0.1.0`. Repositories pin their grammar with `grammar: 1` in `glossmark.yaml`, importers pin ontology versions with ranges and `glossmark.lock`. See [docs/VERSIONING.md](docs/VERSIONING.md).

## Status

Phase 1 (POC) and most of Phase 2 (MVP) of the roadmap are built; see [docs/PLAN.md](docs/PLAN.md) for what is deferred (tree-sitter anchoring, remote import fetch, SSSOM mappings, SCIP join, SHACL instance validation, plugin SDK).
