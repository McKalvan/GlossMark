<!-- @gloss documents gm:Concept scope=file -->
# ADR 0050: a concept points at its fuller documentation with `see-also`

Status: accepted, 2026-11-08.

## Context

A concept's `doc` is one sentence: what it is and why it is needed. That is the right size for the viewer pane, `gloss context`, the AGENTS.md snippet and an MCP answer, and the gate already treats rewriting it as a decision ([ADR 0018](0018-gate-classification.md)). It is the wrong size for a rule with exceptions, a lifecycle, a regulatory basis or a worked example, and that material usually exists already: a Markdown page in the repository, a wiki or Confluence page, a policy document.

Prose in the repository can already point at a concept. `@gloss documents lib:Hold` on a Markdown heading or page is a starter relation, `gloss suggest` proposes it on headings and on AsyncAPI and OpenAPI items ([ADR 0049](0049-links-inferred-from-specs.md)), a manifest's `links:` maps a file that cannot carry a marker ([ADR 0045](0045-manifest-links.md)), and MCP `lookup_concept` lists every such link among the concept's artifacts. The link is fingerprinted, so a rewritten section goes stale and is reviewed again.

Three cases have no answer:

1. **Documentation outside the repository.** A wiki page cannot carry a marker.
2. **Importers.** A core namespace is read by other repositories. The `documents` markers in the owner's Markdown are not in the importer's graph, so an agent working in the importer sees the one-sentence doc and nothing more.
3. **The canonical page.** Several files may document a concept. Nothing says which one its owner treats as the reference.

All three need the pointer to run the other way: from the concept, in the ontology file, where it travels with the namespace.

## Decision

1. **`see-also` on a concept.** An optional list beside `doc`:

   ```yaml
   Hold:
     kind: entity
     aliases: [reservation]
     doc: "A patron's place in line for an item."
     see-also: [https://wiki.acme.org/circulation/holds, docs/holds.md#queue-rules]
   ```

   An entry is an absolute `http`/`https` URL or a path relative to the repository root, as `value-from: { path: }` is, with an optional `#anchor`. Order is the owner's: the first entry is the canonical page. `see-also` extends `doc` and never replaces it, so every surface that shows one sentence keeps working.
2. **GlossMark never fetches an entry.** It is carried on the graph node (`seeAlso`, a list of strings, `rdfs:seeAlso` in the JSON-LD context beside `doc` as `rdfs:comment`) and shown as a link: in MCP `lookup_concept` and `search_concepts` (`see_also`), in `gloss context`, in the viewer's concept pane and in the AGENTS.md snippet, which tells agents the entries are optional further reading they may open with their own tools. For a relative entry the viewer turns it into a link through `source-url` for a namespace this repository owns, and prints it as a path otherwise. The tool stays offline and deterministic: the same tree gives the same output, as tool semver requires, no credentials are needed for private hosts, and no outside text reaches an agent with the ontology's authority.
3. **`gloss check` validates what it can without the network.** An entry that is neither an absolute `http`/`https` URL nor a relative path is an error (GM309). A relative entry whose file is missing, or whose `#anchor` names no heading in a Markdown file, is a warning (GM313): a broken link is a defect in the documentation, not in the model. URLs are not checked.
4. **Gated as `doc` is.** Adding, removing, reordering or changing an entry joins the `label/doc` difference in `gloss diff`: a PATCH bump, SOFT on an implementation concept, HARD on a core concept and on a published one. If `see-also` names the fuller definition, pointing it elsewhere redefines the concept as surely as rewriting `doc`, and a lighter gate would be a way round the review of definitions.
5. **`documents` stays the link for prose in the repository.** It is fingerprinted and reviewed when the prose changes; a `see-also` entry is not. A page the owner names in `see-also` may also carry `@gloss documents`, and should when it is in the same repository.

## Consequences

- A core namespace carries its documentation to importers. An owner who wants importers to reach a page writes an absolute URL: a relative entry names a path in the owner's repository, and the importer's `source-url` points at its own, so the viewer there shows a path, not a link, and `gloss check` there does not look for the file.
- Reading what an entry points at is the agent's job. Fetching, authentication to private hosts, trimming to the anchor and caching are left to the agent's own tools, or to a separate project outside core GlossMark that reads `see_also` from the graph or the MCP answer. Revisit only if agents demonstrably miss context because they cannot fetch.
- A tool release without the key reads `see-also` as an undeclared relation (GM200), so a namespace that uses it needs importers on the release that added it. The key is a file field addition: a tool MINOR, no grammar bump.
- Not done here: `see-also` on a namespace, a boundary manifest or a declared relation, and staleness for URLs.
