<!-- @gloss documents gmi:SuggestCalibration scope=file -->
# ADR 0024: A repository sets the precision auto-accept must meet

Status: accepted, 2026-10-20. Follows ADR 0021. Refs issues #65 (F-CAL-1) and #69 (F-CAL-5). [ADR 0032](0032-suggest-calibration-set-grows.md) adds three calibration sets, holds deprecated proposals out of the threshold, and remeasures (default threshold 0.82; held out, 13.0 % auto-accepted at 92.9 %).

## Context

ADR 0021 made `conf` the precision a proposal's evidence measured on a labelled calibration set, and left the Phase 2 gate unmet: at least 80 % of proposals auto-accepted at at least 90 % precision. The gate still compared `conf` to `policy.soft-conf-threshold`, a raw score a team had to guess. The calibration set was six workspaces, two of them fixtures written alongside the matcher.

The user decided:

- A team chooses the precision auto-accept must meet, not a conf.
- The tool derives the threshold from the calibration.
- The gate is per repository.
- Suggest's settings live in `glossmark.yaml`.
- The calibration set grows with the hand-marked workspaces of glossmark-examples.

## Decision

1. **`suggest.auto-accept-precision`** (0 to 1, default 0.9) is the precision auto-accepted proposals must reach. The threshold is the lowest conf whose cumulative precision meets the target on the calibration data. Cumulative precision counts correct proposals over all proposals at or above that conf, among those at or above `min-conf`. A proposal at or above the threshold is an auto-accept candidate. The gate classes it SOFT unless another rule raises it, and `gloss accept --policy` accepts it. Below the threshold it is HARD with `conf X below threshold Y`. When no conf meets the target, the threshold is above every conf (1.01): nothing is auto-accepted on conf, and `gloss doctor` and `gloss suggest` say so.
2. **The calibration data** is the shipped table (`internal/calib/shipped.go`). It now holds counts per basis (proposals, correct), not fitted values. Conf is still `(correct + 1) / (n + 2)` of a basis seen four times, else of its evidence row. When the file named by `suggest.calibration` exists, the threshold is derived from its counts instead, and conf still comes from the shipped table.
3. **Precedence.**
   - `suggest.auto-accept-precision`, when set, derives the threshold.
   - Otherwise `policy.soft-conf-threshold`, when set, is the threshold as written. This was the rule before this ADR, and `gloss init` wrote it, so existing repositories keep their gate.
   - With neither set, the default precision 0.9 derives the threshold.
   - When both are set, the precision wins and doctor warns.

   `Config.EffectivePolicy().SoftConfThreshold` is the threshold in force, so every gate caller reads it. The rule also applies to a `conf=` an agent writes by hand, which is read as its estimated precision.
4. **Repository config.** `suggest:` in `glossmark.yaml` holds `auto-accept-precision`, `min-conf`, `stopwords`, `labels` and `calibration`:
   - `labels` is the labels file `--calibrate` scores against;
   - `calibration` is the measured counts file.

   A flag overrides each one (`--auto-accept-precision`, `--min-conf`, `--eval`). `gloss init` writes the block commented out at its defaults, instead of `policy: { soft-conf-threshold: 0.7 }`. The values are validated when the file is read.
5. **`gloss suggest --calibrate`** runs the harness on the repository:
   - it copies the workspace's files to a temporary directory and removes every `@gloss` line, keeping ontology files and manifests whole;
   - relative import sources are rebased to the original, so imports from a sibling checkout still resolve;
   - it suggests on the copy with every conf kept, and scores the result against the repository's own accepted links, or against `suggest.labels` or `--eval`. A `@gloss?` proposal is not a label;
   - `--write` saves the counts to `suggest.calibration`, and from then on the threshold is derived from the repository itself.
6. **Reporting.**
   - `gloss suggest` prints the rule on stderr: the threshold, its measured precision and the share of proposals it auto-accepts, or "auto-accept: nothing". It also prints how many of this run's proposals reach the threshold.
   - `--format text` tags each line `auto` or `review`.
   - `--format json` is now an object, `{"auto_accept": {...}, "proposals": [...]}`, and each proposal carries `auto_accept`. `proposals` is `[]`, never null, and is what ADR 0022's `[]` now refers to.
   - `gloss suggest --eval` and `--calibrate` print the rule in force, what it auto-accepts on this eval and at what precision, and the threshold this eval alone would derive.
   - `gloss doctor` has an `auto-accept` line.
