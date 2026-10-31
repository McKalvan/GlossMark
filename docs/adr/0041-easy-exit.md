<!-- @gloss documents gmi:Stripper scope=file -->
# ADR 0049: leaving is one command, and it removes exactly what the scanner reads

Status: accepted, 2026-10-28; `gloss strip` and [docs/LEAVING.md](../LEAVING.md) built 2026-10-28 (#263).

## Context

GlossMark writes comments into source files, and fear of that stops people trying it. The tool is already easy to back out of (markers are comments, the graph exports to JSON-LD and SQLite, every file it owns is in git), but nothing said so and there was no one-step removal. `gloss suggest --calibrate` had a private stripper that dropped every line containing `@gloss` from a temporary copy, which is right for a measurement and wrong for a tree someone keeps: it would delete a string literal, a line of prose that quotes the marker, and a line of a fenced code block.

## Decision

1. **`gloss strip` is the reverse of `gloss init`.** It writes a final export, removes every marker and every file GlossMark owns, and lists the wiring it cannot remove because it sits inside files that are not GlossMark's (hook, MCP server, skills, CI job, Makefile target, `.gitignore` lines). `--keep-ontology` keeps `ontology/` and `glossmark.yaml`; `--dry-run` prints the plan and writes nothing.

2. **The scanner decides what a marker is.** `marker.Strip` walks a file with the same comment table, block-comment state, backtick quoting and Markdown fences as `marker.Scan`, and removes a marker only where `Scan` would have parsed one. Whatever the extractor would have read is removed, whatever it would have ignored stays, and a comment style added under `comment-styles:` is stripped like a builtin one. `exclude:` and `generated:` are honoured because the plan is built over the workspace's own file list.

3. **A comment that held only the marker goes whole; otherwise only the marker text goes.** The whole line goes when nothing else is on it, including a line inside a multi-line block comment (the block stays well-formed); a trailing comment after code goes and the code stays; a comment with other text, and the opening or closing line of a block comment, lose the marker text and keep their shape, since deleting the line would unbalance the comment. The result reads as `git diff` of marker lines removed, with the one rewritten line per trailing marker.

4. **The export is one directory, in three shapes.** `glossmark-export/` holds the graph as JSON Lines with its context (what `gloss build` writes), as one JSON-LD document (`{"@context": ..., "@graph": [...]}`, since a JSON Lines file is not a JSON-LD document and the acceptance test is a standard processor) and as SQLite with the closure views; the ledger as it is and as PROV-O; and copies of `glossmark.yaml`, the ontology files and every manifest. It is written before anything is removed, and a failure there removes nothing.

5. **A sidecar is GlossMark's file.** `foo.json.gloss` holds nothing but markers and comments about them, so it is deleted rather than emptied; the JSON file it annotates is untouched.

6. **The `gloss:begin`/`gloss:end` block goes.** `gloss agents --write` generated it, so it is GlossMark content in a file that is not; strip removes the lines from the begin marker to the end marker when the begin marker stands on a line of its own, and leaves prose that quotes the markers.

## Consequences

- `docs/LEAVING.md` says what GlossMark adds, how to try it on a branch, and how to leave; the README and the Quickstart link it.
- The one-line-per-marker rule of `suggest --calibrate` is unchanged: it measures on a copy and keeps the sidecars, which strip deletes.
- A marker inside a multi-line block whose other lines are only `/*` and `*/` leaves an empty comment behind. That is two lines of noise, not a change of meaning, and the alternative (deleting lines the scanner did not read) breaks the rule in decision 2.
