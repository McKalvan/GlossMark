<!-- @gloss documents gm:Vocabulary scope=file -->
# Setting up a coding agent

This page is for the developer who sets up a coding agent in a repository that uses GlossMark. The agent itself reads the rules block in `AGENTS.md`, its briefing and the MCP tools; it does not need this page. Everything an agent reads is listed, with its stability guarantee, in [AGENT-CONTRACT.md](AGENT-CONTRACT.md).

## Seven layers

An agent harness has seven places to put guidance. GlossMark puts each fact in exactly one of them.

| Layer | Carries | Loaded | Size | Where |
| --- | --- | --- | --- | --- |
| **Rules** | Behaviour that never changes per directory: how to mark, what never to do, what done means | Always, by every harness that reads `AGENTS.md` | about 700 tokens | The rules block in `AGENTS.md`, written by `gloss agents --rules --write AGENTS.md` and by `gloss init` |
| **Briefing** | Where the agent is: boundary, allowed targets, remit, the concepts most relevant to the directory, current state | At session start, per directory | 2 000 tokens by default | `gloss context`, run by the SessionStart hook or the MCP `context` tool |
| **Tools** | Lookups and proposals on demand | When called | n/a | `gloss mcp`: `context`, `search_concepts`, `lookup_concept`, `concepts_for_path`, `propose_link`, `propose_concept`, `impact_of`, `drift` |
| **Guards** | Feedback between an edit and the commit | Before and after each edit, and when the agent stops | a few lines each | `gloss hook guard`, registered by `gloss hook install --guards` (Claude Code) |
| **Workflows** | Multi-step procedures for one kind of task | When the task matches | loaded on demand | Shipped skills, such as `adopt-glossmark`, `gloss-link`, `gloss-resolve-drift` and `gloss-review` |
| **Remits** | What one role may change | Per role | a line | `.claude/agents/<role>.md` with a `may-modify` marker |
| **Backstop** | What a human must see | On commit and on the PR | n/a | The pre-commit hook and the GitHub Action with its PR comment |

Rules are short and static; vocabulary is dynamic. The concept list is never pasted into `AGENTS.md`: the briefing and `search_concepts` rank it by relevance. `gloss agents` without `--rules` still prints the full vocabulary, but on a real ontology it runs to many kilobytes and grows every week.

## Two setups

GlossMark documents one setup for Claude Code and one for every other agent. The second uses only portable conventions, so it works the same in any tool that reads `AGENTS.md` or speaks MCP.

### Claude Code

1. **Rules.** Run `gloss agents --rules --write AGENTS.md` (`gloss init` does it for a new repository). If the repository has a `CLAUDE.md`, it needs a line `@AGENTS.md`: Claude Code reads `AGENTS.md` on its own only when there is no `CLAUDE.md`. `gloss init` adds the line.
2. **Briefing, tools, guards and skills.** Either install the GlossMark plugin, which bundles the hooks, the MCP server registration and the shipped skills at the version of its release:
   ```
   /plugin marketplace add mckalvan/glossmark
   /plugin install glossmark@glossmark
   ```
   or write the same into the repository, so every contributor gets it from git:
   ```sh
   gloss hook install --skill --guards
   ```
   This registers the SessionStart briefing and the three guards in `.claude/settings.json`, the MCP server in `.mcp.json`, and copies every shipped skill into `.claude/skills/`. All of them run as `GLOSS_ACTOR=agent:claude-code`; pass `--actor` to change it. Commit the files.

   Use one or the other, not both: with both, every hook runs twice and every skill appears twice. The plugin updates with its release; files written by `gloss hook install` stay at the version that wrote them until you re-run it.
3. **Remits for subagents.** A subagent file `.claude/agents/<role>.md` carries an `instructs` marker and a `may-modify` marker that lists what the role may link to. `gloss init --role refund-engineer --may-modify "pay:Refund, pay:Refunding, pa:*" --instructs pay:Refunding` writes one and nothing else. The payments example has this one:
   ```markdown
   <!-- .claude/agents/refund-engineer.md -->
   <!-- @gloss instructs pay:Refunding scope=file -->
   <!-- @gloss may-modify pay:Refund, pay:Refunding, pa:* scope=file -->
   # Refund engineer

   You implement refund flows in `services/api`. Propose ontology links with `@gloss?`.
   ```
   Inside a subagent, Claude Code passes the subagent's name to every hook as `agent_type`, and the guard judges each edit for `agent:<agent_type>` and that file's remit. An edit outside the remit gets a warning before it is made; the PR gate makes every link written there HARD. Set `policy.guard-ask: [remit]` in `glossmark.yaml` to have Claude Code ask the user first instead. Links a subagent writes are attributed to the session's actor at `gloss build`, unless the subagent runs with its own `GLOSS_ACTOR`. A session started with `GLOSS_ACTOR=agent:<role>` gets a briefing that shows only that role's remit.
4. **Backstop.** Install the pre-commit hook and the Action, as for any other agent below.

The Stop guard blocks the agent from finishing while `gloss check` reports errors or the committed graph is stale. To report instead of block, set `policy.guard-stop: report`.

### Any other agent

Three steps, each relying on a convention the tool already follows.

