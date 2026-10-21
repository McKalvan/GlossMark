<!-- @gloss documents gmi:SuggestCalibration scope=file -->
# ADR 0032: Three more calibration sets, four matcher fixes they reward, and held proposals

Status: accepted, 2026-10-21. Follows [ADR 0021](0021-suggest-conf-is-measured-precision.md), [ADR 0024](0024-auto-accept-precision.md) and [ADR 0030](0030-suggest-targets-deprecated-not-proposed.md). Refs issues #65 (F-CAL-1) and #69 (F-CAL-5).

## Context

After ADR 0024 and ADR 0030, the calibration set was 13 labelled workspaces with 924 proposals at `--min-conf 0.01`. The held-out estimate scores each set with the table fitted on the other sets. It auto-accepted 9.1 % of the kept proposals at 86.5 % precision, under the 0.9 target. In the held-out bands, precision fell from 78.5 % (0.7 to 0.8) to 47.1 % (0.8 to 0.9). ADR 0030 also left one inconsistency open. `--eval`, `--calibrate` and the threshold derivation read conf alone, so they counted a deprecated proposal at or above the threshold as auto-accepted. `Proposal.AutoAccepts`, the CLI's `auto` tag and the gate never do.

ADR 0021 states the rule this ADR follows: a matcher change counts only if the calibration set rewards it.

## Decision

1. **Three more sets.** Three lab workspaces from glossmark-examples (`ed53dfe`) join the calibration set under `internal/suggest/testdata/calibration/glossmark-examples/lab/`. Their READMEs and `.glossmark/` output are left out (about 430 KB).
   - `lab/cross-cutting` is `cross-cutting-concerns/variant-c-constraints`, the variant that lab's verdict recommends.
   - `lab/deep-hierarchies` is `deep-hierarchies/catalogue`.
   - `lab/monolith-split` is `monolith-decomposition/during`. It holds both sides of the split and a deprecated implementation concept.

   Some lab workspaces are left out. The variants of `ambiguous-surfaces` and the other `cross-cutting-concerns` variants mark the same code with different relations on purpose, so their labels disagree by design. `version-evolution` has three markers per workspace. The other stages of the monolith split repeat the `during` code. The set now has 16 workspaces, 958 proposals and 769 labels.
