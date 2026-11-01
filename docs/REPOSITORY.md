<!-- @gloss documents arch:glossmark scope=file -->
# The GlossMark repository

What is where, how this repository uses GlossMark on itself, how versions are pinned and where the roadmap stands. The [README](../README.md) makes the case; this page is for contributors.

## What is in this repository

| Path | What |
| --- | --- |
| `cmd/gloss` | The CLI: `init`, `harvest`, `suggest`, `import`, `criticality`, `build`, `check`, `drift`, `diff`, `impact`, `coverage`, `accept`, `reject`, `status`, `review`, `propose`, `context`, `hook`, `doctor`, `lock`, `view`, `export`, `publish`, `landscape`, `agents`, `mcp`, `version` |
| `internal/marker` | The frozen `@gloss` grammar, comment table, declaration detector |
| `internal/load`, `internal/extract`, `internal/graph` | Workspace loader, pass-1 extractor, canonical graph (`.glossmark/graph.jsonl`) |
| `internal/review` | `gloss review` (experimental, [ADR 0033](adr/0033-review-sessions.md)): the review queue, decisions by gate authority, agent recommendations and an undo journal |
| `internal/tui` | The interactive terminal for `gloss review`, hand-rolled on `golang.org/x/term`; `gloss review --format frame` renders one frame of it without a terminal |
| `internal/browse` | The read-only graph browser: `gloss review node`, the terminal's Browse tab, `browse_node` |
| `internal/check`, `internal/surface`, `internal/rollup`, `internal/gate` | Lint rules, the surface rules (which surface a file is and the relation it takes, [ADR 0046](adr/0046-surface-rules.md)), rollup rules and drift comparison, AUTO/SOFT/HARD gating with per-view impact |
| `internal/ledger`, `internal/views`, `internal/viewer` | Review-state ledger, view projections, static HTML viewer |
| `internal/scaffold`, `internal/doctor`, `internal/suggest`, `internal/infer`, `internal/importers`, `internal/harvest` | `gloss init` templates and plan; `gloss doctor` checks; `gloss suggest` matcher; the `Source` interface and the AsyncAPI and OpenAPI sources suggest infers links from ([ADR 0049](adr/0049-links-inferred-from-specs.md)); glossary and enum importers; `gloss harvest` evidence for deriving an ontology ([ADR 0025](adr/0025-deriving-the-ontology.md), experimental) |
| `skills/` | Claude Code skills shipped with the binary: `adopt-glossmark` and `derive-ontology` (deriving the core and implementation tiers from an existing application, [ADR 0025](adr/0025-deriving-the-ontology.md)), installed by `gloss hook install --skill` (`--agent claude-code` bundles them in a plugin, `--agent cursor` and `--agent copilot` write a rule or instructions that point at them, [AGENTS.md](AGENTS.md)), before `gloss init` too |
| `internal/store`, `internal/mcp`, `internal/agents` | SQLite export, MCP server, AGENTS.md vocabulary snippet |
| `internal/bench`, `cmd/glossbench`, `bench/` | The agent benchmark ([BENCHMARK.md](BENCHMARK.md)): task files, the scorer and the runner; results not yet collected |
| `ontology/`, `glossmark.yaml`, `.gloss.yaml` | GlossMark applied to itself: the `gm:` core and `gmi:` implementation tiers this source tree is linked to |
| `corpus/` | Conformance corpus for the grammar across 18 file types |
| `schemas/` | JSON Schemas for every file format, LinkML schema for the graph |
| `examples/library` | The worked example: a public library in two repos sharing one core (`circulation` owns both tiers, `holds-worker` imports and extends the core), covering taxonomic lift, the published seam, `value-from`, surfaces, criticality, extension, remits and a landscape; see its [README](../examples/library/README.md) |
| `scripts/screenshots.sh` | Regenerates the README images in `docs/img/` from the [glossmark-examples](https://github.com/McKalvan/glossmark-examples) showcase |
| `docs/` | [Quickstart](QUICKSTART.md) · [Trying it safely, leaving](LEAVING.md) · [When to mark](MARKING.md) · [Agent integrations](AGENTS.md) · [Advanced](ADVANCED.md) · [Grammar](GRAMMAR.md) · [File formats](FILE-FORMATS.md) · [Graph](GRAPH.md) · [Governance](GOVERNANCE.md) · [Versioning](VERSIONING.md) · [Plan](PLAN.md) · [ADRs](adr/) |
| `action.yml`, `hooks/`, `.pre-commit-hooks.yaml` | GitHub Action with PR comment, pre-commit hook |
| `internal/prapp/`, `Dockerfile`, `docs/app/` | The hosted pull-request app (`gloss app serve`, [APP.md](APP.md)): the GitHub App that posts the comment, the check run and the review request with no CI |

`go test ./...` runs the conformance corpus, graph tests and end-to-end runs that assert the design document's worked-example table.

## GlossMark on GlossMark

This repository is its own first user. `glossmark.yaml` at the root owns two namespaces: `gm:` is the core tier (what a comment-native code ontology is about: links, concepts, namespaces, boundaries, extraction, lint, drift, review, gating) and `gmi:` is the implementation tier (the Go packages and commands that realize it). Every package and command carries a marker, the JSON Schemas have sidecars, the docs `documents` what they describe, and `.gloss.yaml` manifests declare what `gloss drift` compares with the code: the system at the root, the `cmd/` and `internal/` containers, seven logical components inside the library and one manifest per library package that implements something, naming its component with `part-of` ([ADR 0035](adr/0035-components-under-a-flat-package-tree.md)). The self-description uses the whole syntax: the root and the containers say `realizes: [inherit]` (plus `gm:Onboarding` at the root) instead of repeating their children, each package declares the `gmi:` mechanisms it implements and the entities it `holds`, the library widens its derived allowed set with `allowed: [derived, gm:GlossMark]`, the HTML viewer `presents` `gm:View` rather than realizing it, the graph format version is bound to `internal/version/version.go` with `value-from` and the default confidence threshold with a `publishes-value` marker (so bumping either in code without the ontology fails `gloss drift`), and `ontology/views.yaml` names each view for its reader: `concepts` (the core tier, grouped by `part-of` into six areas it opens on, [ADR 0039](adr/0039-a-core-view-that-reads.md)), `architecture`, `drift` and a curated `constraints` view. `go test ./internal/e2e/` includes a self-host test that fails on any lint warning or error, violation, gap, dangling reference, orphan, untested mechanism, proposed concept or unbound value in this tree.

```sh
go build -o gloss ./cmd/gloss
./gloss check     # 0 errors, 0 warnings
./gloss drift     # 0 violations, gaps, orphans, untested, proposed concepts or links; stale only for files edited since their last review
./gloss view      # .glossmark/viewer.html
```

The links were written by an agent (`GLOSS_ACTOR=agent:claude`), so the ledger records them as accepted without a reviewer until a human reviews them. Most markers here are `scope=file`, and a file-scope link goes stale whenever its file's code or prose changes ([ADR 0006](adr/0006-file-fingerprint-excludes-markers.md)). So `gloss drift` lists stale links for the files a change touched: check that each link still holds, then `gloss accept --stale <paths>` (or `gloss accept <id>`) re-confirms them. CI holds this repository to the strictest gate the tool offers ([.github/workflows/ci.yml](../.github/workflows/ci.yml)): the committed graph must match the tree (`gloss build --check`); `gloss drift` fails on every finding kind but `overlap` (the command line and the library it calls realize the same concepts by design, [ADR 0019](adr/0019-migration-overlap.md)), stale links included; concept coverage must stay at 100%; and every concept must carry a doc. The workflow runs the Action from this checkout (`uses: ./`, `version: source`), so every PR also exercises `action.yml` and posts its own impact comment. The gate itself is a separate job that builds `gloss` from the PR's base commit, so a PR that changes `action.yml`, the workflow, `cmd/gloss` or `internal/gate` is judged by the code it changes rather than by its own copy; a HARD change needs a reviewer to label the PR `ontology-approved`. `action.yml` and `.github/workflows/**` are in this repository's `hard-paths`, and a path the base's `hard-paths` names stays HARD even when the PR drops it.

## Design in one paragraph

Two tiers plus instances: core is implementation-independent by construction; every implementation concept `realizes` a core concept or is `internal`; instances live outside both. The **namespace** is the unit of modularity, with one repo as the default layout and imports by path (git ref and URL to come). The **published** flag is the only seam between tiers and the only thing importable across repos; touching a published concept is HARD. Importers **extend, never mutate** (own-subject rule). **Views are projections** over tier × zoom; PR impact is "which views changed". Five deterministic rollup rules derive higher-level facts from code and compare them with what manifests declare; the comparison is the drift signal. Why the vocabulary is curated before a ticket is implemented rather than summarised afterwards is [ADR 0034](adr/0034-ontology-first-increments.md).

## Versions

`gloss version` prints the tool release, the grammar version and the file-format versions. Releases are git tags built by the release workflow; pin them in the Action (`uses: mckalvan/glossmark@v0.3.0`), pre-commit (`rev: v0.3.0`), `install.sh v0.3.0` or `go install ...@v0.3.0`. Repositories pin their grammar with `grammar: 1` in `glossmark.yaml`, importers pin ontology versions with ranges and `glossmark.lock`. See [VERSIONING.md](VERSIONING.md).

## Status

Phase 1 (POC) and most of Phase 2 (MVP) of the roadmap are built; see [PLAN.md](PLAN.md) for what is deferred (tree-sitter anchoring, remote import fetch, SSSOM mappings, SCIP join, SHACL instance validation, plugin SDK).
