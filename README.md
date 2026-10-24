<!-- @gloss documents arch:glossmark scope=file -->
# GlossMark

As agents write more of the code, it changes faster than its reviewers can keep track of the concepts it introduces. GlossMark gives every artifact (code, tests, Terraform, configs, CI, docs, agent files) one comment line naming the domain concept it serves, so the concept becomes the join key across all of them, and the vocabulary is owned, reviewed and gated instead of regenerated from the code each time someone asks.

```python
# @gloss implements circ:BookIntake
# @gloss produces lib:Book
def catalog_book(isbn: str, copies: int) -> Book: ...
```
```ts
// @gloss? represents lib:Book conf=0.72        <- proposed by an agent; a human accepts it
export interface Book { isbn: string; dueOn?: Date }
```

One grammar, `@gloss`. Two ontology tiers: a **core** the business owns and an **implementation** engineering owns. One graph that tools diff, gate, project into views and serve to agents. Agents write the links while they write the code; humans own the core and review only what is new.

## See it

**The map.** `gloss view` draws boundaries as boxes and concepts as nodes. This is the engineering view of a clinic-booking monorepo (Next.js, Go, Python, Postgres, Terraform, Kubernetes, GitHub Actions) sliced to one feature, `clinic:Booking`: the Go handler that realizes it, the page that presents it, the Kubernetes deployment the service runs in, and the drift findings beside them.

![The GlossMark viewer: the engineering view of a clinic-booking monorepo, sliced to clinic:Booking. Boundaries are boxes (Patient site, Booking API, Kubernetes deployment, Reminder worker), the Booking concept is highlighted with its realization and the panel lists the drift findings.](docs/img/viewer-slice.png)

**The business view of the same graph.** The implementation tier folds into what it realizes, so a practice manager sees Booking, Cancellation, Availability and the two values engineering publishes, and nothing named after a Go package.

![The business view: only core concepts, with the two published values ringed.](docs/img/viewer-business.png)

**The review.** `gloss review` is the queue of everything an agent proposed, every link that went stale, every rogue concept and every finding, each with the gate it needs (AUTO, SOFT, HARD) and who may decide it. An agent clears the AUTO and SOFT items itself and leaves a recommendation on the HARD ones.

