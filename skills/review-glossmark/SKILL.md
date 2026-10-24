---
name: review-glossmark
description: Work the GlossMark review queue after adoption - re-confirm stale links, accept or reject SOFT proposals, propose concepts for NEW markers, and recommend on HARD items for a person to decide, all through gloss review. Use when asked to "clean up the GlossMark backlog", "triage the @gloss proposals", "fix the stale links", "review the gloss queue", or after gloss suggest.
---

<!-- @gloss instructs gm:Reviewing scope=file -->

# Review the GlossMark queue

You are working through what waits on review in a repository that already uses GlossMark: proposed links (`@gloss?`), stale links whose code changed since they were accepted, `ns:NEW:` markers, proposed concepts, drifted values and drift findings. Every item goes through `gloss review`, so each decision carries a reason, is gated, and can be undone. Work through the steps in order.

## 0. Ground rules

- **Run as yourself.** Every command runs with `GLOSS_ACTOR=agent:<your name>` (the SessionStart hook and the MCP server already set it). Never set a `human:` actor, never edit `.glossmark/ledger.jsonl` or `graph.jsonl` by hand, and never run `gloss accept` to get around the queue.
- **AUTO and SOFT are yours; HARD is a person's.** An item's actions say `agent_may: true` when you may take them. Everything else (a new or proposed concept, a link outside `allowed` or on the published seam, a waiver, a doc edit, a published value) you may only **recommend**.
- **Read before you decide.** `gloss review show <id>` gives the marker, the code around it, what changed since it was last reviewed, and the concept's doc. Decide from that, never from the summary line.
- **The ontology is the owner's.** When no concept fits, you propose one; you never add or rename one in an ontology file.

## 1. Read the queue

```sh
gloss drift > $SCRATCH/drift-before.txt; gloss coverage > $SCRATCH/coverage-before.txt
gloss review --format json                      # the whole queue
gloss review --format json --kind stale-link,proposed-link --path services/billing   # narrow it
gloss review show <id>                          # one item with its context and history
```

Each item has an `id`, a `kind`, a `gate` and `actions`, each action with its own `gate` and `agent_may`. Work one boundary or directory at a time when the queue is long. Items that already carry a `recommendation` are waiting on a person: leave them unless it reads as outdated, then recommend again.

## 2. Decide what you may

Write decisions as JSON Lines and apply them; every decision carries a `reason` a reviewer can check.

- **Stale link** (`reconfirm`): read what changed. If the code still does what the link says, re-confirm it. If it no longer does, `reject` it with `"remove": true` and, when another concept now fits, write a new `@gloss?` link with `write-link`.
- **SOFT proposal** (`accept` or `reject`): accept only when the target is the concept this surface serves, at the right tier (an implementation concept that names the symbol beats the core concept it realizes) and the relation is right (`implements` for the mechanism, `represents` for a data shape, `verifies` for a test, `presents` for a page). Reject package clauses, client copies of a model, test doubles and fixtures, and stem collisions (`appointment` is not `AppointmentReminding`), with `"remove": true`.
- **`ns:NEW:` marker** (`propose`, with `"doc"` and optionally `"kind"`): propose the concept with a one-sentence doc when nothing in the ontology fits. The concept itself is HARD; your proposal puts it in front of the owner.

```sh
cat > $SCRATCH/decisions.jsonl <<'EOF'
{"id":"m-12e4355fa3","action":"accept","reason":"renew() is the renewal mechanism; lib:Renewal is the concept it implements"}
{"id":"m-1f52fe4e9f","action":"reject","remove":true,"reason":"package clause, not a surface"}
EOF
gloss review apply $SCRATCH/decisions.jsonl   # one result line per decision
gloss review undo                              # if a decision was wrong
```

Over MCP the same engine is `review_queue`, `review_item`, `review_decide` and `review_undo`.

## 3. Recommend on what you may not

For each HARD item, apply the decision you would take with `"recommend": true`. It is recorded in `.glossmark/recommendations.jsonl`, shown beside the item, and changes nothing else:

```json
{"id":"m-9a0c1d2e3f","action":"accept","recommend":true,"reason":"inside the boundary's work but outside allowed: widen allowed in services/circulation/.gloss.yaml"}
```

## 4. Walk the owner through the HARD items (checkpoint: stop and wait)

Group the HARD items (new concepts, links outside `allowed`, seam and published changes, waivers, doc edits) and ask the owner about each group: multiple choice, your recommendation first, with *leave it open* always an option. Use an ask-the-user tool if you have one, otherwise numbered questions with lettered choices in chat. Then **end your turn and wait**. Never answer your own questions, never assume the recommended option, and never apply a HARD decision yourself.

When the owner answers, hand the decisions back for them to apply as themselves: a decisions file they run with `GLOSS_ACTOR=human:<their name> gloss review apply <file>`, or `gloss review` on a terminal, where `y` takes your recommendation. Only if the owner tells you outright to stop asking do you leave the rest open, and you say so in the report.

## 5. Report

```sh
gloss build && gloss drift > $SCRATCH/drift-after.txt; gloss coverage > $SCRATCH/coverage-after.txt
```

Close with: what you decided (counts by kind, and every rejection with its reason), what you recommended and on which items, what is still open and why, and the drift and coverage lines before and after. Commit `.glossmark/graph.jsonl`, `ledger.jsonl` and `recommendations.jsonl` with the marker edits.
