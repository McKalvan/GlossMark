---
name: gm-add-diagnostic
description: Add, change or remove a GM diagnostic code - pick the next free code in the right range, emit it as a full literal, add its row to internal/diag, regenerate docs/DIAGNOSTICS.md, and test it on a minimal fixture. Use whenever gloss check, gloss build or the loader gains, loses or rewords a GMnnn code.
---

<!-- @gloss instructs gmi:Diagnostics scope=file -->

# Add a diagnostic

Every code `gloss` emits has exactly one row in `internal/diag/diag.go`, and `docs/DIAGNOSTICS.md` is generated from those rows. `internal/diag/diag_test.go` enforces both.

## Pick the code

1. Find the range in `diag.Ranges` (`internal/diag/diag.go`) and the package that emits it:

   | Range | What | Emitted in |
   | --- | --- | --- |
   | GM0xx | Marker grammar | `internal/marker` |
   | GM1xx | Workspace, imports and targets: loading (GM100–GM112), target resolution (GM120–GM122), redundant markers (GM130–GM131) | `internal/load`, `internal/extract`, `internal/check` |
   | GM2xx | Relations and references | `internal/check` |
   | GM3xx | Ontology entries | `internal/check`, `internal/extract` (GM360) |
   | GM4xx | Own-subject rule | `internal/check` |
   | GM5xx | Manifests and allowed lists | `internal/check` |
   | GM6xx | Governance | `internal/check` |
   | GM9xx | Gate | `internal/gate` |

2. Take the next free number after the last row of that sub-family: `./gloss explain GM3xx` lists a range. Never reuse a removed code.

## Emit it

3. Write the code as a full string literal on the emitting line, with the severity on the same line when it is fixed: `c.add(path, line, "warning", "GM6nn", fmt.Sprintf(...))` in `internal/check/check.go`. `TestEveryEmittedCodeHasARow` rejects a code built from a fragment such as `"GM3" + n`, and `TestEveryRowIsEmitted` compares the row's severity with the emitting line.
4. Write the message for whoever hits it, human or agent: what is wrong and where, in one line.

## Document it

5. Add the row to `diag.Codes` in code order: code, severity (`"error"`, `"warning"` or `"error or warning"`), meaning, an actionable fix, and the doc page with an anchor that exists (`GRAMMAR.md#rules`). `TestRows` checks the anchor against the page's headings.
6. Regenerate the page from the repository root, never edit it by hand:
   ```sh
   go run ./cmd/gloss explain --all --format markdown > docs/DIAGNOSTICS.md
   ```
   `TestDiagnosticsDocIsGenerated` fails until you do.
7. If the fix edits an ontology file or a manifest, say in the fix that the edit is HARD in the PR gate.

## Test it

8. Add an end-to-end test in `internal/e2e/` that writes the smallest fixture that triggers the code (see `TestOntologyFileLint` and `TestOwnSubjectRule` in `internal/e2e/e2e_test.go`), runs `check.Run`, and asserts the code, and that a corrected fixture does not raise it.
9. Say in the commit whether the gate changes. A `gloss check` error fails CI but does not change `gloss diff`'s AUTO/SOFT/HARD classification; only `internal/gate` (GM900) does.

## Finish

10. `make lint test`, `go build -o gloss ./cmd/gloss && ./gloss check` (0 errors, 0 warnings: a new warning that fires on this repository fails `TestSelfHost`), `./gloss explain <code>`, then `./gloss build -q` and commit `.glossmark/`. Add a line under Unreleased in `CHANGELOG.md`: a new code is MINOR, a removed or renumbered one MAJOR.
