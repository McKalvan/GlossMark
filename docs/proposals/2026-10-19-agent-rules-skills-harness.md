<!-- @gloss documents gm:Onboarding, gm:Vocabulary scope=file -->
# Proposal: agent rules, skills and harness docs

Status: implemented on branch `claude/agent-harness-proposal`, 2026-10-19, against `main` at 5d20c47. Open questions decided the same day (section 6); what was built and where it differs is in section 7.

GlossMark's real users are coding agents: both adoptions were done by an agent reading `QUICKSTART.md`, and the design assumes agents write the links while humans review novelty. This proposal sets out what an agent should read, run and be stopped by, for two audiences:

- **Adopters**: developers, and their agents, working in a repository that uses GlossMark.
- **Contributors**: developers, and their agents, working on GlossMark itself.

Every draft below is meant to be copied into place as written. Items marked *needs code* depend on a CLI change that this proposal describes but does not build.

## 1. Where things stand

Everything below was reproduced by building `gloss` from 5d20c47 and running it on this repository.

### What an agent gets today

| Surface | What it does | Gap |
| --- | --- | --- |
| SessionStart hook (`gloss hook install`) | Relevance-ranked briefing, 2 000-token budget, opens with what uncommitted changes touch | Claude Code only. Ephemeral: not visible to harnesses without hooks, and cut first under budget. At the root of this repo, the allowed-targets line lists 41 CURIEs, so `--budget 600` leaves room for 6 concepts |
| MCP server (`gloss mcp`) | 8 tools: `context`, `lookup_concept`, `search_concepts`, `concepts_for_path`, `propose_link`, `propose_concept`, `impact_of`, `drift` | Good and portable. Nothing tells an agent *when* to call which tool, beyond the server's one-line `instructions` |
| `gloss agents --write AGENTS.md` | A vocabulary snippet between `gloss:begin`/`gloss:end`: grammar, proposal sentence, every concept in a table | Mostly vocabulary, two lines of behaviour. 9 KB on this repo and grows with the ontology. `gloss init` does not write it |
| Skill `adopt-glossmark` | Day-0 adoption, step by step | The only skill. Opt-in via `--skill`. Nothing for day 2: marking while coding, fixing drift, triaging proposals, upgrading imports |
| Agent files with `may-modify` | A role's remit; the briefing shows only that remit when `GLOSS_ACTOR=agent:<role>` | Only in `examples/`. No template, no doc. The hook sets one actor per session, so a Claude Code subagent never gets its own remit |
| pre-commit hook, GitHub Action | `check`, `build`, `drift`, PR comment | The only guards. An agent hears about a GM error, a HARD link or a remit breach at commit or CI time, never at edit time |
| Diagnostics | 41 GM codes emitted by `internal/check` and `internal/extract` | About half have no row in any doc (GM200–GM230, GM300–GM303, GM340, GM350, GM500, GM501). An agent that hits one cannot look up the fix |

### Single-sourcing, partly done

`agents.ProposalSentence` is the right pattern: one Go constant, used verbatim by the briefing, the snippet and both MCP proposal tools, and pinned by `TestBriefingV2`. Two copies escape it: `skills/adopt-glossmark/SKILL.md` and `docs/QUICKSTART.md` paste the sentence by hand, and no test reads either file. The same is true of everything else an agent is told: the starter five, "one marker per public surface", "commit the ledger" are restated in five places with no test.

### GlossMark does not dogfood its own agent setup

- No `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md` or `CHANGELOG.md`. `VERSIONING.md` promises that deprecations are "announced in the changelog".
- `gloss doctor` on this repo warns twice: no SessionStart hook or MCP server, and no CI job that pins GlossMark (CI builds from source, which is right for this repo but should be said).
- `gloss status` reports 43 stale links and 6 proposed concepts (`gm:Criticality`, `gmi:CriticalityStamper`, `gmi:OntologyEditor`, `gmi:ImportLock`, `gmi:Doctor`, `gmi:FormatVersions`) waiting on a human.
- `gmi:AdoptionSkill`, `gmi:Publisher` and `gmi:ReviewCommands` are implemented and untested.
- The contributor conventions are real but implicit: the frozen grammar and corpus-first changes (PLAN §5.2, §5.10), format headers (VERSIONING), ADRs for modelling changes (ADR 0001), one end-to-end test per issue against an adopter fixture (PLAN §10), the self-host test, and the commit style `Verb phrase (#issue)`. A new contributor's agent learns them by breaking `TestSelfHost`.

## 2. Design: seven layers, each with one job

An agent harness has seven places to put guidance. Put each fact in exactly one of them.

| Layer | Carries | Loaded | Size | Adopter file | Contributor file |
| --- | --- | --- | --- | --- | --- |
| **Rules** | Behaviour that never changes per directory: how to mark, what never to do, what done means | Always, every harness | ≤ 600 tokens | `AGENTS.md` rules block, generated | `AGENTS.md` at this repo's root |
| **Briefing** | Where you are: boundary, allowed targets, remit, relevant concepts, state | Session start, per directory | ≤ 2 000 tokens | SessionStart hook (exists) | same, dogfooded |
| **Tools** | Lookups and proposals on demand | When called | n/a | MCP server (exists) | same |
| **Guards** | Feedback between an edit and the commit | On edit and on stop | one line each | `gloss hook guard` *needs code* | Stop hook running fast checks |
| **Workflows** | Multi-step procedures for one kind of task | When the task matches | loaded on demand | Shipped skills, `gloss-*` | Repo skills, `gm-*` |
| **Remits** | What one role may change | Per role | short | `.claude/agents/<role>.md` with `may-modify` | not needed yet |
| **Backstop** | What a human must see | On PR | n/a | Action + PR comment (exists) | CI (exists) |

