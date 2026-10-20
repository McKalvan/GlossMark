<!-- @gloss documents gm:Boundary scope=file -->
# ADR 0007: retiring boundaries, and manifests that survive an unknown key

Status: accepted, 2026-10-20. Closes issue #99 (F-SPLIT-6).

## Context

Concepts have `deprecated` and `replaced-by`; boundaries had nothing. During a monolith decomposition the obvious thing to write on the retiring module's manifest is `deprecated: true`, `replaced-by:` and a date. Each was an unknown key, and the manifest loader decoded strictly: one unknown key discarded the whole manifest. Its files then fell through to the parent boundary, `gloss check` reported their markers as outside the parent's allowed set (GM503) and `gloss drift` reported violations on the parent. One mistake in one file became a failing build that blamed another boundary, and the GM110 message printed an absolute path at line 1.

## Decision

1. **Lenient manifest decoding.** An unknown top-level key is GM110 (error) at the key's own line, with the workspace-relative path. The known keys still apply, so the boundary exists and keeps its files. Errors that leave no usable boundary (YAML syntax, no `boundary:`, an invalid `level`, `criticality` or `retire-by`) still drop the manifest.
2. **Three manifest keys for a boundary being retired:**
   - `deprecated: true`
   - `replaced-by: <boundary CURIE>`: the boundary that takes over. It is not required to exist in this workspace, because the replacement is usually a new repository.
   - `retire-by: YYYY-MM-DD`: when it is due to go.
3. They are carried on the boundary node in `graph.jsonl` (`deprecated`, `replaced-by`, `retire-by`) and in the export. A link into a deprecated boundary gets the same treatment as a link to a deprecated concept: GM222 in `check`, HARD in the gate.

## Not decided here

Drift does not yet treat a retiring boundary differently: its gaps and violations are reported as usual, and `retire-by` passing raises nothing. Those rules need the decomposition semantics (what moves where, and when a gap becomes expected) and can be added without a format change.

## Alternatives considered

- **Keep strict decoding and improve the message.** Rejected: the cascade onto the parent boundary is the actual harm, and a better message does not stop it.
- **Ignore unknown keys silently.** Rejected: a misspelt `realises:` would quietly declare nothing.
