<!-- @gloss documents gmi:ReviewLedger scope=file -->
# ADR 0027: One proposal keeps one ledger row, and the review trail exports its agents

Status: accepted, 2026-10-21. Follows ADR 0005. Closes issue #158.

## Context

Adoption in an agent-heavy repository, where three subagents propose links and a reviewer accepts or rejects them, found the review trail hard to audit:

- A marker id is derived from the marker's path, relation, targets and anchor. `gloss propose --from-new` rewrites `pepper:NEW:MenuAvailability` to `pepper:MenuAvailability`, so the id changes: the ledger recorded the old row as `removed` and started a new `proposed` row. `gloss reject --concept --remove` rewrites the markers back to `ns:NEW:Name` and split the history again. A `removed` row then meant either "someone deleted the marker" or "a review command rewrote it", so the ledger could not tell a decision from a deletion.
- `gloss status` printed `[HARD]` with no reason, and a gate on stale and rejected rows, where none applies.
- `gloss export --prov` had no `prov:Agent` nodes, so `prov:wasAssociatedWith` pointed at nothing, and it left out the author's rationale.
- The SessionStart hook set `GLOSS_ACTOR` for its own `gloss context` command only, so a `gloss build` the agent ran later fell back to the git user.

## Decision

1. **A review command that rewrites a marker carries its row.** After `propose --from-new`, `propose concept` and the MCP `propose_concept` rewrite `ns:NEW:Name`, and after `reject --concept --remove` rewrites it back, the command matches each marker before and after by path, line and column (a rewrite changes no position) and moves the ledger row to the new id. The row keeps its actor, `created`, status, rationale and review fields, and lists the ids it had in a new field, `previous_ids`. No `removed` row is left. A row already at the new id that is `removed`, which an older release left behind, is replaced and its `previous_ids` are carried over. A live row there is kept, and nothing moves.

2. **`removed` means the marker disappeared without a decision.** A rejection stays `rejected` when the line is deleted, by hand or with `gloss reject --remove` (ADR 0005). A rewrite moves the row (1). `removed` is left for a marker deleted outside the review commands.

3. **`gloss status` prints the reason `gloss diff` prints.** A proposal shows `[SOFT: auto-accept eligible]`, or `[HARD: <reasons>]` with the classifier's reasons, for example `outside allowed; conf 0.55 below threshold 0.70`. Stale, drifted and rejected rows get no gate: a stale link was accepted and a rejected one was decided, and neither waits on a gate.

4. **The PROV export names its agents.** Every author and reviewer becomes a `prov:Agent` node (`urn:gloss:actor:<actor>`), also typed `prov:Person` for `human:` and `prov:SoftwareAgent` for `agent:`, with `rdfs:label` and, when the actor names one, `gm:model`. Each activity adds `gm:rationale` (the author's `note=`), `gm:reviewedAt`, `gm:relation`, `gm:target`, `gm:path` and `gm:previousId` beside `gm:reviewReason`. The JSON-LD context types `prov:wasAssociatedWith`, `prov:generated`, `gm:reviewedBy` and `gm:previousId` as IRIs, so an RDF load links each activity to its agents. `gm:reviewedBy` is now the reviewer's IRI rather than the bare actor string.

5. **The hook persists the actor for the session.** The SessionStart hook appends `export GLOSS_ACTOR=<actor>` to `$CLAUDE_ENV_FILE`, which Claude Code sources before every shell command of the session (`/dev/null` outside Claude Code). `gloss doctor` warns about a hook that sets the actor for the briefing only. Subagents share the session's environment and its MCP server, so each sets its own actor on its gloss commands, written into its agent file (docs/QUICKSTART.md).

The ledger changes are additive: `previous_ids` is optional, the ledger `format` stays 1, and existing `removed` rows still read as before.

## Alternatives considered

- **Record the new id on the old row** (`replaced_by`) and leave the old row in place. Rejected: the proposal's history would still be split across two rows, and every reader of the ledger (status, PROV, suggest's rejected-link check) would have to follow the pointer. Moving the row keeps one row per proposal, and `previous_ids` still answers "what was this called before".
- **Keep the id stable by writing `id=<old id>` into the rewritten marker.** Rejected: it adds a key to every lifted marker in the source that exists only to serve the ledger, and the next `--from-new` rewrite would carry it forever.
- **Derive a subagent's actor from the agent file that issued the edit.** Rejected for now: neither the hook nor the MCP server learns which subagent made a tool call, so there is nothing to derive it from. Documented instead (5).
