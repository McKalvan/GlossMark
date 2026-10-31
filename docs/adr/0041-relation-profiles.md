<!-- @gloss documents gm:Relation scope=file -->
# ADR 0041: a starter relation profile, the rest behind a switch

Status: accepted, 2026-10-28 (issue #261).

## Context

The vocabulary has about twenty relations and eight concept kinds. A repository on its first day needs five or six of them: what code does (`implements`), what data it is (`represents`), what tests check (`verifies`), which docs describe it (`documents`), what flows through it (`produces`, `consumes`) and how boundaries nest (`part-of`), plus `is-a` to shape the ontology. The rest (`presents`, `hosts`, `configures`, `provisions`, `deploys`, `instructs`, `may-modify`, `publishes-value`, `publishes`, `subscribes`, `holds`, `constrains`, `guards`, `guarded-by`, `has-part`) each answer a question a repository asks later, if at all. The AGENTS.md snippet and the MCP server listed every inline relation to every agent, and the docs put them all on the table at once.

## Decision

1. `glossmark.yaml` takes `relations: starter | full`. The starter profile is `implements`, `represents`, `verifies`, `documents`, `produces`, `consumes`, `part-of` and, in ontology files, `is-a`. The keys the tier seam itself asks for (`realizes`, which GM320 requires on an implementation concept; `replaced-by`, which GM340 asks of a deprecated one; `published-as`) belong to every profile: a profile that hinted against what another check demands would contradict itself. The same goes for `owns`, `same-as` and `derived-from`, which the tool writes from CODEOWNERS, Backstage or an import rather than anyone choosing them.
2. `gloss init` writes `relations: starter`. An unset key means `full`, so a repository written before the key existed reads exactly as before.
3. The profile never changes the graph. Under `starter`, an advanced relation still builds its edge; `gloss check` adds a hint (GM250, a third severity below warning) that names the relation, the starter set and the switch. `--strict` fails on warnings, not on hints, and `gloss build`, `gloss drift` and the gate do not read the profile at all. Switching an existing repository between the two profiles therefore leaves `.glossmark/graph.jsonl` byte for byte the same.
4. The agent surfaces show the active profile only: the AGENTS.md snippet lists the starter inline relations and say where the switch is; the MCP server's `initialize` instructions carry the same sentence and `propose_link`'s `relation` description lists the starter relations, and a proposal that uses an advanced one carries a `hint`. Under `full` every surface reads as it did.

## Consequences

- A new repository meets five inline relations, and an agent reading its snippet cannot pick `hosts` for a handler because it never sees it; the hint tells a human who does write one where the switch is.
- The profile is documentation and lint, not semantics: a tool that reads the graph needs no change, and a repository that outgrows the starter set flips one key with no rebuild.
- The starter set is a list in `internal/model` (`StarterRelations`); moving a relation across the line is a one-line change and a docs change, not a format change.
