<!-- @gloss documents gm:Relation scope=file -->
# ADR 0020: `replaced-by` is an edge, and a successor needs no reverse import

Status: accepted, 2026-10-20. Closes issues #100 (F-SPLIT-7) and #101 (F-SPLIT-8).

## Context

- `replaced-by` was a node attribute. A migration view could not select it (`relations: [replaced-by]` matched nothing), view nodes and the concept pane did not say what was deprecated, and an unknown key in `views.yaml` aborted every command.
- When the successor lived in the new service's repository, `replaced-by: inv:InvoiceRun` in the monolith failed GM341 unless the monolith imported the service's namespace. That import points the wrong way (old depends on new only to name its successor) and closes an import cycle, while GM222 on the same concept already told callers to use the unresolvable name.

## Decision

1. **An edge.** The `replaced-by:` key on a concept and on a manifest is also emitted as a declared `replaced-by` edge (concept → concept, boundary → boundary), a builtin relation allowed in ontology files and manifests. The node attribute stays. Views select and draw it like any relation; projections carry `deprecated`, `replacedBy` and, for boundaries, `retireBy` on nodes; the viewer draws deprecated nodes dotted and shows the successor and what a concept replaces in the pane. A deprecation change counts as a changed view node.
2. **No reverse import.** A concept's `replaced-by` may be a CURIE whose prefix is only under `prefixes:`, or a full IRI (shown as a CURIE when a prefix matches). `gloss check` reports GM341 when the successor's namespace is owned or imported and the concept is missing, or when the reference cannot be resolved at all; otherwise the check is deferred. `gloss drift` does the same for its dangling finding, and `gloss landscape`, which loads both repositories, reports a dangling successor. A boundary's successor is never required to exist.
3. **GM222** names the replacement as "use X" only when X resolves; otherwise it says the replacement is unresolved here.
4. **The seam still applies** when the successor is loaded: naming an unpublished concept of an imported implementation namespace is GM230, as for any reference.
5. **`views.yaml`** decodes leniently: an unknown key is GM117 (error) at its line, and the rest of the file applies.

`views.yaml` selection by property (`where:`) is not added: the edge and the node flags make a migration view expressible with `include:` and `relations:`.

## Alternatives considered

- **Keep `replaced-by` an attribute and teach views a special case.** Rejected: the graph is what every consumer reads (export, MCP, landscape); an edge reaches all of them.
- **Allow the reverse import but exempt it from cycle checks.** Rejected: the importer would still need the successor's ontology on disk and in its lock for one name.
