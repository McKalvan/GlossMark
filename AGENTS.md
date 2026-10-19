<!-- @gloss instructs arch:glossmark scope=file -->
# Working on GlossMark

GlossMark is a Go CLI, `gloss`, that compiles `@gloss` comment markers, ontology files and boundary manifests into one graph, then lints, diffs, gates and serves it. This repository is also its own first user: it owns the `gm:` (core) and `gmi:` (implementation) namespaces and links its own code to them.

## Commands

```sh
make build            # ./gloss, version from git describe (git-ignored; never commit it)
make lint             # gofmt: fails when gofmt -l . lists a file
make test             # go vet ./... && go test ./...
./gloss check         # 0 errors, 0 warnings on this repo, always
./gloss drift         # Violations: 0, Gaps: 0, no dangling, orphan or presented-only finding
./gloss build -q      # rewrite .glossmark/graph.jsonl and ledger.jsonl; commit both
./gloss explain GM305 # what a diagnostic code means and how to fix it (GM3xx for a range)
./gloss doctor        # the setup checks an adopter sees, run on this repo
go run ./cmd/gloss explain --all --format markdown > docs/DIAGNOSTICS.md   # after any change to internal/diag
```

CI (`.github/workflows/ci.yml`) also checks the examples. Run the same before you push when you touch `internal/` or `cmd/`:

```sh
(cd examples/horse-factory && ../../gloss check && ../../gloss build -q)
(cd examples/payments/payments-api && ../../../gloss check)
(cd examples/payments/settlement-worker && ../../../gloss check)
./gloss landscape examples/payments/payments-api examples/payments/settlement-worker --fail-on dangling
```

`gloss build -q` in `examples/horse-factory` rewrites that example's `.glossmark/`; commit the result only when your change is meant to alter it.

## Done means

1. `make lint test` passes. That includes `TestSelfHost`, which fails on any `gloss check` error or warning in this repository and on any violation, gap, dangling reference, orphan or presented-only finding.
2. `./gloss build -q` was re-run and `.glossmark/` is committed with the change.
3. The examples still check, as in CI.
4. Every doc that describes what you changed is updated in the same commit: the README command table, `docs/QUICKSTART.md`, `docs/ADVANCED.md`, `docs/FILE-FORMATS.md`, `docs/GRAMMAR.md` or `docs/AGENT-CONTRACT.md` as relevant, and `CHANGELOG.md` under Unreleased.
5. The commit message is a verb phrase, ending in the issue number when there is one: `Add gloss doctor: one line per setup check with its fix (#22)`.

## Never

- **Never change the frozen grammar.** `marker.ReferenceRegex` in `internal/marker/grammar.go` is the definition of grammar 1. Scanner changes start with a corpus file in `corpus/testdata/` and its entry in `corpus/expected.json`, and `TestReferenceRegexAgreesOnPlainMarkers` must still pass. A change to how an existing marker parses is a grammar bump: read `docs/VERSIONING.md` first.
- **Never change an on-disk format silently.** A new or changed field in `graph.jsonl`, `ledger.jsonl` or `glossmark.lock` bumps the constant in `internal/version/version.go`, `docs/VERSIONING.md`, and, for the graph, the `value:` of `gmi:GraphFormatVersion` in `ontology/gmi.gloss.yaml`. `gloss drift` fails if you forget the last one.
- **Never make output non-deterministic.** `graph.jsonl` is sorted so `git diff` means something. A PATCH release never changes output for the same input.
- **Never add a dependency** beyond `gopkg.in/yaml.v3` and `modernc.org/sqlite` without an issue that says why. No CLI framework: subcommands use `flag` and `register()` in `cmd/gloss`.
- **Never paraphrase an agent-facing sentence.** `agents.ProposalSentence` and `agents.Rules` in `internal/agents` are the only wording. MCP tool names, their required arguments, the briefing's section names and the hook command lines are API; renaming one is a MAJOR change (`docs/AGENT-CONTRACT.md`).
- **Never edit `docs/DIAGNOSTICS.md` or the rules block below by hand.** Both are generated; `TestDiagnosticsDocIsGenerated` and `gloss doctor` catch a hand edit.
- **Never accept this repository's own proposals.** `proposed: true` concepts and stale links wait for a maintainer.

