---
name: gm-grammar-or-format
description: Change how @gloss markers are scanned (comment styles, quoting, keys, anchoring) or the shape of a file GlossMark writes (.glossmark/graph.jsonl, ledger.jsonl, glossmark.lock) without breaking repositories that already use it - corpus first, the frozen reference regex, and the version constants. Use for changes in internal/marker, corpus/, internal/version, internal/graph serialisation or internal/ledger serialisation.
---

<!-- @gloss instructs gmi:MarkerGrammar, gmi:FormatVersions scope=file -->

# Change the grammar or a file format

Grammar 1 and format 1 are promises to every adopter (`docs/VERSIONING.md`). Read that page before you start.

## Scanner changes

1. Start with the corpus. Add or change a file in `corpus/testdata/` that shows the case, and its entry in `corpus/expected.json`: every marker the scanner must find, with its anchor, and every diagnostic it must raise. Decoys (string literals, docstrings, prose in backticks) belong in the same file.
2. Run `go test ./internal/marker/` and watch the new case fail.
3. Change the scanner: `internal/marker/scan.go` (finding markers), `internal/marker/comments.go` (comment syntax per extension), `internal/marker/anchor.go` and `internal/marker/decls.go` (binding to the next declaration).
4. Never edit `ReferenceRegex` in `internal/marker/grammar.go`; it is the definition of grammar 1. `TestReferenceRegexAgreesOnPlainMarkers` must still pass: on every plain marker, the scanner and the reference regex agree.
5. Decide which kind of change it is:
   - **Recognising more** (a new comment style, a new file extension): a tool MINOR. Note it under Unreleased in `CHANGELOG.md` and in `docs/GRAMMAR.md`.
   - **A new relation or key**: an addition; it ships in a MINOR without a grammar bump. Relations are a modelling change: use the `gm-modelling-change` skill.
   - **Changing how an existing marker parses**: a grammar bump. Stop and open an issue. The bump needs `Grammar` and `SupportedGrammars` in `internal/version/version.go`, a `gloss migrate --grammar N+1` command (it does not exist yet), two further MINOR releases that keep reading N, and a deprecation line in the changelog and in `gloss check`.
6. A new GM0xx code: use the `gm-add-diagnostic` skill.

## File-format changes

7. A new or changed field in `graph.jsonl`, `ledger.jsonl` or `glossmark.lock` bumps its constant in `internal/version/version.go` (`GraphFormat`, `LedgerFormat`, `LockFormat`).
8. For the graph, also change `value:` of `gmi:GraphFormatVersion` in `ontology/gmi.gloss.yaml` to the new number. It is bound to the constant with `value-from`, so `./gloss drift` reports value drift until both agree. This is a HARD edit; say so in the PR.
9. Update `docs/VERSIONING.md` (File formats), `docs/FILE-FORMATS.md` or `docs/GRAPH.md`, and the JSON Schema in `schemas/` (`graph-record.schema.json`, `ledger-entry.schema.json`, `glossmark.schema.json`).
10. Keep the refuse-newer guarantee: a build reads every format up to its own and refuses a newer one by name. `TestGraphFormatGuard` in `internal/e2e/e2e_test.go` pins it for the graph; extend it for the file you changed.
11. Keep output sorted and byte-stable. Rebuild an example twice and diff: `(cd examples/horse-factory && ../../gloss build -q -o /tmp/a.jsonl --no-ledger && ../../gloss build -q -o /tmp/b.jsonl --no-ledger && cmp /tmp/a.jsonl /tmp/b.jsonl)`.

## Finish

12. `make lint test`, then `./gloss check`, `./gloss drift` and `./gloss build -q`; commit `.glossmark/` and, if the change alters it, `examples/horse-factory/.glossmark/`. The changelog entry names the grammar or format number.
