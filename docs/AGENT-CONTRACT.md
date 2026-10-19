<!-- @gloss documents gmi:AgentsSnippet, gmi:McpServer, gmi:HookGuard scope=file -->
# Agent contract

Everything a coding agent reads from GlossMark is API: adopters' agents, skills, hooks and CI depend on its exact names and wording. This page lists every part of it, what a MINOR and a MAJOR change to it look like, and the test that pins it. For setting an agent up, see [AGENT-HARNESS.md](AGENT-HARNESS.md).

The tool-semver rules in [VERSIONING.md](VERSIONING.md#tool-releases) apply: a PATCH never changes any output for the same input, so any change below is at least a MINOR. Before 1.0 a MINOR may also make a MAJOR change, and `CHANGELOG.md` says so. Every change to this page's surfaces gets a changelog line.

## Rules block

`gloss agents --rules` renders it; `--write AGENTS.md` and `gloss init` put it into `AGENTS.md`; `gloss doctor` compares it with what the installed `gloss` renders.

| Part | Where | What it is |
| --- | --- | --- |
| `RulesBegin`, `RulesEnd` | `internal/agents/rules.go` | The comments that delimit the block: `<!-- gloss:rules:begin ...-->` and `<!-- gloss:rules:end -->`. A block is found by the prefix `<!-- gloss:rules:begin`, so the begin comment's tail may change |
| `RulesHeading`, `RulesIntro` | `internal/agents/rules.go` | `## GlossMark` and the paragraph that says when the rules apply |
| `Rules` | `internal/agents/rules.go` | The ten numbered rules, in order. Other text cites them by number ("rule 5") |
| `ProposalSentence` | `internal/agents/context.go` | The one proposal workflow, rule 5 verbatim. Also quoted by the briefing, the vocabulary snippet, the `propose_link` and `propose_concept` tool descriptions, every shipped skill and `docs/QUICKSTART.md` |
| `RuleSkills` | `internal/agents/rules.go` | The skills the block names for longer tasks: `gloss-link`, `gloss-resolve-drift`, `gloss-review`, `adopt-glossmark` |
| `RulesDocURL` | `internal/agents/rules.go` | The link to [AGENT-HARNESS.md](AGENT-HARNESS.md) |
| `ClaudeImport` | `internal/agents/rules.go` | `@AGENTS.md`, the line `gloss init` adds to `CLAUDE.md` and `gloss doctor` looks for |
| `MCPInstructions()` | `internal/agents/rules.go` | The MCP server's `instructions`: rules 1, 3, 4, 5 and 8 without Markdown emphasis |

- **MINOR**: rewording a rule without changing what it asks for or forbids; adding a rule at the end; adding a skill to `RuleSkills`. Adopters re-run `gloss agents --rules --write AGENTS.md` after upgrading, and `gloss doctor` tells them to.
- **MAJOR**: removing or renumbering a rule; changing what a rule permits or forbids (for example letting agents accept); changing `RulesEnd` or the begin prefix, which strands every existing block; changing `ClaudeImport`.
- **Pinned by**: `TestRulesBlock` (ten rules, rule 5 is `ProposalSentence`, the block names `RuleSkills` and `RulesDocURL`, stays under 3 000 characters, about 700 tokens; the MCP instructions quote the rules without emphasis), `TestUpsertRules` and `TestClaudeImport` in `internal/agents/rules_test.go`; `TestAgentRulesBriefing` (the block creates no links and passes `gloss check`); `TestDoctor` in `internal/doctor/doctor_test.go`; `TestShippedSkillsContract` and `TestQuickstartContract` in `internal/e2e/contract_test.go` (every shipped skill and `QUICKSTART.md` quote `ProposalSentence` verbatim; every skill in `RuleSkills` ships).

## Briefing

`gloss context [--dir <dir>] [--changed] [--budget <tokens>]` and the MCP `context` tool render it with `agents.Context` (`internal/agents/context.go`). Its sections, in order:

1. `# GlossMark context`.
2. The opening paragraph: what the repository does, how to write a proposal, `ProposalSentence`, the MCP tools to ask (`search_concepts`, `concepts_for_path`, `propose_link`, `impact_of`), and, when GlossMark skills are installed under `.claude/skills/`, `Skills for longer tasks, in .claude/skills/: <names>.`
3. `## You are touching`, only with `--changed` (MCP `changed: true`): what the uncommitted changes touch.
4. `Boundary: <curie> (<level>)`, with the default namespace (`` `.Name` = `<ns>:Name` ``), what the boundary realizes and its criticality, when a manifest applies to the directory.
5. `Allowed targets: ...`. More than twelve patterns collapse to the globs and a count.
6. The remit line: `Your remit as <actor>: you may modify ...` when `GLOSS_ACTOR` names an agent with a file in `.claude/agents/`, otherwise `Agent remits: ...` listing every one; absent when no file carries `may-modify`.
7. `Concepts (<n>, most relevant to this directory first)`, followed by `, first <k>; use search_concepts for the rest` when the budget cuts it, then one line per concept: `- <curie> (<kind>): <tags> — <doc>`.
8. `State: ...`: coverage, proposals, stale links, violations, gaps, untested.
9. `Implemented but untested here: ...`, when there are any.

The budget is estimated at four characters a token. The concept list is cut first; the whole text is truncated only as a last resort.

- **MINOR**: a new tag on a concept line; a new line inside a section; a ranking change; a new optional section after the ones above.
- **MAJOR**: renaming, removing or reordering a section; changing the remit line's wording, which agents match on; changing the default budget of 2 000 tokens; removing a `gloss context` flag or a `context` tool argument.
- **Pinned by**: `TestBriefingV2` (relevance order, budget, cut notice, remit scoping, the proposal sentence) and `TestAgentRulesBriefing` (skills named in the first paragraph, the collapsed allowed line, a 600-token budget) in `internal/e2e/e2e_test.go`; `TestAllowedLine` and `TestInstalledSkills` in `internal/agents/rules_test.go`.

## MCP tools

`gloss mcp` serves MCP over stdio as server `glossmark`, protocol `2024-11-05`. The tools are the `tools` table in `internal/mcp/server.go`:

| Tool | Required arguments | Optional arguments |
| --- | --- | --- |
| `context` | none | `dir`, `budget` (default 2000), `changed` |
| `search_concepts` | `query` | `namespace`, `kind`, `tier`, `limit` (default 20) |
| `lookup_concept` | `curie` | none |
| `concepts_for_path` | `path` | none |
| `propose_link` | `path`, `relation`, `target` | `conf`, `note`, `scope` |
| `propose_concept` | `curie`, `kind` | `doc`, `realizes`, `is_a`, `internal`, `aliases` |
| `impact_of` | none, but at least one of `proposals`, `paths`, `base` | each proposal: `path`, `relation`, `target` required, `conf`, `note` optional |
| `drift` | none | none |

- **MINOR**: a new tool; a new optional argument; a new field in a tool's result; a clearer description.
- **MAJOR**: renaming or removing a tool or an argument; making an optional argument required; removing or renaming a result field; renaming the server.
- **Pinned by**: `TestToolContract` in `internal/mcp/tools_test.go` (server name, instructions, every tool and its required arguments); `TestSearchByMeaning` in `internal/e2e/e2e_test.go` for `search_concepts` results.

## Hook command lines

The lines are built in `internal/agents/hooks.go`. `gloss hook install` writes the first two; `--guards` adds the three guards; the Claude Code plugin ships the same lines in `hooks/hooks.json` and `.mcp.json` for actor `agent:claude-code`. `<actor>` is `--actor`, default `agent:claude-code`.

| Where | Event and matcher | Command (`SessionStartCommand`, `GuardCommand`) |
| --- | --- | --- |
| `.claude/settings.json` | SessionStart | `GLOSS_ACTOR=<actor> gloss context --dir "$PWD" --changed --budget 2000 2>/dev/null \|\| true` |
| `.claude/settings.json` | PreToolUse, `Edit\|Write` (`GuardMatcher`) | `GLOSS_ACTOR=<actor> gloss hook guard --event pre-edit 2>/dev/null \|\| true` |
| `.claude/settings.json` | PostToolUse, `Edit\|Write` | `GLOSS_ACTOR=<actor> gloss hook guard --event post-edit 2>/dev/null \|\| true` |
| `.claude/settings.json` | Stop | `GLOSS_ACTOR=<actor> gloss hook guard --event stop` |
| `.mcp.json` | server `glossmark` | command `gloss`, args `["mcp"]`, env `GLOSS_ACTOR=<actor>` |

Re-running `gloss hook install` replaces the entries whose command contains `gloss context` or `gloss hook guard` and keeps every other hook. `gloss doctor` recognises the SessionStart hook by the text `gloss context` and `GLOSS_ACTOR=` in `.claude/settings.json`, and the MCP server by `"gloss"` and `GLOSS_ACTOR` in `.mcp.json`. A wrapper that runs a different binary (this repository's own `.claude/settings.json` does) keeps working as long as it keeps those substrings.

- **MINOR**: a new flag on `gloss hook install`; a new hook under a new flag; a new optional `gloss hook guard` flag.
- **MAJOR**: changing an existing line so that an earlier install is no longer recognised and replaced (the `gloss context` and `gloss hook guard` substrings); removing an event, the matcher or a `--event` value; the edit guards no longer failing open.
- **Pinned by**: `TestHookInstallGuards` in `internal/e2e/guard_test.go` (the exact guard lines and matcher, idempotent re-install, other hooks kept, `--skills`); `TestClaudeCodePluginContract` in `internal/e2e/contract_test.go` (`hooks/hooks.json` and `.mcp.json` equal what `gloss hook install` writes); `TestDoctor` for the detection substrings.

## Hook guard

`gloss hook guard --event pre-edit|post-edit|stop` reads a Claude Code hook payload on stdin: `hook_event_name`, `tool_name`, `tool_input.file_path`, `cwd`, `agent_type` and `stop_hook_active`. It prints at most one JSON object (`guard.Output` in `internal/guard/guard.go`) and exits 0; it exits 1 only on a usage error and never exits 2. It prints nothing when it has nothing to say, and nothing when the payload, the repository or the graph cannot be read.

| Event | Output fields | Content |
| --- | --- | --- |
| `pre-edit` | `hookSpecificOutput.hookEventName` = `PreToolUse`, `additionalContext`, and with `policy.guard-ask` also `permissionDecision` = `ask` and `permissionDecisionReason` | Lines starting `GlossMark: <path>`: criticality `high` or `critical` and its source, published-seam concepts, a governance file, or outside the acting agent's `may-modify` remit |
| `post-edit` | `hookSpecificOutput.hookEventName` = `PostToolUse`, `additionalContext` | `GlossMark: gloss check flags <n> findings in <path>`, then one line per finding with its GM code |
| `stop` | `decision` = `block`, `reason` | `gloss check` errors or a stale `.glossmark/graph.jsonl`, the fix, and the count of proposals that do not block. Nothing when `stop_hook_active` is true. With `policy.guard-stop: report` the reason goes to stderr and nothing blocks |

The acting agent is `agent:<agent_type>` when `.claude/agents/<agent_type>.md` exists, otherwise `GLOSS_ACTOR`. The policy keys are `guard-stop` (`block` or `report`) and `guard-ask` (criticality levels and `remit`) in `glossmark.yaml`.

- **MINOR**: more context in `additionalContext`; a new `guard-ask` entry; a new event under a new `--event` value.
- **MAJOR**: renaming or removing an event or an output field; blocking a stop for a new reason by default (it never blocks on proposals); asking by default; exiting 2.
- **Pinned by**: `TestGuardPreEdit`, `TestGuardPostEdit`, `TestGuardStop`, `TestGuardFailsOpen` and `TestHookInstallGuards` in `internal/e2e/guard_test.go`.

## Shipped skills

`skills.Names` in `skills/skills.go`, each `skills/<name>/SKILL.md` embedded in the binary, installed by `gloss hook install --skill` (all) or `--skills <name>,...`, and served by the plugin:

| Skill | For |
| --- | --- |
| `adopt-glossmark` | Bringing a repository under GlossMark |
| `gloss-link` | Keeping links correct during any change |
| `gloss-resolve-drift` | Resolving a `gloss drift` finding |
| `gloss-review` | Helping a human review proposals |
| `gloss-criticality` | Classifying criticality with a human |
| `gloss-upgrade-import` | Upgrading an imported namespace |

The briefing names the ones installed under `.claude/skills/`, and the rules block names four of them.

- **MINOR**: a new skill; changed steps inside a skill; a sharper `description`.
- **MAJOR**: renaming or removing a skill, since the rules block, the briefing, `--skills` and adopters' installed copies refer to it by name.
- **Pinned by**: `TestShippedSkillsContract` (`skills.Names` equals the directories; frontmatter `name` equals the directory; a description; an `instructs` marker; `ProposalSentence` verbatim; embedded byte for byte) and `TestQuickstartContract` (`QUICKSTART.md` names every skill) in `internal/e2e/contract_test.go`; `TestInstalledSkills` in `internal/agents/rules_test.go`.

## Diagnostic codes

Agents read GM codes in `gloss check` output and look them up with `gloss explain <code>` (`--format json` for tools). The table is `diag.Codes` in `internal/diag/diag.go`; [DIAGNOSTICS.md](DIAGNOSTICS.md) is generated from it.

- **MINOR**: a new code; a clearer meaning or fix.
- **MAJOR**: removing or renumbering a code; raising a warning to an error, which fails `gloss check` where it passed.
- **Pinned by**: `TestEveryEmittedCodeHasARow`, `TestEveryRowIsEmitted`, `TestRows` and `TestDiagnosticsDocIsGenerated` in `internal/diag/diag_test.go`.
