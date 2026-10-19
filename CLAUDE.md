<!-- @gloss instructs arch:glossmark scope=file -->
@AGENTS.md

## Contributor skills

These live in `.claude/skills/` and are for working on GlossMark itself; they are not shipped.

- `gm-add-command`: a new `gloss` subcommand or flag.
- `gm-add-diagnostic`: a new GM code, its row in `internal/diag` and the regenerated `docs/DIAGNOSTICS.md`.
- `gm-grammar-or-format`: the marker scanner, the corpus, or an on-disk format.
- `gm-modelling-change`: a relation, a rollup rule or a concept kind, with its ADR.
- `gm-agent-surface`: the briefing, the rules block, the MCP tools, the hook guard or a shipped skill.
- `gm-selfhost-sync`: bring this repository's own graph, manifests and ontology up to date after a change.
- `gm-adoption-regression`: compare a branch build against `main` on adopter repositories.
- `gm-release`: cut a release tag with its changelog entry.
