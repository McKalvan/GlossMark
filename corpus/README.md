<!-- @gloss documents gmi:MarkerGrammar scope=file -->
# Conformance corpus

One file per supported comment style, each carrying `@gloss` markers in the
positions the grammar must recognise and decoys (string literals, docstrings,
prose) it must ignore. `expected.json` lists, per file, every marker the
reference implementation must find, with its anchor, and the diagnostics the
malformed file must raise.

Any implementation of the grammar in another language should pass this corpus:
parse every file, produce the same `(line, proposed, relation, targets, keys)`
tuples and no others. Anchors (`scope`, `symbol`) are the expected output of
the pass-1 declaration detector; a tree-sitter or SCIP-backed implementation
may produce richer symbols but must agree on scope.

Run `go test ./internal/marker/` to execute it.
