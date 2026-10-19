<!-- @gloss documents gmi:GlossCli scope=file -->
# Changelog

Releases are git tags `vX.Y.Z`; `docs/VERSIONING.md` says what MINOR and PATCH mean for the tool, the grammar, the file formats and namespaces. Agent-facing changes are classified in `docs/AGENT-CONTRACT.md`. Deprecations are announced here.

## Unreleased

### Added

- `gloss agents --rules` prints the GlossMark rules block for `AGENTS.md`: how to mark, that agents propose and humans accept, and what to run before finishing. `--rules --write AGENTS.md` replaces the block between its `gloss:rules:begin` and `gloss:rules:end` markers and keeps the rest of the file. The ten rules are constants in `internal/agents`; rule 5 is the proposal sentence the briefing and the MCP tools already use.
- `gloss init` writes the rules block into `AGENTS.md` and adds an `@AGENTS.md` line to an existing `CLAUDE.md`.
- `gloss doctor` checks the rules block: missing, stale against the installed `gloss`, or not imported by `CLAUDE.md`.
- `gloss explain <code>` prints a diagnostic's severity, meaning, fix and doc page; `gloss explain GM3xx` prints a range, `--all --format markdown|json` the whole table. Every GM code `gloss` emits (66, GM001–GM900) has one row in `internal/diag`, and a test fails when a code is emitted without one.
- `docs/DIAGNOSTICS.md`, generated from that table.
- `gloss hook guard --event pre-edit|post-edit|stop` reads a Claude Code hook payload on stdin. Before an Edit or Write it names a `high` or `critical` file's criticality and its source, the published-seam concepts the file touches, and whether the file is outside the acting agent's `may-modify` remit (`agent:<agent_type>` when `.claude/agents/<agent_type>.md` exists). After an edit it reports that file's `gloss check` findings. At Stop it blocks while `gloss check` reports errors or `.glossmark/graph.jsonl` is stale. It never blocks on proposals, fails open, and writes nothing.
- `gloss hook install --guards` registers the three guard hooks next to the SessionStart hook, replacing an earlier install's guard entries and keeping every other hook.
- `gloss hook install --skills name,...` installs named shipped skills; `--skill` still installs all of them.
- Policy keys `guard-stop` (`block`, the default, or `report`) and `guard-ask` (criticality levels and `remit`, for which the pre-edit guard returns `permissionDecision: "ask"`) in `glossmark.yaml` and its JSON Schema.
- Proposed concepts `gmi:Diagnostics` and `gmi:HookGuard` (`proposed: true`).
- Contributor and agent documentation: `AGENTS.md`, `CLAUDE.md` and `CONTRIBUTING.md` at the root, `docs/AGENT-HARNESS.md` (setting up an agent) and `docs/AGENT-CONTRACT.md` (everything an agent reads, as API), contributor skills in `.claude/skills/gm-*`, and Claude Code hooks for this repository in `.claude/settings.json`.
- End-to-end tests for `gloss publish` and for `gloss accept`, `reject` and `status` (`gmi:Publisher`, `gmi:ReviewCommands`), and a test that pins the MCP server's tool names and required arguments.

### Changed

- The session briefing collapses an allowed set of more than twelve patterns to its globs and a count (`gloss check --explain` lists it in full), and its first paragraph names the GlossMark skills installed under `.claude/skills/`.
- The MCP server's `instructions` quote the rules about the briefing, finding targets, proposing and reporting what a change touches.
- The partial diagnostic tables in `docs/GRAMMAR.md` and `docs/VERSIONING.md` are replaced by links to `docs/DIAGNOSTICS.md`.

### Fixed

- Linked git worktrees inside a repository, such as `.claude/worktrees/<name>/` created by an agent harness, are skipped when walking a workspace. `gloss` read every file twice, so `gloss check` and the self-host test failed on the second copy. Git submodules are still walked.
- `gloss accept` and `gloss reject` read flags after a positional argument: `gloss reject --concept pa:RefundHandler --reason "..." --remove`, as `docs/QUICKSTART.md` shows it, stopped parsing at the CURIE and failed with "--reason is required".
- `gloss publish` wrote a published implementation concept's `published-as` twice in `ontology.jsonld`, as a two-element array.

## v0.1.0

The release every install instruction, the GitHub Action, the pre-commit hooks and `gloss doctor` pin. The tag is pending (PLAN §10, issue #4): until it exists, `uses: mckalvan/glossmark@v0.1.0` and `install.sh v0.1.0` fall back or fail, and a build from source reports `0.0.0-dev`. It will cover everything on `main` up to 5d20c47: the `@gloss` grammar 1, `gloss build`, `check`, `drift`, `diff`, `impact`, `coverage`, `accept`, `reject`, `status`, `propose`, `context`, `hook install`, `doctor`, `init`, `suggest`, `import`, `criticality`, `lock`, `view`, `export`, `publish`, `landscape`, `agents` and `mcp`, graph and ledger format 1, lock format 1, and the GitHub Action with its PR comment.
