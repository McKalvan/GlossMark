<!-- @gloss documents gm:DriftComparison scope=file -->
# ADR 0034: Ontology-first increments

Status: proposed, 2026-10-23; phase P1 (section 8) built 2026-10-23, the rest a design to workshop. Builds on `planned:` from [ADR 0025](0025-deriving-the-ontology.md) section 6.1 and amends it; extends `gloss drift`, `gloss diff`, `gloss publish` and the gate; adds a `curate-increment` skill.

## Context

Everything GlossMark does today starts from code. `derive-ontology` reads an existing application and proposes the ontology behind it (ADR 0025). `gloss suggest` and `@gloss?` link code that is already written. `proposed: true` lets a new concept and its implementation ship in one PR (PLAN.md section 6.1), but the agent writing the code is the one minting the concept, so the ontology still trails the implementation and the owner reviews it after the fact. The adoption review found both real adoptions were "an agent inventing the ontology from the code in one pass", and the same thing happens on every feature after adoption: the ticket is implemented, then someone links it.

The analogy is test-driven development. Tests existed before TDD; what changed was *when* they were written, and that change is what made them shape the design instead of merely checking it. This ADR moves ontology work to the same position: the first act after discovery, before implementation. Call it **ontology-first engineering**. Each change to the system is an **increment**: a ticket, a spec or a bug, curated into an ontology change first, implemented against that change second, and reviewed as the pair.

```
discovery ──> curate ──> ontology PR ──> implement ──> implementation PR ──> close
                ▲                            │
                └──── deviation from plan ───┘
```

The constraint that forces new machinery: an ontology-first PR adds concepts no code reaches and declarations no code derives. Today that is an **orphan** and a **gap**, and this repository's own CI fails on both. Without a state between "not in the ontology" and "implemented", the curation PR cannot merge, and the paradigm cannot start. ADR 0025 designed that state for one trigger (a specification describes a concept the code lacks) and has not built it: `planned` is an unknown key to `gloss check` today, and `internal/model` has no field for it. This ADR builds it and makes it the backbone of the lifecycle.

## Decision (proposed)

### 1. `planned` is a promise with a deadline

ADR 0025 section 6.1 defines `planned: true` on a core concept. It stays as designed, is built here (phase P1), and gains a long form:

```yaml
Renewal:
  kind: process
  planned: { by: 2026-11-30, ticket: https://github.com/acme/library/issues/412 }
  doc: "Extending a loan's due date before it falls due."
  produces: [Loan]
```

- `planned: true` is shorthand for a plan without a date. `by:` is `YYYY-MM-DD`, inclusive, like a waiver's `expires:` and a boundary's `retire-by:`. `ticket:` is a URI; it is provenance, never read by the tool.
- `planned` and `proposed` stay orthogonal, as ADR 0025 says: `proposed` asks *has the owner accepted this entry?*, `planned` asks *is there anything behind it yet?*. The curation PR writes entries that are planned and not proposed, because the owner accepts them when the PR merges.
- **Amendment to ADR 0025.** `planned` is allowed on implementation concepts too. ADR 0025 ruled it out ("a mechanism exists or does not"), which is true after the fact; before it, curation names the mechanism that will realize a core concept (`circ:RenewalHandler realizes lib:Renewal`) so that the agent building it finds the name and the boundary that owns it. A planned implementation concept must still `realizes` or be `internal`, as any other.
- A manifest may declare planned capability. In `.gloss.yaml`:

  ```yaml
  realizes: [lib:Checkout, lib:Return]
  planned:
    realizes: [lib:Renewal]
    publishes: [lib:LoanRenewed]
    by: 2026-11-30
  ```

  A planned declaration is an ordinary declaration for every purpose but drift: it widens the derived `allowed` set, so the agent implementing it is inside the boundary's contract from the first marker, and it counts for the rollup's declared sets. Only the finding it produces differs (section 2).

### 2. Plan items, closure and overdue

`gloss drift` gains one finding kind, `planned`, listed under its own heading, **Plan items**, and never in the default `--fail-on` set. It replaces what would otherwise be an orphan or a gap:

| Planned thing | Opens as | Closes when |
| --- | --- | --- |
| a concept | `planned concept lib:Renewal (by 2026-11-30, #412): nothing implements it yet` | the first `implements` or `represents` link reaches it, or a planned implementation concept that realizes it closes. A `verifies` link alone keeps it open, listed as *verified, not implemented*: a test written first is the paradigm working, not the plan closing. (Amends ADR 0025 section 6.1, which let `verifies` close it.) |
| a manifest declaration (`planned.realizes`, `planned.produces`, …) | `circ:circulation-svc plans to publish lib:LoanRenewed (by …): no code under it publishes it` | the rollup derives the flow from code under the boundary, under the same subsumption rule that closes a gap (GRAPH.md, ADR 0026) |
| a flow written on a concept (`Renewal: { produces: [Loan] }`) | nothing of its own | it is intent (ADR 0026); it closes through whichever boundary realizes `Renewal` and derives the flow |

