<!-- @gloss documents arch:glossmark scope=file -->
# Quickstart

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/mckalvan/glossmark/main/install.sh | sh -s -- v0.3.0   # binary into ~/.local/bin, sha256 verified
go install github.com/mckalvan/glossmark/cmd/gloss@v0.3.0                                          # with a Go toolchain
make build                                                                                         # from a checkout; injects the git tag as the version
gloss version                                                                                      # tool, grammar and file-format versions
```

Pin a release everywhere the tool runs (`@v0.3.0`, never `@latest` or `@main`): binaries for Linux, macOS and Windows are attached to each [GitHub release](https://github.com/mckalvan/glossmark/releases), and `go install ...@v0.3.0` reports the same version. See [VERSIONING.md](VERSIONING.md).

## 0. Scaffold

```sh
gloss init --dry-run     # print the plan: prefixes guessed from the repo name, one manifest per deployable directory, then hook and build
gloss init               # write it, install the agent hook, build the first graph (.glossmark/graph.jsonl, a header-only ledger.jsonl)
gloss check              # passes on the empty scaffold, and names the next step while no concept is defined
```

`init` ends with the path the tooling supports: draft concepts from the code with `gloss suggest --concepts` (or let an agent interview you with the adopt-glossmark skill, `gloss hook install --skill`), review the drafts and write the ones you keep into the tier files (sections 2 and 3), then `gloss suggest` to propose `@gloss?` markers for the code (section 5), and `gloss check && gloss build && gloss drift`.

`gloss init` writes `glossmark.yaml` (with `repo-iri:`, so boundary prefixes need no `prefixes:` entry), the two tier files with the litmus test in a comment, `ontology/views.yaml`, a root manifest and one container manifest per deployable directory, `.gitignore` lines for the viewer and `.claude/settings.local.json` (the graph and the ledger are committed), a `gloss` target in the `Makefile` or a script in `package.json`, a CI job pinned to the release, and the SessionStart hook and MCP server, then builds the first graph. It never overwrites a file; run it again and it skips what exists. The sections below explain what it wrote.

- **Deployable directories.** Each top-level directory holding source, schema (`.avsc`, `.proto`, `.graphql`), infrastructure (Terraform, Kubernetes or Helm YAML) or configuration files gets a manifest. A top-level directory with no project file of its own (`package.json`, `go.mod`, `pyproject.toml`, `Cargo.toml`, `Dockerfile`, ...) whose children have one is a grouping (`apps/`, `services/`): each child gets the manifest instead (`apps/web`, `services/booking-api`). A directory named like shared code (`utils`, `util`, `helpers`, `common`, `core`, `lib`, `shared`, `misc`, ...) gets none unless it holds an entry point (a project or build file, a `Dockerfile`, or a `main`, `server`, `wsgi` or `routes` source file); the plan lists it as "not a boundary" and its code stays under the parent's manifest.
- **Prefixes.** `--namespace` (or `--core`), `--impl` and `--boundary-prefix` override the guesses from the directory name (initials, which rarely read as a domain: pick a word). `--namespace inv` gives `inv:`, `invi:` and `invb:`; the plan says when the prefixes are guessed, and every marker carries them, so settle them with `--dry-run` first.
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
gloss import --from enums enums.yaml --namespace lib [--values]          # each enum an entity; --values adds each value as is-a the enum (--value-names prefixed: LoanStatusActive); --instances Plan,Region imports those enums but not their values, which are catalogue instances, not concepts
gloss export --status -o status.json                                     # implemented / untested / unimplemented / orphan / planned / proposed per concept, deprecated: true when retired
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
gloss hook install --skill           # installs the adopt-glossmark, derive-ontology, review-glossmark and curate-increment skills: the whole adoption, deriving the ontology from what exists, working the review queue after, and planning each feature in the ontology before it is built, as agent workflows (works before gloss init)
gloss hook install --skill --cursor  # the same for Cursor: an always-applied rule in .cursor/rules/glossmark.mdc that points at the skills
```

Generate the vocabulary agents should see first:

```sh
gloss agents --dir services/circulation --write AGENTS.md
```

The block gives the marker syntax (with `id=`), the inline relations (`verifies` for tests, not its alias `tests`), and one table row per concept with its kind, its effective criticality, and notes: `PROPOSED` for a concept awaiting acceptance, its doc, what it realizes and its aliases.

## 6. Run the tooling

