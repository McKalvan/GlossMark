---
name: review-glossmark
description: Work a GlossMark repository's review queue with headless gloss review - re-confirm stale links, decide SOFT proposals, propose concepts for ns:NEW markers, record recommendations on HARD items, and walk the owner through what only a person may decide. Use after gloss suggest, after a refactor leaves links stale, or when asked to "review the GlossMark backlog", "triage the @gloss? proposals" or "clean up drift".
---

<!-- @gloss instructs gmi:ReviewEngine scope=file -->

# Review GlossMark

You are working the review queue of a repository that has adopted GlossMark: proposed links (`@gloss?`), stale links, proposed concepts, drifted published values and drift findings. You decide what an agent may decide, recommend on the rest, and put those decisions to the owner. You do not decide them yourself. Every step goes through `gloss review` (ADR 0033, experimental), so the gates, the undo journal and the ledger stay the record.

## 0. Ground rules

- **The gate says who decides.** In `gloss review --format json` each action has `gate` and `agent_may`. Take an action only where `agent_may` is true (AUTO or SOFT). On every other action you **recommend** (`"recommend": true`) and a person decides. Never get around the gate: no running `gloss accept` on a HARD item, no editing an ontology file or `glossmark.yaml` by hand, no setting `GLOSS_ACTOR` to a human's name.
- **Run as yourself.** Every gloss command runs with your agent actor (`GLOSS_ACTOR=agent:<you>`; the hook or the Cursor rule sets it). Check with `gloss doctor` if unsure: a review taken as `human:…` is a forged sign-off.
- **Read before you decide.** `gloss review show <id> --format text` for every item before any decision on it. It gives the marker's lines, what changed in the file since the last review, and the target concepts' docs. Never decide from the summary line or from `conf` alone.
- **Every decision has a reason** in your own words, one line, naming the evidence ("renew.py:renew is the renewal handler; its only caller is the renewal route").
- **Stop at the checkpoint.** At step 4, ask and end your turn. Use the ask-the-user tool when you have one. When you do not (Cursor, Codex, a plain chat), write numbered questions with lettered choices, your recommendation first, and **end your turn**. Never answer your own questions, never assume the recommended option, and never apply a HARD decision because the owner is not there.
- **Undo is cheap, so use it.** If a decision turns out wrong, `gloss review undo` reverts the last one, files and ledger together. Do not hand-edit around a mistake.

## 1. Take stock

```sh
gloss drift | tail -1                 # the counts before; you report them again at the end
gloss review --format json > $SCRATCH/queue.json
gloss review --format text            # the same queue, readable
```

Narrow a large queue with `--kind` (`recommendation`, `proposed-link`, `stale-link`, `proposed-concept`, `value-drift`, `finding`) and `--path <dir>`. If the user named a scope ("just services/billing", "only the stale ones"), keep to it.

Sort the queue into two piles by `agent_may` on the action you would take: **yours** (AUTO, SOFT) and **the owner's** (HARD). An item with both kinds of action (a rogue `ns:NEW:` finding: `propose` is SOFT, `waive` is HARD) is yours if the action you would take is.

## 2. Decide your pile

Go kind by kind. Write decisions as JSON Lines in `$SCRATCH/mine.jsonl` and apply them in small batches, reading each result line (`"ok": false` means it did not happen; read `error`).

```sh
gloss review apply $SCRATCH/mine.jsonl
```

