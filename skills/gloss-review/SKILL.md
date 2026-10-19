---
name: gloss-review
description: Help a human review GlossMark proposals - proposed links, proposed concepts and stale links - by preparing one decision table and then running exactly the accepts and rejects the human chooses. Use only when a human asks to review, triage or accept GlossMark proposals.
---

<!-- @gloss instructs gm:Review scope=file -->

# Review proposals with a human

Accepting is a human decision. You prepare it and carry it out; you never make it. The GlossMark rules in `AGENTS.md` apply, and this skill is the one case where you run `gloss accept` and `gloss reject`: only for the rows a human decided, in this session.

## 1. Gather

1. Run `gloss status` for every proposed, stale and rejected link with its id. For scripting, read `.glossmark/ledger.jsonl`: one JSON entry per link, with `marker_id`, `path`, `status` and `targets`.
2. Run `gloss diff --base <base branch>` for the gate of each change: AUTO, SOFT or HARD, with its reasons and criticality.
3. Run `gloss drift` for the proposed concepts (`proposed-concept`) and `ns:NEW:Name` markers (`rogue`) still waiting.

## 2. Judge each row

- **Proposed link**: open its `path:line`. Check the relation (the starter five: `implements`, `represents`, `verifies`, `documents`, `instructs`), the target, and that it sits on a public surface rather than a helper. The marking guide is https://github.com/mckalvan/glossmark/blob/main/docs/MARKING.md.
- **Proposed concept**: apply the litmus test. A domain expert would nod at the term: core. Only an engineer would: implementation. Check its `realizes:` and its `doc:`, and check with `search_concepts` that it duplicates nothing.
- **Stale link**: read what changed in the declaration and say whether the link still holds.
- **Rogue marker** (`ns:NEW:Name`): it is not reviewable yet. Proposals reach review through the proposal rule:

  If no concept fits, write `@gloss? <relation> <ns>:NEW:<Name> note="why"`, then run `gloss propose --from-new --doc "what it is"` (MCP: propose_concept) so the concept ships in the same PR with `proposed: true`; a human accepts it with `gloss accept --concept <ns>:<Name>`. Never invent a concept name any other way, and never hand-edit an ontology file to make a link resolve.

## 3. Show one table

One row per item: id, `path:line`, link, gate, criticality, your recommendation (accept, reject with a reason, or retarget to a named concept), and a one-line reason. Put HARD and `critical` rows first. Say how many rows there are in total.

## 4. Wait

Wait for the human's decisions. Do not act on a row they did not decide. A recommendation is not a decision.

## 5. Carry out only what they decided

Record the human as the reviewer by setting `GLOSS_ACTOR` to them for every command:

```sh
GLOSS_ACTOR=human:<their name> gloss accept <id>                     # or path:line; a stale link is re-accepted the same way
GLOSS_ACTOR=human:<their name> gloss accept --concept <ns:Name>
GLOSS_ACTOR=human:<their name> gloss reject <id> --reason "..."      # --remove also deletes the marker line
GLOSS_ACTOR=human:<their name> gloss reject --concept <ns:Name> --reason "..." --remove
GLOSS_ACTOR=human:<their name> gloss accept --value <ns:Name>        # confirm a drifted published value as is
```

Use `gloss accept --policy` only when the human says to accept every SOFT proposal. Use `gloss accept -n` first to show what a command would accept. Never use `gloss accept --all` unless the human names it. A retarget is a new proposal: edit the marker, keep the `?`, and leave it for the next review.

## 6. Report

Run `gloss build -q` and `gloss status`. Include `.glossmark/graph.jsonl` and `.glossmark/ledger.jsonl` in the change. Report what was accepted, what was rejected and why, and what is left.
