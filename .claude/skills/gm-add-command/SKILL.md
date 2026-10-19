---
name: gm-add-command
description: Add or change a gloss subcommand or flag in cmd/gloss - flag parsing on the standard flag package, register() and its help line, the README and QUICKSTART entries, the gmi concept or marker, an end-to-end test, and a gloss doctor check when the command adds setup. Use for any new command, subcommand or flag in this repository.
---

<!-- @gloss instructs gmi:GlossCli scope=file -->

# Add a command

The rules in AGENTS.md apply. Commands are hand-rolled on `flag`; never add a CLI framework.

## Write it

1. Create `cmd/gloss/<name>.go`, or extend the file that already holds related commands (`accept.go` holds accept, reject and status; `impact.go` holds impact, context and hook). Start a new file with its marker: `// @gloss implements gmi:<Concept> scope=file`.
2. Register it in an `init()`:
   ```go
   func init() {
   	register(command{"name", "one line: what it does, its main flags", cmdName})
   }
   ```
   The second field is the line `gloss` prints in its usage; keep it to one line.
3. Parse flags with `fs, root := newFlags("name")` (it adds `--root`). Use `fs.Parse(flagsLast(args))` when positional arguments may come before flags. Return 2 on a usage error, `fail(err)` (exit 1) on a runtime error, 0 on success.
4. Load the repository with `p, err := buildGraph(*root, false)`; pass `true` only when the command must write `.glossmark/ledger.jsonl`. `p.WS`, `p.Graph`, `p.Ledger` and `p.Diags` are what other commands use.
5. Output is deterministic: sort maps before printing. A new output field or finding kind is a MINOR change (`docs/VERSIONING.md`); renaming or removing one is MAJOR and needs a `CHANGELOG.md` line.

## Link it

6. Find the concept: `./gloss context --dir cmd/gloss --budget 2000` lists the concepts most relevant to the directory, and MCP `search_concepts` searches by meaning. Prefer an existing `gmi:` concept.
7. No concept fits: `./gloss propose concept gmi:<Name> --kind process --realizes gm:<Core> --doc "<one sentence>"`. It ships with `proposed: true`; never accept it yourself.
8. If the command realizes a core concept `cmd/.gloss.yaml` does not declare, add it to `realizes:` there (a HARD edit; say so in the summary).

## Test and document it

9. Add one end-to-end test in `internal/e2e/` that runs the command on a copy of an example (`copyTree(t, example(t, "payments/payments-api"), root)`) or a minimal fixture written with `t.TempDir()`. Drive the binary with the `gloss(t, dir, stdin, env, args...)` helper in `internal/e2e/guard_test.go` when exit codes or stdout matter. The test file carries `// @gloss verifies gmi:<Concept> scope=file`.
10. Update the `cmd/gloss` row of the README table, the command block in `docs/QUICKSTART.md`, and `docs/ADVANCED.md` or `docs/FILE-FORMATS.md` if the command reads or writes a file. Add a line under Unreleased in `CHANGELOG.md`.
11. If the command adds something an adopter must set up (a file, a hook, an environment variable), add a check to `internal/doctor/doctor.go` and a case to `internal/doctor/doctor_test.go`.
12. If it emits a new GM code, use the `gm-add-diagnostic` skill.

## Finish

13. `make lint test`, `go build -o gloss ./cmd/gloss && ./gloss check && ./gloss drift`, then `./gloss build -q` and commit `.glossmark/` with the change.
