---
name: gloss-resolve-drift
description: Resolve a gloss drift finding - violation, gap, value-drift, dangling, rogue, proposed-concept, stale, untested, orphan or presented-only - by fixing the code or the marker, or by preparing the governance change for a human. Use when gloss drift, gloss check or the GlossMark CI job fails or reports findings.
---

<!-- @gloss instructs gm:DriftComparison scope=file -->

# Resolve drift

Run `gloss drift` and handle each finding by kind. The GlossMark rules in `AGENTS.md` apply: you propose, a human accepts, and ontology files change only through `gloss propose`. `gloss drift --format json` gives the same findings with paths, and `gloss check` gives the diagnostics behind them; `gloss explain <code>` prints the meaning and fix of any GM code.

| Finding | What it means | Fix it yourself | Leave for a human |
| --- | --- | --- | --- |
| **violation** | Code under a boundary realizes, holds, produces or publishes something its manifest does not declare | The marker is wrong: retarget it. Or the code is in the wrong boundary: say so | The boundary really does this now: add it to the manifest's list and call out the HARD edit |
| **gap** | The manifest declares something nothing under it implements | The implementing code exists but is unmarked: add `@gloss? implements` | The declaration is aspirational: propose removing it, HARD |
| **value-drift** | Code bound by `value-from` or `publishes-value` changed and the ontology `value:` did not | The code change was a mistake: revert it | The change is intended: update `value:` in the same PR and say it is a published value, or a human confirms the code as is with `gloss accept --value <ns:Name>` |
| **dangling** | A marker names a concept that does not exist (GM215) | A typo: find the real one with `search_concepts` | Nothing fits: rewrite it as `ns:NEW:Name` and follow the proposal rule below |
| **rogue** | A `ns:NEW:Name` marker | Run `gloss propose --from-new --doc "..."` | The decision to accept the concept |
| **proposed-concept** | A concept with `proposed: true` waits for review | Nothing | `gloss accept --concept <ns:Name>` or `gloss reject --concept <ns:Name> --reason "..."` |
| **stale** | The declaration under an accepted marker changed | The link no longer holds: retarget or remove the marker | The link still holds: list it for re-acceptance with `gloss accept <id>` |
| **untested** | Implemented, nothing verifies it | A test already checks it: add `@gloss? verifies`. None does: write one if the task allows | Whether a test is worth writing |
| **presented-only** | A page presents it and nothing implements it | The mechanism exists unmarked: mark it | The mechanism is missing |
| **orphan** | No artifact links to the concept | Nothing, unless your task implements it | Whether the concept should exist |

The proposal rule, verbatim from `AGENTS.md`:

If no concept fits, write `@gloss? <relation> <ns>:NEW:<Name> note="why"`, then run `gloss propose --from-new --doc "what it is"` (MCP: propose_concept) so the concept ships in the same PR with `proposed: true`; a human accepts it with `gloss accept --concept <ns>:<Name>`. Never invent a concept name any other way, and never hand-edit an ontology file to make a link resolve.

## Order of work

1. Fix errors from `gloss check` first. Drift is computed from the graph, and a broken marker hides what it would derive.
2. Handle violations, gaps, value-drift and dangling markers next. They fail `gloss drift` and CI by default, and a published value is HARD in the PR gate.
3. Handle rogue markers, then the stale, untested, presented-only and orphan findings that your change caused.
4. Run `gloss build -q` and include `.glossmark/graph.jsonl` and `.glossmark/ledger.jsonl` in the change.

## Never

- Never make a finding disappear by deleting a correct marker, widening `allowed:`, or marking unrelated code.
- Never edit `ontology/*.gloss.yaml` by hand. Never edit a `.gloss.yaml` manifest unless that edit is the fix, and then say so in your summary.
- Never run `gloss accept` or `gloss reject` unless a human asked you to in this session.

## Report

Re-run `gloss drift` when you are done. List each finding: what you changed, what you left for a human, and why.