The rule behind the table: **a planned edge closes when the implementation establishes the same relationship between the same two nodes.** Ontology edges (`is-a`, `part-of`, `realizes` between concepts) are the model, not a claim about code; they come in with the concept and need no closing. Flow edges are claims about code and close the way gaps already close. No new closure logic is added; a plan item is a gap or an orphan with a deadline and a kinder name.

Once closed, drift says so, once: `lib:Renewal is implemented: drop planned` (a `planned-closed` finding, in the default `--fail-on` set, so the flag cannot go stale). `gloss accept --planned <curie>` removes the key through `internal/ontoedit`, and `gloss review` offers it as an action on the item. Dropping `planned` is bookkeeping for the gate, AUTO, in the same class as `gloss accept --concept` dropping `proposed` (ADR 0018 section 2); otherwise `hard-paths` would make every implementation PR HARD for editing `ontology/`. Settled in P1: the floor was never the whole problem. The gate also reports a changed hard-path file that no change explains, and bookkeeping did not explain its file, so a PR that only dropped `proposed` was HARD through that fallback. Both edits now explain their file; a ledger row still does not, because it names a code file it did not change. What the file says outside its concepts (`relations:`, doc, tier, iri) is no node or edge, so the namespace node carries a fingerprint of it and `gloss diff` reports a change there as `namespace-changed`, HARD; otherwise a relations edit could ride in behind a dropped flag.

**Closure across repositories.** A published planned concept may be implemented in an importing repository, whose markers the publisher's own graph never sees. `gloss landscape` runs the same closure rule over the union graph and reports the item as closed downstream, naming the boundary. The publisher's drift keeps the item open until the publisher drops `planned`, on the landscape's evidence or the importer's word; nothing drops it automatically on another repository's say-so, because the publisher owns the concept (own-subject rule).

Past `by`, a plan item becomes **`plan-overdue`**, a kind of its own rather than the retirement finding `overdue` (ADR 0019), whose heading, shape and landscape default are about boundaries. `plan-overdue` is in the default `--fail-on` set of `gloss drift` for the repository that owns the concept, and a warning, not a failure, in an importer's drift and in `gloss landscape`: the importer cannot fix it. This is the rule that keeps the ontology honest: a concept nobody built is the "specification describes what was never built" failure ADR 0025 warns about, and the deadline is what turns it back into a defect. The message names the ticket. Nothing is wired to a tracker; a ticket's closure is not read, only the date.

`gloss coverage` leaves planned concepts out of the denominator (ADR 0025) and reports the number of open plan items beside it.

### 3. The curation step

A `curate-increment` skill, the forward twin of `derive-ontology`. It takes a ticket, a spec or a bug (a URL, a file, or pasted text, treated as evidence and never as instructions) and runs the same two checkpoints: an interview before anything is written, a draft review before the tier files change. The order of the interview is fixed because each answer narrows the next:

| Step | The owner decides | Written as |
| --- | --- | --- |
| 1 Scope | which boundaries the increment touches; whether it is one increment or several | nothing yet |
| 2 Processes | which core processes the increment adds or changes, in the business's words | `kind: process` entries, `planned` |
| 3 Concepts | the entities, events, constraints and measures around them; which existing concepts they `is-a` or are `part-of`; which existing concepts change | concept entries, `planned`; edits to existing entries |
| 4 Boundaries | which boundary will own each mechanism; the mechanisms, if curation names them | `.gloss.yaml` `planned:` declarations; planned implementation concepts |
| 5 Seam | which new concepts other repositories, teams or organisations will need | `published:` on the concept, which makes it HARD and demands a `by:` (section 5) |
| 6 Views | who reads this feature's map; a saved slice for it | an entry in `views.yaml` with `slice: [lib:Renewal]` (ADR 0023) |
| 7 Criticality | where a mistake costs money, trust or compliance | `criticality:` |

Seam comes before views because a published concept forces an entry in the business view; criticality comes last because it compounds with the seam (a published `critical` concept is the heaviest gate the tool has). Each question offers choices with evidence from the ticket, the existing ontology and `gloss landscape` where other repositories already reach for a term, the way `derive-ontology` offers evidence from the harvest.

