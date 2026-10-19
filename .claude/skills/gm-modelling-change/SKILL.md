---
name: gm-modelling-change
description: Change GlossMark's metamodel - add or change a relation, a lifting or rollup rule, a drift finding kind, a concept kind or a manifest key - with an ADR grounded in adopter evidence, the vocabulary and schemas, the rollup lift, the ADVANCED.md tables and an end-to-end test on an adopter fixture. Use for changes in internal/model/vocab.go, internal/rollup or schemas/.
---

<!-- @gloss instructs gm:Relation, gmi:RollupEngine scope=file -->

# Change the model

A modelling change alters what every adopter's graph means. It needs a written decision before it needs code.

1. **Write the ADR first.** Copy the shape of `docs/adr/0001-presents-and-hosts.md` into `docs/adr/NNNN-<slug>.md`: Status, Context with a table of what the adopters actually wrote (cite `docs/reviews/2026-10-18-adoption-review.md` or the adopter branches, never invented cases), Decision as numbered rules, Consequences including what changes in existing graphs. Mark it `<!-- @gloss documents gm:Relation scope=file -->` or the concept it is about. Leave Status as `proposed` for the maintainer.
2. **Vocabulary.** Relations live in `BuiltinRelations` in `internal/model/vocab.go`: name, domain, range, where it may be written (`Inline`, `Ontology`, `Manifest`), whether it is `Derived`, and a one-line `Doc`. Concept kinds are `ConceptKinds`; manifest keys are on the manifest type in `internal/model/files.go`.
3. **Schemas.** Add the relation or key to `schemas/ontology.schema.json`, `schemas/manifest.schema.json` and `schemas/graph.linkml.yaml` as it applies.
4. **Rollup and drift.** Lifting rules are in `internal/rollup/derive.go`; what `gloss drift` compares is `ComparedRelations` and the finding kinds in `internal/rollup/compare.go`. A new finding kind is a MINOR change; renaming one is MAJOR.
5. **Lint.** Domain, range and placement errors come from `internal/check/check.go`. New codes: use the `gm-add-diagnostic` skill.
6. **Docs.** Update the tables in `docs/ADVANCED.md` (Every relation, Finding kinds), `docs/FILE-FORMATS.md` for a new key, and `docs/MARKING.md` if it changes what an adopter should mark.
7. **Test on an adopter fixture.** Add one end-to-end test in `internal/e2e/` on `examples/horse-factory` or `examples/payments`, or on a minimal fixture that reproduces the adopter case, in the style of `TestPresentsAndHosts` and `TestRealizesInheritance`. Assert the lifted edges and the drift findings, not only the absence of errors.
8. **Use it on GlossMark itself** if it fits: the root `.gloss.yaml`, `cmd/.gloss.yaml`, `internal/.gloss.yaml` and `ontology/gmi.gloss.yaml` are the self-description. `TestSelfHost` must stay clean.
9. **Measure the effect** on real adoptions with the `gm-adoption-regression` skill, and record the result in the ADR's Consequences.
10. **Finish:** `make lint test`, `./gloss check`, `./gloss drift`, `./gloss build -q`, the CI example commands from AGENTS.md, and a `CHANGELOG.md` line under Unreleased that names the ADR.