Three principles follow from the table.

1. **Rules are static and short; vocabulary is dynamic.** The concept table moves out of `AGENTS.md` and into the briefing and `search_concepts`, which already rank by relevance. `AGENTS.md` keeps only behaviour. A rules file that grows with the ontology is read less carefully every week.
2. **Every sentence an agent reads is a Go constant with a test.** Extend the `ProposalSentence` pattern to a small `agents.Rules` set. The rules block, the briefing, the MCP `instructions`, the shipped skills and `QUICKSTART.md` all render from it or are tested against it.
3. **Harness-neutral first, Claude Code second.** `AGENTS.md` and MCP work in every harness. Hooks, skills and subagent files are richer but Claude Code specific, so they add to the neutral layer and never replace it.

## 3. Track A: for adopters

### A1. Rules: an `AGENTS.md` rules block

*Needs code (S).* `gloss agents --rules` renders this block from `agents.Rules`. `gloss init` writes it into `AGENTS.md`, creating the file if needed, between its own markers so the repository's other text is untouched. `gloss agents` without `--rules` keeps printing the vocabulary for anyone who wants it. `gloss doctor` gains a check that the block is present and current. Repositories that keep a `CLAUDE.md` add one line, `@AGENTS.md`, and `gloss init` writes that line when a `CLAUDE.md` exists. Recent Claude Code builds read `AGENTS.md` natively, but by default only when there is no `CLAUDE.md`, so the import is the reliable path.

The draft, as `gloss agents --rules` would print it. The ten rules are the proposal; the wording of rule 5 is `ProposalSentence` verbatim.

````markdown
<!-- gloss:rules:begin (generated by gloss agents --rules; do not edit by hand) -->
## GlossMark

This repository links code to its domain ontology with one-line `@gloss` comments, and `gloss` checks them. These rules apply whenever you add, change, move or delete code, tests, docs, infrastructure, config, CI or agent files.

1. **Get the briefing first.** Before you write in a directory, read its briefing: MCP `context`, or `gloss context --dir <dir>`. It names the boundary, the concepts you may link to and your remit.
2. **Mark public surfaces, not helpers.** One marker per handler, model, table, test file or doc page, directly above the declaration and its decorators. Use the starter five: `implements` for code that does the work, `represents` for data shapes, `verifies` in tests, `documents` in docs, `instructs` in agent files. Add `scope=file` when the whole file is about one concept.
3. **Propose, never accept.** Write every link you add as `@gloss? <relation> <target> conf=<0-1>`. Never write a bare `@gloss`, never delete a `?`, and never run `gloss accept` or `gloss reject` unless a human asked you to in this session.
4. **Find targets, do not guess them.** Use `search_concepts` to find a concept and `propose_link` to get the exact line and its gate.
5. If no concept fits, write `@gloss? <relation> <ns>:NEW:<Name> note="why"`, then run `gloss propose --from-new --doc "what it is"` (MCP: propose_concept) so the concept ships in the same PR with `proposed: true`; a human accepts it with `gloss accept --concept <ns>:<Name>`. Never invent a concept name any other way, and never hand-edit an ontology file to make a link resolve.
6. **Markers move with their code.** Moving or renaming a marked declaration moves its marker. Deleting it deletes its marker. A marker must never end up above unrelated code.
7. **Governance files are HARD.** Edit `ontology/*.gloss.yaml` only through `gloss propose`. Edit a `.gloss.yaml` manifest only when that edit is the fix, and say so in your summary. Do not edit `glossmark.yaml` unless asked.
8. **Say what you touched.** If the briefing or `impact_of` reports a `high` or `critical` file, or a concept on the published seam, say so before you change it and keep the change minimal.
9. **Leave it clean.** Before you finish: `gloss check` reports 0 errors, `gloss build` has been re-run, and `.glossmark/graph.jsonl` and `.glossmark/ledger.jsonl` are part of your change.
10. **Report the impact.** End your summary with what `gloss impact --base <base branch>` reports: HARD items, criticality touched, published concepts touched, and every proposal you added.

Workflows for longer tasks: skills `gloss-link`, `gloss-resolve-drift`, `gloss-review`, `adopt-glossmark`. Rule details: https://github.com/mckalvan/glossmark/blob/v0.1.0/docs/AGENT-HARNESS.md
<!-- gloss:rules:end -->
````

Notes on the choices:

- Rule 3 makes "agents propose, humans accept" a rule, not a convention. Today nothing stops an agent from running `gloss accept --policy` on its own proposals, and the ledger would record it as an agent acceptance.
- Rule 7 is narrower than "never touch manifests", because fixing a violation sometimes *is* a manifest edit. The gate already makes every such edit HARD, so the rule only asks the agent to say so.
- Rules 9 and 10 turn the pre-commit hook and the PR comment into something the agent does before a human sees it.

### A2. Skills: one per recurring task

Ship these in `skills/` and embed them in the binary as `adopt-glossmark` is today. `gloss hook install --skill` installs all of them. `--skill <name>` installs one. Each skill opens with the rules it relies on by reference, not by copy, so the rules block stays the single source.

