<!-- @gloss documents arch:glossmark scope=file -->
# GlossMark: ontology and UX review of the tool and its first two adoptions

Reviewed on 2026-10-18 against GlossMark `main` (first at 6a9377f, then re-run at 681517a after `main` moved), `pfinance` branch `claude/glossmark-ontology` (e3d15fe) and `AiArena` branch `claude/glossmark-ontology` (2b6f07f). Everything below was reproduced by building `gloss` from the checkout and running it on all three trees; the viewer was rendered in headless Chromium. Section 5.2 and 3.11 were rewritten for 681517a (boundaries as containers, drill-in, filters, criticality governance).

## 1. Scale of the two adoptions

| | pfinance | AgentArena |
|---|---|---|
| Markers | 249 in 64 of 66 files | 970 in 391 of 414 files |
| Concepts | 55 core + 56 impl | 168 core + 110 impl |
| Manifests | 7 | 19 |
| PR size | 78 files, +3 941 lines | 418 files, +11 295 lines |
| Committed graph | 2 843 lines | 9 265 lines |
| `gloss check` / `build` | 0.08 s / instant | 0.46 s / 0.47 s |
| Viewer file | n/a | 768 KB |
| Drift today | 0 violations, 0 gaps, 6 untested, 1 proposal | 0 violations, 0 gaps, 11 orphans, 8 untested, 1 rogue |

### Regeneration check (added after review)

Both maps were rebuilt with today's `main` binary and diffed against the committed graphs.

| | Committed graph built by | Rebuilt with `main`: drift, coverage, findings | Lines changed on rebuild | What changed |
|---|---|---|---|---|
| pfinance (adopted 2026-10-18 05:30 UTC) | `gloss 0.1.0`, no `format` header (predates the versioning commit) | identical: 0/0/0, 6 untested, 1 proposal, 97 % | 296 of 2 843 | `format: 1` header; `prov.actor` on every inline edge |
| AgentArena (adopted 2026-10-18 05:45 and 08:40 UTC) | `gloss 0.0.0-dev`, `format: 1` (already current) | identical: 0/0/0, 11 orphans, 8 untested, 1 rogue, 94 % | 1 330 of 9 265 | `prov.actor` on every inline edge; two file-scope anchors on `views.yaml` |

GlossMark `main` itself moved during the review (6a9377f to 681517a: viewer redesign and criticality governance). Re-running the 681517a build on both adoption branches gives the same `check` and `drift` results again; `context` and `impact` gain a criticality line that reads "unclassified" everywhere because neither repo declares a level; the PR comment is unchanged. The viewers attached to the chat are from 681517a.

So the maps do not need regenerating: the markers, manifests and ontology files are hand-authored content and every finding in sections 3 to 6 reproduces unchanged under the current tool. What the two branches do not yet use, because the features landed hours after adoption, is `aliases:`, `proposed: true` concepts via `gloss propose`, `gloss hook install` (`.mcp.json` and the SessionStart hook are absent in both), `gloss impact`, and the `grammar:` directive. A follow-up pass that adds those would change the agent experience, not the analysis.

The rebuild also sharpened one finding (4.1 below): the committed graphs carry the correct `agent:claude-code` actor because the adopting sessions set `GLOSS_ACTOR` at build time, but my rebuild without it rewrote every edge's actor to the local git user, and both repos git-ignore the ledger, so attribution and review state are not durable.

Both adoptions were done by an agent in a single big-bang pull request. Both ended green and both authors found real facts about their codebase (untested invariants, an unmetered measure, a fallback importer with no test). That is the product working. The friction is in getting there and in what happens on the next PR.

## 2. What works well

- The grammar is tiny, the parser is strict, and error messages name the file, line, column, the code and the allowed values. `GM002`/`GM005`/`GM006`/`GM200` output is better than most linters.
- Build is fast enough to run on every save (sub-second on a 400-file repo).
- The drift vocabulary (violation, gap, orphan, rogue, untested) maps to decisions a human can make, and the findings carry a file and line.
- `impact` and `propose_link` give an agent a gate verdict before it writes anything. That is the right loop.
- `instructs` and `may-modify` on agent role files are a genuinely new idea: agent remits as first-class ontology edges.
- The incremental gate is correct: a second `implements` link to a concept the boundary already realizes is SOFT with no view change. The pain is concentrated in adoption, not steady state.

