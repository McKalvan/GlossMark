<!-- @gloss documents arch:glossmark scope=file -->
# Quickstart

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/mckalvan/glossmark/main/install.sh | sh -s -- v0.1.0   # binary into ~/.local/bin, sha256 verified
go install github.com/mckalvan/glossmark/cmd/gloss@v0.1.0                                          # with a Go toolchain
make build                                                                                         # from a checkout; injects the git tag as the version
gloss version                                                                                      # tool, grammar and file-format versions
```

Pin a release everywhere the tool runs (`@v0.1.0`, never `@latest` or `@main`): binaries for Linux, macOS and Windows are attached to each [GitHub release](https://github.com/mckalvan/glossmark/releases), and `go install ...@v0.1.0` reports the same version. See [VERSIONING.md](VERSIONING.md).

## 0. Scaffold

```sh
gloss init --dry-run     # print the plan: prefixes guessed from the repo name, one manifest per deployable directory, then hook and build
gloss init               # write it, install the agent hook, build the first graph (.glossmark/graph.jsonl)
gloss check              # passes on the empty scaffold
```

`gloss init` writes `glossmark.yaml` (with `repo-iri:`, so boundary prefixes need no `prefixes:` entry), the two tier files with the litmus test in a comment, `ontology/views.yaml`, a root manifest and one container manifest per deployable directory, `.gitignore` lines for the viewer only (the graph and the ledger are committed), a `gloss` target in the `Makefile` or a script in `package.json`, a CI job pinned to the release, and the SessionStart hook and MCP server, then builds the first graph. It never overwrites a file; run it again and it skips what exists. The sections below explain what it wrote.

- **Deployable directories.** Each top-level directory holding source, schema (`.avsc`, `.proto`, `.graphql`), infrastructure (Terraform, Kubernetes or Helm YAML) or configuration files gets a manifest. A top-level directory with no project file of its own (`package.json`, `go.mod`, `pyproject.toml`, `Cargo.toml`, `Dockerfile`, ...) whose children have one is a grouping (`apps/`, `services/`): each child gets the manifest instead (`apps/web`, `services/booking-api`). A directory named like shared code (`utils`, `util`, `helpers`, `common`, `core`, `lib`, `shared`, `misc`, ...) gets none unless it holds an entry point (a project or build file, a `Dockerfile`, or a `main`, `server`, `wsgi` or `routes` source file); the plan lists it as "not a boundary" and its code stays under the parent's manifest.
- **Prefixes.** `--core`, `--impl` and `--boundary-prefix` override the guesses from the directory name (initials, which rarely read as a domain: pick a word).
- **IRIs.** `--iri` is a base shared by sibling repositories: each namespace gets `<iri><prefix>/` and the repository `<iri><name>/`, so two siblings initialised with the company base never share a namespace IRI. `gloss landscape` refuses two prefixes bound to one IRI.
- **Importers.** `--import ../shipment-core` (repeatable) imports the sibling's core namespace instead of owning one: `imports:` gets its IRI, `^<current version>` and a `source:` path, no core tier file is written, and `init` runs `gloss lock`.
- **A workspace below the git root.** GitHub runs workflows only at the git root, so `init` writes no `.github/` there and prints the job with `root:` set to the workspace instead; `gloss doctor` looks for it in the git root's workflows.
- **An existing vocabulary.** A `glossary.md` or an `enums.yaml` in the tree is listed with the `gloss import` command that reads it (section 2).

## 1. Declare the namespaces

`glossmark.yaml` at the repo root:

```yaml
owns:
  lib:  { iri: https://onto.example.com/library/,     tier: core,           version: 0.1.0, owner: library-services }
  circ: { iri: https://onto.example.com/circulation/, tier: implementation, version: 0.1.0, owner: circulation-eng }
prefixes:
  acme: https://arch.example.com/boundaries/
```

## 2. Write the core in words the business recognises

`ontology/lib.gloss.yaml`:

```yaml
namespace: lib
concepts:
  Loan:     { kind: entity }
  Checkout: { kind: process, produces: [Loan] }
  Return:   { kind: process, consumes: [Loan] }
```

Litmus test: if a domain expert would nod at the term, it is core; if only an engineer would, it is implementation.

If the repository already keeps its vocabulary, import it instead of retyping it, and feed the result back to whatever checks it:

```sh
gloss import --from markdown-glossary docs/glossary.md --namespace lib   # headings → concepts, Def. → doc, Aliases, Kind, Status (not implemented → proposed: true)
gloss import --from enums enums.yaml --namespace lib [--values]          # each enum an entity; --values adds each value as is-a the enum (--value-names prefixed: LoanStatusActive)
gloss export --status -o status.json                                     # implemented / untested / unimplemented / orphan / proposed per concept
```

A glossary entry is a `##` to `####` heading followed by field lines: `**Def.**` (the doc; else the first paragraph), `**Aliases:**`, `**Kind:**` (`entity`, `process`, `event`, `constraint`, `measure`, ...), `**Is-a:**` and `**Status:**` (`planned` makes it `proposed: true`; `deprecated ... use Patron` or `replaced by Patron` sets `deprecated: true` and `replaced-by:`). A heading with no field line that is followed by a deeper heading is a section (`## Retired terms` above `### Borrower`) and is skipped. Without a `Kind:` line the kind is guessed from the definition (one that starts "Lending ..." or "The process of ..." is a process; one that says "never", "at most" or "how long" is a constraint), then from the name's ending; `gloss import` prints each added name with its kind and where the guess came from. Review them.

Imports are re-runnable: new terms are appended, and everything else in the file (kinds you fixed, comments, alignment) is left alone. `.glossmark/import-record.jsonl` (commit it) records what each import read from the source, so a re-run updates a definition or alias list only when the source changed since the last import: a doc you improved by hand stays until the source changes, and a concept you deleted or renamed after an import is not added again.

## 3. Write the implementation that realizes it

`ontology/circ.gloss.yaml`:

```yaml
namespace: circ
concepts:
  CheckoutHandler: { kind: process, realizes: lib:Checkout }
  LoanStore:       { kind: resource, internal: true }
```

## 4. Put a manifest on each service directory

`services/circulation/.gloss.yaml`:

```yaml
boundary: acme:circulation-svc
level: container
realizes: [lib:Checkout, lib:Return]
default-namespace: circ
# allowed: is optional. Without it, links may target circ:*, what the manifest declares, their parts and subtypes, what realizes them and the cores they realize;
# gloss check --explain prints the set. Write it only to narrow or to add a concept the boundary uses without declaring.
```

## 5. Mark the code

Five relations cover almost every marker; learn these and ignore the rest until you need them ([MARKING.md](MARKING.md) says when to mark, [ADVANCED.md](ADVANCED.md) has everything else):

```python
# @gloss implements .CheckoutHandler         # the code that does the work (handler, pipeline, module)
def check_out(card_number, barcode): ...
```
```python
# @gloss represents lib:Loan                 # the data shape: a model, a table, a schema
class Loan: ...
```
```python
# @gloss verifies lib:Checkout               # in tests
def test_checkout_refuses_expired_cards(): ...
```
```markdown
<!-- @gloss documents lib:Checkout -->       # in docs
<!-- @gloss instructs circ:CheckoutHandler scope=file -->   # in agent files
```

Mark one public surface per concept, not every helper; add `scope=file` when a whole file is about one concept, and then no symbol markers in it. The marker goes above the declaration and its decorators.

Agents write `@gloss?` for links they propose. The one proposal workflow, which the briefing, `AGENTS.md` and the MCP server state in the same words:

> If no concept fits, write `@gloss? <relation> <ns>:NEW:<Name> note="why"`, then run `gloss propose --from-new --doc "what it is"` (MCP: propose_concept) so the concept ships in the same PR with `proposed: true`; a human accepts it with `gloss accept --concept <ns>:<Name>`. Never invent a concept name any other way, and never hand-edit an ontology file to make a link resolve.

On an existing codebase, let the tool draft the first pass and review it rather than writing every marker by hand:

```sh
gloss suggest > suggestions.patch   # @gloss? proposals for unmarked public surfaces, from names, docstrings, tables touched and test calls
git apply suggestions.patch          # or gloss suggest --write; then delete the wrong ones and retarget the near misses
gloss hook install --skill           # installs the adopt-glossmark and derive-ontology skills: the whole adoption, and deriving the ontology from what exists, as agent workflows (works before gloss init)
```

Generate the vocabulary agents should see first:

```sh
gloss agents --dir services/circulation --write AGENTS.md
```

## 6. Run the tooling

```sh
gloss check                      # lint: syntax, dangling refs, domain/range, own-subject rule, tiers, import ranges, lockfile
gloss lock                       # pin imports: glossmark.lock + vendored copies in .glossmark/imports/ (--update to move)
gloss build                      # .glossmark/graph.jsonl + context.jsonld + ledger.jsonl
gloss drift                      # violations, gaps, orphans, rogues, stale, untested, metrics
gloss diff --base origin/main    # AUTO/SOFT/HARD per change, views changed, PR comment (grouped by boundary; counts per boundary when the base has no graph, or --baseline)
gloss status                     # proposed / stale / rejected links
gloss accept --policy            # accept every SOFT proposal (or: gloss accept <id|path:line>)
gloss view                       # .glossmark/viewer.html + views.js, static, no server: boundaries are boxes, click to drill in, double-click to focus, click a concept for its code, tests and findings
gloss coverage --fail-under 80   # headline: concepts with an artifact and a verifier (implementation concepts, internal ones only at high or critical, and core concepts code implements directly; n/a when none); files are secondary
gloss coverage --fail-under current --changed-only   # ratchet: never below the base's coverage; score only the files this change touches
gloss export --sqlite graph.db
gloss publish                    # dist/glossmark/<ns>/ontology.jsonld and <ns>.gloss.yaml (published concepts only for impl namespaces, + graph.jsonl)
gloss landscape ../repo-a ../repo-b   # union several repos, context-map edges, cross-repo drift, import versions, deprecated concepts still in use
gloss mcp                        # MCP server on stdio for agents
gloss doctor                     # one line per setup check with its fix: gloss on PATH (and that it is GlossMark), GLOSS_ACTOR, hook, graph current, ledger in git, lockfile, CI pinned, docs intact
```

### Who wrote a link, and what was decided

`.glossmark/ledger.jsonl` is the review record: who wrote each marker, whether a human accepted or rejected it, and whether it went stale. **Commit it** together with `graph.jsonl`; without it a fresh clone or a CI run forgets every decision. `gloss hook install` says so, and the pre-commit hook stages it.

The actor belongs to a marker's first appearance, not to whoever runs the last build. For a marker the ledger does not know yet, `gloss build` takes, in order: the actor the last built `graph.jsonl` recorded for it; `GLOSS_ACTOR` (`agent:claude-code@claude-fable-5-1`, `human:alice`); the `git blame` author of the marker line when it is committed; else `human:<git user.name>`. `gloss hook install` writes `GLOSS_ACTOR=agent:claude-code` into the SessionStart hook and the MCP server's env (`--actor` to change it), and calls `gloss` by name only when `PATH` finds the binary running the install (else by its absolute path), so links an agent writes are attributed to the agent and the agent-remit gate (`may-modify`) applies to them. Set `GLOSS_ACTOR` yourself in any other agent harness. `gloss build` warns when a rebuild would change the actor on an existing marker.

## Agent experience

Four commands exist so an agent can work the ontology without a human in the loop until the review:

```sh
gloss hook install                      # SessionStart hook → 'gloss context --changed --budget 2000'; MCP server in .mcp.json
gloss context --dir services/circulation # the briefing: boundary, allowed targets, your remit, concepts most relevant to the directory first, state
gloss context --changed --budget 2000   # open with what the uncommitted changes touch; fit the whole briefing in ~2 000 tokens
gloss impact --base origin/main         # what the working tree touches: published seam, constraints, tests, blast radius
gloss impact services/circulation/loans/renew.py
gloss propose concept circ:RenewalHandler --kind process --realizes lib:Renewal --doc "Handles renewal requests"
gloss propose --from-new                     # lift every ns:NEW:Name marker into a proposed concept and rewrite the markers
gloss accept --concept circ:RenewalHandler   # a human accepts the proposal (drops proposed: true)
gloss reject --concept circ:RenewalHandler --reason "duplicate of circ:RenewalFlow" --remove
```

The briefing ranks concepts by relevance: those linked by code under the directory, then those its manifest declares, then its default namespace, then the rest. When `GLOSS_ACTOR` names an agent with an agent file (`agent:cataloging-assistant` and `.claude/agents/cataloging-assistant.md`), it shows only that agent's remit. In a retiring module it adds a "Retiring here" line: the deprecated boundary (or one enclosing it) with its `replaced-by` and `retire-by`, and each deprecated concept the code links to with its successor and the boundary that realizes it.

A proposed concept carries `proposed: true` and `proposed-by:` in the ontology file, the ontology's equivalent of `@gloss?`. It exists so links resolve, it shows in `gloss drift` and `gloss status`, and it is HARD until a human accepts it, so the proposal and its implementation ship in one PR. `search_concepts` matches aliases (`aliases: [borrower]` on `lib:Patron`), CamelCase words, stems and small typos, and follows `realizes`/`is-a` one hop, so an agent that asks for "pickup window" finds `hw:PickupClock`.

Over MCP the same capabilities are `context`, `impact_of` (with `paths` or `base`), `propose_concept`, `propose_link`, `search_concepts`, `concepts_for_path`, `lookup_concept`, `slice_of` and `drift`. `gloss slice <curie>` (MCP `slice_of`, `gloss context --concept`) shows everything relevant to one feature; see [ADVANCED.md](ADVANCED.md#feature-slices-gloss-slice).

## 7. Wire CI

`.github/workflows/gloss.yml`:

```yaml
on: [pull_request, push]
permissions: { contents: read, pull-requests: write }
jobs:
  gloss:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: mckalvan/glossmark@v0.1.0
        with: { fail-on: HARD, coverage-fail-under: "0" }
```

On the first PR (the base has no graph) the comment reports counts per boundary and per view plus the decision requests instead of one row per link; later PRs list changes grouped by boundary in collapsible sections, each capped. `coverage-fail-under: current` fails only when a PR lowers the base's concept coverage.

A repository with several workspaces runs the Action once per `root:` (a matrix); each root keeps its own comment, keyed by `<!-- glossmark-impact:<root> -->` and named in its heading, and a re-run updates it in place.

The Action downloads the release binary named by `version:` (default: the Action's own release) and falls back to `go install` only when no binary exists for it.

Pre-commit: `ln -s ../../hooks/pre-commit .git/hooks/pre-commit`, or add the hooks from `.pre-commit-hooks.yaml` to your `.pre-commit-config.yaml` with `rev: v0.1.0`.

## Review workflow

- Humans review novelty and hierarchy; everything else flows through policy.
- **AUTO**: fingerprint-only changes, derived edges. Cleared by the engine.
- **SOFT**: a link to an existing concept, valid, inside `allowed`, at or above the auto-accept threshold derived from `suggest.auto-accept-precision`, altering only the engineering view. Cleared by the engine; audit a sample later.
- **HARD**: a new concept proposed, a link outside an agent's `may-modify` remit or outside `allowed`, any edit to `ontology/` or manifests, anything touching a published concept, MAJOR bumps, cross-repo imports. A human decides in the PR.

Suggested policy (not framework behaviour): review per PR, never per marker; auto-accept SOFT; audit a random share weekly.

**Choose the precision, not the threshold.** `suggest.auto-accept-precision` in `glossmark.yaml` (default 0.9) is how often an auto-accepted proposal must be right. The tool derives the conf threshold from it and prints it, with its measured precision and the share it auto-accepts, in `gloss suggest` and `gloss doctor`. With the shipped calibration the default derives `conf >= 0.80`. Once some of your code is reviewed, measure the repository itself: `gloss suggest --calibrate` strips your markers in a temporary copy, suggests on it and scores the result against your own links. With `suggest: { calibration: glossmark.calibration.tsv }` set, `gloss suggest --calibrate --write` saves the counts, and the threshold is derived from your repository from then on. When nothing reaches the target, nothing is auto-accepted and doctor says so ([ADR 0024](adr/0024-auto-accept-precision.md)). `gloss suggest --labels > labels.tsv` and `gloss suggest --eval labels.tsv` on a copy with the markers stripped still do the same by hand.
