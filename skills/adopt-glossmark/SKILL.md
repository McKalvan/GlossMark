---
name: adopt-glossmark
description: Adopt GlossMark in a repository - scaffold it, draft the core and implementation ontology from the code and docs, propose @gloss markers with gloss suggest, check and drift, and open one reviewable PR. Use when asked to "add GlossMark", "set up gloss", "adopt the ontology", or to link a codebase to its domain vocabulary.
---

<!-- @gloss instructs gm:Onboarding scope=file -->

# Adopt GlossMark

You are adding GlossMark to this repository: a two-tier ontology (a **core** tier the business owns, an **implementation** tier engineering owns) and one-line `@gloss` comments that link code to it. The result is one PR a human can review in an hour. Work through the steps in order and do not skip the checks.

## 0. Ground rules

- **The core tier is the owner's domain.** You never decide it alone. Before any core concept is written, the person who owns the domain has told you what the business does, its core processes and concepts, how fine-grained to model and which views they want (step 1). Ask, then **end your turn and wait**: with an ask-the-user tool if you have one, otherwise as numbered questions with lettered choices in chat, your recommendation first. Never answer your own questions or assume the recommended option to keep moving. Only if the owner tells you outright to go on without them do you continue, and you say so in the PR.
- Use the **starter five** relations only, unless a step below says otherwise: `implements` (the code that does the work), `represents` (a data shape: model, table, schema), `verifies` (tests), `documents` (docs), `instructs` (agent files). Add `scope=file` when a whole file is about one concept.
- Every marker you write is a **proposal**: `@gloss?` with `conf=`. Never write a bare `@gloss`; a human removes the `?`.
- One marker per **public surface** (the handler, the pipeline entry point, the model, the table, the test file), never one per helper. `gloss check` flags runs of three identical markers (GM131) and a symbol marker that repeats a file marker (GM130).
- A page or screen that shows a concept `presents` it; the app shell `hosts` it. Neither `implements` it.
- If no concept fits, write `@gloss? <relation> <ns>:NEW:<Name> note="why"`, then run `gloss propose --from-new --doc "what it is"` (MCP: propose_concept) so the concept ships in the same PR with `proposed: true`; a human accepts it with `gloss accept --concept <ns>:<Name>`. Never invent a concept name any other way, and never hand-edit an ontology file to make a link resolve.
- Write every `doc:` quoted: `doc: "One account at an institution (checking, savings)."`. An unquoted doc with a comma inside `{ }` is cut at the comma (GM305).

## 1. Inventory (read, do not write)

**An existing application with no ontology yet:** use the `derive-ontology` skill instead of steps 1, 3 and 4. That is the default. It scouts the repository, harvests evidence with `gloss harvest`, interviews the domain owner on processes, concepts, granularity and views, drafts both tiers, reviews the draft with them, and hands back here at step 2 with the prefixes to use. The steps below are the short path, only for a small repository whose vocabulary the owner already keeps written down (a glossary, an enum file); its interview is shorter, not skipped.

1. Read the README, the docs folder and any glossary, enum file or domain model doc. If the repo already has a vocabulary (a markdown glossary, an `enums.yaml`), plan to import it in step 3 rather than retyping it.
2. List the top-level source directories and what each does in one line.
3. Note the 10 to 40 terms a domain expert would recognise (accounts, orders, grading, settlement), the processes the business runs, and the mechanisms an engineer would name (handlers, pipelines, queues, pages).
4. **Interview the owner (checkpoint: stop and wait).** Show what you found in a few lines, then ask, multiple choice with your recommendation first: is the written vocabulary current and authoritative; which processes are core; which terms are core and which are wrong or technical; how fine-grained to model (main entities only, plus lifecycle states and rules, or everything the code distinguishes); which views they want (a business overview, an engineering view, one per core process); where a mistake costs money, trust or compliance (`criticality`); and the prefixes for `gloss init`. End your turn until they answer. If the answers show the vocabulary is thin or stale, switch to `derive-ontology`.

## 2. Scaffold