## 3. Ontology findings

### 3.1 Flow-style YAML silently truncates every doc with a comma (bug, high)

`doc: One account at an institution (checking, savings, ...)` inside `{ }` ends the scalar at the first comma. pfinance's `pf:Account` doc in the graph is "One account at an institution (checking", and `savings`, `credit card`, `brokerage`, `retirement`, `loan` and `...)` became null-valued keys on the concept. `gloss check` reports 0 warnings because a null relation produces no edge. Nine core concepts in pfinance are affected (Account, Institution, Category, CashFlowAnalysis, CarryingCost, Assumption, InstitutionExport, Holding, FirePlanning...). The truncated docs shipped into `AGENTS.md`, so agents read "A bank" as the definition of Institution. AgentArena avoided it only because its author quoted every doc.

Fix: warn on any concept key with a null value (GM-new), warn when a flow mapping's doc ends without terminal punctuation and the next key is not a known key, and document block style or quoted strings as the canonical form. `gloss propose` should write quoted docs.

### 3.2 `realizes` is overloaded

In pfinance, `pf:CashFlowAnalysis` is realized by `pfi:CashFlowEngine` (computes it), `pfi:CashFlowPage` (shows it), `pfb:pfinance-lib`, `pfb:analytics` and `pfb:dashboard` (contain it). In AgentArena every `.tsx` file `implements .WebApp` and `aa:WebApp realizes arena:AgentArena`. One relation is carrying "computes", "presents", "contains" and "is the shell of". The business view then cannot tell a page from an engine, and `untested` cannot tell that the engine is tested while the page is not.

Recommend: add `presents` (UI surface for a concept) and `hosts` (a shell or container concept) as inline relations with their own lift rules, and keep `realizes` for the mechanism that computes or enforces. Alternatively make the kind pair drive it: a `process` of kind `page` realizes by presentation.

### 3.3 `realizes` does not inherit, so manifests and ontology files restate each other

Seven importers each repeat `is-a: Importer, realizes: pf:Importing`. The `pfinance-lib` manifest lists 20 `realizes` entries that are exactly the union of what its sub-boundaries realize. AgentArena found that a container must list every component concept that is `part-of` it (their finding 3). This is the biggest source of manifest boilerplate.

Recommend: inherit `realizes` through `is-a` and roll it up through `part-of` for both concepts and boundaries. A manifest should be able to say `realizes: inherit` (compare against the union of children) and only list what it adds.

### 3.4 `allowed` lists do not scale

pfinance's analytics manifest has 36 `allowed` entries. AgentArena gave up and wrote `allowed: [arena:*, aa:*]` on every container, which turns the HARD gate for "outside allowed" off repo-wide. Either way the list is not doing its job.

Recommend a default: when `allowed` is absent, allowed = the `default-namespace` plus every concept the manifest declares (realizes, produces, consumes, publishes, subscribes) plus their `part-of` descendants and the core concepts they realize. Most manifests then need no `allowed` at all, and writing one becomes a deliberate narrowing.

### 3.5 Only `implements` lifts to `realizes`

A Terraform resource that `provisions pfi:AwsHost`, a package a TOML file `represents`, a table a migration `represents`: none of these satisfy a manifest's `realizes`. AgentArena's `exercises/`, `training/` and `deploy/` manifests declare nothing for that reason and rely on orphan detection only. Recommend: `provisions`, `deploys` and `represents` on a resource-kind concept lift to `realizes`; `represents` on an entity lifts to a new `holds` comparison so data boundaries can be checked too.

### 3.6 Two relations for one fact: `tests` and `verifies`

The docs say "use `verifies` in tests"; the `test-paths` inferrer turns `implements` in a test path into `tests`; AgentArena uses both (371 `verifies`, 9 `tests`). Pick one (`verifies`) and have the inferrer emit it, or define the difference (`tests` = unit of code, `verifies` = concept-level behaviour) and lint mixed use.

### 3.7 Published values are free text with no binding to code

