---
name: gm-agent-surface
description: Change anything a coding agent reads from GlossMark - the session briefing, the AGENTS.md rules block or vocabulary, the MCP tools and their descriptions, the hook command lines, the hook guard's output, a shipped skill or the Claude Code plugin - without the wording drifting between them. Use for changes in internal/agents, internal/mcp, internal/guard, cmd/gloss/guard.go, the hook installer in cmd/gloss/impact.go, skills/, hooks/hooks.json, .mcp.json or .claude-plugin/.
---

<!-- @gloss instructs gmi:AgentsSnippet scope=file -->

# Change the agent surface

What an agent reads is API. It must say one thing, in one wording, everywhere. `docs/AGENT-CONTRACT.md` lists every part of it, what counts as MINOR or MAJOR, and the test that pins it.

1. **Find the constant.** Behaviour sentences live in `internal/agents`: `Rules`, `RulesIntro`, `RuleSkills` and the begin/end markers in `rules.go`, `ProposalSentence` in `context.go`. Change the constant; never edit a rendered copy (`AGENTS.md`, a skill, `docs/QUICKSTART.md`).
2. **Quotes follow the constant.** Shipped skills in `skills/` and `docs/QUICKSTART.md` quote `ProposalSentence`. The contract test (`internal/e2e/contract_test.go`) fails until each quote matches; update the quotes, never the test.
3. **Re-render the rules block** after any change to `rules.go`: `go run ./cmd/gloss agents --rules --write AGENTS.md`. `TestRulesBlock` keeps the block under 3 000 characters (about 700 tokens); `gloss doctor` warns on a stale block in any repository, this one included.
4. **MCP tools.** Names, descriptions and input schemas are the `tools` table in `internal/mcp/server.go`; `TestToolContract` in `internal/mcp/tools_test.go` pins the names and required arguments. Adding a tool or an optional argument is MINOR. Renaming or removing a tool or a required argument, or making an optional argument required, is MAJOR and needs a `CHANGELOG.md` entry. The server's `instructions` come from `agents.MCPInstructions()`; do not write them by hand.
5. **Hooks and the guard.** The hook command lines are `SessionStartCommand`, `GuardCommand`, `GuardEvents` and `GuardMatcher` in `internal/agents/hooks.go`; `cmd/gloss/impact.go` and `cmd/gloss/guard.go` write them, and the plugin ships them in `hooks/hooks.json` and `.mcp.json`. The guard's output fields are `guard.Output` in `internal/guard/guard.go`. `TestHookInstallGuards` and the other tests in `internal/e2e/guard_test.go`, and `TestClaudeCodePluginContract` in `internal/e2e/contract_test.go`, pin them: change the plugin files in the same commit. `gloss doctor` finds the SessionStart hook by the text `gloss context` in `.claude/settings.json`: keep that substring in any new command line. The dogfood hooks in this repository's `.claude/settings.json` mirror the installer's lines; update them in the same change.
6. **Skills.** A shipped skill is `skills/<name>/SKILL.md`, embedded by `skills/skills.go` and listed in `skills.Names`. Frontmatter on line 1 (`name` equal to the directory, a specific `description`), then an `instructs` marker, then steps. It refers to the rules block by reference and never restates a rule.
7. **Update `docs/AGENT-CONTRACT.md`** for any change to the above, and the Agent experience section of `docs/QUICKSTART.md` when an adopter would notice.
8. **Read the briefing yourself.** Build with `make build`, then:
   ```sh
   ./gloss context --budget 600
   ./gloss context --dir internal/agents --budget 2000
   (cd examples/payments/payments-api && GLOSS_ACTOR=agent:refund-engineer ../../../gloss context --dir services/api --budget 600)
   ```
   Check that the first screen says where the agent is and what it may link to, that the remit line names only the refund engineer's remit in the last one, and that nothing important was cut by the budget. `TestBriefingV2` and `TestAgentRulesBriefing` pin the ordering, the budget and the collapsed allowed line.
9. Run `make test` and `./gloss check`, then `./gloss build -q` and commit `.glossmark/`.