2. **SQL functions and procedures implement.** A SQL function or procedure takes the `func` role. Its relation is `implements`, so the process whose doc names it (`sp_renew_loan` in `RenewalHandler`'s doc) can be its target. Before this, it took the `table` role and `represents`. That rejected every process as a target and proposed the entity in its name instead (`sp_renew_loan represents lib:Loan`). The set has four routines. Three are labelled, all with `implements`.
3. **Terraform resources that only support another are not surfaces.** This adds to ADR 0022's list of non-surfaces. The resource types are:
   - subnet, parameter and option groups;
   - IAM roles, policies and attachments;
   - event targets and task definitions;
   - lifecycle, versioning, public-access and encryption settings;
   - KMS keys and aliases, and security groups and their rules;
   - log groups, metric alarms and permissions.

   Issue #65 names two of them (`aws_db_subnet_group.booking`, `aws_iam_role.reminders`). None of the 11 such resources in the set is labelled. The resource each one supports (the database, the bucket, the service) still is.
4. **A type named in a process's doc represents the concept it is named after.** Take a type whose `named-in-doc` hit links it with `implements` to a process, for example "the request is marked picked up and ShipmentPickedUp is published". If the type's own name is an entity or event it can represent, the name match wins (`ShipmentPickedUp represents frt:ShipmentPickedUp`). The displaced process does not mark the proposal `/tie`. No implementation concept is preferred over the represented one either. `named-in-doc` still beats a stemmed core match for functions and members, as ADR 0022 says.
5. **A type named for an implementation-tier measure implements it.** Examples are `OnTimeRateCalculator` and `NoShowReport`. An implementation measure names the code that computes the measure. A type named after a core measure still represents it.
6. **A rare basis reads its role row before its evidence row.** A basis seen fewer than `MinBasisN` (4) times has too few proposals for its own conf. It now reads the pooled precision of its evidence and role, ties included (`name-contains/table/*`), when that pool has 4 or more proposals and the basis is no tie. Otherwise it reads its evidence row, as before. Evidence on a table differs more from the same evidence on a heading than a basis differs from its tie. Rule 2 left `name-contains/table` with 3 proposals, all correct. It took `name-contains`'s evidence row at 0.14, outside the sampling error `TestCalibrationTableIsFitted` allows. A rare tie still reads the evidence row, which is lower. `calib.Table.Row` names the row a basis reads, and the fitted-table test groups rare bases by it.
7. **Tests follow links right more often than wrong.** A test verifies the target of a symbol it calls when suggest linked that symbol with conf at least 0.5. The bar was the 0.3 floor. When refitting moved `name-partial/func` from 0.25 to 0.33, the tests followed every weak name match. `calls/test` grew from 32 proposals to 98, at 29 % precision, and the next refit would have moved it again. At 0.5 the table is a fixed point.
8. **Held proposals.** `calib.Count` gains `Held` and `HeldCorrect`: the proposals of a basis with a deprecated target, and how many of those were correct. They count toward the basis's conf, as ADR 0030 decided. They are never auto-accepted, so they are excluded from the threshold's precision and from its auto-accepted count. The share still counts them in its denominator.
   - `Fit` counts held proposals.
   - `Derive` and `At` subtract them.
   - `Evaluate` and the held-out harness count a proposal as auto-accepted only when `Proposal.AutoAccepts` says so.
   - The calibration file has two more columns, held and held correct. A file with three columns still reads.

   `--eval`, `--calibrate`, the threshold derivation and `gloss suggest` now agree.
9. **Refit.** `internal/calib/shipped.go` holds the counts of the 16 sets. The derived default threshold moves from conf 0.80 to 0.82.

## Measured (2026-10-21)

Pooled, at the default `--min-conf 0.3` and the default target 0.9:

| | sets | proposals (labels) | correct | kept | in sample: auto-accepted, precision | held out: auto-accepted, precision |
| --- | ---: | ---: | ---: | ---: | --- | --- |
| before (base branch) | 13 | 924 (715) | 250 | 237 | 55 of 237 (23.2 %), 94.5 % | 37 of 406 (9.1 %), 86.5 % |
| before, with the three new sets | 16 | 972 (769) | 273 | 258 | 63 of 258 (24.4 %), 95.2 % (table not refitted) | 56 of 438 (12.8 %), 87.5 % |
| after | 16 | 958 (769) | 278 | 326 | 72 of 326 (22.1 %), 94.4 % | 56 of 431 (13.0 %), 92.9 % |

"Kept" is in sample. Held out, each set keeps what the other sets' table gives a conf of at least 0.3. The two "before" rows counted deprecated proposals as auto-accepted. The "after" row does not.

Held out, by conf band:

| band | before (13 sets) | after (16 sets) |
| --- | ---: | ---: |
| < 0.5 | 18.3 % (688) | 20.2 % (718) |
| 0.5 to 0.6 | 29.9 % (67) | 15.2 % (46) |
| 0.6 to 0.7 | 18.8 % (48) | 44.0 % (75) |
| 0.7 to 0.8 | 78.5 % (79) | 63.9 % (61) |
| 0.8 to 0.9 | 47.1 % (17) | 92.6 % (27) |
| >= 0.9 | 100 % (25) | 93.5 % (31) |

In sample, precision rises with every band: 15.5 %, 51.2 %, 61.9 %, 74.7 %, 91.3 % and 100 %. A threshold sweep from 0.3 to 0.9 changes the selected set at every 0.1 step, both in sample and held out.

Per set, held out at the 0.9 target:

- **Nine sets meet it.** circulation, clinic-monorepo, clinic-booking (10 of 39 auto-accepted), claims-events, shipment-core, tracking, and the three lab sets.
- **Three miss it.**
  - kitchen-agents is at 83.3 % (5 of 6). Its miss is `find_conflicts`: a concept's doc names it, but the reviewer marked only the other function.
  - library-legacy is at 86.7 % (13 of 15). Its misses are `FineCalculator.overdueFine`, which is named in a doc while the reviewer marked the class, and the `FineCalculator` class, which ties between two docs.
  - The repository itself auto-accepts 1 proposal, and it is wrong: `DefaultPolicy`, which a measure's doc names because it holds the measure's value.
- **Four auto-accept nothing.** holds-worker, warehouse-legacy, dispatch and opstool.

The change to the matcher cut the high-conf errors that held out showed:

- The `0.8 to 0.9` band rose from 47 % to 93 %.
- The pooled held-out precision rose from 87.5 % to 92.9 % on the same 16 sets.
- The held-out share stayed at about 13 %.

Two changes were tried and dropped because the harness did not reward them:

- **No named-in-doc to a concept with a published value.** It fixed `DefaultPolicy` but lost a correct `requireWithinFilingWindow implements tide:FilingWindowRule`, a net zero.
- **A role row for rare ties too.** Held-out precision fell to 89.8 %.

`MinBasisN` 2 or 3 looked better on this matcher (held out 93.5 % and 93.2 %), but worse on the base branch (81.0 % at both, against 84.2 % at 4, with rule 6). It was left at 4 rather than tuned to the data.

## Consequences

- **#69.** Its done-when holds in sample: the bands are non-decreasing, conf >= 0.9 is 100 % correct, and the sweep changes at every 0.1 step. Held out, conf >= 0.9 is 93.5 % correct and the sweep changes at every step. One dip remains at the bottom (20.2 % under 0.5, 15.2 % from 0.5 to 0.6), below any threshold a target derives. It does not affect auto-accept.
- **#65.** Under ADR 0024's per-repository reading, the default target is met held out on nine of the twelve sets that auto-accept anything. clinic-booking, the codebase #65 names, is one of them. The pooled held-out precision now meets the target (92.9 %), but the share it buys is 13 %. The fixed 80 % share of the original Phase 2 gate is far away. ADR 0024 dropped it as a gate, and PLAN.md says so. **The Phase 2 gate is not recorded as met.** A repository still meets its configured precision only on its own eval (`gloss suggest --calibrate`).
- `gloss suggest --calibrate --write` writes five columns. A calibration file written before this change reads as before, with no held proposals.
- The default threshold is 0.82. A repository with no `policy:` threshold auto-accepts slightly less than before, at about the same precision.
- The SQL routine and Terraform rules are in `gloss suggest -h` and MARKING.md.