| Skill | When it triggers | Status |
| --- | --- | --- |
| `adopt-glossmark` | "add GlossMark", "set up gloss" | Exists. Changes: write the rules block in step 2; add an incremental path (one boundary per PR, `coverage --fail-under current --changed-only`); test its proposal sentence |
| `gloss-link` | Any coding task in a repository with `glossmark.yaml` | New, drafted below. The day-2 loop |
| `gloss-resolve-drift` | `gloss drift` or CI reports a finding | New, drafted below |
| `gloss-review` | A human asks for help reviewing proposals | New, drafted below |
| `gloss-criticality` | "classify criticality", or a repo with nothing classified | New, outlined |
| `gloss-upgrade-import` | Bumping an imported namespace, or GM105–GM109 | New, outlined |

#### Draft: `skills/gloss-link/SKILL.md`

````markdown
---
name: gloss-link
description: Keep @gloss links correct while you change code in a repository that has glossmark.yaml - find the right concept, write the proposal, move or delete markers with their code, add verifies on tests, and report the impact. Use for any feature, fix or refactor in a GlossMark repository, and whenever gloss check or the briefing mentions a marker.
---

<!-- @gloss instructs gm:Vocabulary scope=file -->

# Link as you code

The GlossMark rules in AGENTS.md apply throughout. This skill is the order to apply them in.

## Before you edit

1. Call MCP `context` with `dir` set to the directory you will work in, and `changed: true`. Note the boundary, its criticality, your remit and the allowed targets.
2. For each file you will change, call `concepts_for_path`. If a file is `high` or `critical`, or on the published seam, say so to the user now.

## While you edit

3. New public surface (handler, model, table, test, doc page): find its concept with `search_concepts`. Prefer, in order: an implementation concept in the boundary's default namespace; a core concept when the artifact *is* the domain thing (`represents pay:Payment`); `ns:NEW:Name` as the last resort.
4. Call `propose_link` for the exact line and gate. Write it above the declaration and its decorators. If the gate is HARD because the target is outside the allowed set, look for a better target before accepting that.
5. Changed surface: if what it does changed, re-run step 3 and retarget the marker. If not, leave the marker alone.
6. Moved or renamed: move the marker with the declaration. Deleted: delete the marker.
7. Tests: when a test you write or change checks a concept's behaviour, add `@gloss? verifies <concept>` on it. Never add `verifies` to a test that does not check that concept.
8. No concept fits: follow rule 5 of the rules block exactly, then run `gloss propose --from-new --doc "<one sentence>"`.

## Before you finish

9. Run `gloss check`. Fix every error. Read every warning: GM130 and GM131 mean you marked too much; GM502 and GM503 mean a target is outside the boundary.
10. Run `gloss build -q` and include `.glossmark/graph.jsonl` and `.glossmark/ledger.jsonl` in the change.
11. Run `gloss drift`. A violation or gap your change introduced is yours: use the `gloss-resolve-drift` skill.
12. Run `gloss impact --base <base branch>` and end your summary with its HARD items, the criticality it reports, published concepts touched, and the proposals you added.
````

#### Draft: `skills/gloss-resolve-drift/SKILL.md`

````markdown
---
name: gloss-resolve-drift
description: Resolve a gloss drift finding - violation, gap, value-drift, dangling, rogue, stale, untested, orphan or presented-only - by fixing the code or the marker, or by preparing the governance change for a human. Use when gloss drift, gloss check or the GlossMark CI job fails or reports findings.
---

<!-- @gloss instructs gm:DriftComparison scope=file -->

# Resolve drift

Run `gloss drift` and handle each finding by kind. The GlossMark rules in AGENTS.md apply: you propose, a human accepts, and ontology files change only through `gloss propose`.

| Finding | What it means | Fix it yourself | Leave for a human |
| --- | --- | --- | --- |
| **violation** | Code under a boundary realizes, holds, produces or publishes something its manifest does not declare | The marker is wrong (retarget it) or the code is in the wrong boundary (say so) | The boundary really does this now: add it to the manifest's list and call out the HARD edit |
| **gap** | The manifest declares something nothing under it implements | The implementing code exists but is unmarked: add `@gloss? implements` | The declaration is aspirational: propose removing it, HARD |
| **value-drift** | Code bound by `value-from` or `publishes-value` changed, the ontology `value:` did not | The code change was a mistake: revert it | The change is intended: update `value:` in the same PR and say it is a published value |
| **dangling** | A marker names a concept that does not exist | A typo: find the real one with `search_concepts` | Nothing fits: rewrite as `ns:NEW:Name` and follow the proposal rule |
| **rogue** | A `ns:NEW:Name` marker | Run `gloss propose --from-new --doc "..."` | The decision to accept the concept |
| **stale** | The declaration under an accepted marker changed | The link no longer holds: retarget or remove the marker | The link still holds: list it for re-acceptance (`gloss accept <id>`) |
| **untested** | Implemented, nothing verifies it | A test already checks it: add `@gloss? verifies`. None does: write one if the task allows | Whether a test is worth writing |
| **presented-only** | A page presents it, nothing implements it | The mechanism exists unmarked: mark it | The mechanism is missing |
| **orphan** | No artifact links to the concept | Nothing, unless your task implements it | Whether the concept should exist |

Never make a finding disappear by deleting a correct marker, widening `allowed:`, or marking unrelated code. When you are done, re-run `gloss drift` and list, per finding, what you changed and what you left for a human and why.
````