```sh
gloss check                      # lint: syntax, dangling refs, domain/range, own-subject rule, tiers, import ranges, lockfile
gloss lock                       # pin imports: glossmark.lock + vendored copies in .glossmark/imports/ (--update to move)
gloss build                      # .glossmark/graph.jsonl + context.jsonld + ledger.jsonl
gloss drift                      # violations, gaps, orphans, rogues, stale, untested, metrics
gloss diff --base origin/main    # AUTO/SOFT/HARD per change, views changed, PR comment (grouped by boundary; counts per boundary when the base has no graph, or --baseline; ends with each touched concept's recent history)
gloss status                     # proposed / stale / rejected links; each proposal with its gate and the reason gloss diff gives ([HARD: outside allowed; conf 0.55 below threshold 0.70])
gloss accept --policy            # accept every SOFT proposal (or: gloss accept <id|path:line>)
gloss view                       # .glossmark/viewer.html + views.js, static, no server: boundaries are boxes, click to drill in, double-click to focus, click a concept for its code, tests and findings
gloss diff --base origin/main --format json -o d.json && gloss view --diff d.json   # the map with the change overlaid: added green, removed red ghosts, changed dashed; the Changes selector (or #changes=0..3) shows only what changed (0: changed nodes and both ends of a changed edge) or that plus everything within 1 to 3 hops over edges and part-of, fainter with each hop, opening every box that holds them (a boundary is shown but does not pass a hop on, since its edges reach most of the system); the Change filter splits added, removed, changed, edge changed, 1, 2 and 3 away, and contains changes
gloss view --history             # every commit in a History panel: click one to overlay it (the map as of that commit, newer additions hidden), shift+click another for the net change between them, [ and ] to step, #commit=<sha> or #commit=<a>..<b> to link one
gloss view --source-url 'https://github.com/o/r/blob/main/{path}#L{line}'   # link every path in the concept pane and drift list to the code host ({path}, {line}); or vscode to open them in VS Code (writes this machine's repository path into the page, so keep it out of published pages); default: source-url in glossmark.yaml, else no links
gloss history gm:Increment       # when a concept or boundary was added, changed, deprecated or removed, by whom and at which gate (ADR 0040)
gloss coverage --fail-under 80   # headline: concepts with an artifact and a verifier (implementation concepts, internal ones only at high or critical, and core concepts code implements directly; n/a when none); files are secondary
gloss coverage --fail-under current --changed-only   # ratchet: never below the base's coverage; score only the files this change touches
gloss export --sqlite graph.db
gloss report compliance --format html -o compliance.html   # per rule: code, tests, published value, who decided each link and why, open findings and waivers (docs/GOVERNANCE.md)
gloss publish                    # dist/glossmark/<ns>/ontology.jsonld and <ns>.gloss.yaml (published concepts only for impl namespaces, + graph.jsonl)
gloss landscape ../repo-a ../repo-b   # union several repos, context-map edges, cross-repo drift, import versions, deprecated concepts still in use
gloss mcp                        # MCP server on stdio for agents
gloss doctor                     # one line per setup check with its fix: gloss on PATH (and that it is GlossMark), GLOSS_ACTOR, hook, graph current, ledger in git, lockfile, CI pinned, docs intact
```

### Who wrote a link, and what was decided

`.glossmark/ledger.jsonl` is the review record: who wrote each marker, whether a human accepted or rejected it, and whether it went stale. **Commit it** together with `graph.jsonl`; without it a fresh clone or a CI run forgets every decision. `gloss hook install` says so, and the pre-commit hook stages it.

The actor belongs to a marker's first appearance, not to whoever runs the last build. For a marker the ledger does not know yet, `gloss build` takes, in order: the actor the last built `graph.jsonl` recorded for it; `GLOSS_ACTOR` (`agent:claude-code@claude-fable-5-1`, `human:alice`); the `git blame` author of the marker line when it is committed; else `human:<git user.name>`. `gloss hook install` writes `GLOSS_ACTOR=agent:claude-code` into the MCP server's env and into the SessionStart hook (`--actor` to change it), so links an agent writes are attributed to the agent and the agent-remit gate (`may-modify`) applies to them. The committed files (`.claude/settings.json`, `.mcp.json`) always call `gloss` by name, so a clone works on any machine with `gloss` on `PATH`. When `PATH` does not find the binary running the install, it also writes the hook with that binary's absolute path to `.claude/settings.local.json` (git-ignored, per machine) and says the MCP server needs `gloss` on `PATH`; `gloss doctor` warns when committed agent config names gloss by an absolute path. The hook also appends `export GLOSS_ACTOR=...` to `$CLAUDE_ENV_FILE`, which Claude Code sources before every shell command of the session, so a `gloss build` the agent runs later carries the actor too, not only the briefing. Set `GLOSS_ACTOR` yourself in any other agent harness.