`pfi:DefaultSafeWithdrawalRate` publishes `value: "4% (25x annual expenses)"`. The code has `swr: float = 0.04` as a default argument. Nothing links them, so the value can drift from the code and the HARD gate on published values protects prose, not behaviour. Recommend `value-from: { path, symbol }` or a marker form (`@gloss publishes-value .DefaultSafeWithdrawalRate scope=line`) so the fingerprint of the constant line participates in stale detection, and the viewer shows the live value.

### 3.8 Coverage rewards noise

File-level coverage with `--fail-under 80` produced: `# @gloss implements .AmountObfuscation` on five helpers in a row, `implements .WebApp` on `main.tsx`, `App.tsx`, `auth.tsx`, and double tagging (a `scope=file` marker plus a symbol marker for the same concept in `App.tsx`, `sessions.py`, `Appearance.tsx`). `gloss check` has no redundancy warning; I confirmed a file-level and symbol-level marker for the same concept passes silently and both become edges.

Recommend: (a) a redundancy lint (symbol marker shadowed by a file marker with the same relation and target); (b) a concept-side coverage metric ("every non-internal impl concept has at least one artifact and one verifier") as the headline number, with file coverage as a secondary; (c) guidance that one marker per public surface is the target, not one per function.

### 3.9 Three sources of truth in AgentArena

AgentArena already had `glossary.md` (836 lines, the definition of every term, with Code, UI and Status fields), `enums.yaml` (1 092 lines) and `check_ontology.py` (1 529 lines). `arena.gloss.yaml` now restates 168 of those terms by hand, `check_ontology.py` does not read the gloss files, and GlossMark's orphan/untested findings do not feed the glossary's Status column. The branch's own doc says "a new concept goes into arena.gloss.yaml in the same PR as the glossary entry", which is two edits per concept forever.

Recommend a generator or adapter: `gloss import --from markdown-glossary docs/ontology/glossary.md --namespace arena` that emits or refreshes the gloss file from headings, Def and Aliases, and a `gloss export --status` that writes implemented/untested/orphan per concept so `check_ontology.py` can compare it with Status. More generally, a plugin point for "my vocabulary already lives in X" is the fastest path into repos that already care about ontology, which are the early adopters.

### 3.11 Criticality governance (new in 681517a)

The model is sound: declare `criticality` once on the core concept, let it flow through `realizes` to implementation concepts, boundaries and artifacts, record provenance (`criticality-from`), warn when a manifest understates its level (GM600), and let policy raise the gate (`critical` is HARD by default). The payments example reads well in `context` and `impact` ("criticality critical (inherited from pay:Authorization)"). Four things to settle before the two adoptions pick it up:

- **Overloaded `realizes` spreads criticality too far.** A page that `realizes` a critical process inherits `critical`, so with the current modelling (3.2) `pfi:FirePage` would be as critical as `pfi:FireEngine`, and in AgentArena every `.tsx` that `implements .WebApp` would inherit whatever `arena:AgentArena` is given. A `presents` relation, or a rule that only `implements` carries criticality, is needed first.
- **Artifacts take the boundary's level.** GOVERNANCE.md says a critical platform may contain a low-criticality metrics module whose concepts keep their level, but an artifact's effective level is the highest of its boundary and its concepts, so every file in a critical boundary is critical for review purposes anyway. Either artifacts should take the concept level only, or the doc should say that the boundary level is a floor.
- **Nothing bootstraps classification.** Both adoptions show "unclassified 169" and "unclassified 51" in the filter. AgentArena already names its 19 invariants as `constraint` concepts and lists them in an `invariants` view; a `gloss criticality suggest` (constraints and published values default to `high`, concepts with `verifies` from security-named tests to `high`, everything else unclassified) would give a first cut to review.
- **Boundary level sorting without data.** The top-level list sorts "highest criticality first", which is a no-op until something is classified; the empty state should say so.

### 3.10 Smaller modelling points

