<!-- @gloss instructs dineb:kitchen-agents conf=0.9 scope=file -->
# Claude Code in this repository

Read `AGENTS.md` first; it is the shared brief for every agent. This file adds what is specific to Claude Code.

- Delegate with the Task tool to the subagent whose remit covers the change (`.claude/agents/`). If a change spans two remits, split it into two delegations.
- Skills in `.claude/skills/` walk through the two changes that go wrong most often: adding a menu item and auditing the allergen rule.
- Commands: `make test`, `make test-api`, `make test-kds`. Python uses `uv run pytest` inside `api/`.
- Keep pull requests small. One concern per PR; the reviewer is one person covering three restaurants.