#### Draft: `skills/gloss-review/SKILL.md`

````markdown
---
name: gloss-review
description: Help a human review GlossMark proposals - proposed links, proposed concepts and stale links - by preparing one decision table and then running exactly the accepts and rejects the human chooses. Use only when a human asks to review, triage or accept GlossMark proposals.
---

<!-- @gloss instructs gm:Review scope=file -->

# Review proposals with a human

Accepting is a human decision. You prepare it and carry it out; you never make it.

1. Run `gloss status` and `gloss diff --base <base branch>`.
2. For each proposed link, open its `path:line` and judge it against MARKING.md: right relation, right target, public surface rather than helper. For each proposed concept, apply the litmus test (would a domain expert nod: core; only an engineer: implementation), check its `realizes`, and check it duplicates nothing (`search_concepts`). For each stale link, say whether the declaration change breaks the link.
3. Show one table: id, `path:line`, link, gate, criticality, your recommendation (accept, reject with reason, retarget to X), one-line reason. Put HARD and `critical` rows first.
4. Wait for the human's decisions. Do not act on rows they did not decide.
5. Carry out only what they decided, recording them as the reviewer: `GLOSS_ACTOR=human:<their name> gloss accept <id>`, `gloss accept --concept <curie>`, `gloss reject <id> --reason "..."`. Use `gloss accept --policy` only when they say to accept every SOFT proposal.
6. Run `gloss build -q` and `gloss status`, and report what is left.
````

#### Outlines

- **`gloss-criticality`**: run `gloss criticality suggest`; explain each suggestion's source (constraint, published, security-verified); ask which to raise to `critical` and which to drop; apply with `--write` only after the human answers; run `gloss check` for GM600; summarise the gate changes the `criticality-gates` policy will cause.
- **`gloss-upgrade-import`**: read the upstream's version and changelog; widen the range under `imports:`; `gloss lock`; `gloss check` (GM105–GM109); `gloss drift` and, with sibling checkouts, `gloss landscape`; list every MAJOR change the upgrade pulls in.

### A3. Guards: feedback between the edit and the commit

*Needs code (M).* One new subcommand, `gloss hook guard`, reads the harness's hook payload on stdin and prints at most one line. It reuses `impact` and `check`, which run in about 0.3 s on this repository. `gloss hook install --guards` registers it next to the SessionStart hook.

| Event | What `gloss hook guard` does | Blocks? |
| --- | --- | --- |
| Before an edit or write | For the target file: criticality and where it comes from, published-seam concepts, and whether it is outside the current actor's `may-modify` remit. Returned as `additionalContext`, so the agent knows before it writes | No by default. With `policy.guard-ask` set, returns `permissionDecision: "ask"` for out-of-remit files or listed criticality levels |
| After an edit or write | Markers that `check` now flags in that file, such as GM130, GM131 or GM502, as `additionalContext` | No |
| Stop | `gloss check`; the count of proposals added in the working tree; whether `graph.jsonl` is current | Yes by default, on `check` errors or a stale graph, with the reason. `policy.guard-stop: report` turns blocking off. Never when `stop_hook_active` is already true, and never on proposals |

Field names follow Claude Code's hook reference as of this writing: `hookSpecificOutput.additionalContext` on PreToolUse and PostToolUse, `permissionDecision` on PreToolUse, and `decision: "block"` with `reason` on Stop. The hook payload carries `tool_input.file_path` for Edit and Write, and `agent_type` when the hook fires inside a subagent. The matcher is `Edit|Write`. Add MultiEdit only after a logging hook confirms its name and input shape.

````json
{
  "hooks": {
    "SessionStart": [
      { "hooks": [ { "type": "command", "command": "GLOSS_ACTOR=agent:claude-code gloss context --dir \"$PWD\" --changed --budget 2000 2>/dev/null || true" } ] }
    ],
    "PreToolUse": [
      { "matcher": "Edit|Write",
        "hooks": [ { "type": "command", "command": "GLOSS_ACTOR=agent:claude-code gloss hook guard --event pre-edit 2>/dev/null || true" } ] }
    ],
    "PostToolUse": [
      { "matcher": "Edit|Write",
        "hooks": [ { "type": "command", "command": "GLOSS_ACTOR=agent:claude-code gloss hook guard --event post-edit 2>/dev/null || true" } ] }
    ],
    "Stop": [
      { "hooks": [ { "type": "command", "command": "GLOSS_ACTOR=agent:claude-code gloss hook guard --event stop" } ] }
    ]
  }
}
````

Two design rules for the guard:

- **It fails open.** If `gloss` is missing or the graph cannot be built, it prints nothing and exits 0, like the SessionStart hook. `gloss doctor` is where missing setup gets reported.
- **It never blocks on proposals.** Proposals are the point. It blocks a stop only for what CI would reject anyway.

Harnesses without hooks get the same checks from rules 9 and 10 and from the pre-commit hook.

### A4. Remits: a role-agent template

Document the pattern `examples/` already uses, and ship a template that `gloss init` can write on request.

````markdown
---
name: refund-engineer
description: Implements and fixes refund flows in services/api. Use for refund handlers, refund models and their tests.
---

<!-- @gloss instructs pay:Refunding scope=file -->
<!-- @gloss may-modify pay:Refund, pay:Refunding, pa:* scope=file -->

# Refund engineer

You change refund behaviour in `services/api`. The GlossMark rules in AGENTS.md apply.

