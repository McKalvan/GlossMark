<!-- @gloss documents arch:glossmark scope=file -->
# Contributing to GlossMark

GlossMark is a Go CLI, `gloss`. This repository is also its own first user: `glossmark.yaml` owns the `gm:` (core) and `gmi:` (implementation) namespaces, and every package, command, schema and doc links to them. `AGENTS.md` holds the same rules for coding agents; this page adds the issue and release process.

## Set up

You need Go 1.24 (the version CI uses) and nothing else.

```sh
make build            # ./gloss, version from git describe; git-ignored
make lint             # gofmt
make test             # go vet ./... && go test ./...
```

Optional, to keep the committed graph current on every commit: `ln -s ../../hooks/pre-commit .git/hooks/pre-commit`. The hook runs `gloss` from your `PATH`, so `make install` first or it does nothing.

If you use Claude Code, `CLAUDE.md` imports `AGENTS.md`, `.claude/skills/gm-*` hold the contributor workflows, and `.claude/settings.json` registers the briefing, the edit guards and a Stop hook that blocks on unformatted Go files. The hooks use `./gloss` when it exists, so run `make build` before a session. The MCP server is registered in `.mcp.json`. Do not also install the GlossMark plugin in this checkout: its hooks would run a second time.

## Done means

1. `make lint test` passes. That includes `TestSelfHost` (`internal/e2e/selfhost_test.go`), which fails on any `gloss check` error or warning in this repository and on any drift violation, gap, dangling reference, orphan or presented-only finding.
2. `./gloss check` reports 0 errors and 0 warnings, and `./gloss drift` ends with `Violations: 0   Gaps: 0`.
3. `.glossmark/graph.jsonl` and `.glossmark/ledger.jsonl` are regenerated with `./gloss build -q` and committed with the change.
4. The examples still check, as in `.github/workflows/ci.yml`:
   ```sh
   (cd examples/horse-factory && ../../gloss check && ../../gloss build -q)
   (cd examples/payments/payments-api && ../../../gloss check)
   (cd examples/payments/settlement-worker && ../../../gloss check)
   ./gloss landscape examples/payments/payments-api examples/payments/settlement-worker --fail-on dangling
   ```
5. Every doc that describes the change is updated in the same commit, and `CHANGELOG.md` has a line under Unreleased.
6. The commit message is a verb phrase, ending in the issue number when there is one: `Add gloss doctor: one line per setup check with its fix (#22)`.

## Issues

Every issue states what ships, what it touches, the finding it closes, and a done-when that can be tested on an adopter fixture (`docs/PLAN.md`, section 10). Sizes:

| Size | Effort | Typical change |
| --- | --- | --- |
| S | a day | a lint rule, a flag, a doc page, a small command |
| M | a few days | a new command with its docs, a briefing or viewer change |
| L | a week plus an ADR | a modelling change: a relation, a rollup rule, a new finding kind |

Each issue ships with one end-to-end test in `internal/e2e/` that runs the change on `examples/horse-factory`, `examples/payments`, or a minimal fixture reproducing the adopter case, and carries a `verifies` marker for the concept it tests. A modelling change (size L) starts with an ADR in `docs/adr/`, in the shape of `docs/adr/0001-presents-and-hosts.md`.

## Rules that protect adopters

- The `@gloss` grammar is frozen: `ReferenceRegex` in `internal/marker/grammar.go` defines grammar 1. Scanner changes start with a file in `corpus/testdata/` and its entry in `corpus/expected.json`. A change to how an existing marker parses is a grammar bump (`docs/VERSIONING.md`).
- A new or changed field in `graph.jsonl`, `ledger.jsonl` or `glossmark.lock` bumps its constant in `internal/version/version.go`; for the graph, also the `value:` of `gmi:GraphFormatVersion` in `ontology/gmi.gloss.yaml`.
- Output is deterministic. A PATCH release never changes output for the same input.
- No dependencies beyond `gopkg.in/yaml.v3` and `modernc.org/sqlite` without an issue that says why. Subcommands use the standard `flag` package and `register()`.
- What agents read is API: the rules constants, the briefing's sections, the MCP tool names and required arguments, the hook command lines and the shipped skill names. `docs/AGENT-CONTRACT.md` lists them with the test that pins each.

## Linking your change

- New Go files start with `// @gloss implements gmi:<Concept> scope=file`; tests with `// @gloss verifies <concept> scope=file`; docs with an HTML comment carrying `documents`. Write markers to existing `gm:` and `gmi:` concepts bare, without `?`: `TestSelfHost` checks them all.
- A new mechanism with no concept: `./gloss propose concept gmi:<Name> --kind process --realizes gm:<Core> --doc "<one sentence>"`. It ships with `proposed: true` and waits for a maintainer; do not accept it yourself.
- A package that realizes a core concept its container does not declare: add it to `realizes:` in `cmd/.gloss.yaml` or `internal/.gloss.yaml`.

## Generated files

| File | Regenerate with | Pinned by |
| --- | --- | --- |
| `.glossmark/graph.jsonl`, `.glossmark/ledger.jsonl` | `./gloss build -q` | `gloss doctor` ("graph"), the pre-commit hook |
| `docs/DIAGNOSTICS.md` | `go run ./cmd/gloss explain --all --format markdown > docs/DIAGNOSTICS.md` | `TestDiagnosticsDocIsGenerated` |
| The rules block in `AGENTS.md` | `go run ./cmd/gloss agents --rules --write AGENTS.md` | `gloss doctor` ("agent rules") |
| `examples/horse-factory/.glossmark/` | `(cd examples/horse-factory && ../../gloss build -q)` | CI |

Never edit a generated file by hand.

## Releases

Releases are git tags `vX.Y.Z`; the docs, the Action, the pre-commit hooks and `gloss doctor` pin `v0.1.0`.

1. On a green `main`, run `make release-check` (lint, vet, tests, and the version it would embed).
2. Choose MINOR or PATCH by the tool-semver rules in `docs/VERSIONING.md`, using the Unreleased section of `CHANGELOG.md`.
3. Move the Unreleased entries under `## vX.Y.Z - YYYY-MM-DD` and start a new Unreleased section.
4. If the version changes, update every pin in one commit: `grep -rn "v0\.1\.0" --exclude-dir=.git --exclude-dir=.glossmark .` lists them.
5. Tag and push: `git tag vX.Y.Z && git push origin vX.Y.Z`. `.github/workflows/release.yml` runs the tests and attaches binaries for Linux, macOS and Windows with sha256 files.
6. Check that `install.sh vX.Y.Z` and `uses: mckalvan/glossmark@vX.Y.Z` download the release binary.

The `gm-release` skill in `.claude/skills/` has the full checklist.

## Writing docs

Plain declarative sentences, one idea each. Tables for reference material. Examples come from `examples/horse-factory` or `examples/payments`, never invented. No marketing words.
