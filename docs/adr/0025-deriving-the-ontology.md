<!-- @gloss documents gm:Onboarding scope=file -->
# ADR 0025: Deriving the core and implementation ontology from an existing application

Status: proposed, 2026-10-21. A design to workshop, not yet built. Extends the `adopt-glossmark` skill (steps 1, 3 and 4) and [ADR 0016](0016-adoption-scaffold-and-imports.md).

## Context

GlossMark assumes every application has a platonic **core** ontology (what the business is about) and an **implementation** ontology (how that was mapped onto code, configuration, infrastructure and docs). Almost every adopter arrives with the second and not the first: an application, often several, with some README files, a product spec somewhere, a schema, tests, and a vocabulary that lives in people's heads.

Today the path from that state to a core tier is one sentence of the adoption skill ("read the README and docs, note the 10 to 40 terms") plus two helpers: `gloss import` for a glossary or enum file that already exists, and `gloss suggest --concepts`, which clusters declarations into `NEW:` candidates. The [adoption review](../reviews/2026-10-18-adoption-review.md) found that both real adoptions were "an agent reading QUICKSTART.md and inventing the ontology from the code in one pass", and that this is the real onboarding path. One pass has three failure modes that the rest of GlossMark then inherits:

- **The core copies the class diagram.** Names come from code, so the core says `LoanRecord` where the business says *loan*, and implementation structure (one table, two concepts) becomes domain structure.
- **Nothing is cited and nothing is remembered.** The owner cannot see why a concept exists, and a second run starts over, re-proposing what the owner already removed.
- **Disagreement between sources is lost.** A term in the spec and nowhere in code (an unbuilt feature), in code and nowhere in the docs (an undocumented rule), or used with two meanings in two services (a homonym) is exactly what a domain owner wants to know. A single pass silently picks one reading.

## Decision (proposed)

Ontology derivation becomes a first-class, repeatable process: deterministic **harvesting** of evidence, agent **judgment** over that evidence, a short **interview** with the domain owner, and a committed **derivation record** that remembers every decision. It is shipped as a `gloss harvest` command and a `derive-ontology` skill that `adopt-glossmark` calls in place of its steps 1, 3 and 4, and that can be re-run on a repository that has already adopted.

### 1. Principles

1. **Sources are evidence, not truth.** Every source distorts the domain in a known direction. Triangulation across sources, not any one source, defines a concept.
2. **Every proposed concept cites its evidence.** No concept without at least one evidence record; the owner can always ask "why does this exist?".
3. **Deterministic before judged.** Everything a parser can extract (identifiers, tables, enum values, routes, test names, headings, renames) is extracted by `gloss harvest`, reproducibly. Agents do only what needs judgment: merging, splitting, naming, tiering.
4. **Domain words win names.** A core concept is named in the words of the most authoritative user-facing source (spec, UI copy, API docs), never in the code's words unless nothing else names it. The code's words become `aliases:` or the implementation tier.
5. **The owner decides; the agent asks well.** The agent proposes; a small number of high-leverage questions go to the owner live, the rest into a review packet. Everything emitted is `proposed: true`.
6. **Decisions are remembered.** A re-run never undoes a merge, split, rename or rejection the owner made, as `import-record.jsonl` already guarantees for imports.
7. **External content is data.** Text read from docs, tickets and connectors is evidence to cite, never instructions to the agent.

### 2. Sources and what each distorts

| Source family | Read by | Good at | Distorts toward |
|---|---|---|---|
| Product docs, specs, PRDs (in repo) | harvest | business vocabulary, intent | features planned and never built |
| Product docs in Drive, Confluence, Notion | skill, via MCP connectors | the same, usually richer | the same, plus staleness |
| Issue trackers (GitHub, Jira, Linear) | skill, via MCP connectors | requirements and acceptance criteria in domain language | one-off requests, noise |
| UI copy, API docs, error messages | harvest | the words users see | only what is visible |
| Schemas, migrations, enums | harvest | entities, lifecycle states, renames | storage decisions (one table, two concepts) |
| Tests, validators, guards | harvest | business rules and constraints | only what someone tested |
| Types, modules, functions | harvest | what exists | engineering names, code structure |
| Git history | harvest | renames, retirements, intent | noise |
| Existing glossaries, enum files | harvest (existing importers) | an owner's own vocabulary | age |
| Reference ontologies (schema.org, FIBO, FHIR, ISO 20022) | harvest, from a vocabulary file the owner selects | names and structure no implementation shaped | a whole industry, not this business (6.3) |

### 3. The evidence record