1. **Rules.** Keep the rules block in `AGENTS.md`: `gloss agents --rules --write AGENTS.md`. Most coding agents read `AGENTS.md` at the repository root; check your tool's documentation for whether it also reads nested ones.
2. **Tools.** Register `gloss mcp` as an MCP server with the environment variable `GLOSS_ACTOR=agent:<tool>`. Each tool documents where it registers MCP servers (a project file, a user settings file or a command); follow that documentation and give it this server:

   | Field | Value |
   | --- | --- |
   | command | `gloss` |
   | arguments | `mcp` |
   | environment | `GLOSS_ACTOR=agent:<tool>` |
   | transport | stdio |

   The server's instructions quote the rules, and its `context` tool returns the same briefing the Claude Code hook prints. Rule 1 of the rules block tells the agent to call it first.
3. **Backstop.** Install the pre-commit hook (`ln -s ../../hooks/pre-commit .git/hooks/pre-commit`, or the hooks in `.pre-commit-hooks.yaml` with `rev: v0.1.0`) and the GitHub Action (`uses: mckalvan/glossmark@v0.1.0`, see [QUICKSTART.md](QUICKSTART.md#7-wire-ci)).

A tool that reads neither `AGENTS.md` nor MCP still gets the pre-commit hook and the PR comment. Rules 9 and 10 of the rules block ask the agent to run what the guards would have run: `gloss check`, `gloss build`, and `gloss impact --base <base branch>` for its summary.

## Actor names

`GLOSS_ACTOR` is recorded in `.glossmark/ledger.jsonl` for every link, and decides whose remit applies.

| Actor | Use it for |
| --- | --- |
| `agent:<harness>` | A coding agent's session, such as `agent:claude-code` |
| `agent:<harness>@<model>` | The same, when audits need the model, such as `agent:claude-code@claude-opus-5-5`; the model is recorded separately in the ledger |
| `agent:<role>` | A role agent with a remit file `.claude/agents/<role>.md`; the briefing then shows only that remit |
| `human:<name>` | Anyone who accepts or rejects: `GLOSS_ACTOR=human:alice gloss accept <id>` |

Unset, `GLOSS_ACTOR` defaults to `human:<git user.name>`, which is right for a person and wrong for an agent: the remit gate applies only to `agent:` actors.

## Token budgets

| What | Budget | Where it is set |
| --- | --- | --- |
| Rules block | about 700 tokens (2 850 characters at four characters a token), read in full every session | `internal/agents`; a test keeps it under 3 000 characters |
| Briefing | 2 000 tokens by default | `gloss context --budget`; the SessionStart hook passes `--budget 2000` |
| Skills | 0 until a task matches; then one skill | loaded on demand by the harness |
| Vocabulary | never pasted | `search_concepts` and the briefing's ranked list |

Under a small budget the briefing cuts the concept list first and says how many it left out; an allowed set of more than twelve patterns is shown as its globs and a count.

## Failure modes

| What goes wrong | Effect | What catches it |
| --- | --- | --- |
| `gloss` is not on `PATH` where the agent runs | The SessionStart and edit hooks print nothing (their commands end in `\|\| true`), the Stop guard does not run, the MCP server does not start | `gloss doctor` ("gloss on PATH") |
| `GLOSS_ACTOR` is not set for the agent | Agent links are attributed to the git user, and the remit gate does not apply | `gloss doctor` ("GLOSS_ACTOR", "agent hook") |
| `.glossmark/ledger.jsonl` is not committed, or git-ignored | Every accept, reject and stale decision is lost on clone and in CI; actors are re-derived | `gloss doctor` ("ledger"); `gloss build` warns when it rewrites existing actors |
| The committed graph is stale | The PR comment and `gloss diff` compare against the wrong base | the pre-commit hook rebuilds it; the Stop guard blocks; `gloss doctor` ("graph") |
| `gloss check` has errors | Links that do not resolve, markers that do not parse | the post-edit guard names them in the edited file; the Stop guard blocks; the pre-commit hook and CI fail |
| The rules block is missing or older than the installed `gloss` | The agent follows outdated rules | `gloss doctor` ("agent rules"); fix with `gloss agents --rules --write AGENTS.md` |
| `CLAUDE.md` exists without `@AGENTS.md` | Claude Code never reads the rules | `gloss doctor` ("agent rules") |
| A subagent edits outside its remit | Links outside its `may-modify` list | the pre-edit guard warns (or asks, with `guard-ask: [remit]`); the PR gate makes the links HARD |
| An agent accepts its own proposals | A link reaches `accepted` without a human | rule 3 of the rules block; the ledger records `reviewed_by: agent:...`, so an audit finds it |

The hooks and the guard fail open by design: a broken setup never stops an agent, so `gloss doctor` is where it is reported. Run it after setup and after every upgrade.

## What an agent may decide alone

| Decision | The agent | How |
| --- | --- | --- |
| Add a link to an existing concept | Yes | As a proposal, `@gloss? ... conf=<0-1>`; SOFT links are accepted by a human or by `gloss accept --policy` when a human runs it |
| Propose a new concept | Yes | `ns:NEW:Name` in the marker, then `gloss propose --from-new` (MCP `propose_concept`); it ships with `proposed: true` |
| Move or delete a marker with its code | Yes | Rule 6 |
| Edit a boundary manifest (`.gloss.yaml`) | Only when that edit is the fix | Say so in the summary; the gate makes it HARD |
| Edit an ontology file | Only through `gloss propose` | Never by hand to make a link resolve |
| Accept or reject a link or concept | No | Only when a human asks in the session, recorded as `GLOSS_ACTOR=human:<name>` |
| Set or change criticality | No | A human applies `gloss criticality suggest --write` |
| Edit `glossmark.yaml` | No | Unless asked |