- Your remit is the `may-modify` line above. A link you write outside it is HARD-gated; if your task needs one, say why in your summary.
- Run with `GLOSS_ACTOR=agent:refund-engineer` so the briefing shows only your remit and the ledger attributes your links to you.
````

*Needs code (S); behaviour decided in section 6.* `GLOSS_ACTOR` is fixed per session by the SessionStart hook, so a Claude Code subagent runs as `agent:claude-code` and its remit is neither shown nor gated. Claude Code passes the subagent's name as `agent_type` to every hook that fires inside it, and it has a SubagentStart event. So the fix is in the guard: when `agent_type` names a file in `.claude/agents/`, treat the actor as `agent:<agent_type>`. A SubagentStart hook then gives the subagent its own briefing, scoped to its remit. Links the subagent writes are still attributed to the session's actor at `gloss build`, unless the subagent runs with its own `GLOSS_ACTOR`.

### A5. Packaging for Claude Code: one plugin

*Needs code (M).* Today an adopter runs `gloss hook install --skill` and gets files copied into the repository, which then drift from the binary that wrote them. A Claude Code plugin in this repository would bundle the skills, the hooks and the MCP server registration, versioned with the release tag:

```
.claude-plugin/plugin.json        name: glossmark, version from the tag
.claude-plugin/marketplace.json   name, owner, plugins: [{ name: glossmark, source: "./" }]
skills/<name>/SKILL.md            the same files the binary embeds
hooks/hooks.json                  { "hooks": { SessionStart, PreToolUse, PostToolUse, Stop } } as in A3
.mcp.json                         gloss mcp with GLOSS_ACTOR
```

Adopters install it with `/plugin marketplace add mckalvan/glossmark` and `/plugin install glossmark@glossmark`. `hooks/hooks.json` sits next to the existing `hooks/pre-commit`, which is harmless. The plugin does not ship the `gloss` binary, so its hooks still fail open when `gloss` is missing. `gloss hook install` stays for harnesses and teams that want the files in the repository. Validate with `claude plugin validate .` before each release.

### A6. Harness doc: `docs/AGENT-HARNESS.md`

One page for the developer setting up an agent, not for the agent. The name avoids `docs/AGENTS.md`, which agents that read nested `AGENTS.md` files would load as instructions. Outline:

1. **The seven layers**, the table from section 2 of this proposal.
2. **Two setups, not one per tool.** *Claude Code*: the plugin, or `gloss hook install --skill --guards`, which brings rules, briefing, MCP, guards, skills and subagent remits. *Any other agent*: three steps that rely only on portable conventions. Keep the rules block in `AGENTS.md`, register `gloss mcp` with `GLOSS_ACTOR=agent:<tool>` wherever that tool registers MCP servers, and install the pre-commit hook. The page links to each tool's own MCP documentation instead of copying its config paths, because those change faster than GlossMark releases. A tool that reads neither `AGENTS.md` nor MCP still gets the pre-commit hook and the PR comment.
3. **Actor naming.** `agent:<harness>` for a session, `agent:<harness>@<model>` when the model matters for audits, `agent:<role>` for a role agent with a remit file, `human:<name>` for anyone accepting or rejecting.
4. **Token budgets.** Rules block ≤ 600 tokens, briefing 2 000 by default, skills loaded on demand, vocabulary never pasted.
5. **Failure modes and what catches them**: `gloss` not on `PATH` makes every hook silently empty (doctor); `GLOSS_ACTOR` unset attributes agent links to a human (doctor, build warning); ledger not committed loses every decision (doctor); stale graph (pre-commit, Stop guard).
6. **What an agent may decide alone**, as a table: SOFT links yes; `NEW:` proposals yes; manifest edits yes but flagged; ontology edits only through `gloss propose`; accept, reject, criticality and `glossmark.yaml` never.

### A7. Diagnostics reference: `docs/DIAGNOSTICS.md` and `gloss explain`

*Needs code (S).* Move every GM code into one table in code (code, severity, meaning, fix, doc link), generate `docs/DIAGNOSTICS.md` from it, and add `gloss explain GM305`. A test fails when a code is emitted without a row. The `GRAMMAR.md`, `VERSIONING.md` and `FILE-FORMATS.md` tables link to it instead of keeping partial copies.

### A8. Briefing fixes

*Needs code (S).*

- Collapse the allowed-targets line when it is long: `gmi:*, everything the manifest declares (39 concepts)`. Today it costs about 40 % of a 600-token budget at the root of this repository.
- Name the installed skills in the briefing's first paragraph, so an agent in a harness without skill discovery knows they exist.

## 4. Track B: for GlossMark contributors

### B1. Rules: root `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md`

`AGENTS.md` at the root is the contributor rules file and is harness-neutral. `CLAUDE.md` is one line, `@AGENTS.md`, plus the list of repository skills. `CONTRIBUTING.md` is the same content for humans, with the release and issue process added. Draft of `AGENTS.md`:

````markdown
<!-- @gloss instructs arch:glossmark scope=file -->
# Working on GlossMark

GlossMark is a Go CLI, `gloss`, that compiles `@gloss` comment markers, ontology files and boundary manifests into one graph, then lints, diffs, gates and serves it. This repository is also its own first user: it owns the `gm:` (core) and `gmi:` (implementation) namespaces and links its own code to them.

## Commands

