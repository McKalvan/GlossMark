<!-- @gloss documents gmi:BackstageExport, gmi:BackstagePlugin scope=file -->
# ADR 0053: a per-boundary document for portals, and the Backstage plugin that reads it

Status: proposed, 2026-11-01; `gloss export --backstage`, `gloss landscape --backstage` and `integrations/backstage` built 2026-11-01 (issue #266). The MkDocs and Docusaurus plugins are not built yet.

## Context

The viewer is a standalone page. Platform teams already keep a developer portal (Backstage) or a docs site (MkDocs, Docusaurus), and a page of its own is one more place nobody opens. What a team wants beside its component's README is the same question `gloss drift` and the concept pane answer, scoped to that component: which concepts it realizes and exchanges, how critical they are, where the code and tests are, what is drifting, who decided what.

The viewer's `views.json` is the whole repository in one file keyed by concept, with no boundary slicing, no criticality on the concepts it lists and no decisions. `gloss export --status` is per concept, not per boundary. A portal plugin that read either would have to redo the comparator's work in TypeScript, badly. The join to the portal's entities is a boundary: a manifest's directory is a Component or System.

## Decision

1. **One self-contained JSON document per boundary, versioned.** `internal/backstage` builds, for every manifest whose boundary is in the graph, a document with the boundary's identity and governance (label, level, owner, criticality and where it is inherited from, part-of, deprecation), its concepts (every compared relation from the boundary node, declared by the manifest or derived from the code beneath it, with kind, tier, criticality, `gloss export --status`'s status, the code under the boundary and the tests anywhere, each link resolved through `source-url`), its coverage (its share of the repository's: the concepts it relates to that are in the coverage denominator of [ADR 0013](0013-data-shapes-and-coverage.md) and have code and a test; the files under it with an accepted link), its open drift findings (on the boundary, on a path under it, or on a concept it relates to when the finding names no boundary) and the last twenty review decisions under it with the gate class each had. `format: 1` rises only when a reader of the previous format would misread a document; a field may be added without a bump. `schemas/backstage-export.schema.json` describes it; the cmd test validates the claims-events export against the schema, so the Go types, the schema and the fixture the plugin test reads cannot drift apart silently.

2. **One index, `landscape.json`, in the same directory.** Every boundary with its document's file name, repository, owner, criticality, concept count, open findings by kind, worst severity and coverage, plus the repositories and the drift metrics. `gloss export --backstage` writes it for one repository; `gloss landscape --backstage dir` writes the same shape over the union of several, with each boundary's repository as the first segment of its path, each link resolved through its own repository's `source-url`, the union's ledger for decisions, and the context map. A reader needs no other file: the index to list, the document to show.

3. **File names are slugs of the boundary CURIE.** Lower case, every run of characters outside `[a-z0-9]` one hyphen, numbered on a collision; the plugin derives the same slug from an annotation, so an entity names its boundary by CURIE and the document is found without a lookup table. The document carries the full CURIE and IRI.

4. **The plugin is a consumer and only that.** `integrations/backstage` is a standard frontend plugin (`createPlugin`, `@backstage/core-plugin-api`, `@backstage/plugin-catalog-react`) that fetches the document named by `glossmark.dev/export-url`, or `glossmark.baseUrl` plus the slug of `glossmark.dev/boundary`, checks `kind` and `format`, and renders it with Backstage's own components. It computes nothing the export did not: grouping by relation, sorting by criticality and severity, and wording coverage are its only logic, isolated in `mapping.ts` and unit-tested against the fixture `gloss export --backstage` wrote. There is no GlossMark backend, no catalog processor and no live call to `gloss`: the documents are built in CI with the graph and published as static files.

5. **The join is by annotation, not by convention, for now.** Deriving the boundary from the entity's `backstage.io/source-location` and the manifest tree, and the reverse direction ([issue #207](https://github.com/McKalvan/GlossMark/issues/207): `owns` from `spec.owner`), are left open until both are wanted; an annotation is explicit and costs one line per entity.

## Consequences

- A docs-site plugin (MkDocs, Docusaurus) reads the same documents; the rendering is the only new part. The glossary page the issue asks for is a projection of the ontology files, which `gloss export --status` and `gloss publish` already serve, and is a follow-up.
- The per-boundary coverage is a share, not a partition: a concept two boundaries realize counts for both, and a core concept's verifier counts wherever it is. The repository's figure is in `landscape.json`'s metrics.
- The documents are as fresh as the last CI run that published them; the tab says when and by which `gloss` they were generated.
- Imported concepts have no status (the comparator reports owned concepts) and appear with their links only.