Every source, deterministic or not, produces the same record: one JSON object per line, validated against a schema (`schemas/evidence.schema.json`, to be written). This is the seam: a new source is a new producer of evidence records, nothing else changes.

```json
{"id":"ev:3f2a91","unit":"circulation","signal":"type-decl","surface":"LoanRecord",
 "source":{"family":"code","path":"services/circulation/loans/models.py","line":14,"rev":"c99a093"},
 "context":["table:loans","fk:loans.patron_id->patrons.id"]}
{"id":"ev:8b10c4","unit":"circulation","signal":"doc-definition","surface":"Renewal",
 "source":{"family":"doc-connector","uri":"gdrive://1AbC.../Circulation PRD","locator":"heading:Renewals",
           "modified":"2025-03-01","hash":"sha256:9e1f..."}}
```

- `signal` is one of a closed list: `type-decl`, `function-decl`, `inherits`, `enum-type`, `enum-value`, `table`, `column`, `foreign-key`, `schema-type` (proto, Avro, GraphQL, OpenAPI), `route`, `topic` (queue, event, cron), `test-name`, `guard-message`, `ui-string`, `doc-heading`, `doc-definition`, `glossary-entry`, `rename`, `issue-title`, `acceptance-criterion`.
- `unit` is the application the evidence belongs to (section 7).
- **Quotes are not committed.** A record keeps a locator and a hash, never the source text, unless the owner opts in (`derive: { commit-quotes: true }` in `glossmark.yaml`). Connector content stays out of the repository by default; concept docs are written by the agent in its own words.

### 4. `gloss harvest` (deterministic)

`gloss harvest [--unit <path>] [--since <rev>] > evidence.jsonl` walks the workspace with the loader's include and exclude rules and emits evidence records. It reuses what already exists: the declaration detector (`internal/marker`), suggest's tokeniser, stemmer and stoplist (`internal/suggest`), init's deployable detection for units (ADR 0016), and the glossary and enum importers.

- **Code:** type and function declarations, inheritance, enum types and values, exported constants.
- **Data:** SQL DDL and migrations (tables, columns, foreign keys), ORM models, `.proto`, `.avsc`, `.graphql`, `.prisma`, OpenAPI.
- **Interfaces:** HTTP routes, CLI commands, topic and queue names, scheduled job names.
- **Behaviour:** test names and `describe`/`it` strings, validation and exception messages.
- **Surfaces:** i18n string files, page and route titles.
- **Docs:** Markdown headings, definition lists, bold-term definitions, ADR titles.
- **Git:** `ALTER TABLE ... RENAME`, file and type renames across history, first and last commit a term appears in (candidates for `deprecated` and `replaced-by`).