```sh
make build            # ./gloss with the git tag as version
make lint             # gofmt
make test             # go vet ./... && go test ./...   (about 15 s cold)
./gloss check         # 0 errors, 0 warnings on this repo, always
./gloss drift         # no violations, gaps, dangling, orphans or presented-only
./gloss build -q      # rewrite .glossmark/graph.jsonl and ledger.jsonl; commit both
```

CI also runs `check` on every example and `gloss landscape` on the payments pair (`.github/workflows/ci.yml`). Run the same before you push when you touch `internal/`.

## Done means

1. `make lint test` passes. That includes `TestSelfHost`, which fails on any lint warning in this repository.
2. `./gloss build -q` was re-run and `.glossmark/` is committed with the change.
3. The examples still check, as in CI.
4. Every doc that describes what you changed is updated in the same commit: the README command table, `QUICKSTART.md`, `ADVANCED.md`, `FILE-FORMATS.md` or `GRAMMAR.md` as relevant.
5. The commit message is a verb phrase ending in the issue number: `Add gloss doctor: one line per setup check with its fix (#22)`.

## Never

- **Never change the frozen grammar.** `marker.ReferenceRegex` is the definition of grammar 1. Scanner changes start with a corpus file and `corpus/expected.json`, and the reference must still agree on every plain marker. A change to how an existing marker parses is a grammar bump: read `docs/VERSIONING.md` first.
- **Never change an on-disk format silently.** A new or changed field in `graph.jsonl`, `ledger.jsonl` or `glossmark.lock` bumps the constant in `internal/version`, `docs/VERSIONING.md`, and the `value:` of `gmi:GraphFormatVersion`. `gloss drift` fails if you forget the last one.
- **Never make output non-deterministic.** `graph.jsonl` is sorted so `git diff` means something. A PATCH release never changes output for the same input.
- **Never add a dependency** beyond `yaml.v3` and `modernc.org/sqlite` without an issue that says why. No CLI framework: subcommands use `flag` and `register()`.
- **Never paraphrase an agent-facing sentence.** `agents.ProposalSentence` and `agents.Rules` are the only wording. MCP tool names, their required arguments and the briefing's section names are API; renaming one is a MAJOR change (`docs/AGENT-CONTRACT.md`).
- **Never accept this repository's own proposals.** `proposed: true` concepts and stale links wait for a maintainer.

## Linking your change to the ontology

- Every new Go file carries `// @gloss implements gmi:<Concept> scope=file` in its header. Tests carry `// @gloss verifies <concept> scope=file`.
- A new mechanism with no `gmi:` concept: `./gloss propose concept gmi:<Name> --kind process --realizes gm:<Core> --doc "<one sentence>"`. It ships with `proposed: true`.
- A command or package that realizes a core concept its container does not declare yet: add it to `realizes:` in `cmd/.gloss.yaml` or `internal/.gloss.yaml`.
- Links to existing `gm:` and `gmi:` concepts are written as bare `@gloss` here. This is the one place the adopter rule "propose, never accept" does not apply, because `TestSelfHost` checks every link in this tree. New concepts still ship with `proposed: true`.
- Docs carry `<!-- @gloss documents <concept> scope=file -->`. To write about a marker in prose, put it in backticks or a fenced block; a bare marker in an HTML comment is a real link.

## Where things are

| Change | Start in | Also touch |
| --- | --- | --- |
| A command | `cmd/gloss/<name>.go`, `register()` | `usage` text, README command table, `QUICKSTART.md` |
| A lint rule | `internal/check` | the diagnostics table, an e2e test |
| A relation or a rollup rule | `internal/model/vocab.go`, `internal/rollup` | `schemas/`, `ADVANCED.md`, an ADR in `docs/adr/` |
| The briefing, MCP or a shipped skill | `internal/agents`, `internal/mcp`, `skills/` | `docs/AGENT-CONTRACT.md`, the contract tests |
| The viewer | `internal/views`, `internal/viewer/assets` | a headless render check |

Use the matching skill in `.claude/skills/` for each of these.

## Writing docs

Plain declarative sentences, one idea each. Tables for reference material. Examples come from `examples/horse-factory` or `examples/payments`, never invented. No marketing words.
````

### B2. Contributor skills: `.claude/skills/gm-*`

These live in this repository only and are not shipped.

| Skill | Steps it encodes |
| --- | --- |
| `gm-add-command` | Write the command on `flag` and `register()` → help line → README table and `QUICKSTART.md` → `gmi:` concept or marker → e2e test → `doctor` check if it adds setup |
| `gm-add-diagnostic` | Pick the next free code in the right range (0xx grammar, 1xx resolution and imports, 2xx relations, 3xx ontology files, 4xx manifests, 5xx allowed lists, 6xx governance) → emit it → diagnostics table → e2e test with a minimal fixture → say whether the gate changes |
| `gm-grammar-or-format` | Corpus file and `expected.json` first → scanner change → `TestReferenceRegexAgreesOnPlainMarkers` → if a parse changes: grammar bump, `gloss migrate`, `VERSIONING.md` → if a file shape changes: format constant, `gmi:GraphFormatVersion`, refuse-newer test |
| `gm-modelling-change` | ADR from the 0001 template (context table from adopter evidence, decision, consequences) → vocab and schemas → rollup lift → `ADVANCED.md` table → e2e test on an adopter fixture → re-run both adoption repos and record the diff |
| `gm-agent-surface` | Any change to the briefing, rules, MCP tools or shipped skills: update the constant, not a copy → contract tests → `AGENT-CONTRACT.md` → re-render the briefing at the root and in a nested boundary at `--budget 600` and `2000` and read both |
| `gm-selfhost-sync` | After any change: `./gloss build -q`, `check`, `drift`, `status` → propose missing `gmi:` concepts → fix violations and gaps in the manifests → report stale links and proposed concepts for the maintainer |
| `gm-adoption-regression` | Rebuild pfinance and AgentArena with the branch binary → diff graphs, drift, coverage and timings against `main` → append to the review's regeneration table |
| `gm-release` | `make release-check` → `CHANGELOG.md` entry with MINOR or PATCH justified by `VERSIONING.md` → tag → confirm the Action, `install.sh` and pre-commit pins in docs match |