## Linking your change to the ontology

- Every new Go file carries `// @gloss implements gmi:<Concept> scope=file` in its header. Tests carry `// @gloss verifies <concept> scope=file`.
- A new mechanism with no `gmi:` concept: `./gloss propose concept gmi:<Name> --kind process --realizes gm:<Core> --doc "<one sentence>"`. It ships with `proposed: true`.
- A command or package that realizes a core concept its container does not declare yet: add it to `realizes:` in `cmd/.gloss.yaml` or `internal/.gloss.yaml`.
- Docs carry `<!-- @gloss documents <concept> scope=file -->`, agent files and skills `<!-- @gloss instructs <concept> scope=file -->`. To write about a marker in prose, put it in backticks or a fenced block; a bare marker in an HTML comment is a real link.

## Where things are

| Change | Start in | Also touch |
| --- | --- | --- |
| A command | `cmd/gloss/<name>.go`, `register()` | its one-line help in `register()`, README command table, `docs/QUICKSTART.md` |
| A lint rule or diagnostic code | `internal/check`, `internal/extract`, `internal/load` | the row in `internal/diag`, `docs/DIAGNOSTICS.md` (regenerated), an e2e test |
| A relation or a rollup rule | `internal/model/vocab.go`, `internal/rollup` | `schemas/`, `docs/ADVANCED.md`, an ADR in `docs/adr/` |
| The briefing, the rules, MCP, the hooks, the guard, a shipped skill or the plugin | `internal/agents`, `internal/mcp`, `internal/guard`, `skills/`, `hooks/hooks.json`, `.claude-plugin/` | `docs/AGENT-CONTRACT.md`, the tests it names |
| The viewer | `internal/views`, `internal/viewer/assets` | a headless render check |

Use the matching skill in `.claude/skills/` for each of these: `gm-add-command`, `gm-add-diagnostic`, `gm-grammar-or-format`, `gm-modelling-change`, `gm-agent-surface`, `gm-selfhost-sync`, `gm-adoption-regression`, `gm-release`.

## Writing docs

Plain declarative sentences, one idea each. Tables for reference material. Examples come from `examples/horse-factory` or `examples/payments`, never invented. No marketing words.

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

Workflows for longer tasks: skills `gloss-link`, `gloss-resolve-drift`, `gloss-review`, `adopt-glossmark`. Rule details: https://github.com/mckalvan/glossmark/blob/main/docs/AGENT-HARNESS.md
<!-- gloss:rules:end -->

## On this repository

The rules block above is the one every GlossMark adopter gets. It applies here with one exception and two reminders:

- **Rule 3 does not apply to links to existing `gm:` and `gmi:` concepts.** Write them bare, as `@gloss`, never `@gloss?`. `TestSelfHost` checks every link in this tree, so the test is the review.
- **New concepts still go through `gloss propose`** and ship with `proposed: true` (rule 5). Never hand-edit `ontology/gm.gloss.yaml` or `ontology/gmi.gloss.yaml` to add one.
- **Never run `gloss accept` or `gloss reject` on this repository's own proposals**, links or concepts, unless a maintainer asks you to in this session.

This repository builds `gloss` from source in CI instead of pinning a release, because the code under test is the tool itself. `gloss doctor` therefore warns "CI pins: no GitHub workflow or pre-commit config references GlossMark" here; that warning is expected. The Claude Code hooks in `.claude/settings.json` run the dev build `./gloss` when it exists (`make build`), and `gloss` on `PATH` otherwise; a further Stop hook blocks while `gofmt -l` lists a Go file. Edit that file by hand: `gloss hook install` would replace the dev-build commands with the adopter ones.