**Cursor.** `gloss hook install --cursor` also writes `.cursor/mcp.json` (the same MCP server, as `agent:cursor` unless `--actor` is given) and `.cursor/rules/glossmark.mdc`, an always-applied rule. Cursor has no SessionStart hook, so the rule asks the agent to run `gloss context` itself, with `GLOSS_ACTOR` on every gloss command. Cursor reads the skills from `.claude/skills/`. The skills stop for the domain owner twice, before drafting and before writing a tier file, and ask in chat when the harness has no ask-the-user tool; `gloss doctor` accepts the Cursor setup in place of the Claude Code hook.

**Subagents need their own actor.** Subagents share the session's shell environment and its MCP server, so without more they all write as the session's actor, and the remit gate checks every link against one agent. Give each subagent its actor in its agent file (`.claude/agents/prep-cook.md`): "run gloss commands as `GLOSS_ACTOR=agent:prep-cook gloss build`". The name after `agent:` is the agent file's name, which is what ties the link to that agent's `may-modify` remit and lets `gloss context` show only its remit. A link a subagent writes through the shared MCP server is attributed to the server's actor; prefer writing `@gloss?` lines and running `gloss build` with the subagent's actor. `gloss build` warns when a rebuild would change the actor on an existing marker.

**In CI the actor comes from GitHub.** The ledger and graph a PR commits are the PR author's to edit, so `gloss diff --actor` takes the actor from outside them: the Action passes the PR author (`agent:<name>` for a bot account, `human:<login>` otherwise, or its `actor:` input). A link new in the PR is gated as that actor and as the one its provenance claims, and the stricter holds; a change that rewrites the author or reviewer the base ledger recorded for a marker is listed as `actor-changed` or `reviewer-changed`.

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

A proposed concept carries `proposed: true` and `proposed-by:` in the ontology file, the ontology's equivalent of `@gloss?`. It exists so links resolve, it shows in `gloss drift` and `gloss status`, and it is HARD until a human accepts it, so the proposal and its implementation ship in one PR. `search_concepts` matches aliases (`aliases: [borrower]` on `lib:Patron`), CamelCase words, stems and small typos, and follows `realizes`/`is-a` one hop, so an agent that asks for "pickup window" finds `hw:PickupClock`. `published: true` keeps only the published seam and `false` drops it; the viewer's Filters panel has the same Published row.

