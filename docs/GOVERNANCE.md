<!-- @gloss documents gm:Criticality scope=file -->
# Governance: criticality

Not every file is worth the same review. A metrics exporter and the handler
that moves money both compile into the same graph, but a mistake in one is an
annoyance and in the other is an incident. GlossMark lets the ontology say
which is which, once, and then carries that classification to every place a
reviewer looks: the PR comment, `gloss impact`, the agent briefing and the
viewer.

## One attribute, four levels

`criticality` takes one of `low`, `medium`, `high`, `critical`. It is declared
in two places:

```yaml
# ontology/pay.gloss.yaml — on a concept
concepts:
  Payment:  { kind: entity, criticality: critical }     # money moves on this
  Merchant: { kind: entity, criticality: high }         # holds PII
  Capture:  { kind: process, consumes: [Payment], criticality: high }

# services/api/.gloss.yaml — on a boundary
boundary: acme:payments-api
level: container
criticality: critical
```

Rough meanings, which a repository is free to redefine for itself:

| Level | A mistake here… |
| --- | --- |
| `low` | is an annoyance: telemetry, dashboards, internal tooling |
| `medium` | degrades the product |
| `high` | costs money, trust or compliance: PII, access control, audit |
| `critical` | moves money, breaks the law or stops the business |

Leave it off where it does not matter; unclassified is a valid state, and the
viewer and reports show it as such.

## Inheritance

Declare the level where the business sees it (the core concept) and let it
flow to the code that implements it. `gloss build` stamps every concept,
boundary and artifact in the graph with an effective level and where it came
from (`criticality-from`: `declared`, or the CURIE it was inherited from). A
declared level always wins.

- **Concept.** Without its own level, an implementation concept takes the
  highest level of what it `realizes` or is `published` as; then of the concept
  it is `part-of`; then of its `is-a` ancestors. So `pa:AuthorizeHandler
  realizes pay:Authorization` is critical without anyone saying so twice.
- **Boundary.** Without its own level, a boundary takes the highest level of
  the concepts its code realizes or represents (declared in the manifest or
  derived from the markers beneath it), else its parent boundary's.
- **Artifact.** The highest level of the concepts it links to, except concepts
  it `presents` or `hosts`. Its boundary's level is a **floor** only when the
  boundary's manifest says so with `criticality-floor: true`; then every
  artifact beneath it, nested boundaries included, is at least that level.

Criticality follows the mechanism, never the surface: a page that `presents`
a critical concept, or the app shell that `hosts` it, inherits nothing from it
([ADR 0001](adr/0001-presents-and-hosts.md)).

Inheritance never goes *down* from a boundary to a concept: a critical
platform may contain a low-criticality metrics module, and the module's
concepts, and the files that implement them, keep their own level. Set
`criticality-floor: true` on a boundary when that is wrong for it: a payments
service where every file touches money, a compliance boundary where every
change needs the owning reviewer. In the PR comment a link change is judged
by what it links to, plus the floor when one applies.

## Getting started: `gloss criticality suggest`

Nothing is classified in a fresh adoption, and the viewer says so. `gloss
criticality suggest` proposes a first pass from facts already in the graph,
printed as a diff of the ontology files (`--write` applies it):

- every `constraint` concept: a rule the business relies on;
- every published concept: it crosses the seam into the business view;
- every concept whose only verifiers are security tests (paths or names with
  auth, permission, csrf, crypto, pii, token and the like).

Each is `high`. Concepts that already have a declared level, or inherit `high`
or above, are left alone; everything else stays unclassified. Treat the diff
as a draft: raise money movement to `critical`, lower what does not matter.

`gloss check` warns (GM600) when a boundary declares a level **below** a
concept its code realizes: the manifest understates its risk, and reviews use
the higher level regardless.

## Where it shows up

**`gloss diff` and the PR comment.** Every change is tagged with the highest
level among its subject, its object and the boundary of its path, and the
report says what was touched:

```
Ontology impact — payments-api (acme:payments-api)   base main   gate HARD   criticality critical
+ implements pay:Capture                services/api/handlers/capture.py:capture   [HIGH, agent, conf 0.90, SOFT]
! pay:Payment value none → …            ontology/pay.gloss.yaml:7                  [CRITICAL, HARD: edit to ontology; criticality critical: pay:Payment]
Criticality touched: pay:Payment critical (declared), acme:payments-api critical (via pay:Authorization), pay:Capture high (declared)
```

The gate policy can raise the review level by criticality. The default is that
anything touching a `critical` concept or boundary is HARD, so an agent's
proposal in that area always reaches a human; set it per repository:

```yaml
policy:
  criticality-gates: { critical: HARD, high: SOFT }
```

Bookkeeping changes (an accepted proposal, a stale fingerprint) are reported
with their level but never raised by it.

One classifier gates a link everywhere: `gloss diff`, `gloss impact`,
`gloss context --changed`, `gloss status`, `gloss accept --policy` and the MCP
`propose_link` and `impact_of` tools apply the same rules, `criticality-gates`
included, so `accept --policy` never auto-accepts a link the PR gate would
stop. A link through `presents` or `hosts` takes no level from its target,
in the gate as in the graph.

**`gloss impact`** and the MCP `impact_of` tool report a level per changed
file (`criticality`, `criticality_from`) and per concept, with a hint when a
file touches a `high` or `critical` area so an agent knows to flag it. A
file's level is the one the graph stamps on its artifacts; a page that
presents a critical concept says so in a hint without being rated by it.

**`gloss context`** names the boundary's level in the session-start briefing,
and tags `high`/`critical` concepts in the vocabulary list.

**The viewer** draws a coloured ring around classified concepts and colours
boundary boxes by level, shows the level and its provenance in the panel, lists
top-level boundaries highest-criticality first, and offers a criticality
filter (including "unclassified") so a reviewer can look at only the parts of
the system that matter.

**`gloss export`** writes `criticality` and `criticality_from` columns to the
SQLite `nodes` table, so `SELECT path FROM nodes WHERE node='artifact' AND
criticality IN ('high','critical')` lists the files that deserve the most
careful review.

## Changing a level

A change to a declared level is an ontology or manifest edit and is HARD, like
every other governance decision (`concept-changed` with `criticality low →
critical`, or `boundary-changed`). It also changes every view the concept
appears in, so the business sees it.
