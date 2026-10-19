---
name: adopt-glossmark
description: Adopt GlossMark in a repository - scaffold it, draft the core and implementation ontology from the code and docs, propose @gloss markers with gloss suggest, check and drift, and open one reviewable PR. Use when asked to "add GlossMark", "set up gloss", "adopt the ontology", or to link a codebase to its domain vocabulary.
---

<!-- @gloss instructs gm:Onboarding scope=file -->

# Adopt GlossMark

You are adding GlossMark to this repository: a two-tier ontology (a **core** tier the business owns, an **implementation** tier engineering owns) and one-line `@gloss` comments that link code to it. The result is one PR a human can review in an hour. Work through the steps in order and do not skip the checks.

## 0. Ground rules

- Use the **starter five** relations only, unless a step below says otherwise: `implements` (the code that does the work), `represents` (a data shape: model, table, schema), `verifies` (tests), `documents` (docs), `instructs` (agent files). Add `scope=file` when a whole file is about one concept.
- Every marker you write is a **proposal**: `@gloss?` with `conf=`. Never write a bare `@gloss`; a human removes the `?`.
- One marker per **public surface** (the handler, the pipeline entry point, the model, the table, the test file), never one per helper. `gloss check` flags runs of three identical markers (GM131) and a symbol marker that repeats a file marker (GM130).
- A page or screen that shows a concept `presents` it; the app shell `hosts` it. Neither `implements` it.
- If no concept fits, write `@gloss? <relation> <ns>:NEW:<Name> note="why"`, then run `gloss propose --from-new --doc "what it is"` (MCP: propose_concept) so the concept ships in the same PR with `proposed: true`; a human accepts it with `gloss accept --concept <ns>:<Name>`. Never invent a concept name any other way, and never hand-edit an ontology file to make a link resolve.
- Write every `doc:` quoted: `doc: "One account at an institution (checking, savings)."`. An unquoted doc with a comma inside `{ }` is cut at the comma (GM305).

## 1. Inventory (read, do not write)

1. Read the README, the docs folder and any glossary, enum file or domain model doc. If the repo already has a vocabulary (a markdown glossary, an `enums.yaml`), plan to import it in step 3 rather than retyping it.
2. List the top-level source directories and what each does in one line.
3. Note the 10 to 40 terms a domain expert would recognise (accounts, orders, grading, settlement) and the mechanisms an engineer would name (handlers, pipelines, queues, pages).

## 2. Scaffold

```sh
gloss init --dry-run        # check the guessed prefixes; override with --core/--impl if they read badly
gloss init                  # writes glossmark.yaml, tier files, manifests, views, CI job, AGENTS.md rules block, agent hook
gloss hook install --skill --guards   # Claude Code: the shipped skills and the edit and stop guards
gloss check                 # must pass on the empty scaffold
```

`gloss init` writes the GlossMark rules block into `AGENTS.md`, creating the file when there is none and keeping the rest of an existing one. When the repository has a `CLAUDE.md`, it adds the line `@AGENTS.md` to it, because Claude Code reads `AGENTS.md` on its own only when there is no `CLAUDE.md`. It also registers the SessionStart briefing and the MCP server. `gloss hook install --skill --guards` adds the shipped skills under `.claude/skills/` and the guards that run `gloss hook guard` before and after each edit and when the agent stops. Teams on the GlossMark Claude Code plugin get the skills and guards from the plugin instead. A role agent with a remit is one more command: `gloss init --role <name> --may-modify "<targets>"`.

### Whole repository or one boundary at a time

A small repository is adopted in one PR, as the steps below describe. A large one can be adopted one boundary per PR:

1. Run steps 1 to 4 once for the whole repository, so the ontology exists.
2. Each PR fills one manifest and marks the code under it (steps 5 to 7 for that directory only).
3. Turn on the coverage ratchet in CI: `gloss coverage --fail-under current --changed-only`. Concept coverage never drops below the base branch's, and file coverage is scored only over the files the PR changes, so unmarked boundaries do not fail it.

## 3. Draft the core tier

In `ontology/<core>.gloss.yaml`, one entry per business term, block style or quoted docs:

```yaml
concepts:
  Account:
    kind: entity
    doc: "One account at an institution: checking, savings, credit card or loan."
  CashFlowAnalysis: { kind: process, doc: "Income minus spending per period." }
```

Litmus test: if a domain expert would nod at the term, it is core; if only an engineer would, it is implementation. Core never mentions implementation names. Mark `criticality: high` or `critical` only where a mistake costs money, trust or compliance, and say why in a comment.

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

Then read every proposal. Delete the wrong ones, retarget the near misses, and add the markers `suggest` cannot see (tests that verify a concept by behaviour, docs, agent files). Aim for every implementation concept to have one artifact and one verifier, not for every file to have a marker.

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

Commit the ontology, manifests, markers, `.glossmark/graph.jsonl`, `.glossmark/ledger.jsonl`, `AGENTS.md`, the CI job and the hook config in one PR (one PR per boundary on the incremental path). The description lists: the core concepts and why each is core; any `NEW:` proposals and proposed concepts; the drift summary; and what is deliberately left unmarked. The diff comment will show counts per boundary because the base has no graph; that is expected.

## After adoption

Day-2 work has its own skills: `gloss-link` while changing code, `gloss-resolve-drift` when `gloss drift` reports findings, `gloss-review` when a human reviews proposals, `gloss-criticality` to classify what deserves the most review, and `gloss-upgrade-import` when an imported namespace releases a new version.