Draft of the one most likely to be needed first:

````markdown
---
name: gm-agent-surface
description: Change anything a coding agent reads from GlossMark - the session briefing, the AGENTS.md rules or vocabulary, the MCP tools and their descriptions, or a shipped skill - without the wording drifting between them. Use for changes in internal/agents, internal/mcp or skills/.
---

<!-- @gloss instructs gmi:AgentsSnippet scope=file -->

# Change the agent surface

What an agent reads is API. It must say one thing, in one wording, everywhere.

1. Find the constant. Behaviour sentences live in `internal/agents` (`ProposalSentence`, `Rules`). Change the constant; never edit a rendered copy.
2. Shipped skills and `docs/QUICKSTART.md` quote those constants. The contract test fails until each quote matches; update the quotes, never the test.
3. An MCP tool's name and required arguments are API. Adding a tool or an optional argument is MINOR. Renaming or removing either is MAJOR and needs a changelog entry.
4. Update `docs/AGENT-CONTRACT.md` for any change to the above.
5. Render and read the briefing yourself: `./gloss context --budget 600`, `./gloss context --dir internal/agents --budget 2000`, and the same in `examples/payments/payments-api` with `GLOSS_ACTOR=agent:refund-engineer`. Check that the first screen says where the agent is and what it may link to, and that nothing important was cut.
6. Run `make test` and `./gloss check`.
````

### B3. Contributor guards

Dogfood A3 on this repository, with the dev build instead of an installed binary, because the binary under development is what contributors want to exercise:

- `make build` before the session; hooks call `./gloss`, falling back to `gloss` on `PATH`.
- SessionStart: the standard briefing.
- Stop: `gofmt -l .`, `./gloss check`, and a graph-current check. These take under a second. `go test ./...` stays out of the Stop hook because it takes about 15 s cold; the "done means" list asks for it instead.

### B4. Contributor docs

- **`docs/AGENT-CONTRACT.md`**: the agent-facing API in one place. It lists the rules constants, the briefing's sections and their order, the MCP tool names with required arguments, the hook command lines that `gloss hook install` writes, and the shipped skill names. For each, it states what a MINOR and a MAJOR change look like and names the test that pins it.
- **`CHANGELOG.md`**: starts at v0.1.0. `VERSIONING.md` already depends on it.
- **`CONTRIBUTING.md`**: B1's content for humans, plus how issues are sized (S, M, L as in PLAN §10) and that each issue ships with an e2e test.

### B5. Make the rules true before writing them down

Small changes, each a test or a cleanup:

1. **A contract test**: `SKILL.md` for every shipped skill and `QUICKSTART.md` contain `ProposalSentence` verbatim, and `skills.Names` matches the directories under `skills/`. This also gives `gmi:AdoptionSkill` its missing verifier.
2. **Review the 43 stale links and 6 proposed concepts** with the `gloss-review` skill, so the self-host graph shows a clean ledger to anyone who copies its setup.
3. **Run `gloss hook install`** on this repository and commit `.claude/settings.json` and `.mcp.json`, so `gloss doctor` is clean here.
4. **Tests for `gmi:Publisher` and `gmi:ReviewCommands`**, the other two untested concepts.

## 5. Order

Sizes follow PLAN §10: S is a day, M a few days.

| Step | Items | Size | Why in this order |
| --- | --- | --- | --- |
| 1 | B5.1 contract test, B1 contributor `AGENTS.md`/`CLAUDE.md`, B5.3 dogfood the hook | S | Cheap, no CLI change, and every later step is written by agents that read them |
| 2 | A1 `agents.Rules` and `gloss agents --rules`; `init` and `doctor` learn about it | S | The neutral layer every harness gets |
| 3 | A2 skills `gloss-link`, `gloss-resolve-drift`, `gloss-review`; B2 `gm-agent-surface`, `gm-selfhost-sync` | S | Day-2 workflows; mostly text, pinned by step 1's test |
| 4 | A7 diagnostics table and `gloss explain`; A8 briefing fixes | S | Agents can fix what they hit |
| 5 | A3 `gloss hook guard`; B3 contributor guards | M | The only new runtime behaviour; built on `impact` and `check` |
| 6 | A6 `AGENT-HARNESS.md`, B4 `AGENT-CONTRACT.md`, `CHANGELOG.md`, `CONTRIBUTING.md` | S | Written once the surfaces above are settled |
| 7 | A5 plugin; A4 subagent actor | M | Depends on the guard |
| 8 | Remaining skills and B5.2, B5.4 | S | Cleanup |

Each step is testable the way PLAN §10 asks: run the rendered rules, briefing and skills on the pfinance and AgentArena adoption branches, and on this repository, and check that an agent following them leaves `gloss check` clean and `gloss drift` no worse.

