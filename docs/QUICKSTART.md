# Quickstart

## Install

```sh
go install github.com/mckalvan/glossmark/cmd/gloss@latest
# or, from a checkout
go build -o gloss ./cmd/gloss
```

## 1. Declare the namespaces

`glossmark.yaml` at the repo root:

```yaml
owns:
  pay: { iri: https://onto.example.com/pay/, tier: core,           version: 0.1.0, owner: payments-product }
  pa:  { iri: https://onto.example.com/pa/,  tier: implementation, version: 0.1.0, owner: payments-eng }
prefixes:
  acme: https://arch.example.com/boundaries/
```

## 2. Write the core in words the business recognises

`ontology/pay.gloss.yaml`:

```yaml
namespace: pay
concepts:
  Payment:       { kind: entity }
  Authorization: { kind: process, produces: [Payment] }
  Capture:       { kind: process, consumes: [Payment] }
```

Litmus test: if a domain expert would nod at the term, it is core; if only an engineer would, it is implementation.

## 3. Write the implementation that realizes it

`ontology/pa.gloss.yaml`:

```yaml
namespace: pa
concepts:
  AuthorizeHandler: { kind: process, realizes: pay:Authorization }
  CaptureQueue:     { kind: resource, internal: true }
```

## 4. Put a manifest on each service directory

`services/api/.gloss.yaml`:

```yaml
boundary: acme:payments-api
level: container
realizes: [pay:Authorization, pay:Capture]
default-namespace: pa
allowed: [pay:Payment, pa:*]
```

## 5. Mark the code

```python
# @gloss implements .AuthorizeHandler
def authorize(req): ...
```

Agents write `@gloss?` for links they propose and `ns:NEW:Name` for concepts that do not exist yet. Generate the vocabulary they should see first:

```sh
gloss agents --dir services/api --write AGENTS.md
```

## 6. Run the tooling

```sh
gloss check                      # lint: syntax, dangling refs, domain/range, own-subject rule, tiers
gloss build                      # .glossmark/graph.jsonl + context.jsonld + ledger.jsonl
gloss drift                      # violations, gaps, orphans, rogues, stale, untested, metrics
gloss diff --base origin/main    # AUTO/SOFT/HARD per change, views changed, PR comment
gloss status                     # proposed / stale / rejected links
gloss accept --policy            # accept every SOFT proposal (or: gloss accept <id|path:line>)
gloss view                       # .glossmark/viewer.html, static, no server
gloss coverage --fail-under 80
gloss export --sqlite graph.db
gloss publish                    # dist/glossmark/<ns>/ontology.jsonld (+ graph.jsonl for impl namespaces)
gloss landscape ../repo-a ../repo-b   # union several repos, context-map edges, cross-repo drift
gloss mcp                        # MCP server on stdio for agents
```

Set `GLOSS_ACTOR=agent:claude-code@claude-fable-5-1` (or `human:alice`) so the ledger attributes links correctly.

## Agent experience

Four commands exist so an agent can work the ontology without a human in the loop until the review:

```sh
gloss hook install                      # SessionStart hook → 'gloss context' briefing; MCP server in .mcp.json
gloss context --dir services/api        # the briefing: boundary, allowed targets, remits, concepts, state
gloss impact --base origin/main         # what the working tree touches: published seam, constraints, tests, blast radius
gloss impact services/api/handlers/refund.py
gloss propose concept pa:RefundHandler --kind process --realizes pay:Refunding --doc "Handles refund requests"
gloss propose --from-new                # lift every ns:NEW:Name marker into a proposed concept and rewrite the markers
gloss accept --concept pa:RefundHandler # a human accepts the proposal (drops proposed: true)
gloss reject --concept pa:RefundHandler --reason "duplicate of pa:RefundFlow" --remove
```

A proposed concept carries `proposed: true` and `proposed-by:` in the ontology file, the ontology's equivalent of `@gloss?`. It exists so links resolve, it shows in `gloss drift` and `gloss status`, and it is HARD until a human accepts it, so the proposal and its implementation ship in one PR. `search_concepts` matches aliases (`aliases: [chargeback]` on `pay:Refund`), CamelCase words, stems and small typos, and follows `realizes`/`is-a` one hop, so an agent that asks for "settlement cutoff" finds `sw:CutoffClock`.

Over MCP the same capabilities are `context`, `impact_of` (with `paths` or `base`), `propose_concept`, `propose_link`, `search_concepts`, `concepts_for_path`, `lookup_concept` and `drift`.

## 7. Wire CI

`.github/workflows/gloss.yml`:

```yaml
on: [pull_request, push]
permissions: { contents: read, pull-requests: write }
jobs:
  gloss:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: mckalvan/glossmark@main
        with: { fail-on: HARD, coverage-fail-under: "0" }
```

Pre-commit: `ln -s ../../hooks/pre-commit .git/hooks/pre-commit`, or add the hooks from `.pre-commit-hooks.yaml` to your `.pre-commit-config.yaml`.

## Review workflow

- Humans review novelty and hierarchy; everything else flows through policy.
- **AUTO**: fingerprint-only changes, derived edges. Cleared by the engine.
- **SOFT**: a link to an existing concept, valid, inside `allowed`, above the confidence threshold, altering only the engineering view. Cleared by the engine; audit a sample later.
- **HARD**: a new concept proposed, a link outside an agent's `may-modify` remit or outside `allowed`, any edit to `ontology/` or manifests, anything touching a published concept, MAJOR bumps, cross-repo imports. A human decides in the PR.

Suggested policy (not framework behaviour): review per PR, never per marker; auto-accept SOFT; audit a random share weekly and calibrate `soft-conf-threshold` from agreement.