`gloss harvest --ingest <file>` validates evidence produced elsewhere (by the skill's connector readers) against the schema and merges it, so every record the skill reasons over has passed the same validation.

`gloss harvest --cluster` groups surfaces lexically (stem, CamelCase split, singular and plural, shared tables and foreign keys, co-occurrence in one doc section) into candidate clusters. Lexical clustering only proposes; the agent decides.

### 5. The `derive-ontology` skill (judgment)

| Phase | Who | Output |
|---|---|---|
| 1. Survey | agent | units (section 7); source inventory with family, authority and age; asks the owner which external docs are authoritative and which are stale |
| 2. Harvest | `gloss harvest`; one subagent per connector or tracker in parallel, each writing evidence through `--ingest` | `evidence.jsonl` (scratch, regenerable) |
| 3. Cluster | `gloss harvest --cluster`, then the agent merges and splits | candidate concepts, each with its evidence |
| 4. Canonicalise | agent | one name per concept from the highest-authority user-facing source; other surfaces as `aliases:`; homonyms split and qualified by unit or context |
| 5. Tier | agent | core, implementation (with the core concept it `realizes`), instance, or noise. Test: *would this exist if the business ran on paper?* |
| 6. Structure | agent | `kind`, `is-a` (from inheritance, discriminator columns, enum families), `part-of` (from foreign keys and composition), constraints (from guards and tests), events (from topics) |
| 7. Triangulate | deterministic over the clusters | per concept, which source families agree (section 6) |
| 8. Critique | a fresh agent that sees the draft and evidence but not the drafter's reasoning | findings against a fixed checklist (below) |
| 9. Interview | agent with the owner, live | answers to the highest-leverage questions, recorded as decisions |
| 10. Emit | agent, then `gloss check` | tier files with `proposed: true`; the derivation record; the review packet; seeds for `gloss suggest` |

**Critique checklist.** Core names that equal a type name with no non-code evidence (the class-diagram smell); a core concept that names a technology, table or framework; an implementation concept that realizes nothing and is not `internal`; a core tier over about 100 concepts in one namespace (a sign of several bounded contexts); a concept whose only evidence is one test name or one heading; two concepts whose evidence sets overlap by more than half (a missed merge); one surface with evidence in two units whose contexts share nothing (a missed homonym).

**Interview.** At most about ten questions live, ranked by leverage: the evidence records and concepts an answer would change, weighted by disagreement between sources. Question types are fixed so answers map to decisions: *same thing?* (merge), *different things?* (split), *core or implementation?*, *which name?*, *is this doc stale or is the code wrong?*, *is this a concept or an instance?* Everything below the cut goes into the review packet as a ranked decision list.

**Who answers.** The person answering is the namespace's `owner` (the `owner:` of its `owns:` entry in `glossmark.yaml`). Derivation runs before `init`, so the survey phase asks who owns each proposed namespace and writes that into the namespace plan (section 7). Live questions go to the owner in the session. Each packet question carries a `route-to:` naming the stakeholder best placed to answer it (the product manager for checkout, the compliance lead for a constraint), and can be answered in the PR, so an owner who is not in the session loses nothing. Every decision records its actor and whether the owner confirmed it. When the person deciding is not the namespace owner (a solo developer, or an engineer standing in), the decision stands but is marked `owner-confirmed: false`. `gloss status` counts unconfirmed core decisions, so business sign-off is visible debt rather than a blocker.

### 6. Triangulation is the main output

For each candidate concept, the record states which source families evidence it. The categories are findings the owner reviews, not just inputs:

| Evidence | Reading | What derivation does |
|---|---|---|
| docs and code agree | corroborated | propose at high confidence |
| docs only | intended, not built; or a stale doc | propose as core with `planned: true` (section 6.1); ask if the doc may be stale |
| code only, user-visible (UI, route, error) | undocumented domain concept | propose as core; ask for a definition |
| code only, not user-visible | implementation, or an undocumented rule | propose as implementation, `realizes` the nearest core concept or `internal` |
| one surface, two meanings | homonym | split; qualify by unit or context; ask |
| two surfaces, one meaning | synonym | merge; canonical name from the domain source, the rest as `aliases:` |
| renamed in history | retired name | old name as alias, or a `deprecated` concept with `replaced-by` |

#### 6.1 Planned concepts

A concept the specification describes and nothing implements is real domain knowledge, and an agent about to build the feature should link to it rather than mint a second name. It is also not a defect, so it must not count against coverage. The core tier gains a structural key:

```yaml
Renewal:
  kind: process
  planned: true          # specified, not yet implemented
  doc: "Extending a loan's due date before it falls due."
```

- `planned` is allowed on core concepts only; an implementation concept is a mechanism, which exists or does not.
- It is orthogonal to `proposed`. `proposed` is about review (has the owner accepted the entry?), `planned` is about existence (is there anything behind it?). A derived docs-only concept starts as both.
- `gloss coverage` leaves planned concepts out of the denominator and `gloss drift` leaves them out of orphans. Drift lists them on a line of their own, and once an artifact `implements`, `represents` or `verifies` one, drift reports *planned concept now implemented: drop `planned`*, which keeps the flag from going stale.
- Agents see planned concepts in `AGENTS.md`, `gloss context` and `search_concepts`, marked as planned, so the agent that builds the feature finds the owner's name for it.
- A `planned` concept is to the ontology what a manifest gap is to a boundary: intended capability not yet built. The difference is who intends it: the business (planned) or the architecture (gap).

#### 6.2 Concepts and instances

Enum values and configuration split by what the business does with them:

- **Lifecycle and kind values are concepts.** A state the business reasons about and moves things between (`Overdue`, `Returned`, `Authorised`, `Settled`) or a kind it treats differently (`Book`, `DVD` as kinds of `Item`) becomes a core concept that `is-a` its enum, as the enum importer already does.
- **Catalogue values are instances.** Values the business configures rather than reasons about (plan names, regions, currencies, feature flags, branch locations) are instances of a concept (`Plan`, `Region`), not concepts. Instances have no file format yet ([PLAN.md](../PLAN.md), deferred with `shapes:`), so derivation records them in the derivation record as `instance-of <concept>` and emits only the concept.
- **Signals.** Lifecycle: the enum is a `status` or `state` column, it is assigned in several places, it is compared in guards or tests, or it names transitions (`renew`, `return`). Catalogue: the values appear in seeds, fixtures, configuration files, pricing tables, or a localised display-name list, and code mostly looks them up rather than branching on them. The agent decides from these signals; an enum family with mixed signals becomes an interview question.
- `gloss import --from enums` needs a matching option (`--values instances` for a named enum) so that an imported catalogue enum does not add its values as concepts. That is a follow-up to ADR 0016, decision 10.

#### 6.3 Reference ontologies

A core derived from one application's material is still shaped by that application. Published industry vocabularies are the one source that no implementation of *this* business shaped, so they are closest to the platonic core. They are used narrowly:

- **Opt-in, by file.** The owner selects a reference vocabulary in the survey phase; `gloss harvest --reference <file>` reads it (RDF/OWL or JSON-LD) into `reference-term` evidence. Nothing is fetched implicitly, and each vocabulary's licence is checked before it is bundled or recommended.
- **Never evidence of existence.** A reference term with no evidence from the user's own material is not proposed. Derivation does not import FIBO; it borrows from it.
- **Tie-breaker for names.** When the user's own sources disagree on a name (`Customer`, `Client`, `Party`), a reference term that matches one of them breaks the tie. The other surfaces stay aliases.
- **Structure hints.** A reference `subClassOf` or part-whole relation between two concepts that both have their own evidence is proposed as `is-a` or `part-of`, at the confidence its kind of evidence measures (8.1).
- **Mappings.** A matched concept records `exact-match` or `close-match` (SKOS semantics) to the reference IRI in the derivation record. These are the seed for the SSSOM mappings deferred in [PLAN.md](../PLAN.md), so an ontology derived today can be exported as mapped to an industry standard later without a second alignment.

### 7. Several applications

The unit of derivation is the **application**, as `gloss init` already defines one: a deployable with a project file (ADR 0016, decision 1). A single-application repository is the case of one unit. Derivation runs per unit and then an **alignment** phase compares units:

- the same concept in two or more units, with agreeing evidence: a **shared** core concept, a candidate for a shared, `published` core namespace;
- the same surface with different meanings: a **homonym across contexts**, giving separate core namespaces (bounded contexts), not one concept;
- different surfaces with one meaning: a **translation**, giving one shared concept with the local names as aliases.

Alignment outputs a namespace plan: which core namespaces exist, which are published, which units import which. That plan is the input to `gloss init --core/--impl/--import`. **Alignment runs before `init` mints IRIs**, because ADR 0016 makes namespace IRIs permanent and a homonym found after markers exist is a rename across the codebase.

Scope by phase: units inside one workspace (a monorepo) in the first version, because the data model is the same and cross-application homonyms are the most valuable and least reversible finding. Sibling repositories by local path, the same mechanism as `init --import`, follow next. Remote fetch waits for the import fetch already deferred in [PLAN.md](../PLAN.md).

### 8. The derivation record

`.glossmark/derivation.jsonl` (committed, format 1) remembers decisions, not evidence:

- per concept: the surfaces it claims, the units it spans, its tier, and the fingerprints (id and hash) of the evidence it was derived from;
- per decision: merge, split (with the discriminating unit or path glob), rename, tier, reject-as-noise, instance; the actor (`human:<id>` or `agent:<name>`) and the date.

On a re-run, new evidence whose surface a concept already claims attaches to that concept; a rejected surface stays rejected; a split stays split. The re-run reports what changed: **new terms** (evidence with no concept), **lost evidence** (a concept whose code evidence disappeared, a candidate for `deprecated`), and **changed sources** (a doc whose hash moved since the concept was accepted). Run on a repository that has already adopted GlossMark, this is ontology drift at the product level, alongside `gloss drift` at the code level.

#### 8.1 Confidence and auto-accept

Every derived concept, alias, tier assignment and structural edge carries a `conf`, and as with `gloss suggest` (ADR 0021) `conf` is not a feeling: it is the precision its kind of evidence measured on the gold sets (section 10). A decision's kind of evidence is a small, fixed feature set: which source families corroborate it, which family supplied the name, whether a reference ontology matched, and whether the tier test was unanimous across those families. On a gold set, a derived concept counts as correct when it matches a gold concept by name or alias in the same tier.

As in ADR 0024, a repository chooses a precision (`derive: { auto-accept-precision: 0.9 }` in `glossmark.yaml`), and the threshold on `conf` is derived from it and printed with the precision measured. At or above it a decision is auto-accepted: written without `proposed: true` and recorded in the derivation record and ledger with the agent as actor and no reviewer, exactly as agent-written links already are. The owner still sees it in the review packet, collapsed under "accepted automatically", and can reverse it.

Auto-accept never applies to the decisions that make the core the business's rather than the agent's:

- a homonym split, or a merge across applications (section 7);
- a concept the alignment phase proposes as `published`, since publishing is the HARD seam;
- a concept with `criticality: high` or `critical`, or a `constraint` of any criticality;
- a `planned` concept, whose only evidence is a document that may be stale.

These always go to the owner. Until a repository has run `gloss suggest --calibrate`'s derivation counterpart on its own reviewed output, the shipped threshold comes from GlossMark's gold sets, and the packet says so.

### 9. Hand-off to linking

Code-family evidence is a location with a concept attached, so it is already a `@gloss?` candidate: a `type-decl` in a concept's cluster suggests `represents`, a `route` suggests `implements`, a `test-name` suggests `verifies`, a `doc-heading` suggests `documents`. `gloss suggest` reads the derivation record as a new kind of evidence, with its precision measured on the calibration sets like every other kind (ADR 0021), and the aliases derivation finds widen the matches suggest already makes.

### 10. Evaluation

Derivation is measured like `suggest`: on held-out repositories with a known answer.

| Gold set | Hidden for the run | Tests |
|---|---|---|
| AgentArena | `docs/ontology/` and the vocabulary sections of `CLAUDE.md` | recovery of an owner-written glossary, enums and invariants from code and remaining docs |
| GlossMark | `ontology/` | core versus implementation on a tool whose core is abstract |
| `examples/library` (two repositories) | both ontologies | alignment: shared `lib:` core, `circ:` extension, cross-repo homonyms |
| pfinance | its adopted ontology, after owner review | the docs-poor case |
| `glossmark-examples` lab layouts | their ontologies | breadth of layouts and languages |

Metrics: concept recall and precision against the gold set (matched by name or alias through a labelled mapping, so the score is deterministic once labelled); tier accuracy; synonym and homonym F1; `kind` accuracy; interview efficiency (concepts changed per owner answer); and downstream, `gloss suggest` coverage and precision with and without derived aliases. An ablation by source family (code only; code and repo docs; plus git; plus connectors and trackers) shows what each source buys, which is the evidence for whether the connectors are worth their setup.

### 11. Phases

| Phase | Ships | Done when |
|---|---|---|
| D1 | `planned:` in the ontology schema, coverage, drift and the agent surfaces (6.1); evidence schema; `gloss harvest` over repo and git; `--cluster`; the `derive-ontology` skill for the units of one workspace, with interview, packet, record and alignment; `adopt-glossmark` steps 1, 3 and 4 call it | on AgentArena with `docs/ontology/` hidden, recall and precision of core concepts are measured and reported; a re-run after owner decisions changes nothing |
| D2 | connector and tracker readers through `--ingest`; authority and staleness weighting; `suggest` reads the record | the ablation table in section 10 is filled in |
| D3 | alignment across sibling repositories; the namespace plan feeds `gloss init`; refresh mode for adopted repositories | `examples/library` derives `lib:` as shared and published, and `circ:` as its extension |

When D1 is built, `gm:` gains the concepts it introduces (evidence, derivation, the derivation record) and `gmi:` the harvester and skill, in the same change, as the self-host test requires.

## Alternatives considered

- **A pure skill, with agents reading everything directly.** Fastest to ship, but not reproducible, not calibratable and expensive to re-run; a second run would not see the same evidence.
- **A model call inside `gloss` (`gloss derive`).** Self-contained, but it puts an API key, a model choice and non-determinism into a binary that is otherwise deterministic and offline.
- **Derive the core from code alone.** It is the class-diagram failure the review found; docs, UI copy and history are what separate the domain from its mapping.
- **Commit the evidence.** It is large, regenerable, and for connector sources possibly confidential; the record keeps fingerprints instead.

## Open questions

Settled while workshopping: docs-only concepts are `planned` (6.1); lifecycle and kind values are concepts and catalogue values are instances (6.2); reference ontologies break naming ties and seed mappings but never add concepts (6.3); the namespace owner answers, packet questions route to stakeholders and can be answered in the PR, and stand-in decisions are marked unconfirmed (section 5); derived decisions carry a calibrated `conf` with auto-accept above a repository's chosen precision, except for the decisions listed in 8.1.

1. What is a sensible concept budget per core namespace before derivation insists on splitting into bounded contexts?
2. How does naming work when the docs are in one language and the code in another?
3. Which reference vocabularies are worth bundling, and under which licences?
