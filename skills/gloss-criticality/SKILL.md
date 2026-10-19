---
name: gloss-criticality
description: Classify how much review each GlossMark concept deserves - run gloss criticality suggest, explain where each suggestion comes from, ask the human which concepts move money or carry compliance risk, and apply only the levels they choose. Use when asked to classify criticality, when a GlossMark repository has nothing classified, or when gloss check reports GM600.
---

<!-- @gloss instructs gm:Criticality scope=file -->

# Classify criticality

Criticality says how much review a concept deserves: `low`, `medium`, `high` or `critical`. A level is a governance decision, so a human chooses every level. You prepare the choice, apply exactly what the human decided, and report what it changes. The GlossMark rules in `AGENTS.md` apply.

| Level | A mistake here |
| --- | --- |
| `low` | is an annoyance: telemetry, dashboards, internal tooling |
| `medium` | degrades the product |
| `high` | costs money, trust or compliance: PII, access control, audit |
| `critical` | moves money, breaks the law or stops the business |

Declare a level on the core concept, where the business sees it. Implementation concepts, boundaries and files inherit it through `realizes`, `part-of` and `is-a`. A page that `presents` a concept and a shell that `hosts` it inherit nothing. The full rules are in https://github.com/mckalvan/glossmark/blob/main/docs/GOVERNANCE.md.

## 1. Draft

1. Run `gloss criticality suggest`. It prints a diff of the ontology files and one line per suggestion on stderr: the concept, the level and the reason. It writes nothing.
2. Each suggestion is `high` and comes from one of three sources. Name the source for each row:
   - **constraint**: the concept is a `constraint`, a rule the business relies on;
   - **published**: the concept crosses the published seam into other repositories;
   - **security-verified**: the concept's only verifiers are security tests (auth, permission, csrf, crypto, pii, token and similar).
3. Concepts that already declare a level, or inherit `high` or above, are left out. Everything else stays unclassified, which is a valid state.
4. When the command reports nothing to suggest, list the core concepts in the core ontology file that carry no `criticality:`, and ask the human whether any of them moves money or carries compliance risk.

## 2. Ask

Show one table: concept, suggested level, source, what implements it (from `concepts_for_path` or `gloss impact`), and your recommendation. Ask the human three questions:

1. Which of these move money, break the law or stop the business, and so should be `critical`?
2. Which do not matter enough for `high`, and should be dropped or lowered?
3. Which concepts not in the table deserve a level?

Wait for the answers. Do not apply a level the human did not choose.

## 3. Apply

1. When the human accepts every suggestion as is, run `gloss criticality suggest --write`.
2. When the human changes some, run `gloss criticality suggest --write`, then change exactly the `criticality:` values the human named in the written ontology files, and nothing else in them. Say in your summary that these ontology edits were the human's decision.
3. Ontology edits are HARD in the PR gate. That is expected for this change.

Never classify on your own initiative, and never use a level to make a gate easier to pass. A new concept follows the proposal rule:

If no concept fits, write `@gloss? <relation> <ns>:NEW:<Name> note="why"`, then run `gloss propose --from-new --doc "what it is"` (MCP: propose_concept) so the concept ships in the same PR with `proposed: true`; a human accepts it with `gloss accept --concept <ns>:<Name>`. Never invent a concept name any other way, and never hand-edit an ontology file to make a link resolve.

## 4. Check

1. Run `gloss check`. GM600 means a boundary's manifest declares a `criticality:` below a concept its code realizes. The fix is to raise the manifest's level or remove it so the boundary inherits. That is a manifest edit: propose it to the human, and make it only when they agree. `gloss explain GM600` prints the details.
2. Run `gloss build -q` and include `.glossmark/graph.jsonl` and `.glossmark/ledger.jsonl` in the change.

## 5. Report the gate changes

The `policy.criticality-gates` map in `glossmark.yaml` raises the gate by level. By default anything that touches a `critical` concept or boundary is HARD. A repository can set, for example, `criticality-gates: { critical: HARD, high: SOFT }`.

1. Read `policy:` in `glossmark.yaml` and state which levels raise which gates.
2. Run `gloss diff --base <base branch>` and report its `Criticality touched` line.
3. List the boundaries and the files whose effective level changed, and say what that means: agent proposals there now reach a human; the pre-edit guard of `gloss hook guard` names the level before each edit; `policy.guard-ask` can make it ask first.