- A third prefix (`pfb:`, `arch:`) for boundaries looks like a namespace but is not one; it has no file, no version and no tier. Call it what it is in the docs ("boundary prefix") and consider deriving boundary IRIs from the repo IRI so most repos need no `prefixes:` at all.
- Marker placement between a decorator and the `def` (the workaround for the multi-line decorator bug) fights Python style and will be moved by formatters and docstring tools. The anchoring bug should be fixed rather than documented.
- `search_concepts("foal birth")` returns `zoo:Foal`, `zoo:Foaling`, `zoo:FoalBorn` and four more all at score 50, `matched: name`, with no doc text in the hits. The agent cannot pick. Ranking should prefer whole-word and kind matches, include the doc, and treat `birth`/`born` stems.
- `gloss propose` reflows the entire ontology file (alignment removed, CURIEs quoted), so one proposed concept is a 27-line HARD diff that hides the addition. The plan calls `ontoedit` formatting-preserving; it is not. A proposed concept also takes its `doc` from the marker's `note=`, which is a rationale, not a definition.

## 4. Agent-experience findings

### 4.1 Attribution and review state are not durable

The committed graphs record the right actor (`agent:claude-code@claude-fable-5-1` in pfinance, `agent:claude-code` in AgentArena) because the adopting sessions exported `GLOSS_ACTOR` before `gloss build`. Nothing persists that choice: no Makefile target, CI job, hook or `.mcp.json` in either repo sets it, so the next `gloss build` by anyone else rewrites `prov.actor` on every inline edge to `human:<git user.name>` (1 330 changed lines in AgentArena, 296 in pfinance) and `gloss status` then reports every proposal "by human:claude". Both repos git-ignore `ledger.jsonl`, which is the only store of accepted, rejected and stale state, so review decisions evaporate on every clone and on every CI run. The agent-remit gate (`outside may-modify remit`) only fires for `agent:` actors, so with a rebuilt graph it is inert.

Fix: the actor belongs to the marker's first appearance, not to whoever ran the last build. Take it from the ledger (or from `git blame` of the marker line) when `GLOSS_ACTOR` is unset, rather than overwriting it; have `gloss hook install` write `GLOSS_ACTOR=agent:claude-code` into the hook and MCP env; have `gloss init` emit a Makefile and CI job that set it; and either commit the ledger by default or move review state into the graph so a clone is complete. `gloss check` should warn when a rebuild would change the actor on existing edges.

### 4.2 The SessionStart briefing is the wrong shape

The installed hook runs `gloss context --dir "$PWD"`. Claude Code starts in the repo root, so the briefing is always the root boundary: 8.2 KB listing every agent's remit and the first 60 concepts in alphabetical order (`arena:Admin`, `arena:Agent`, `arena:AgentArena`...). The first 60 are not chosen by relevance to the directory. The `allowed: [arena:*, aa:*]` line adds nothing.

Recommend: rank concepts by relevance (already linked under the directory, then declared by its manifest, then its namespace, then the rest), show remits only for the current role when `GLOSS_ACTOR` names one, add `--budget <tokens>`, and have the hook pass the working tree's changed files to `impact` so the briefing opens with "you are touching X, which is on the published seam".

### 4.3 Two proposal paths with conflicting advice

`gloss context` says "use `gloss propose concept`". `AGENTS.md` says "write `@gloss? ... ns:NEW:Name`". AgentArena's `CLAUDE.md` says "never add it to the ontology files to make your own proposal resolve". An agent reading all three will not know which is sanctioned. Pick one loop (marker with `NEW:` first, `propose --from-new` as the steward's tool, or the reverse), state it once, and make `context`, `agents` and the MCP tool descriptions say the same sentence.

### 4.4 `gloss check` is silent about proposals and rogues

AgentArena's finding 6 still holds: `check` passes `@gloss?` and `NEW:` targets without a count. A reviewer running the pre-commit hook never sees them. Add a summary line ("3 proposals, 1 rogue concept; run `gloss status`").

### 4.5 Anchoring above a multi-line decorator is still broken

A marker above `@decorator(\n  arg=1)\ndef shoe()` does not anchor to `shoe`; the symbol is missing from the graph and the marker degrades to a block anchor whose id can collide with the next one. Confirmed on `main`.

## 5. Human-UX findings

### 5.1 The adoption PR comment

`gloss diff --base main` on the pfinance branch classifies all 300-odd changes HARD, because the base has no graph, so every link "affects the business view". The rendered reason reads "existing concept, valid, inside allowed; business view affected: business", which says SOFT and HARD in one breath. Adding the repo's own namespaces is labelled "cross-repo import". The markdown comment is a single code block of 120-column lines; at AgentArena's size it would exceed GitHub's comment limit.