![The review terminal: a queue of proposed links, a stale link, a proposed concept, a rogue concept and untested findings, each tagged HARD, SOFT or AUTO, with the selected item's marker, concept doc and source beside it.](docs/img/review-terminal.png)

**The drift report.** `gloss drift` compares what each boundary's manifest declares with what the code under it derives, and names the difference:

```
$ gloss drift
✗ Violations (boundary exceeds its declared capability)
  acme:circulation-svc produces lib:Laptop but declares produces {lib:Media}; rollup: produces {lib:Item} (from lib:Book, lib:DVD, lib:Laptop, lib:Media) ⊄ declared [proposed link]
○ Gaps (declared, not implemented)
  acme:library-platform declares publishes lib:HoldReady but nothing under it publishes it: intended capability not implemented, or the declaration is wrong
? Rogue concepts (decision requests)
  services/circulation/loans/returns.py:10 implements circ:NEW:DamageCheck: rogue concept; becomes a decision request
◇ Presented but not implemented (a surface with no mechanism)
  lib:Hold is presented (apps/opac/src/PlaceHold.tsx#PlaceHold) but nothing implements it: a surface with no mechanism behind it
Concepts covered 22% (2/9 with an artifact and a verifier)   Orphans: 6   Rogue: 1   Violations: 2   Gaps: 1   Stale: 0   Untested: 5   Proposed links: 4   Proposed concepts: 0
```

The report is from [`examples/library`](examples/library/README.md) in this repository; the screenshots are from the [glossmark-examples](https://github.com/McKalvan/glossmark-examples) showcase, rendered by [`scripts/screenshots.sh`](scripts/screenshots.sh) with a `gloss` built from this tree.

## How it works in 60 seconds

- **The marker.** `@gloss <relation> <target>` in any comment, in 18 file types. `@gloss?` is a proposal with a `conf=`; `scope=file` links the whole file; `ns:NEW:Name` proposes a concept that does not exist yet. [Grammar](docs/GRAMMAR.md), [when to mark](docs/MARKING.md).
- **Two tiers.** A core concept (`lib:Book`, `clinic:Booking`) is implementation-independent by construction. An implementation concept either `realizes` one or is `internal`. The `published` flag is the only seam between the tiers and the only thing importable across repositories; touching it is HARD.
- **Manifests.** A `.gloss.yaml` on a directory declares what that boundary `realizes`, `produces`, `consumes`, `publishes` and `holds`, which concepts code under it may target, and how critical it is. [File formats](docs/FILE-FORMATS.md).
- **The graph.** `gloss build` compiles markers, ontology and manifests into `.glossmark/graph.jsonl`: deterministic, sorted, committed, meaningful in `git diff`. [Graph](docs/GRAPH.md).
- **Drift.** Five rollup rules derive from the code what each boundary does; `gloss drift` compares that with what the manifest declares. The difference is a violation, a gap, an orphan, a rogue concept, an untested mechanism, a stale link or a drifted value.
- **The gate.** `gloss diff` classifies a change AUTO, SOFT or HARD by what it touches (the published seam, constraints, criticality, agent remits) and the GitHub Action posts the impact on the PR: which views changed, which concepts, which blast radius. [Governance](docs/GOVERNANCE.md).
- **Views and agents.** `views.yaml` projects the graph by tier and zoom. The MCP server and a SessionStart hook give an agent the concepts for the directory it is in, the gate verdict before it writes, and the review tools.

## Why ontology-first

Tests existed before TDD. What changed was *when* they were written, and that is what let them shape the design instead of merely checking it. GlossMark moves the system's vocabulary to the same position: curated before a ticket is implemented, implemented against it, reviewed as the pair ([ADR 0034](docs/adr/0034-ontology-first-increments.md)). Three things follow that no linter and no summary gives you:

- **One concept spans every artifact type.** ArchUnit, dependency-cruiser, import-linter and Nx boundaries stop at one language's import graph. In the clinic above, `clinic:Booking` reaches a Go handler, a Next.js page, a Playwright test, a Go test, a Kubernetes deployment, a GitHub Actions workflow and the architecture doc, because the marker is a comment and comments are everywhere. Terraform, SQL, protobuf, Avro and AGENTS.md link the same way.
- **The vocabulary has an owner.** An agent can regenerate a summary of a codebase on demand, but nobody reviews it, nothing in it goes stale on purpose, and two runs disagree. An ontology entry has a namespace owner, a review trail in the ledger, a `proposed` state until someone accepts it, and a `planned` state with a deadline that turns a concept nobody built into a defect.
- **An agent is inside the contract from its first marker.** The session brief says which concepts the directory it is in may touch. `propose_link` returns the gate verdict before the code is written. A new concept ships as a proposal in the same PR as its implementation. Novelty is what people review; everything else is bookkeeping an agent can do.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/mckalvan/glossmark/main/install.sh | sh -s -- v0.2.0   # Linux/macOS binary into ~/.local/bin
go install github.com/mckalvan/glossmark/cmd/gloss@v0.2.0                                          # with a Go toolchain
```

Binaries for Linux, macOS and Windows are attached to each [release](https://github.com/mckalvan/glossmark/releases); the script verifies the sha256. In CI, `uses: mckalvan/glossmark@v0.2.0` downloads the same binary. Releases, grammar and file-format versions are pinned as described in [docs/VERSIONING.md](docs/VERSIONING.md).

## Try it

```sh
go build -o gloss ./cmd/gloss
cd examples/library/circulation
../../../gloss check       # 0 errors, 1 warning: the Laptop link is outside the boundary's allowed list
../../../gloss drift       # the drift report above
../../../gloss review      # the queue: 4 proposed links and a rogue concept to decide
../../../gloss view        # .glossmark/viewer.html: boundaries are boxes, click to drill in, click a concept for its code, tests and findings
cd ..
../../gloss landscape circulation holds-worker   # cross-repo context map; the HoldReady gap closes
```

To adopt it on your own repository, `gloss init` scaffolds the files and the [Quickstart](docs/QUICKSTART.md) walks the first ontology; the `adopt-glossmark` and `derive-ontology` skills (`gloss hook install --skill`) let an agent do the first pass. The [showcase](https://github.com/McKalvan/glossmark-examples) has five realistic adoptions to copy from: a monorepo, three repositories sharing one core, an event-driven system, an agent-heavy repository and a brownfield legacy stack.

## For agents

`gloss hook install` registers a SessionStart hook that briefs an agent on the directory it is working in (boundary, allowed targets, remits, concepts, current state) and an MCP server (`gloss mcp`) with concept search, `concepts_for_path`, `propose_link`, `propose_concept`, `impact_of`, `slice_of`, `drift` and the review tools. An agent re-confirms stale links and decides SOFT proposals itself, and records a recommendation for a person on anything HARD. See [docs/QUICKSTART.md](docs/QUICKSTART.md#agent-experience).

## Design in one paragraph

Two tiers plus instances: core is implementation-independent by construction; every implementation concept `realizes` a core concept or is `internal`; instances live outside both. The **namespace** is the unit of modularity, with one repo as the default layout and imports by path (git ref and URL to come). The **published** flag is the only seam between tiers and the only thing importable across repos; touching a published concept is HARD. Importers **extend, never mutate** (own-subject rule). **Views are projections** over tier × zoom; PR impact is "which views changed". Five deterministic rollup rules derive higher-level facts from code and compare them with what manifests declare; the comparison is the drift signal.

## Governance

Not every subsystem carries the same risk. `criticality: low | medium | high | critical` on a concept or a boundary flows through `realizes` to the code that implements it (a boundary's level reaches its files only with `criticality-floor: true`; `gloss criticality suggest` drafts a first classification), tags every change in the PR comment and `gloss impact`, raises the gate by policy (`critical` is HARD by default), colours the viewer and gives agents a reason to flag what they touch. See [docs/GOVERNANCE.md](docs/GOVERNANCE.md).

## More

- [docs/REPOSITORY.md](docs/REPOSITORY.md): what is in this repository, GlossMark applied to itself (this tree is its own first user and CI holds it to the strictest gate), versions and roadmap status.
- Docs: [Quickstart](docs/QUICKSTART.md) · [When to mark](docs/MARKING.md) · [Advanced](docs/ADVANCED.md) · [Grammar](docs/GRAMMAR.md) · [File formats](docs/FILE-FORMATS.md) · [Graph](docs/GRAPH.md) · [Governance](docs/GOVERNANCE.md) · [Versioning](docs/VERSIONING.md) · [Plan](docs/PLAN.md) · [ADRs](docs/adr/)
- [glossmark-examples](https://github.com/McKalvan/glossmark-examples): the showcase behind the screenshots, and a lab that turns rough edges into issues.
