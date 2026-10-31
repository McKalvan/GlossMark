<!-- @gloss documents gmi:LinkMapper scope=file -->
# ADR 0045: links from manifests, by path glob or symbol

Status: accepted, 2026-10-28; `links:` in `.gloss.yaml` and `glossmark.yaml`, extraction, review and display built 2026-10-28. `gloss suggest` emitting mappings is not built yet.

## Context

Inline `@gloss` markers are the main cost of adopting GlossMark and the main objection to it ("comments all over our code"). Many links are coarse anyway: a directory implements one mechanism, every test under a path verifies one capability, one model file represents one entity. `guarded-by:` in a manifest ([ADR 0015](0015-guards.md)) already lets a boundary say something about every artifact beneath it without a marker on each.

A link stated in a manifest has to be as good as a marker: anchored to the code it means (so a change there is drift), fingerprinted, carried through the ledger (proposed, accepted, stale, rejected), gated, counted by coverage, and visible as such wherever links are shown, or it becomes a second-class citizen that the comparator and the reviewer each handle differently.

## Decision

1. **A manifest, or `glossmark.yaml`, takes a `links:` list.** Each entry maps a `path` glob (relative to the directory of the file that holds it) or a declaration `symbol` (or both: the symbol in the files the glob matches) to concepts under any relation a marker could carry, with `proposed`, `conf` and `note` as entry keys. Every key that is not one of those five is a relation, as on a concept.

2. **The mapping is resolved to the same artifacts a marker would make.** A `path` entry links every scanned file it matches at file scope, with the file fingerprint of [ADR 0006](0006-file-fingerprint-excludes-markers.md). A `symbol` entry runs the declaration detector over each file beneath the directory and links the declaration of that name with the anchor a marker above it would record: the same symbol, line range, decorator stack and fingerprint ([ADR 0014](0014-decorator-stack-in-fingerprint.md)), qualified on collisions ([ADR 0008](0008-same-named-declarations.md)). The marker id is derived the same way (path, relation, targets, symbol, scope), so a marker rewritten as a mapping keeps its ledger row, its review state and its actor.

3. **Provenance names the manifest.** The edge's `source` is `manifest`; `prov.path` and `prov.line` name the code, as for a marker, and `prov.from` names the entry (`path:line`). Everything that reads links (`gloss check`, drift, the gate, coverage, slices, `gloss context`, MCP, the viewer, `gloss review node`) reads inline and manifest links alike through one predicate, and the places that show where a link is written show `manifest <file:line>`. The ledger row carries `source: manifest`.

4. **A marker overrides a mapping on the same artifact and relation.** Inline markers are extracted first; a mapped link of a relation the artifact already carries from a marker is not added, whatever its target. The mapping is the default for the files that say nothing; a file that says what it implements is believed. Two mappings that produce the same link (overlapping globs) add it once.

5. **Review edits the entry.** `gloss accept` drops `proposed: true` from the entry as it drops the `?` from a marker; `gloss reject --remove` deletes the entry. Both take the entry's `path:line` as a selector, as they take a marker's. The undo journal snapshots manifests for that reason.

6. **Diagnostics sit on the entry.** An entry with neither `path` nor `symbol`, no relation, a relation without a concept, or a `conf` outside [0,1] is GM125; one that matches no scanned file or declaration is GM126; a relation a marker could not carry is GM201, reported at the entry.

## Consequences

- A repository can be described with no comment in its code, at the cost of the link not being visible beside the declaration. The marking guide says when each is the better choice.
- Only scanned files are mapped: a file in a language without a comment style is not in the walk, so a glob does not reach it. Lifting that is a separate change to the loader.
- `gloss suggest` still emits markers. Emitting mappings instead, for a directory whose proposals all share one relation and target, is the natural follow-up and is left open.
- A symbol entry is matched by the detector's name, not by a language server, so it has the detector's reach and no more ([ADR 0029](0029-anchoring-more-languages.md)).
