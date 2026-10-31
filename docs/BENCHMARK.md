<!-- @gloss documents gmi:AgentBenchmark scope=file -->
# The agent benchmark

**Status: harness and task set only. No results have been collected yet.** The
numbers this page describes exist nowhere until someone runs `bench/run.sh`
against real agents; when they do, the results go here and a line in the
README, positive or not (#271).

The claim GlossMark makes is that it keeps agent-written code consistent with
the domain: a coding agent that is given the vocabulary, the gate and the
review loop reuses the concepts that exist, leaves the rules it must not
weaken alone, keeps a published value in step with the code it is bound to,
and names things in the business's words. This benchmark measures that claim
on the [showcase repositories](https://github.com/McKalvan/glossmark-examples):
the same ticket, the same agent, with and without GlossMark context, scored
by the tool's own checks.

## Method

**Tasks.** `bench/tasks/*.yaml` holds eleven tickets against five showcase
workspaces. Each is written as a product owner would write it and never
mentions GlossMark; the task loader refuses one that does. Each names the
domain pitfalls it sets and the assertions that check them:

| Task | Repository | Pitfalls |
| --- | --- | --- |
| `claims-payout-failed-notification` | claims-events | reuse `claim:PayoutFailed` (schema and manifest already have it); close the declared gap; do not invent PaymentFailed |
| `claims-raise-auto-approval-limit` | claims-events | `tide:AutoApprovalRule` is a published value bound to a constant: both move, and the gate is HARD; the rule's order is not weakened |
| `claims-theft-needs-police-report` | claims-events | a police report is `claim:LossEvidence` (an alias says so), not a new concept; the limit does not move |
| `library-kiosk-place-hold` | library-legacy | reuse `lib:Hold` and `circ:HoldQueue`; the kiosk manifest must say it holds a hold; a blocked patron still cannot |
| `library-grace-period-five-days` | library-legacy | the rule exists twice (Java and SQL), both linked to `circ:GracePeriodRule`; the cap does not move |
| `library-overdue-patron-export` | library-legacy | `lib:Borrower` is deprecated (`replaced-by: Patron`); the letter is an `OverdueNotification`; patron data is high criticality |
| `clinic-waitlist-offer` | clinic-booking | `clinic:Waitlist` is designed and presented but not built; the booking rule holds; the booking API's manifest widens |
| `clinic-reminder-lead-time` | clinic-booking | `lark:LeadTimeSetting` is published by a `publishes-value` marker; the cancellation window is a different value |
| `clinic-noshow-by-practitioner` | clinic-booking | the measure's definition is fixed; `clinic:Practitioner` is outside the worker's written `allowed:` list, so the manifest, not the marker, gives |
| `kitchen-allergen-alert-at-the-pass` | kitchen-agents | `dine:AllergenRule` is critical, so the change is HARD by policy and the rule must not weaken; the display's `allowed:` list is written |
| `freight-delivery-window-three-hours` | freight-federation/shipment-core | `shc:DeliveryWindowPolicy` is published across repositories; the on-time target is a different value |

**Conditions.** Every task runs under two conditions, from the same copy of
the repository, committed as one git commit so the agent sees a clean tree:

- `bare`: the repository as it is, with the agent surface removed: no
  `.mcp.json`, no SessionStart hook, no Cursor rule, no `gloss` on `PATH`,
  and the generated vocabulary block stripped from `AGENTS.md` and
  `CLAUDE.md`. The markers and ontology files stay, as they would in any
  adopted repository.
- `glossmark`: `gloss hook install --cursor` (hook, MCP server, Cursor rule),
  `gloss agents --write AGENTS.md`, `gloss` on `PATH`, and the ticket followed
  by [`bench/prompts/glossmark.md`](../bench/prompts/glossmark.md), which
  tells the agent to read the vocabulary, link what it adds, propose rather
  than invent, and run `gloss check` and `gloss diff` before it finishes.

**Scoring.** `glossbench score` builds the graph of the tree the agent saw
and the tree it left, the way `gloss build` does, and compares them with the
packages the gate, the drift report and the linter are built from
(`internal/gate`, `internal/rollup`, `internal/check`). From that it reports:

- *Rule mistakes*: a boundary now exceeds its manifest (violation), a
  published value's code moved and the value did not (value-drift), a
  dangling or deprecated reference, a lint error, a file the ticket said to
  leave alone was changed, a constant or an ontology value does not say what
  the ticket asked for.
- *Concept mistakes*: a concept added to an ontology file without
  `proposed: true`; a new concept (added, proposed or named with `ns:NEW:`)
  whose name or alias an existing concept already carries (a duplicate); a
  new concept where the ticket needed none; the expected link to an existing
  concept missing; a link to a deprecated concept.
- *Naming mistakes*: a forbidden term names a new concept, an alias, a marker
  target or, where the task says so, appears in a changed file.
- *Gate*: the class `gloss diff` gives the whole change, and whether it is
  what the task expects (HARD where a published value or a critical rule
  moved, at most SOFT where nothing needed a person).

Four assertions run on every task (`check-clean`, `no-new-finding`,
`no-unproposed-concept`, `no-duplicate-concept`); the task adds its own.
Each failed assertion is one mistake in its category; gate expectations are
counted apart because the gate's verdict is a property of the change, not a
mistake the agent made. `glossbench report` averages mistakes per run for
each task and condition. The JSON score keeps the evidence (the new
concepts, the new markers, the HARD changes, the changed files, each
assertion's message) so a reader can audit any row.

**What is not measured.** The showcase code is inert, so tests are not run
and "tests pass but the domain is violated" is not a column. Human review
time needs a reviewer. Both are in the issue and both remain to do.

## How to run it

```sh
git clone https://github.com/McKalvan/glossmark-examples ../glossmark-examples
bench/run.sh --examples ../glossmark-examples --agent claude --runs 3
bench/run.sh --examples ../glossmark-examples --agent cursor-agent --only clinic-waitlist-offer
bench/run.sh --examples ../glossmark-examples --agent cmd:'my-agent --prompt "$BENCH_PROMPT"' --conditions glossmark
```

The script builds `gloss` and `glossbench` from this tree, copies each task's
repository into `bench/results/<timestamp>/work/<task>.<condition>.<run>`,
applies the condition, runs the agent there with the prompt in
`$BENCH_PROMPT` (and `$BENCH_PROMPT_FILE`), saves the agent's output and the
resulting patch under `logs/`, scores the tree into `scores/<name>.json` and
writes `REPORT.md`. The presets for `claude`, `cursor-agent` and `codex` use
each CLI's non-interactive mode with edits allowed; check the flags against
the version you have, or pass `cmd:` with your own command. `--dry-run`
prepares the copies and the prompts without running anything, and `--only`
selects one task.

To score a tree you produced some other way:

```sh
go run ./cmd/glossbench score --task bench/tasks/clinic-waitlist-offer.yaml \
    --base ../glossmark-examples/showcase/clinic-booking --head /path/to/agent/tree
go run ./cmd/glossbench tasks        # validate and list the task files
go run ./cmd/glossbench report bench/results/<timestamp>/scores
```

`bench/results/` is git-ignored except for what someone decides to publish.

## Adding a task

A task file has `repo`, `title`, `prompt`, `pitfalls` and `assertions`.
The assertion kinds and the fields each reads are documented in
[`internal/bench/task.go`](../internal/bench/task.go); `glossbench tasks`
validates every file and names the field a kind is missing. Write the prompt
without the tool in it, name the pitfalls as a domain expert would, and
assert on each: the concept that must be linked (`link`), the term that must
not appear (`forbidden-terms`), the finding that must close
(`finding-closed`), the value that must follow (`concept-value`), the file
that must not move (`file-unchanged`), and what the gate must say (`gate`).
A quick check that the assertions are sound is to score the repository
against itself: only the "did the agent do the ticket" assertions should
fail.

## Reading the results

The headline is *mistakes per run* under each condition, per task and in
total, with the category breakdown beside it. A difference in the concept
and naming columns is what the vocabulary surface (briefing, MCP, AGENTS.md)
should buy; a difference in the rule column and in the gate column is what
the gate and the drift report should buy. Three runs per task and agent is
the least that says anything, since one agent run varies as much as two
conditions do. Results, when they exist, are quoted here and in the README
with the agent, its version, the date, the number of runs and a link to the
`scores/` directory they were computed from.