```sh
gloss init --dry-run        # check the guessed prefixes; override with --core/--impl if they read badly
gloss init                  # writes glossmark.yaml, tier files, manifests, views, CI job, agent hook
gloss check                 # must pass on the empty scaffold
```

## 3. Draft the core tier

In `ontology/<core>.gloss.yaml`, one entry per business term, block style or quoted docs:

```yaml
concepts:
  Account:
    kind: entity
    doc: "One account at an institution: checking, savings, credit card or loan."
  CashFlowAnalysis: { kind: process, doc: "Income minus spending per period." }
```

Write only what the owner confirmed in step 1, at the granularity they chose, and show them the list before you go on to step 4. Litmus test: if a domain expert would nod at the term, it is core; if only an engineer would, it is implementation. Core never mentions implementation names. Mark `criticality: high` or `critical` only where the owner said a mistake costs money, trust or compliance, and say why in a comment. Edit `ontology/views.yaml` to the views the owner chose.

If the repo has a glossary, import it instead: `gloss import --from markdown-glossary docs/glossary.md --namespace <core>` (and `--from enums` for enum files), then edit.

## 4. Draft the implementation tier

In `ontology/<impl>.gloss.yaml`, one entry per mechanism worth naming: the engine, the importer family, the page, the queue. Each carries `realizes: <core concept>`, `presents:`/`hosts:` for a page or a shell, `is-a: <another impl concept>` to inherit, or `internal: true`. Use `is-a` for families (`CsvImporter: { kind: process, is-a: Importer }`) instead of repeating `realizes`.

## 5. Fill the manifests

Each `.gloss.yaml` gets `realizes:` for what the directory computes, `holds:` for entities whose data lives there, `presents:` for what its pages show. A container that only contains its components writes `realizes: inherit`. Leave `allowed:` out; the derived set is usually right (`gloss check --explain` shows it).

## 6. Propose markers

```sh
gloss suggest > suggestions.patch   # @gloss? proposals for unmarked declarations, as a patch
git apply suggestions.patch          # or: gloss suggest --write
```

Each proposal's `conf` is the precision its kind of evidence measured on GlossMark's calibration set, so review the low ones hardest; `--min-conf` raises the floor (default 0.3). Proposals at or above the auto-accept threshold, which is derived from `suggest.auto-accept-precision` in `glossmark.yaml` (default 0.9), are marked `auto`; suggest prints the threshold and the precision it measured. A proposal to a deprecated concept carries a note naming its `replaced-by` and is never `auto`: keep it, or retarget it to the replacement. Concepts still `proposed: true` are not targets until accepted. If an owned namespace is nearly empty, suggest says so; `gloss suggest --concepts` drafts `ns:NEW:Name` candidates from declarations that share tables and docstring words. `gloss suggest -h` lists the relations and file types it covers.

Then triage the proposals with the `review-glossmark` skill: it reads each one through `gloss review show`, accepts or rejects the SOFT ones through `gloss review apply` with a reason, and leaves a recommendation on the HARD ones for the owner. Retarget the near misses, and add the markers `suggest` cannot see (tests that verify a concept by behaviour, docs, agent files). Aim for every implementation concept to have one artifact and one verifier, not for every file to have a marker.

## 7. Check, drift, review

```sh
gloss check            # 0 errors; read every warning and fix or justify it
gloss build            # writes .glossmark/graph.jsonl and ledger.jsonl: commit both
gloss drift            # violations and gaps mean a manifest and the code disagree: fix one of them
gloss coverage         # concepts with an artifact and a verifier; files are secondary
gloss doctor           # the setup is complete
gloss diff --base origin/main --format markdown   # what the reviewer will see
```

Violations and gaps must be zero, or each one explained in the PR description. Orphans and untested concepts are fine to leave, listed in the PR.

## 8. One PR

Commit the ontology, manifests, markers, `.glossmark/graph.jsonl`, `.glossmark/ledger.jsonl`, the CI job and the hook config in one PR. The description lists: what the owner decided in the interview (processes, granularity, views) and anything they left to you; the core concepts and why each is core; any `NEW:` proposals and proposed concepts; the drift summary; and what is deliberately left unmarked. The diff comment will show counts per boundary because the base has no graph; that is expected.