7. **The Phase 2 gate is per repository:** a repository's configured auto-accept precision is met on its own eval. `Report.GateMet` checks that the proposals the rule in force auto-accepts are at least the target correct. Auto-accepting nothing does not meet it. The fixed 80 % share is dropped. The share is what the chosen precision buys, and it is reported next to it.
8. **More calibration data.** Eight workspaces copied from [glossmark-examples](https://github.com/McKalvan/glossmark-examples) at `ed53dfe` join the calibration set: `clinic-booking`, `claims-events`, `kitchen-agents`, `library-legacy`, the three `freight-federation` repositories and the concept-poor `opstool`. They live under `internal/suggest/testdata/calibration/glossmark-examples/`, without their READMEs, demo scripts and expected outputs (about 740 KB). Their hand-written markers are the labels. `lab/confidence-calibration/meterworks` is not a set: it has no markers, and its `labels.csv` covers only what suggest v0.1.0 proposed, so it cannot serve as truth under the harness's rule that an unlabelled proposal is wrong.

## Measured (2026-10-20, 13 sets, 911 proposals at `--min-conf 0.01`, 703 labels)

Pooled, in sample (the shipped table), by target precision, among the 230 proposals at the default `--min-conf 0.3`:

| target | derived threshold | auto-accepted | precision |
| ---: | ---: | ---: | ---: |
| 0.5, 0.6 | conf >= 0.30 | 230 (100 %) | 64.8 % |
| 0.7 | conf >= 0.33 | 201 (87.4 %) | 70.1 % |
| 0.8 | conf >= 0.62 | 128 (55.7 %) | 80.5 % |
| **0.9 (default)** | **conf >= 0.80** | **54 (23.5 %)** | **94.4 %** |
| 0.95 | conf >= 0.82 | 41 (17.8 %) | 97.6 % |

Held out: each set is scored with the table fitted on the other twelve, and its threshold is derived from them at 0.9. Pooled, 36 of 366 proposals are auto-accepted (9.8 %), at 86.1 % precision. Per set:

- Six sets meet 0.9: circulation, clinic-monorepo, clinic-booking (5 of 38 at 100 %), claims-events, library-legacy (90.0 %) and shipment-core.
- Three miss it: kitchen-agents (83.3 %), dispatch (0 of 2) and the repository itself (0 of 1).
- Four auto-accept nothing.

In-sample precision rises with every band: below 0.5 it is 14.6 %, and at or above 0.9 it is 100 % (24 of 24). Held out it does not: the 0.8 to 0.9 band is 47.1 % (n = 17), below the 0.7 to 0.8 band (74.6 %). The default target is met in sample by construction. On a repository the calibration never saw, the shipped threshold lands near 86 %, not 90 %. That is why `--calibrate --write` exists: a repository that measures itself gets a threshold fitted to its own code.

The new data also moved conf. `doc/func` evidence fell from 0.38 to 0.29, under the default floor, and `data/func` from 0.67 (its evidence row) to about 0.2. On the concept-poor fixture, docstring and table evidence is now proposed only below the default `--min-conf`. TestConceptPoorEvidence runs at 0.05 for that reason.

## Consequences

- A team writes `auto-accept-precision: 0.95` and reads the share it buys, rather than tuning a conf. A repository with a fixed `soft-conf-threshold` behaves as before. A repository with no `policy:` threshold moves from 0.7 to the derived 0.80.
- `gloss suggest --format json` changes shape (object, not array). The one in-repo consumer was the CLI test.
- The shipped threshold is a measurement and moves when the calibration set moves. `TestCalibrationTableIsFitted` fails when the shipped counts or the derived threshold drift from what the set measures, and prints the table to paste.
- Issue #65's done-when ("MET for both codebases") holds for clinic-booking under the per-repository reading, held out included. The other codebase, meterworks, has no reviewed markers to measure against. Issue #69's done-when holds in sample: bands are non-decreasing, conf >= 0.9 is 100 % correct, and the sweep changes at every 0.1 step. Held out, one band falls. Neither issue is closed by this change.
