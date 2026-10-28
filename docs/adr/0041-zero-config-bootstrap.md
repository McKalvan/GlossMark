<!-- @gloss documents gmi:Bootstrapper scope=file -->
# ADR 0041: a zero-config bootstrap drafts everything as proposals

Status: accepted, 2026-10-28. Closes issue #257. Amends [ADR 0030](0030-suggest-targets-deprecated-not-proposed.md) (decision 5) for one command.

## Context

Before GlossMark shows a new user anything, they write YAML: namespaces, two tier files, a manifest per service, markers. The pieces for a fast first result already exist, each a command of its own that needs some prior knowledge: `gloss init` scaffolds, `gloss import` reads a glossary or an enums file, `gloss harvest --cluster` lists candidate concepts, `gloss suggest` proposes markers once concepts exist, `gloss view` draws the result. A first-time user does not know the order, and the `derive-ontology` skill that does needs an agent.

The adoption review of 2026-10-28 (#256) asked for one command on an unmodified repository that ends with the viewer open on recognisable services and domain concepts, with nothing enforced until a person accepts it.

## Decision

1. **One command, `gloss onboard`, chains the existing packages.** scaffold plans and writes what `gloss init` would (without the agent hook; `gloss hook install` is a separate decision); importers reads a `glossary.md` or an `enums.yaml` it finds; harvest clusters the vocabulary; `internal/bootstrap` turns the clusters into concepts and the deploy manifests and Terraform into a boundary map; suggest writes the markers; `gloss view` renders the viewer. Nothing is re-implemented: a better harvest, importer or suggester improves the draft.
2. **Everything written is a proposal.** Every concept carries `proposed: true` and `proposed-by`, every marker is `@gloss?`, and every `realizes:` the command writes into a manifest sits under a comment saying it is a draft. `gloss check` passes on the result (a proposed implementation concept without `realizes:` is a warning, GM320, not an error), `gloss status` and `gloss review` list it all, and the gate classes every link to a proposed concept HARD. Nothing is enforced until `gloss accept`.
3. **Which candidates become concepts.** Harvest's ranking is kept: candidates named by more kinds of signal and seen by more families first. A corroborated, docs-only or code-only-visible candidate goes to the core tier when at least two families observe it or it is named twice; a code-only-internal one goes to the implementation tier as `internal: true` when it is named three times by two families. The name is the cluster's best surface, singularised, in PascalCase, with engineering affixes dropped (CirculationService is the circulation); a bare affix, a transport (SMS, SQS) or a phrase with an article or a conjunction is no concept. The kind comes from the name's ending, then from what named it (a topic is an event, rule sentences a constraint, anything with a table or a type an entity). The doc is a placeholder that says where the term was seen and asks to be replaced. `--top` caps the core tier at 40 by default: enough to recognise the domain, few enough to review in one sitting.
4. **The boundary map.** scaffold's directories are the boundaries. A Kubernetes workload (Deployment, StatefulSet, DaemonSet, CronJob, Job, Rollout) or a Compose service with a `build:` becomes a proposed `system-type`; it is matched to the directory whose name its own contains (reminders-worker is built from services/reminders), and that directory's manifest and the one holding the deploy file both get `realizes: [it]`. A Terraform resource that is a thing of its own (a queue, a database, a bucket, a cluster; not a role, a policy, a subnet group, a task definition or a lifecycle configuration) becomes a proposed `internal: true` resource the infrastructure directory realizes. A directory that holds code and no deploy file names gets a system-type of its own, so the map has a service in every box. A system-type realizes the core processes only its directory names, when the harvest can tell.
5. **suggest targets proposed concepts under onboard.** ADR 0030 excludes proposed concepts from suggest's targets because their names are unreviewed and proposals built on them were noise in a brownfield patch. Under onboard every concept is proposed, so the rule would leave the draft without a single link. `suggest.Options.Proposed` includes them; the gate already classes each such link HARD, so the links are reviewed with the concepts. `gloss suggest` on the command line keeps the ADR 0030 rule.
6. **The output ends with one next step.** The last line names the viewer and `gloss review`. The command is re-runnable: an existing concept, a manifest with `realizes:`, a marked declaration and a scaffolded file are left alone.

## Measured

On the three single-repository showcases of glossmark-examples with their GlossMark files stripped (`glossmark.yaml`, `.gloss.yaml`, `ontology/`, `.glossmark/`, every `@gloss` line), each run takes about one second:

- **clinic-booking:** 24 core concepts (Booking, Practitioner, Appointment, Slot, Patient, Reminder, Clinic, Cancellation, LateCancellation, Availability, Waitlist, NoShowRate among them), 7 implementation concepts (BookingApi, RemindersWorker, WebService, NoshowReportService from the Kubernetes manifests; BookingDatabase and ReminderEventsQueue from the Terraform), `realizes:` in five manifests, 13 proposed links; check 0 errors, 4 warnings.
- **claims-events:** 33 core concepts (Claim, Assessment, FiledClaim, Withdrawal, Referral, Payout, FilingWindow, FraudAlert), 5 implementation concepts (one per Deployment), 7 links; 0 errors, 2 warnings.
- **library-legacy:** the glossary (27 entries) and the enums file (20) imported as proposals, 28 more core concepts from the harvest, the ECS cluster, database, services and the notices bucket from the Terraform, a system-type per code directory; 0 errors, 0 warnings.

The tail of each core list is noise (a config key, a UI label, a report name): the draft is meant to be reviewed, not accepted whole.

## Consequences

- A new user runs one command and sees a map. The vocabulary they then review is what the tooling could infer, named in the words their own code, docs and schemas use, with a placeholder doc on every entry.
- `gloss review` opens on a long queue: every concept and link. The HARD class on links to proposed concepts and the `proposed concept awaiting acceptance` note say why.
- The agent hook is not installed; `gloss init` still installs it, and `gloss hook install` does on its own.
- Not done here: `--dry-run`, mapping markers from manifests instead of inserting `@gloss?` lines (#229), Helm charts and Compose files without `build:`, and a `realizes:` for a system-type from anything but the harvest's unit attribution. A monorepo that imports its core from a sibling is set up with `gloss init --import` first; onboard then drafts only the implementation tier and the map.
