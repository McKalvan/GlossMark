<!-- @gloss documents gmi:DeclarationAnchor scope=file -->
# ADR 0006: A file fingerprint leaves out the markers, and accept re-confirms a stale link

Status: accepted, 2026-10-20. Closes issue #42 (F-ADOPT-8).

## Context

A `scope=file` anchor (and a sidecar `scope=range` anchor, which falls back to the file) was fingerprinted over every line of the file, its own marker included. `gloss accept` rewrites `@gloss?` to `@gloss` in that file, so every accepted file-scope proposal was stale the moment it was accepted. Adding a second marker to a file, or regenerating the `gloss:begin`…`gloss:end` block that `gloss agents --write` maintains, staled every file link in it too. The stale message then said "re-confirm with `gloss accept <id>`", and `gloss accept <id>` refused anything not proposed, so there was no way back short of editing the ledger by hand.

## Decision

1. A file fingerprint hashes every line except metadata about the file: lines that carry `@gloss`, and the lines between `gloss:begin` and `gloss:end` (the markers themselves included). Symbol, block and line fingerprints are unchanged; they never covered the marker line.
2. `gloss accept <id>` on a stale link re-confirms it: the current fingerprint becomes the baseline and the status goes back to `accepted`, reviewed by the caller.
3. Migration: an accepted ledger entry whose fingerprint equals the file hashed the old way is moved to the new fingerprint on the next build instead of going stale. Links that were already stale stay stale until re-confirmed.

## Consequences

- An edit to the code or prose of a file still stales its file-scope links; a line of prose that mentions `@gloss` no longer counts as content for the fingerprint.
- The first `gloss diff` across the upgrade shows AUTO `fingerprint-changed` entries for file-scope links, because the graph stores the new fingerprints.

## Alternatives considered

- **Exclude only the line of the marker being fingerprinted.** Rejected: adding a second marker to the file would still stale the first.
- **Fingerprint after accept rewrites the file.** Rejected: fixes `gloss accept` but not a hand edit of the `?`, nor a new marker or a regenerated block.