- **Stale links** (`reconfirm`, `reject`). The code under a marker changed. Read the change in `show`. Re-confirm when the declaration still does what the target concept says: `{"id":"m-…","action":"reconfirm","reason":"…"}`. If it no longer does (the function moved, split or changed job), reject the link with `"remove": true` and a reason. Then, if the right concept is clear, write the new link as a proposal: `{"action":"write-link","path":"…","line":12,"relation":"implements","target":"ns:Name","reason":"…"}`. A rename with the same behaviour is a re-confirm.
- **SOFT proposals** (`accept`, `reject`). A link to an existing concept, inside the boundary's allowed set and above the auto-accept threshold. Accept when the declaration is the **public surface** for that concept (the handler, the model, the table, the test), the relation is right (`implements` behaviour, `represents` a data shape, `verifies` a test) and the concept's doc matches what the code does. Reject (with `"remove": true` when the marker line should go) when it is a helper, a near miss or the wrong relation. If you wrote the proposal yourself earlier in this session, say so in the reason: an agent accepting its own guess is worth a person's glance.
- **Rogue `ns:NEW:` markers** (`propose`). Propose the concept only when the code shows a real concept that no existing one covers. Check first with `search_concepts` (MCP) or `gloss review node <ns>:<Name>` for near names. Give it a `kind` and a `doc` in the business's words: `{"id":"finding:rogue:…","action":"propose","kind":"process","doc":"…","reason":"…"}`. The concept is then `proposed: true`, which is the owner's decision in step 4. In an implementation namespace it also needs `realizes: <core concept>` or `internal: true` before it can be accepted, which `propose` does not set: put your pick to the owner with it.
- **Anything else in your pile**, follow the item's own `actions`.

Do not decide an item you are unsure about; move it to the owner's pile with your best guess as the recommendation. A wrong SOFT decision costs more than a question.

## 3. Recommend on the owner's pile

For each HARD item, read `show`, then record a recommendation with your verdict and parameters, so it is waiting in `.glossmark/recommendations.jsonl` and on the terminal UI whether or not the owner answers you now:

```json
{"id":"m-…","action":"accept","reason":"…","recommend":true}
{"id":"concept:ns:Name","action":"accept","reason":"…","recommend":true}
{"id":"finding:orphan:ns:Name","action":"waive","owner":"…","expires":"YYYY-MM-DD","reason":"…","recommend":true}
{"action":"edit-doc","curie":"ns:Name","doc":"…","reason":"…","recommend":true}
```

HARD covers a new or proposed concept, a link outside `allowed` or on the published seam, a waiver, a doc edit and a published value. These are the owner's domain decisions (what the business has, where its boundaries are, which gaps are accepted). If you cannot pick a verdict, record none and ask an open question.

## 4. Walk the owner through it (checkpoint: stop and wait)

Group the owner's pile so they answer once per pattern, not once per item. Examples: "these 6 links all cross from the OPAC into circulation", "these 3 orphans are planned features", "2 proposed concepts from `ns:NEW:` markers". Lead with the decisions that change the most: proposed concepts and boundary crossings before single waivers.

Ask up to about 10 questions. Each is multiple choice with your recommendation first. Each states its evidence in one line (path, what the code does, the concept's doc) and offers *leave it open*. Then **end your turn**.

When the answers come, you still do not apply them; the gate is the gate. Write the owner's decisions to a file they apply as themselves, and say which lines differ from your recommendations:

```sh
# $SCRATCH/owner-decisions.jsonl: one decision per line, without "recommend"
gloss review apply $SCRATCH/owner-decisions.jsonl   # the owner runs this, as themselves
```

Or point them at `gloss review` on a terminal, where each recommendation sits beside its item and `y` takes it, `d` dismisses it. Update your recommendations first (`recommend: true` again with the owner's verdict) so what they see matches what they said. A HARD change still needs its CI approval (`ontology-approved` or the repository's rule) on the PR.

## 5. Check and report

```sh
gloss check
gloss build                 # commit .glossmark/graph.jsonl and ledger.jsonl with the review
gloss drift | tail -1       # the counts after
gloss coverage
```

Report in a few lines:
- what you decided, by kind, with counts;
- what you recommended, and which recommendations the owner overrode;
- what is left open and why;
- the drift and coverage counts before and after.

If the owner took every recommendation, say so: a reviewer may want to look again at the ones that mattered most. Any decision you took can be reverted with `gloss review undo`, newest first; mention that once.
