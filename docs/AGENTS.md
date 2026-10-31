<!-- @gloss documents gmi:AgentIntegrations scope=file -->
# Agent integrations

GlossMark's agent surface is the session-start briefing (`gloss context`), the MCP server (`gloss mcp`), the `AGENTS.md` vocabulary snippet (`gloss agents`) and the four shipped skills (`adopt-glossmark`, `derive-ontology`, `review-glossmark`, `curate-increment`). [QUICKSTART.md](QUICKSTART.md#agent-experience) explains each. This page is how to wire them into an agent in one step, so that a fresh clone plus one command gives an agent that asks `concepts_for_path` before it edits and writes `@gloss?` proposals.

```sh
gloss hook install                       # Claude Code, the plain form: SessionStart hook in .claude/settings.json, MCP server in .mcp.json
gloss hook install --agent claude-code   # Claude Code as a plugin: skills, hook and MCP server in one installable unit
gloss hook install --agent cursor        # Cursor: an always-applied rule with the vocabulary, and .cursor/mcp.json
gloss hook install --agent copilot       # GitHub Copilot: a section in .github/copilot-instructions.md, and .vscode/mcp.json
gloss hook install --agent cursor,copilot --skill   # several at once; --skill adds the skills to .claude/skills/ for agents that read them there
gloss doctor                             # one line per installed integration, with the fix when a piece is missing
```

Every file these write is meant to be committed, so each runs `gloss` **by name** and never by this machine's path (the bug behind #225 and #226). Whoever clones the repository needs `gloss` on `PATH` ([install](QUICKSTART.md#install)); `gloss doctor` says so when it is not, and warns when a committed file names an absolute path. Each integration runs as its own actor (`agent:claude-code`, `agent:cursor`, `agent:copilot`) unless `--actor` is given, so a link an agent writes is attributed to the agent that wrote it and the agent-remit gate applies to it. Rerunning the command rewrites what `gloss` owns and keeps everything else in the same files.

## Claude Code plugin (`--agent claude-code`)

Writes a [Claude Code plugin](https://code.claude.com/docs/en/plugins) at `.claude/plugins/glossmark/`, in the standard layout:

| File | What |
| --- | --- |
| `.claude-plugin/plugin.json` | the manifest: name `glossmark`, the gloss release as its version, description, author |
| `skills/<name>/SKILL.md` | the four skills, as `/glossmark:adopt-glossmark`, `/glossmark:derive-ontology`, `/glossmark:review-glossmark`, `/glossmark:curate-increment` |
| `hooks/hooks.json` | the SessionStart hook: `gloss context --changed --budget 2000` for the directory the session starts in, and `export GLOSS_ACTOR` into the session's shell |
| `.mcp.json` | the MCP server `gloss mcp`, as `plugin:glossmark:glossmark` in `/mcp`, with `GLOSS_ACTOR` in its env |

and lists it in `.claude-plugin/marketplace.json` at the repository root (created, or the `glossmark` entry added to a marketplace the repository already keeps), so the repository is its own marketplace. Then, once per machine:

```sh
claude plugin marketplace add ./ && claude plugin install glossmark@<repo>-plugins --scope project
claude --plugin-dir .claude/plugins/glossmark      # or: load it for one session without installing
```

`<repo>-plugins` is the marketplace name derived from the directory name (`gloss hook install` prints the exact command). `--scope project` records the plugin in the committed `.claude/settings.json`, so it is enabled for everyone on the repository; each teammate still runs the install once, because Claude Code copies a plugin into its own cache. The plain `gloss hook install` (no `--agent`) and the plugin can coexist; the hook then runs twice at session start, so pick one.

The plugin passes `claude plugin validate`. It carries no path into this repository: the hook and the server run `gloss` by name and the hook briefs on `$PWD`, so one installed plugin serves every GlossMark repository on the machine.

## Cursor (`--agent cursor`, or `--cursor`)

Cursor has no session hook, so the always-applied rule `.cursor/rules/glossmark.mdc` tells the agent to run `gloss context --dir <dir> --changed --budget 2000` before working in a directory, to run every gloss command as `GLOSS_ACTOR=agent:cursor`, to write `@gloss?` on what it adds, the proposal workflow for a concept that does not exist, and which skill in `.claude/skills/` to follow when asked to adopt, derive, review or curate (Cursor reads skills from there; `--agent cursor` installs them before `gloss init`, `--skill` after). After the rule comes the `AGENTS.md` vocabulary snippet for the whole repository, between `<!-- gloss agents: begin -->` and `<!-- gloss agents: end -->`, so the agent knows the names without calling the server; rerun `gloss hook install --agent cursor` when the ontology changes, and keep anything you add outside those markers. `.cursor/mcp.json` registers the MCP server in the `mcpServers` format Cursor shares with Claude Code.

## GitHub Copilot (`--agent copilot`)

Copilot reads `.github/copilot-instructions.md` on every request, so the integration is a `## GlossMark` section in that file, between `<!-- glossmark: begin -->` and `<!-- glossmark: end -->`: ask the `glossmark` MCP server `concepts_for_path` (or run `gloss context`) before editing a file, run gloss as `GLOSS_ACTOR=agent:copilot`, write `@gloss?`, the proposal workflow, the MCP tools, and the skills to follow; the vocabulary snippet follows inside the same section. Everything else in the file is kept. `.vscode/mcp.json` registers the server the way VS Code reads it (`servers`, `"type": "stdio"`); Copilot's coding agent on GitHub reads the same instructions file and needs the MCP server configured in the repository's Copilot settings, with `gloss` installed in its environment.

## What `gloss doctor` checks

| Line | Passes when |
| --- | --- |
| `agent hook` | the plain hook and server are in `.claude/settings.json` and `.mcp.json`; or, when neither is, at least one integration below is installed |
| `claude-code plugin` | the manifest has a name; the four skills are there; `hooks/hooks.json` has a SessionStart hook that runs `gloss context` by name with `GLOSS_ACTOR`; `.mcp.json` runs `gloss mcp` by name with `GLOSS_ACTOR`; `.claude-plugin/marketplace.json` lists the plugin |
| `cursor` | the rule is `alwaysApply: true` and asks for `gloss context`; `.cursor/mcp.json` runs `gloss mcp` by name with `GLOSS_ACTOR` |
| `copilot` | the instructions have the GlossMark section asking for `concepts_for_path` or `gloss context`; `.vscode/mcp.json` runs `gloss mcp` by name with `GLOSS_ACTOR` |

An integration that is not installed prints nothing. A failing line's fix is the `gloss hook install --agent <name>` that rewrites it.

## Other agents

For any other harness, `gloss agents --write AGENTS.md` gives it the vocabulary, `gloss mcp` is a standard stdio MCP server (command `gloss`, args `["mcp"]`, env `GLOSS_ACTOR=agent:<name>`), and `gloss context --dir <dir>` is the briefing to run at session start. Set `GLOSS_ACTOR` on every gloss command the agent runs so its links are attributed to it.
