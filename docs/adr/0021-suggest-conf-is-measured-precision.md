<!-- @gloss documents gmi:SuggestCalibration scope=file -->
# ADR 0021: suggest's conf is the precision its evidence measured

Status: accepted, 2026-10-20. Refs issues #65 (F-CAL-1) and #69 (F-CAL-5). The auto-accept threshold and the Phase 2 gate are revised by [ADR 0024](0024-auto-accept-precision.md); the calibration set, the rare-basis fallback and the latest measurement by [ADR 0032](0032-suggest-calibration-set-grows.md).

## Context

`gloss suggest` wrote `conf = min(0.95, score/110)`, where `score` was the search score of the best hit. The gate reads `conf` as an agent's confidence: a proposal at or above `soft-conf-threshold` (0.7) is SOFT. On 172 labelled proposals from two codebases, `conf >= 0.7` was 57 % correct. The value `0.95` was right less often than `0.91`. `conf` took ten values, so every threshold from 0.65 to 0.80 picked the same proposals. Nothing measured any of this: PLAN.md listed the Phase 2 spec gate (at least 80 % auto-accept at at least 90 % precision) as built, with "confidence calibration tooling" deferred.

## Decision

1. **The harness.** `internal/suggest/calibrate_test.go` holds a labelled calibration set: the repository itself, the three example workspaces, and two fixtures under `internal/suggest/testdata/calibration/` built from the adoption findings (`warehouse-legacy`, the concept-poor case of #58 to #64, and `clinic-monorepo`, the package-clause, fake, DTO, Terraform and Kubernetes case of #65 to #69). Each set is a reviewed workspace. Its own markers are the labels. The harness strips them, runs suggest on the copy, and scores each proposal: *correct* when a label has the same file, the same declaration or the whole file, the same relation and the same target; *near* when a label sits on the same declaration with another target; *wrong* otherwise. A proposal the labels do not state counts as wrong, even when a reviewer might keep it. The set is conservative.
2. **The report.** `TestCalibrationReport` (`go test -run TestCalibrationReport -v ./internal/suggest`) prints, per set and pooled, the precision per confidence band, the recall, the auto-accept share (conf at or above `soft-conf-threshold`) and its precision, and whether the Phase 2 gate is met. It also prints a held-out estimate: each set scored with the table fitted on the other sets.
3. **conf is a table lookup.** Each proposal carries a `basis`: the kind of evidence that chose its target (`named-in-doc`, `name-exact`, `alias-exact`, `calls`, `name-contains`, `doc`, `data`, `name-partial`, `name-overlap`, ...), the role of the declaration (`type`, `func`, `member`, `test`, `table`, `resource`, `heading`, `workload`, `page`, `shell`), and `/tie` when an unrelated concept matched as strongly. `conf` is the precision that basis measured on the calibration set, smoothed as (correct + 1) / (n + 2). A basis seen fewer than four times falls back to its evidence row, and evidence never seen falls back to 0.3. The table lives in `internal/suggest/calibration.go`. `TestCalibrationTableIsFitted` fails when a basis's conf is outside the sampling error of what the set measures, and prints the refitted table. `TestConfIsCalibrated` checks that each band's precision is within sampling error of its mean conf, and that precision does not fall as conf rises.
4. **The floor is a conf.** `--min-conf` (default 0.3) replaces `--min-score`. `--min-score N` still works and means `--min-conf N/110`.
5. **The same measurement for any repository.** `gloss suggest --labels` prints a workspace's links as a labels file. `gloss suggest --eval labels.tsv` on a stripped copy prints the same table for that repository.
6. **The gate stays unmet until the harness says otherwise.** PLAN.md marks the Phase 2 precision gate as measured and not met.

## Measured on the calibration set (2026-10-20)

The in-sample numbers below come from the shipped table. The pooled set has 358 proposals at `--min-conf 0.01` and 273 labels, from 6 sets (refitted after merging main at 3e3fd7a):

| | proposals | precision | conf >= 0.7 (SOFT) | SOFT precision |
| --- | ---: | ---: | ---: | ---: |
| main before this change, default `--min-score 60` | 64 | 25.0 % | 18 (28.1 %) | 44.4 % |
| this matcher, default `--min-conf 0.3` | 157 | 47.1 % | 9 (5.7 %) | 100 % |
| this matcher, held out (each set scored by the table fitted on the others) | 358 | 26.8 % | 134 (37.4 %) | 19.4 % |

The default output is nearly twice as precise as before, and SOFT now means what the gate assumes on this set. But only 6 % of proposals reach SOFT, and the held-out estimate shows the table does not carry over between sets as different as these. The repository's own code is over 80 % of the proposals and is unlike the fixtures. **The Phase 2 gate (at least 80 % auto-accept at at least 90 % precision) is not met.** The fixtures were written alongside the matcher, so their numbers flatter it. The repository and the examples were not. A larger labelled set from real adoptions, added as more calibration sets, is what moves these numbers. A matcher change that the set rewards is the only kind that counts.

## Consequences

- `conf` on a suggest proposal means "proposals like this were right this often", not "the string matched this well". The `basis` on every proposal names the row it came from.
- The table moves with the calibration set, and the repository is one of the sets. A code change that moves a basis past its sampling error fails `TestCalibrationTableIsFitted`, which prints the table to paste. That is expected after a matcher change, and rare otherwise.
- Agents that write `conf=` by hand are unaffected. ADVANCED.md distinguishes the two meanings.
