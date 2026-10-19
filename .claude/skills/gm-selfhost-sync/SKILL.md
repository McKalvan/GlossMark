---
name: gm-selfhost-sync
description: Bring GlossMark's own graph up to date after a change to this repository - rebuild, lint, compare, mark new files, propose missing gmi concepts, fix violations and gaps in the manifests, and report stale links and proposed concepts for the maintainer. Use before finishing any change to cmd/, internal/, ontology/, docs/ or a .gloss.yaml manifest here, and whenever TestSelfHost fails.
---

<!-- @gloss instructs arch:glossmark scope=file -->

# Keep the self-description in sync

This repository is its own first user. `TestSelfHost` in `internal/e2e/selfhost_test.go` fails on any `gloss check` error or warning, and on any violation, gap, dangling reference, orphan or presented-only finding. Links to existing `gm:` and `gmi:` concepts are bare here (AGENTS.md, "On this repository").

1. Build the binary under change: `make build` (or `go build -o gloss ./cmd/gloss`).
2. `./gloss check`. Fix every error and warning; `./gloss explain <code>` gives the fix. GM130 and GM131 mean a marker repeats what another already says: remove it.
3. Every new file you added carries a marker in its first lines: Go `// @gloss implements gmi:<Concept> scope=file`, tests `// @gloss verifies <concept> scope=file`, docs and skills an HTML comment with `documents` or `instructs`. `./gloss coverage` shows the file share; `./gloss impact` lists what your changed files touch.
4. No fitting `gmi:` concept for a new mechanism: `./gloss propose concept gmi:<Name> --kind process --realizes gm:<Core> --doc "<one sentence>"`. It ships with `proposed: true`. Never accept it, never hand-edit `ontology/*.gloss.yaml` to add it.
5. `./gloss drift`. Per finding:
   - **violation**: a package now realizes or produces a core concept its container does not declare. Add it to `realizes:` (or `produces:`, `publishes:`) in `cmd/.gloss.yaml` or `internal/.gloss.yaml`, or retarget the marker if it is wrong. Say so in the summary: manifest edits are HARD.
   - **gap**: a manifest declares something nothing implements any more. Mark the code that does, or remove the declaration.
   - **dangling**: a typo in a target; find the real concept.
   - **orphan**: a concept nothing links to. Link the code that implements it, or report it.
   - **untested**: add `verifies` to a test that really checks it, or write one when the task allows.
   - **value-drift** on `gmi:GraphFormatVersion` or `gmi:DefaultSoftThreshold`: the bound constant changed; see the `gm-grammar-or-format` skill.
6. `./gloss build -q`, then `go test ./internal/e2e/ -run TestSelfHost`.
7. `./gloss status`. Do not accept or reject anything. List, for the maintainer, the proposed concepts and the stale links your change created or touched.
8. Commit `.glossmark/graph.jsonl` and `.glossmark/ledger.jsonl` with the change. `./gloss doctor` should report the graph as current.
