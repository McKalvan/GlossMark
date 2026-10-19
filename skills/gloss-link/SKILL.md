---
name: gloss-link
description: Keep @gloss links correct while you change code in a repository that has glossmark.yaml - find the right concept, write the proposal, move or delete markers with their code, add verifies on tests, and report the impact. Use for any feature, fix or refactor in a GlossMark repository, and whenever gloss check or the briefing mentions a marker.
---

<!-- @gloss instructs gm:Vocabulary scope=file -->

# Link as you code

The GlossMark rules in `AGENTS.md` apply throughout. This skill is the order to apply them in. Every link you add is a proposal: `@gloss? <relation> <target> conf=<0-1>`. A human removes the `?`.

## Before you edit

1. Call MCP `context` with `dir` set to the directory you will work in and `changed: true`. Without MCP, run `gloss context --dir <dir> --changed`. Note the boundary, its criticality, your remit and the allowed targets.
2. For each file you will change, call `concepts_for_path`, or run `gloss impact <path>`. If a file is `high` or `critical`, or touches a concept on the published seam, say so to the user now and keep the change minimal.

## While you edit

3. **New public surface** (handler, model, table, test file, doc page): find its concept with `search_concepts`. Prefer, in order: an implementation concept in the boundary's default namespace; a core concept when the artifact is the domain thing itself (`represents pay:Payment`); a new concept as the last resort.
4. Call `propose_link` for the exact line and its gate. Write the line directly above the declaration and its decorators. If the gate is HARD because the target is outside the allowed set, look for a better target before you accept that.
5. Use the starter five: `implements` for code that does the work, `represents` for data shapes, `verifies` in tests, `documents` in docs, `instructs` in agent files. A page that shows a concept `presents` it; the app shell `hosts` it. Add `scope=file` when the whole file is about one concept, and then write no symbol markers in that file.
6. **Changed surface**: if what it does changed, repeat step 3 and retarget the marker. If not, leave the marker alone.
7. **Moved or renamed**: move the marker with the declaration. **Deleted**: delete the marker. A marker must never end up above unrelated code.
8. **Tests**: when a test you write or change checks a concept's behaviour, add `@gloss? verifies <concept>` to it. Never add `verifies` to a test that does not check that concept.
9. **No concept fits**: follow the proposal rule exactly.

   If no concept fits, write `@gloss? <relation> <ns>:NEW:<Name> note="why"`, then run `gloss propose --from-new --doc "what it is"` (MCP: propose_concept) so the concept ships in the same PR with `proposed: true`; a human accepts it with `gloss accept --concept <ns>:<Name>`. Never invent a concept name any other way, and never hand-edit an ontology file to make a link resolve.

10. Mark public surfaces only: one marker per handler, model, table, test file or doc page, never one per helper.

## Before you finish

11. Run `gloss check` and fix every error. Read every warning. GM130 and GM131 mean you marked too much. GM502 and GM503 mean a target is outside the boundary's allowed set. For a code you do not recognise, run `gloss explain <code>`.
12. Run `gloss build -q` and include `.glossmark/graph.jsonl` and `.glossmark/ledger.jsonl` in the change.
13. Run `gloss drift`. A violation or gap your change introduced is yours to resolve: use the `gloss-resolve-drift` skill.
14. Run `gloss impact --base <base branch>`. End your summary with its HARD items, the criticality it reports, the published concepts touched, and every proposal you added.

Never run `gloss accept` or `gloss reject` unless a human asked you to in this session. Never edit `ontology/*.gloss.yaml` except through `gloss propose`.