Recommend: an adoption or baseline mode (`gloss diff --baseline` or "base has no graph") that reports summary counts per boundary and view instead of per link; group the normal comment by boundary with collapsible `<details>`; cap per-section rows with a "and N more"; and fix the two reason strings.

### 5.2 The viewer (re-reviewed at 681517a)

The 9c32be6 redesign fixes the worst of what the first build showed. The page now opens at the top of the system with every boundary closed, a boundary is a box with a "+N inside" badge, the right pane shows a selected boundary's level, namespace, criticality, owner, doc and member list, edges carry relation names and multiplicities, hover tooltips list the folded sub-boundaries, and the filter chips (type, tier, namespace, kind, level, criticality, relation) carry counts. Those were the first three asks of the earlier review and they are done.

What the regenerated pages show next:

- **Containment collapses to the root.** Opening `pfinance` draws 34 concepts directly inside the root box with the six sub-boundaries (lib, analytics, importers, dashboard, hosting, tests) only listed as "folded" in a tooltip; opening `AgentArena` draws 137 of 168 concepts in the root box. The projection places a concept in the deepest boundary whose code realizes it, or in the common enclosing boundary when several do. Because `realizes` is overloaded (3.2: engine, page and library all realize `pf:CashFlowAnalysis`), almost every concept has several realizers and resolves to the root. The drill-in design therefore depends on fixing 3.2 and 3.3, or on a tie-break (prefer the boundary with an `implements` over one with only `presents`, or the one whose manifest declares it).
- **Curated views open closed.** `invariants` (AgentArena) renders as a single closed `AgentArena` box with "966 inside" and 0 edges; `pipeline` (pfinance) as one box plus `pf:FinancialRecord`. A view whose `include:` names concepts explicitly should open expanded, otherwise its purpose is behind a click and the count mixes in artifacts the view did not ask for.
- **Closed-box tooltips leak.** Hovering the closed box in `invariants` shows `arena:Lesson has-part arena:Quiz`, an edge outside the view's include list.
- **Concept selection and search** could not be driven from the automated run (clicking a concept node left the pane on "where you are", and Enter in the search box changed nothing visible); the boundary pane works. Worth a manual check that a concept click opens the same pane with artifacts and `path:line`.
- The page is now 833 KB for AgentArena; the earlier suggestion to split HTML and data stands.

Still recommended from the first pass: a findings (drift) panel in the page, source links (`vscode://` or a URL template), and `include:` pulling in the artifacts of listed concepts at zoom `code`.

### 5.3 Vocabulary load

A newcomer meets 8 concept kinds, 14 inline relations (27 total), 5 scopes, 8 finding kinds, 3 gates, 4 review states, 3 file kinds and 4 version numbers on page one. Both adopters used five relations for 95 percent of markers (`implements`, `verifies`, `represents`, `documents`, `instructs`). Present a "starter five" and move the rest to an advanced page. Two counters confuse: `markers 249: accepted 293` (targets are counted, markers are not) and `Proposed: 3` which mixes links and concepts.

## 6. Reducing friction to adopt

Both adoptions were performed by an agent reading `QUICKSTART.md` and inventing the ontology from the code in one pass. That is the real onboarding path, so make it the product.

