<!-- @gloss documents gmi:DeclarationAnchor scope=file -->
# ADR 0008: same-named declarations in one file are separate artifacts

Status: accepted, 2026-10-20. Closes issue #61 (F-POOR-4).

## Context

An artifact IRI is `urn:gloss:artifact:<path>#<symbol>`, and the declaration detector named a Python, TypeScript or Java method by its bare name. Go methods were already qualified by their receiver (`Inbound.Process`). Two classes in one file with a method of the same name, the norm in `handlers.py`-style files, produced one artifact, `core/handlers.py#process`, which then `implements` both concepts: the graph, `impact`, the viewer and the export all showed one function doing two things. Both ledger entries also carried the same fingerprint, because the signature lines are identical.

## Decision

After every marker in a file is anchored, symbols that name more than one declaration (different start lines) are disambiguated:

1. Each colliding symbol is qualified by its enclosing classes: Python and Ruby by indentation (a nested function counts too), brace languages by tracking which class, interface, struct, object, namespace or Rust `impl` each open brace belongs to. `InboundHandler.process`, `Outbound.process`, `Outer.Inner.run`.
2. Symbols that still collide, such as Java overloads or a redefinition, take an ordinal in file order: the first keeps the name, the next ones get `#2`, `#3`. `StringUtils.check`, `StringUtils.check#2`.
3. The fingerprint of a renamed anchor hashes the qualified symbol with the signature, so the two ledger entries differ.
4. A symbol that does not collide is left as it was. This keeps every existing IRI, marker id and ledger entry stable for the common case.

## Consequences

- Artifact IRIs change only for declarations that used to collide. Their marker ids change with them (the id hashes the symbol), so the ledger records new entries for those markers and the old shared one is no longer seen; review them like any new link.
- Marking a second same-named method renames the first one's artifact. This is the price of rule 4; qualifying every method always would avoid it but would rename every method artifact in every adopting repository.
- The conformance corpus runs the same step (`handlers.py`, `handlers.ts`, `Overloads.java`); another implementation must agree on the qualified symbols.

## Alternatives considered

- **Always qualify methods by their class.** Consistent with Go, and adding a second method would never rename the first. Rejected for now because it changes the IRI of every marked method in existing graphs and ledgers; it remains the natural step when tree-sitter anchoring lands.
- **Append the line number on collision.** Rejected: line moves would change IRIs, which the marker id is designed not to do.