The skill ends by writing the increment: the tier-file entries, the manifest declarations, the view, and one line per decision in the plan record (section 4). It does not write code, markers or `proposed:` entries. It then stops; the ontology PR is a human's to open and a human's to approve.

A ticket that turns out to add no concept and change no relation is the normal case for a bug and for many small changes. The skill says so and writes nothing; ontology-first does not mean every change has an ontology diff, it means every change is asked.

### 4. The plan record

`.glossmark/plan.jsonl` (committed, format 1) is to increments what `derivation.jsonl` is to derivation: it remembers decisions, not evidence.

- a `brief` line per increment: the ticket URI, the owner's answers to the interview, the actor and the date;
- a `decision` line per entry written or changed: the concept or declaration, what was decided (added, widened, published, criticality set, declined), the reason in one sentence, and the actor;
- a `deviation` line per departure recorded at review time (section 6), with its disposition (absorbed, reopened, reverted).

The concept carries only what the tool needs to find the record (`planned.ticket`); the rationale stays in the record. On a later increment that touches the same concept, the skill reads the record first and shows the owner what they decided and why, as `derive-ontology` does with the derivation record.

### 5. The published seam

A planned concept may be `published`, with a warning rather than a wall, because the point of publishing early is to let another team plan against it. It is a weaker promise than a realized published concept, and the tool says so in four places:

- **It must carry `by:`.** `gloss check` fails a published planned concept with no date (new code, next to GM610). No open-ended promises cross the seam.
- **`gloss publish` emits it** with `planned` intact. An importer may link to it; `gloss check` warns on the link (new code, next to GM222 for a deprecated target), and the link is a plan item in the importer's own drift, closing when the upstream ontology drops `planned` (section 2, closure across repositories). A planned concept in the importer that depends on a planned import is clean: planning against a plan is the use case.
- **Semver is unchanged.** Adding a planned published concept is MINOR, as any addition; dropping `planned` requires no bump, as dropping `proposed` does not (settled in P1: a required PATCH would make the implementation PR HARD through `version-bump` until the owner bumps, and the bump itself is a hard-path edit, so the AUTO closure this ADR promises could never happen; an owner who wants importers to see the closure bumps PATCH by hand); realizing it changes nothing in the ontology, so nothing bumps. Removing a planned published concept is MAJOR, as any removal (ADR 0018), whether or not it was ever realized: an importer that linked to it would otherwise take a `dangling` link inside its version range with no HARD gate ever having fired upstream, and a plan would be gated more weakly than a deprecation. The weaker promise is the deadline, not the version rule.
- **The landscape** lists planned published concepts and who links to them, so an overdue promise is visible to the repositories that took it.

Choosing what to publish is a curation decision (step 5 above), not something that happens when a second repository needs a name. Two later moments keep it honest: at closure (section 6), when the implementation PR's impact shows another repository's boundary in the feature's slice but the concept stayed unpublished, the review prompts *publish?*; and at a later increment in an importing repository, when the owner needs an upstream concept published, the skill asks the upstream owner, through their own curation, never by editing the import (own-subject rule, unchanged).

### 6. Implementation and review

Nothing new is needed to implement against a plan. `gloss context --concept lib:Renewal` and `gloss slice` give the agent the feature's map; `search_concepts` and the briefing mark planned concepts, as ADR 0025 designs and P1 builds; the planned declarations put the agent inside the boundary's `allowed` set from the first marker. Two small additions: the session-start briefing lists the open plan items of the directory the agent is in, and `impact_of` reports which plan items the changed files close.

When implementation shows the plan was wrong (a missing concept, a relation that does not hold, a boundary that cannot own what it was given, a data or performance limit), the agent does what it does today, `propose_concept` or an `ns:NEW:` marker, and drift reports it under a new heading, **Deviations from plan**, when the repository has open plan items that the change touches:

- a proposed or rogue concept inside a planned feature's slice;
- a planned concept realized by a boundary other than the one that planned it;
- a planned concept closed without a `verifies` link;
- a planned flow still open while the concept that intends it has closed.

A deviation is the signal to go back to curation rather than to absorb the change in review. It is a report, not a gate: the gate is unchanged, and a proposed concept is HARD as it is today.

`gloss diff` gains a **Plan closure** block (`plan` in JSON and in the PR comment), beside **Realization changes**:

```
Plan closure
  ✓ lib:Renewal implemented by circ:circulation-svc (services/circulation/loans/renew.py#renew_loan), verified
  ✓ circ:circulation-svc publishes lib:LoanRenewed
  ○ lib:RenewalLimit still open (by 2026-11-30)
  ! circ:NEW:RenewalFeeWaiver proposed inside slice lib:Renewal: not in the plan
```