Over MCP the same capabilities are `context`, `impact_of` (with `paths` or `base`), `propose_concept`, `propose_link`, `search_concepts`, `concepts_for_path`, `lookup_concept`, `slice_of`, `history_of` and `drift`. `gloss slice <curie>` (MCP `slice_of`, `gloss context --concept`) shows everything relevant to one feature; see [ADVANCED.md](ADVANCED.md#feature-slices-gloss-slice). `history_of` tells an agent how a concept got to where it is before it changes it; see [ADVANCED.md](ADVANCED.md#history-gloss-history).

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
      - uses: mckalvan/glossmark@v0.3.0
        with: { fail-on: HARD, coverage-fail-under: "0" }
```

On the first PR (the base has no graph) the comment reports counts per boundary and per view plus the decision requests instead of one row per link; later PRs list changes grouped by boundary in collapsible sections, each capped. `coverage-fail-under: current` fails only when a PR lowers the base's concept coverage.

A repository with several workspaces runs the Action once per `root:` (a matrix); each root keeps its own comment, keyed by `<!-- glossmark-impact:<root> -->` and named in its heading, and a re-run updates it in place.

The Action downloads the release binary named by `version:` (default: the Action's own release) and falls back to `go install` only when no binary exists for it.

The Action's drift step fails on `violation,value-drift,dangling,overdue` by default. `drift-fail-on:` takes any `gloss drift --fail-on` list, and `check-graph: "true"` also fails when the committed `.glossmark/graph.jsonl` is not what `gloss build` makes from the tree (`gloss build --check`). A repository that wants nothing to merge unreviewed fails on stale links too, so a PR that edits a marked file re-confirms its links in the same PR:

```yaml
      - uses: mckalvan/glossmark@v0.3.0
        with:
          check-graph: "true"
          drift-fail-on: violation,value-drift,gap,dangling,overdue,planned-closed,plan-overdue,out-of-range,deprecated-use,rogue,orphan,untested,presented-only,stale,proposed
```

Before pushing, `gloss drift` lists what went stale; check that each link still holds and `gloss accept --stale <paths>` re-confirms them (`-n` shows which first). GlossMark's own CI runs this way ([.github/workflows/ci.yml](../.github/workflows/ci.yml)): it also lets a reviewer clear a HARD gate by labelling the PR `ontology-approved`, so the human decision the gate asks for is recorded on the PR.

Pre-commit: `ln -s ../../hooks/pre-commit .git/hooks/pre-commit`, or add the hooks from `.pre-commit-hooks.yaml` to your `.pre-commit-config.yaml` with `rev: v0.3.0`.

## Review workflow

`gloss review` (experimental, [ADR 0033](adr/0033-review-sessions.md)) puts everything that waits on a reviewer in one queue: proposed and stale links, proposed concepts, drifted values and drift findings. Each item carries its context (the marker's lines, what changed in the file since the last review, the target concepts' docs) and its actions with their gates. On a terminal it is interactive: the queue beside the selected item's context, history and concept docs. The list puts **Decisions** first (proposed links and concepts, stale links, drifted values, rogue `ns:NEW:` targets, recommendations) and **Findings** (orphan, untested, violation, gap, …, which are fixed in code or waived) after them; the header counts the opening queue's decisions that have been made ("2 of 4 decided"; undo puts one back). Each row reads `relation target  …/dir/file:line`, cut from the left so the file name stays, and its gate column shows one gate, or the range when the item's actions differ (`SOFT/HARD` for a rogue: propose is SOFT, waive HARD). The footer lists the keys the selected item allows. `a` accepts or re-confirms, `r` rejects (asks a reason), `w` waives a finding, `p` proposes a `ns:NEW:` concept, `e` edits a concept's doc, `y`/`d` take or dismiss an agent's recommendation, `n` writes a link, `o` opens `$EDITOR` at the line, `v` opens the HTML viewer in a browser on the item's concept (`viewer.html#slice=<curie>`; over SSH it prints the URL), `u` undoes, `f` and `/` filter, `tab` opens the read-only graph browser (enter follows an edge, `h` goes back), `?` lists the keys. Headless, for scripts and agents:

```sh
gloss review --format json                     # the queue (--kind stale-link --path internal/)
gloss review show m-1a2b3c4d5e --format text   # one item: context, history, concepts, gates
echo '{"id":"m-1a2b3c4d5e","action":"reconfirm"}' | gloss review apply
gloss review undo                              # reverts the last decision, files and ledger alike
gloss review node gmi:ReviewEngine             # read-only: a node and its edges in and out (a path works too)
```

An actor named `agent:…` takes AUTO and SOFT actions (re-confirm a stale link, accept or reject a SOFT proposal, write a `@gloss?` link, propose a concept for a `ns:NEW:` marker); on a HARD one it adds `"recommend": true` and the verdict waits in `.glossmark/recommendations.jsonl` for a person. `waive` adds a `policy.waivers` entry, `write-link` inserts a marker, `edit-doc` sets a concept's doc; an agent's doc edit becomes a `recommendation` item a person `apply`s or `dismiss`es. Over MCP the same engine serves `review_queue`, `review_item`, `review_decide`, `review_undo`, `write_link`, `edit_concept_doc` and the read-only `browse_node`, as the server's `GLOSS_ACTOR`.

- Humans review novelty and hierarchy; everything else flows through policy.
- **AUTO**: fingerprint-only changes, derived edges. Cleared by the engine.
- **SOFT**: a link to an existing concept, valid, inside `allowed`, at or above the auto-accept threshold derived from `suggest.auto-accept-precision`, altering only the engineering view. Cleared by the engine; audit a sample later.
- **HARD**: a new concept proposed, a link outside an agent's `may-modify` remit or outside `allowed`, any edit to `ontology/` or manifests, anything touching a published concept, MAJOR bumps, cross-repo imports. A human decides in the PR.

Suggested policy (not framework behaviour): review per PR, never per marker; auto-accept SOFT; audit a random share weekly.

**Choose the precision, not the threshold.** `suggest.auto-accept-precision` in `glossmark.yaml` (default 0.9) is how often an auto-accepted proposal must be right. The tool derives the conf threshold from it and prints it, with its measured precision and the share it auto-accepts, in `gloss suggest` and `gloss doctor`. With the shipped calibration the default derives `conf >= 0.80`. Once some of your code is reviewed, measure the repository itself: `gloss suggest --calibrate` strips your markers in a temporary copy, suggests on it and scores the result against your own links. With `suggest: { calibration: glossmark.calibration.tsv }` set, `gloss suggest --calibrate --write` saves the counts, and the threshold is derived from your repository from then on. When nothing reaches the target, nothing is auto-accepted and doctor says so ([ADR 0024](adr/0024-auto-accept-precision.md)). `gloss suggest --labels > labels.tsv` and `gloss suggest --eval labels.tsv` on a copy with the markers stripped still do the same by hand.
