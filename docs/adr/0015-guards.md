<!-- @gloss documents gm:Relation scope=file -->
# ADR 0015: `guards` says what a cross-cutting mechanism applies to

Status: accepted, 2026-10-20. Closes issue #73 (F-XCUT-2).

## Context

A cross-cutting concern (authentication, rate limiting, feature flags, request logging) is one mechanism that applies to many concepts. GlossMark had no way to say so. The companion lab tried three workarounds:

- `implements .Authentication` on every guarded handler: false (the handler is not the mechanism), 14 extra markers for five handlers, and every handler raised to the guard's criticality through the wrong edge.
- `constrains` from a core constraint: the closest built-in relation, but `gloss impact` listed concepts and not files, and `gloss context` never mentioned it.
- A custom `guards:` declared under `relations:`: validated and written to the graph, then ignored by `impact`, `context` and `drift`.

## Decision

1. `guards` is a built-in ontology relation, concept to concept: a mechanism or constraint guards the concepts it applies to. `applies-to` is an accepted alias, stored as `guards` with `prov.alias`. `BearerAuthentication: { kind: process, realizes: exp:AccessControl, guards: [SubmitClaimHandler, ClaimDecisionHandler] }`. A concept is guarded by what guards it or anything it realizes, so guarding a core process guards every handler realizing it. An ontology that already declares `relations: { guards: {} }` keeps working: the declaration tunes the built-in.
2. **Code under a guard** is every artifact linked to a guarded concept (or to a concept standing for one), except through `verifies`/`tests`, `documents`, `instructs` and `may-modify`.
3. `gloss impact` (and MCP `impact_of`) shows both ends: on a guarded file, `guarded by xapi:BearerAuthentication (high)` under the concept; on the guard's file, `guards ...` and a `guarded code:` line listing the files. `constrains` gets the same file list, as `constrained code:`.
4. `gloss context --dir` adds an `Applies here:` line naming each guard and constraint that applies to a concept linked under the directory, with its level.
5. **A guard sets the review level, never the level.** A link to a guarded concept is gated at the higher of its own level and its most critical guard's, in `gloss diff`, `gloss impact` and every caller of the one link classifier, so `policy.criticality-gates` applies to it. The artifact and the concept keep their own effective criticality in the graph; criticality does not flow through `guards` or `constrains`.
6. A changed decorator stack on guarded code (ADR 0014) is at least SOFT: the decorator removed may be the guard.

## Consequences

- A guarded handler with no level of its own shows `criticality high` in `gloss impact` and in its `gloss diff` change, from the guard, while the graph and `gloss criticality` still report it unclassified.
- `guards` targets are checked like any ontology relation: GM215 for a missing target, the tier rule (GM310) for a core concept guarding an implementation concept.

## Amendment, 2026-10-24: the manifest form, custom surfaces and drift (#202)

The three items this ADR deferred are built.

1. **`guarded-by:` in a manifest** (`guarded-by: [xapi:BearerAuthentication]`) is a `guarded-by` edge from the boundary to the concept. Every artifact under the manifest's directory with a link other than a test, a document or an agent file is guarded code, as if the ontology listed its concept under `guards:`: `gloss impact` shows `guarded by` on the file and lists it under the guard's `guarded code:`, `gloss context --dir` names it on `Applies here:` (for the directory, an enclosing one or one beneath it), and decision 5 applies. It is directory-based like the manifest itself, not `part-of`-based. It is not a declared intent: drift does not compare it with derived edges, and it does not widen `allowed`.
2. **`surfaces:` on a relation declaration** (`relations: { audits: { surfaces: [impact, context] } }`) gives a custom concept-to-concept relation decisions 3 and 4: `impact` shows it on both ends with an `<relation> code:` line, `context` puts it on `Applies here:`. It does not get decision 5: a custom relation never sets a review level. Without `surfaces` a declared relation stays inert. An unknown surface is GM308.
3. **`unguarded` in `gloss drift`.** The manifest form has its own drift semantics. Evidence that a handler is guarded is its decorator stack (ADR 0014); the guard's decorator names are the symbols of the declarations that `implement` the guard or a concept realizing it. Under a `guarded-by` directory, a handler (a symbol-anchored artifact with a guarded link, other than the guard's own implementation) that carries none of those names is `unguarded` when at least one other handler there carries one. When no handler carries one the guard is applied some other way (middleware, a router), and the declaration is taken at its word: no finding. The finding is a warning and not in the default `--fail-on` set.

## Deferred

- Nothing from the first version of this ADR.

## Alternatives considered

- **Reuse `constrains`.** Rejected: a constraint is a rule (a value, a policy), a guard is a mechanism with code and tests; one relation for both hides which is which. Both now get the same impact and context treatment.
- **Let criticality flow through `guards`.** Rejected: it repeats the mistake of the `implements` workaround. The handler is reviewed with the guard because a change can bypass it, not because the handler is the guard.