1. **Ship releases.** There are no tags, so `go install ...@latest` resolves to `0.0.0-dev`, the Action compiles from source on every run (about two runner-minutes), both repos pin `@main`, and the docs promise binaries that do not exist. Tag `v0.1.0`, publish binaries, and provide an install script and a Homebrew tap or `npx`-style wrapper for repos without a Go toolchain.
2. **`gloss init`.** Scaffold `glossmark.yaml`, two empty tier files with the litmus-test comment, a root manifest, manifests for the top-level source directories (level guessed from depth), `views.yaml`, the `.gitignore` lines, a `make gloss` target, the CI job and `gloss hook install`. Today each adopter rebuilt all of this by hand from the horse-factory example.
3. **`gloss adopt` or an adoption skill.** Package the agent workflow both repos followed as a Claude Code skill shipped in this repository: inventory the tree, draft core concepts from README and docs, draft impl concepts from module and class names, write `@gloss?` markers with `conf`, run `check` and `drift`, and open one PR. Add `gloss suggest <path>` that proposes markers by matching symbol names and docstrings against concept names and aliases, so the first pass is reviewable instead of hand-written.
4. **Import existing vocabularies** (3.9): markdown glossary, `enums.yaml`, OpenAPI schemas, SQL DDL. A repo that already has a vocabulary should not type it again.
5. **Shrink manifests by default** (3.3, 3.4): inherited `realizes`, derived `allowed`, and a `derive-only` manifest that declares nothing and still participates in orphan, untested and coverage reporting.
6. **Adoption-mode diff** (5.1) so the first PR is readable, plus a coverage ratchet (`--fail-under current`) and a "new and changed files only" coverage gate so a repo can adopt gradually instead of in one 11 000-line PR.
7. **Make the agent loop one sentence** (4.3) and set the actor automatically (4.1).
8. **Lint the foot-guns** (3.1, 3.8, 4.4): null concept keys, truncated docs, redundant markers, proposal counts in `check`.
9. **`gloss doctor`**: is `gloss` on PATH for the hook, is `GLOSS_ACTOR` set, is the committed graph current, is the lockfile present, does the Action pin a version.

## 7. Suggested order

| Priority | Item | Why first | Effort |
|---|---|---|---|
| 1 | Null-key / truncated-doc lint and quoted-doc guidance (3.1) | Silent data loss in a shipped adoption | S |
| 2 | Tag a release, binaries, install script (6.1) | Every quickstart and CI step depends on it | S |
| 3 | Agent actor by default in hook and MCP (4.1) | Remit gate and provenance are inert without it | S |
| 4 | Adoption-mode diff and grouped PR comment (5.1) | First impression on the first PR | M |
| 5 | Inherited `realizes`, derived `allowed` (3.3, 3.4) | Removes most manifest boilerplate | M |
| 6 | Relevance-ranked, budgeted briefing (4.2) | The hook is the agent's first contact | M |
| 7 | Viewer: containment tie-break, curated views open expanded, concept pane check (5.2) | Boxes and drill-in landed in 681517a; they only pay off once concepts stop resolving to the root | M |
| 8 | `gloss init` and the adoption skill (6.2, 6.3) | Turns the two manual adoptions into a path | M |
| 9 | Redundancy lint and concept-side coverage (3.8) | Stops coverage from rewarding noise | S |
| 10 | `presents`/`hosts` relations, lift for `provisions`/`represents` (3.2, 3.5) | Modelling fidelity; needs an ADR | L |
| 11 | Glossary and enums importer, status export (3.9) | Unlocks ontology-first repos like AgentArena | M |
| 12 | Format-preserving `propose`, decorator anchoring (3.10, 4.5) | Known bugs; fix rather than document | S |
| 13 | Criticality: `implements`-only inheritance, artifact level rule, `criticality suggest` (3.11) | Needed before either adoption classifies anything | M |

## Appendix: reproduction notes

- Build: `go build -o gloss ./cmd/gloss` on `main`; `gloss version` prints `0.0.0-dev`.
- Truncation: `python3 -c "import yaml; print(yaml.safe_load(open('ontology/pf.gloss.yaml'))['concepts']['Account'])"` on the pfinance branch.
- Attribution: `gloss status` on either adoption branch.
- Gate reasons: `gloss diff --base main --format markdown` on the pfinance branch.
- Incremental SOFT: add `# @gloss implements impl:FoalingPipeline` above a new function in `examples/horse-factory/services/horse-factory/breeding/engine.py` with the graph committed, then `GLOSS_ACTOR=agent:x gloss diff --base HEAD`.
- Decorator anchoring: marker above `@decorator(\n  arg=1)\ndef shoe(): ...`; `shoe` is absent from `graph.jsonl`.
- Propose reflow: `gloss propose --from-new` on a copy of horse-factory, then `git diff ontology/impl.gloss.yaml`.
- Viewer: `gloss view` on the AgentArena branch, opened in headless Chromium at 1440 and 390 px; `invariants` view reports 20 nodes, 57 edges, no artifact nodes.
