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
# ontology/lib.gloss.yaml — on a concept
concepts:
  Patron:   { kind: entity, criticality: critical }     # reading records are confidential by law
  Hold:     { kind: entity, criticality: high }         # names a patron: PII
  Checkout: { kind: process, consumes: [Item, Patron], criticality: high }

# .gloss.yaml — on a boundary
boundary: acme:library-platform
level: system
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

Classifying is also the second rung of the [adoption ladder](QUICKSTART.md#the-adoption-ladder): with `adoption: 2` in `glossmark.yaml`, `gloss drift` and the PR gate fail only for concepts classified `high` or `critical`, and everything unclassified stays advisory until the repository moves to level 3.

## Inheritance

Declare the level where the business sees it (the core concept) and let it
flow to the code that implements it. `gloss build` stamps every concept,
boundary and artifact in the graph with an effective level and where it came
from (`criticality-from`: `declared`, or the CURIE it was inherited from). An
inherited level also names the concept that declared it (`criticality-source`),
so `cyc:HydraulicDiscBrake critical from cyc:DiscBrake` carries
`criticality-source: cyc:BrakingComponent` and nobody walks the chain by hand.
A declared level always wins.

- **Concept.** Without its own level, a concept takes the highest level of
  what it `realizes` or is `published` as; then of the concept it is
  `part-of`; then of its `is-a` ancestors. So `circ:CheckoutHandler realizes
  lib:Checkout` is high without anyone saying so twice. The rule is
  the same in both tiers: a core concept with no level of its own inherits
  through `part-of` and `is-a` too, so declaring `cyc:BrakingComponent`
  critical makes `cyc:HydraulicDiscBrake`, four `is-a` levels down, critical,
  and a step `cyc:ScanConfirm` that is `part-of` a high `cyc:BinPick` is high.
- **Boundary.** Without its own level, a boundary takes the highest level of
  the concepts its code realizes, represents or configures (declared in the
  manifest or derived from the markers beneath it), else its parent
  boundary's. A surface is the exception: a boundary that presents or hosts
  concepts and realizes, represents or configures none stays unclassified
  instead of taking its parent's level, so a BFF that only shows order
  placement is not rated above one that implements it
  ([ADR 0012](adr/0012-surfaces.md)). Give it its own `criticality:`, or set
  `criticality-floor: true` on the parent, when it should be rated.
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
`criticality-floor: true` on a boundary when that is wrong for it: a circulation
service where every file touches patron records, a compliance boundary where every
change needs the owning reviewer. In the PR comment a link change is judged
by what it links to, plus the floor when one applies.

## Getting started: `gloss criticality suggest`

Nothing is classified in a fresh adoption, and the viewer says so. `gloss
criticality suggest` proposes a first pass from facts already in the graph,
printed as a diff of the ontology files (`--write` applies it):

- every `constraint` concept that is not `internal: true`: a rule the business
  relies on (an internal constraint, such as a rate limit or a flag, is an
  engineering setting and is proposed only on the other rules);
- every published concept: it crosses the seam into the business view;
- every concept whose only verifiers are security tests (paths or names with
  a word such as auth, authorization, permission, csrf, crypto, pii or token,
  matched as a whole word of the name, so `test_auth_flow` counts and
  `TestPayoutAuthorizedNotifies` does not: `authorized` alone is a domain
  state).

Each is `high`. A declared level is the review: a concept that declares one,
or inherits one from a concept that declares it, is left alone, so lowering a
core concept to `medium` on purpose sticks, and its implementation concepts are
not proposed again on the next run. An implementation concept that realizes an
unclassified core concept is not proposed either; the suggestion goes to the
core concept, from which it would inherit ("realized by lark:LeadTimeSetting
(constraint: ...), which would inherit it"). Everything else stays
unclassified. Treat the diff as a draft: raise money movement to `critical`,
lower what does not matter.

`gloss check` warns (GM600) when a boundary declares a level **below** a
concept its code realizes: the manifest understates its risk, and reviews use
the higher level regardless.

## Where it shows up

**`gloss diff` and the PR comment.** Every change is tagged with the highest
level among its subject, its object and the boundary of its path, and the
report says what was touched:

```
Ontology impact — circulation (acme:circulation-svc)   base main   gate HARD   criticality critical
! lib:Patron value none → …          ontology/lib.gloss.yaml:13                              [CRITICAL, HARD: edit to ontology; criticality critical: lib:Patron]
? implements circ:CheckoutHandler    services/circulation/loans/checkout.py:check_out_many   [HIGH, agent, conf 0.90, SOFT: auto-accept eligible]
Criticality touched: lib:Patron critical (declared), circ:CheckoutHandler high (via lib:Checkout)
```

The gate policy can raise the review level by criticality. The default is that
anything touching a `critical` concept or boundary is HARD, so an agent's
proposal in that area always reaches a human; set it per repository:

```yaml
policy:
  criticality-gates: { critical: HARD, high: SOFT }
```

Bookkeeping changes (an accepted proposal, a closed plan item whose `planned`
key is dropped, a stale fingerprint) are reported with their level but never
raised by it, and the two ontology edits among them explain their file to the
`hard-paths` rule, so a PR that only drops `proposed` or `planned` is AUTO. A changed decorator stack on an
unchanged signature (`decorators-changed`, ADR 0014) is not bookkeeping: a
deleted `@requires_role` on a critical handler is HARD under the default.

One classifier gates a link everywhere: `gloss diff`, `gloss impact`,
`gloss context --changed`, `gloss status`, `gloss accept --policy` and the MCP
`propose_link` and `impact_of` tools apply the same rules, `criticality-gates`
included, so `accept --policy` never auto-accepts a link the PR gate would
stop. A link through `presents` or `hosts` takes no level from its target,
in the gate as in the graph. A link to a concept that a mechanism `guards`
is reviewed at the guard's level when that is higher, without taking it as
its own: the graph keeps the handler's level, the gate sees the guard's
([ADR 0015](adr/0015-guards.md)).

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
SQLite `nodes` table, so `SELECT path FROM nodes WHERE node IN
('artifact','file') AND criticality IN ('high','critical')` lists the files
that deserve the most careful review. A file with no marker is not an
artifact; under a `criticality-floor` it is exported as a `node='file'` row
carrying the floor's level (`criticality_from` names the floor boundary), so
an unmarked migration that grants database roles is listed as `gloss impact`
already reports it.

## Audit reports (`gloss report compliance`)

A regulated team is asked, for each rule, to show the code that enforces
it, the tests that verify it, and who approved each change. The graph, the
ledger, the criticality levels and the published values hold all of that;
`gloss report compliance` assembles it into one report a reviewer outside
engineering can follow, and parses nothing itself: it reads the built graph,
`.glossmark/ledger.jsonl` and the drift report.

```
gloss report compliance                                   # every constraint and every critical concept, as Markdown
gloss report compliance --concept claim:FilingWindow --concept claim:AutoApprovalLimit --format html -o filing.html
gloss report compliance --criticality high --format csv -o evidence.csv   # high and critical, one row per piece of evidence
gloss report compliance --since 2026-01-01 --format json    # decisions on or after a date
```

For each selected concept the report gives:

- its definition, kind, tier, criticality (and where an inherited level comes
  from), status (`gloss export --status`), the owner of its namespace and the
  owner of each boundary that realizes it, and the implementation concepts
  that stand for it in code;
- **Where it is enforced**: every artifact linked to it or to a concept that
  realizes it (`implements`, `represents`, `publishes-value`, `configures`,
  ...), with its review state and who linked it; paths link to the code host
  through `source-url` in `glossmark.yaml` (or `--source-url`);
- **How it is verified**: the `verifies` links, the same way;
- **Published value**: the value the ontology publishes, the line it is bound
  to, what that line reads now and whether the binding is `accepted` or
  `drifted`;
- **Decision history**: the ledger entries whose targets resolve to the
  concept, as events in order: who linked or proposed each link and at what
  confidence, who accepted or rejected it, when, and the reason; links that
  went stale or left the tree; value bindings and their drift. A marker that
  no longer exists is still resolved through the manifest's default namespace,
  so a rejected or removed link keeps its place in the history;
- **Open findings and waivers**: the drift findings on the concept and its
  realizers, waived ones marked, and every `policy.waivers` entry naming the
  concept or one of those findings with its owner and expiry.

Formats: `markdown` (default, the text form), `html` (one self-contained
page without scripts, with a print stylesheet: print it from the browser to
file a PDF), `csv` (one row per concept and piece of evidence, columns
`concept, label, kind, criticality, status, owner, row, relation, path, line,
url, state, actor, when, record, reason, detail`, where `row` is `concept`,
`boundary`, `artifact`, `test`, `value`, `decision`, `finding` or `waiver`
and `record` the ledger marker id or finding id) and `json` (the report
itself). Without `--concept` or `--criticality` the report covers every
constraint (an implementation constraint that stands for a core one is
reported under it) and every `critical` concept. See
[ADR 0043](adr/0043-compliance-report.md).

## Changing a level

A change to a declared level is an ontology or manifest edit and is HARD, like
every other governance decision (`concept-changed` with `criticality low →
critical`, or `boundary-changed`). It also changes every view the concept
appears in, so the business sees it.

"Views changed" in `gloss diff` lists the views whose projection changed: a
node added or removed, an edge added or removed, or a concept's declared
`value`, label, kind, publication or criticality edited. A published value
that drifts (the code bound by `value-from` or `publishes-value` changes and
`value:` does not) changes none of those, so a curated view that names the
concept does not count as changed by it. The drift is still HARD on its own (`published-value-drifted`),
and once `value:` is updated to match, every view naming the concept changes.
