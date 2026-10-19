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
gloss init --dry-run     # print the plan: prefixes guessed from the repo name, one manifest per top-level source directory
gloss init               # write it, install the agent hook, build the first graph
gloss check              # passes on the empty scaffold
```

`gloss init` writes `glossmark.yaml` (with `repo-iri:`, so boundary prefixes need no `prefixes:` entry), the two tier files with the litmus test in a comment, `ontology/views.yaml`, a root manifest and one per top-level source directory, `.gitignore` lines for the viewer only (the graph and the ledger are committed), a `gloss` target in the `Makefile` or a script in `package.json`, a CI job pinned to the release, and the SessionStart hook and MCP server. It never overwrites a file; run it again and it skips what exists. `--core`, `--impl` and `--iri` override the guesses. The sections below explain what it wrote.

## 1. Declare the namespaces

`glossmark.yaml` at the repo root:

```yaml
owns:
  pay: { iri: https://onto.example.com/pay/, tier: core,           version: 0.1.0, owner: payments-product }
  pa:  { iri: https://onto.example.com/pa/,  tier: implementation, version: 0.1.0, owner: payments-eng }
prefixes:
  acme: https://arch.example.com/boundaries/
```

## 2. Write the core in words the business recognises

`ontology/pay.gloss.yaml`:

```yaml
namespace: pay
concepts:
  Payment:       { kind: entity }
  Authorization: { kind: process, produces: [Payment] }
  Capture:       { kind: process, consumes: [Payment] }
```

Litmus test: if a domain expert would nod at the term, it is core; if only an engineer would, it is implementation.

If the repository already keeps its vocabulary, import it instead of retyping it, and feed the result back to whatever checks it:

```sh
gloss import --from markdown-glossary docs/glossary.md --namespace pay   # headings → concepts, Def. → doc, Aliases, Status (not implemented → proposed: true)
gloss import --from enums enums.yaml --namespace pay [--values]          # each enum an entity; --values adds each value as is-a the enum
gloss export --status -o status.json                                     # implemented / untested / orphan / proposed per concept
```

Imports are re-runnable: new terms are appended, a changed definition or alias list updates that one line, everything else in the file (kinds you fixed, comments, alignment) is left alone. Kinds are guessed when the source has none; review them.

## 3. Write the implementation that realizes it

`ontology/pa.gloss.yaml`:

```yaml
namespace: pa
concepts:
  AuthorizeHandler: { kind: process, realizes: pay:Authorization }
  CaptureQueue:     { kind: resource, internal: true }
```

## 4. Put a manifest on each service directory

`services/api/.gloss.yaml`:

```yaml
boundary: acme:payments-api
level: container
realizes: [pay:Authorization, pay:Capture]
default-namespace: pa
# allowed: is optional. Without it, links may target pa:*, what the manifest declares, their parts and the cores they realize;
# gloss check --explain prints the set. Write it only to narrow or to add a concept the boundary uses without declaring.
```

## 5. Mark the code

Five relations cover almost every marker; learn these and ignore the rest until you need them ([MARKING.md](MARKING.md) says when to mark, [ADVANCED.md](ADVANCED.md) has everything else):

```python
# @gloss implements .AuthorizeHandler        # the code that does the work (handler, pipeline, module)
def authorize(req): ...
```
```python
# @gloss represents pay:Payment              # the data shape: a model, a table, a schema
class Payment: ...
```
```python
# @gloss verifies pay:Authorization          # in tests
def test_authorize_declines_expired_cards(): ...
```
```markdown
<!-- @gloss documents pay:Authorization -->  # in docs
<!-- @gloss instructs pa:AuthorizeHandler scope=file -->   # in agent files
```

Mark one public surface per concept, not every helper; add `scope=file` when a whole file is about one concept, and then no symbol markers in it. The marker goes above the declaration and its decorators.

Agents write `@gloss?` for links they propose. The one proposal workflow, which the briefing, `AGENTS.md` and the MCP server state in the same words:

> If no concept fits, write `@gloss? <relation> <ns>:NEW:<Name> note="why"`, then run `gloss propose --from-new --doc "what it is"` (MCP: propose_concept) so the concept ships in the same PR with `proposed: true`; a human accepts it with `gloss accept --concept <ns>:<Name>`. Never invent a concept name any other way, and never hand-edit an ontology file to make a link resolve.

On an existing codebase, let the tool draft the first pass and review it rather than writing every marker by hand:

```sh
gloss suggest > suggestions.patch   # @gloss? proposals for unmarked public declarations, matched by name and docstring
git apply suggestions.patch          # or gloss suggest --write; then delete the wrong ones and retarget the near misses
gloss hook install --skill           # installs the adopt-glossmark skill: the whole adoption as a step-by-step agent workflow
```

Generate the vocabulary agents should see first:

```sh
gloss agents --dir services/api --write AGENTS.md
```

## 6. Run the tooling

```sh
gloss check                      # lint: syntax, dangling refs, domain/range, own-subject rule, tiers, import ranges, lockfile
gloss lock                       # pin resolved import versions in glossmark.lock
gloss build                      # .glossmark/graph.jsonl + context.jsonld + ledger.jsonl
gloss drift                      # violations, gaps, orphans, rogues, stale, untested, metrics
gloss diff --base origin/main    # AUTO/SOFT/HARD per change, views changed, PR comment (grouped by boundary; counts per boundary when the base has no graph, or --baseline)
gloss status                     # proposed / stale / rejected links
gloss accept --policy            # accept every SOFT proposal (or: gloss accept <id|path:line>)
gloss view                       # .glossmark/viewer.html + views.js, static, no server: boundaries are boxes, click to drill in, double-click to focus, click a concept for its code, tests and findings
gloss coverage --fail-under 80   # headline: implementation concepts with an artifact and a verifier; files are secondary
gloss coverage --fail-under current --changed-only   # ratchet: never below the base's coverage; score only the files this change touches
gloss export --sqlite graph.db
gloss publish                    # dist/glossmark/<ns>/ontology.jsonld (+ graph.jsonl for impl namespaces)
gloss landscape ../repo-a ../repo-b   # union several repos, context-map edges, cross-repo drift
gloss mcp                        # MCP server on stdio for agents
gloss doctor                     # one line per setup check with its fix: gloss on PATH, GLOSS_ACTOR, hook, graph current, ledger in git, lockfile, CI pinned, docs intact
```

### Who wrote a link, and what was decided

`.glossmark/ledger.jsonl` is the review record: who wrote each marker, whether a human accepted or rejected it, and whether it went stale. **Commit it** together with `graph.jsonl`; without it a fresh clone or a CI run forgets every decision. `gloss hook install` says so, and the pre-commit hook stages it.

The actor belongs to a marker's first appearance, not to whoever runs the last build. For a marker the ledger does not know yet, `gloss build` takes, in order: the actor the last built `graph.jsonl` recorded for it; `GLOSS_ACTOR` (`agent:claude-code@claude-fable-5-1`, `human:alice`); the `git blame` author of the marker line when it is committed; else `human:<git user.name>`. `gloss hook install` writes `GLOSS_ACTOR=agent:claude-code` into the SessionStart hook and the MCP server's env (`--actor` to change it), so links an agent writes are attributed to the agent and the agent-remit gate (`may-modify`) applies to them. Set `GLOSS_ACTOR` yourself in any other agent harness. `gloss build` warns when a rebuild would change the actor on an existing marker.

## Agent experience

Four commands exist so an agent can work the ontology without a human in the loop until the review:

```sh
gloss hook install                      # SessionStart hook → 'gloss context --changed --budget 2000'; MCP server in .mcp.json
gloss context --dir services/api        # the briefing: boundary, allowed targets, your remit, concepts most relevant to the directory first, state
gloss context --changed --budget 2000   # open with what the uncommitted changes touch; fit the whole briefing in ~2 000 tokens
gloss impact --base origin/main         # what the working tree touches: published seam, constraints, tests, blast radius
gloss impact services/api/handlers/refund.py
gloss propose concept pa:RefundHandler --kind process --realizes pay:Refunding --doc "Handles refund requests"
gloss propose --from-new                # lift every ns:NEW:Name marker into a proposed concept and rewrite the markers
gloss accept --concept pa:RefundHandler # a human accepts the proposal (drops proposed: true)
gloss reject --concept pa:RefundHandler --reason "duplicate of pa:RefundFlow" --remove
```

The briefing ranks concepts by relevance: those linked by code under the directory, then those its manifest declares, then its default namespace, then the rest. When `GLOSS_ACTOR` names an agent with an agent file (`agent:breeding-planner` and `.claude/agents/breeding-planner.md`), it shows only that agent's remit.

A proposed concept carries `proposed: true` and `proposed-by:` in the ontology file, the ontology's equivalent of `@gloss?`. It exists so links resolve, it shows in `gloss drift` and `gloss status`, and it is HARD until a human accepts it, so the proposal and its implementation ship in one PR. `search_concepts` matches aliases (`aliases: [chargeback]` on `pay:Refund`), CamelCase words, stems and small typos, and follows `realizes`/`is-a` one hop, so an agent that asks for "settlement cutoff" finds `sw:CutoffClock`.

Over MCP the same capabilities are `context`, `impact_of` (with `paths` or `base`), `propose_concept`, `propose_link`, `search_concepts`, `concepts_for_path`, `lookup_concept` and `drift`.

### Guards: feedback between the edit and the commit

`gloss hook install --guards` also registers three Claude Code hooks that run `gloss hook guard`. Each reads the hook payload on stdin and prints at most a few short lines.

```sh
gloss hook install --guards                 # SessionStart, MCP, and the three guards below
gloss hook install --skills gloss-link      # install named shipped skills; --skill installs all of them
```

| Event | Hook | What the guard does |
| --- | --- | --- |
| `pre-edit` | PreToolUse on `Edit\|Write` | Names the file's criticality when it is `high` or `critical` and where it comes from, the published-seam concepts it touches, and whether it is outside the acting agent's `may-modify` remit. Says nothing about an unmarked, low-risk file. |
| `post-edit` | PostToolUse on `Edit\|Write` | Reports the `gloss check` findings that point at the edited file, such as GM130, GM131 or GM502. Says nothing when the file is clean. |
| `stop` | Stop | Blocks the agent from finishing while `gloss check` reports errors or `.glossmark/graph.jsonl` does not match the tree, and names the fix (`gloss check`, `gloss build -q`). It never blocks on proposals, and never when Claude Code reports that a Stop hook is already active. |

The hooks it writes, with the default actor:

```
PreToolUse  (Edit|Write)  GLOSS_ACTOR=agent:claude-code gloss hook guard --event pre-edit 2>/dev/null || true
PostToolUse (Edit|Write)  GLOSS_ACTOR=agent:claude-code gloss hook guard --event post-edit 2>/dev/null || true
Stop                      GLOSS_ACTOR=agent:claude-code gloss hook guard --event stop
```

Re-running `gloss hook install --guards` replaces these entries and keeps every other hook in `.claude/settings.json`.

Inside a subagent, Claude Code passes the subagent's name as `agent_type`. When `.claude/agents/<agent_type>.md` exists, the guard judges the edit for `agent:<agent_type>` and that file's `may-modify` remit; otherwise it uses `GLOSS_ACTOR`. An edit outside the remit gets a warning, and the PR gate makes any link written there HARD.

Two policy keys in `glossmark.yaml` tune the guards:

```yaml
policy:
  guard-stop: report          # default block; report prints the reason to stderr and lets the agent stop
  guard-ask: [critical, remit]  # default []; ask before an edit to a critical file or one outside the agent's remit
```

`guard-ask` lists criticality levels, each matching files at exactly that level, and `remit`. For a listed case the pre-edit guard returns `permissionDecision: "ask"`, so Claude Code asks the user before the edit.

The guard fails open. When `gloss` cannot find `glossmark.yaml`, cannot read the payload or cannot build the graph, it prints nothing and exits 0. `gloss doctor` reports a broken setup. The guard reads the tree and writes no files; it takes about 0.2 s on GlossMark's own repository.

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

The Action downloads the release binary named by `version:` (default: the Action's own release) and falls back to `go install` only when no binary exists for it.

Pre-commit: `ln -s ../../hooks/pre-commit .git/hooks/pre-commit`, or add the hooks from `.pre-commit-hooks.yaml` to your `.pre-commit-config.yaml` with `rev: v0.1.0`.

## Review workflow

- Humans review novelty and hierarchy; everything else flows through policy.
- **AUTO**: fingerprint-only changes, derived edges. Cleared by the engine.
- **SOFT**: a link to an existing concept, valid, inside `allowed`, above the confidence threshold, altering only the engineering view. Cleared by the engine; audit a sample later.
- **HARD**: a new concept proposed, a link outside an agent's `may-modify` remit or outside `allowed`, any edit to `ontology/` or manifests, anything touching a published concept, MAJOR bumps, cross-repo imports. A human decides in the PR.

Suggested policy (not framework behaviour): review per PR, never per marker; auto-accept SOFT; audit a random share weekly and calibrate `soft-conf-threshold` from agreement.