## 6. Decisions

Taken by the maintainer on 2026-10-19. The sections above already reflect them.

| Question | Decision | Effect |
| --- | --- | --- |
| Which harnesses get first-class docs | Claude Code only. Every other harness gets one generic path | A6 has two setups, not five. The generic path uses only what is portable: `AGENTS.md`, `gloss mcp` and pre-commit. It links to each tool's own docs for where an MCP server is registered, so GlossMark does not maintain per-tool config paths that change every quarter |
| Stop guard strictness | Block by default | A3: the Stop guard blocks on `check` errors and a stale graph. A repository turns blocking off with `policy.guard-stop: report` |
| Subagent remits at edit time | Warn, and gate in the PR | A3, A4: the guard tells a subagent it is outside its remit; the PR gate makes the link HARD, as today. `policy.guard-ask` opts a repository into asking first |
| Self-host marking | Keep the contributor exception | B1: agents working on GlossMark write bare `@gloss` links to existing `gm:` and `gmi:` concepts; new concepts still ship with `proposed: true` |
| Skill distribution | Both | A2, A5: one set of `skills/` files, embedded in the binary for `gloss hook install --skill` and served by the Claude Code plugin |

## 7. Implementation status

Built on `claude/agent-harness-proposal` by five agents in two waves, merged and verified after each wave: `gofmt`, `go vet`, `go test ./...` including `TestSelfHost`, `gloss check` at 0 errors and 0 warnings, `gloss drift` at 0 violations and 0 gaps, and the example checks CI runs. `CHANGELOG.md` lists every change.

| Item | Status | Where |
| --- | --- | --- |
| A1 rules block | Done | `internal/agents/rules.go`, `gloss agents --rules`, `gloss init`, `gloss doctor`, MCP `instructions` |
| A2 skills | Done: all six | `skills/`, embedded by `skills/skills.go` |
| A3 guard | Done | `gloss hook guard`, `gloss hook install --guards`, `internal/guard`, policy `guard-stop` and `guard-ask` |
| A4 role agents | Done, except the SubagentStart briefing | `gloss init --role`; the guard reads `agent_type` |
| A5 plugin | Done; passes `claude plugin validate` | `.claude-plugin/`, `hooks/hooks.json`, `.mcp.json` |
| A6 harness doc | Done | `docs/AGENT-HARNESS.md` |
| A7 diagnostics | Done: 66 codes | `internal/diag`, `gloss explain`, `docs/DIAGNOSTICS.md` |
| A8 briefing fixes | Done | `internal/agents/context.go` |
| B1 contributor rules | Done | `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md` |
| B2 contributor skills | Done: all eight | `.claude/skills/gm-*` |
| B3 contributor guards | Done | `.claude/settings.json`, using `./gloss` when built |
| B4 contract and changelog | Done | `docs/AGENT-CONTRACT.md`, `CHANGELOG.md` |
| B5.1 contract test | Done | `internal/e2e/contract_test.go`, `internal/mcp/tools_test.go` |
| B5.2 review stale links and proposed concepts | Not done: a human decision | `gloss status`; the `gloss-review` skill prepares it |
| B5.3 dogfood the hook | Done | `.claude/settings.json`, `.mcp.json` |
| B5.4 untested concepts | Done | `internal/e2e/publish_review_test.go` |

### Where the build differs from the proposal

- **The rules block is about 700 tokens, not 600.** The approved wording was kept, and a test caps the block at 3 000 characters.
- **The briefing collapses allowed sets above 12 patterns, not 8.** At 8, a useful nine-pattern list in `examples/payments` was hidden.
- **Diagnostic ranges follow the code.** 4xx is the own-subject rule, 5xx is manifests and allowed lists, 1xx includes marker redundancy, and 9xx exists. `gm-add-diagnostic` uses the real ranges.
- **`--skills a,b` replaces `--skill <name>`,** so the boolean `--skill` keeps working.
- **`guard-ask` levels match exactly.** Unlike `criticality-gates`, listing `high` does not also cover `critical`.
- **The contributor Stop hook leaves out `./gloss check`,** because the dogfooded Stop guard already blocks on check errors and a stale graph. Its `gofmt` check ignores `.claude/`.

### Found and fixed while building

- `gloss` walked linked git worktrees, such as `.claude/worktrees/<name>`, as part of the repository, so `gloss check` failed whenever an agent harness had worktrees open.
- `gloss accept` and `gloss reject` stopped reading flags at the first positional argument.
- `gloss publish` wrote `published-as` twice.
- `gloss accept` refused stale links, so the re-acceptance path the `gloss-resolve-drift` and `gloss-review` skills describe did not work.

### Left for a maintainer

- **Proposed concepts.** `gmi:Diagnostics`, `gmi:HookGuard` and `gmi:ClaudeCodePlugin` are new, and the six from before are still open. Each needs `gloss accept --concept` or `gloss reject --concept`.
- **Stale links.** Their count rose from 43 to 55, because the work edited files that carry accepted links. Run `gloss status`, then `gloss accept <id>` for each link that still holds.
- **Two concept docs describe less than the code now does.** `gmi:AgentsSnippet` now also covers the rules block. `gmi:AdoptionSkill` names only one skill. Both are ontology edits for a human.
- **The SubagentStart briefing from A4** was not built. Claude Code's documentation does not confirm how a SubagentStart hook adds context.
- **The v0.1.0 tag** that every doc pins still does not exist.