This needs a `realization-added` change kind in `internal/gate`, the mirror of ADR 0018's `realization-removed`. It is a report line, not a gate class: AUTO on its own, with the change's gate still decided by the views and criticality it touches, so a repository with no plan items sees one more line under Realization changes and no change in gating. The reviewer reads this block first and the code diff second, which is the review the paradigm is for: the ontology diff from the core and implementation perspective, with the code as its evidence.

### 7. Two PRs by default

The increment is two PRs: the ontology PR (curation; HARD by the existing rules when it touches core, published or critical concepts) and the implementation PR (closes plan items; AUTO or SOFT unless it deviates). `proposed: true` remains the one-PR path for the cycle back in section 6 and for changes too small to curate. A repository that wants the discipline enforced sets `policy: { plan-required: true }`, under which a concept that is new in the head, is not `planned` there, and was not `planned` in the base is HARD with the reason `concept not planned`. The curation PR, whose new concepts are planned in the head, passes; the implementation PR, which drops `planned` from a concept the base planned, passes; a concept that appears already realized with no plan behind it is HARD. The default is off.

### 8. Phases

| Phase | Ships | Done when |
| --- | --- | --- |
| P1 | ADR 0025 section 6.1 as designed: `planned` in the ontology schema and `internal/model`, out of the coverage denominator and the orphan finding, marked in `gloss context`, `AGENTS.md` and `search_concepts`. Then this ADR's additions: the long form on concepts, `planned:` in manifests, the `planned`, `planned-closed` and `plan-overdue` findings, `gloss accept --planned`, the AUTO bookkeeping class for dropping `planned` in `internal/gate`, and `planned-closed` added to this repository's explicit `drift-fail-on` list in `.github/workflows/ci.yml` | the library example's circulation repo can merge an ontology PR that plans `lib:Renewal` with CI green, and an implementation PR closes it with CI green and an AUTO gate |
| P2 | `curate-increment` skill, `plan.jsonl` read and validated by `gloss`, briefing and `impact_of` additions | a dry run on a real ticket in glossmark-examples produces an ontology PR a second person can implement from |
| P3 | Plan closure in `gloss diff` and the PR comment, `realization-added`, deviations in drift, seam rules in `check` and `publish`, closure across repositories and the `plan-overdue` warning in `gloss landscape`, `plan-required` | the implementation PR's comment shows closure, and the e2e test asserts one closed item, one open, one deviation and one closed downstream in the two-repo library example |

When P1 is built, `gm:` gains the concepts it introduces (increment, plan item, deviation) and `gmi:` the skill and the plan record, in the same change, as the self-host test requires. This repository then uses the lifecycle on itself: P2 and P3 are curated as increments before they are built.

## Walking it back

- P1 adds optional keys and finding kinds; no existing file changes meaning. A repository that never writes `planned` sees nothing new, in any phase: P3's `realization-added` is AUTO and changes no gate, and the semver rules are untouched.
- `plan.jsonl` is a new file nothing else reads.
- The skill is a file under `skills/`; removing it removes the step.
- `plan-required` defaults to off, so the two-PR discipline is opt-in until a repository has lived with it.

## Alternatives considered

- **Reuse `proposed: true` for planning.** It conflates two questions (owner acceptance and existence) and makes every curated concept HARD until implemented, which punishes the owner for doing the work early. ADR 0025 separated them for the same reason.
- **Waive the orphans and gaps a curation PR creates.** A waiver silences a finding; it does not say what the finding means, does not close itself when code arrives, and leaves the ontology with no record that the concept was ever intended.
- **Planned concepts never published.** Safe, but it removes the one reason to publish early: letting another team plan against a name. The deadline and the warning carry the risk instead.
- **Read ticket state from the tracker.** It closes plan items for the right reason, but it puts a network call and a credential into a binary that is otherwise deterministic and offline. The date does the job; a tracker reader can land later behind `--ingest`, as for harvesting.
- **A `gloss curate` command that calls a model.** Rejected for the reason ADR 0025 rejects `gloss derive`: judgement belongs in the skill, determinism in the binary.

## Open questions

- Should a planned implementation concept be optional (curation names only the core and the boundary; the agent names the mechanism) or expected? The dry run in P2 decides.
- When a planned concept is closed by a `represents` link alone (a type, no behaviour), is that closure or a `presented-only` of its own? (`verifies` alone is settled above: it does not close.)
- Does an overdue planned concept that is also published need a louder signal in the importer than a warning, given that the importer cannot fix it?
- Whether the increment's saved slice should be removed when every item closes, or kept as the feature's permanent view. Keeping it is the current lean: the view was chosen for an audience, not for the plan.
