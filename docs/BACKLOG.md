<!-- @gloss documents arch:glossmark scope=file -->
# Backlog, stack-ranked

The open issues in priority order, as of 2026-10-24. Rank weighs risk first
(a gate that can be bypassed undermines everything else), then what unblocks
other work, then value for size. Issue labels (P2, P3) record the roadmap
phase; this file records the order to work in. Re-rank when an issue opens or
closes.

| # | Issue | Size | Why here |
| ---: | --- | :---: | --- |
| 1 | [#186](https://github.com/McKalvan/GlossMark/issues/186) Gate trusts self-reported actors | M | The CI gate's `may-modify` verdict can be lowered from HARD to SOFT by editing the ledger in the same PR. Correctness of the gate comes before features. |
| 2 | [#187](https://github.com/McKalvan/GlossMark/issues/187) CI gate runs the PR's own `action.yml` and binary | S | Same class: a PR can weaken the gate that judges it. Small workflow change. |
| 3 | [#183](https://github.com/McKalvan/GlossMark/issues/183) `review-glossmark` skill | M | The headless review mode exists and nothing ships that uses it; unblocks agent upkeep after adoption. Mostly docs and an embed. |
| 4 | [#205](https://github.com/McKalvan/GlossMark/issues/205) `import --values instances` | S | Small, clear spec, stops catalogue enums flooding the core. |
| 5 | [#195](https://github.com/McKalvan/GlossMark/issues/195) Published filter in the viewer | S | Small, clear spec, completes the filter panel. |
| 6 | [#191](https://github.com/McKalvan/GlossMark/issues/191) Rethink the impact PR comment | M | The surface every reviewer meets; needs an ADR and an owner decision first. |
| 7 | [#200](https://github.com/McKalvan/GlossMark/issues/200) ADR 0034 P2: curate-increment, `plan.jsonl` | L | Next phase of the planned-increment work; P3 (#201) depends on it. |
| 8 | [#65](https://github.com/McKalvan/GlossMark/issues/65) `suggest` calibration (Phase 2 gate) | L | The Phase 2 gate is unmet; measurement tooling first, then scoring. |
| 9 | [#206](https://github.com/McKalvan/GlossMark/issues/206) Tree-sitter anchoring | L | Phase 2, but needs the cgo / static-binary trade-off decided in an ADR. |
| 10 | [#202](https://github.com/McKalvan/GlossMark/issues/202) Guards: `guarded-by:`, custom relation surfaces | M | ADR 0015 follow-up with a clear spec. |
| 11 | [#203](https://github.com/McKalvan/GlossMark/issues/203) harvest: `--reference`, `topic`, git dates | M | ADR 0025 follow-up; improves derivation evidence. |
| 12 | [#204](https://github.com/McKalvan/GlossMark/issues/204) Derivation record, re-runs, alignment | L | ADR 0025 phase D1; builds on #203. |
| 13 | [#201](https://github.com/McKalvan/GlossMark/issues/201) ADR 0034 P3: plan closure | L | Depends on #200. |
| 14 | [#213](https://github.com/McKalvan/GlossMark/issues/213) Instances file format and SHACL shapes | L | Phase 3; completes #205's instances. |
| 15 | [#210](https://github.com/McKalvan/GlossMark/issues/210) Fetch `git+`/`https://` imports | L | Phase 3; federation without sibling checkouts. |
| 16 | [#212](https://github.com/McKalvan/GlossMark/issues/212) OWL export | M | Phase 3 interop. |
| 17 | [#211](https://github.com/McKalvan/GlossMark/issues/211) SSSOM mappings | M | Phase 3 interop; pairs with #212. |
| 18 | [#207](https://github.com/McKalvan/GlossMark/issues/207) Backstage ownership join | M | Phase 3; useful only where Backstage is in use. |
| 19 | [#208](https://github.com/McKalvan/GlossMark/issues/208) SCIP join | L | Phase 3; needs an external indexer and a protobuf decision. |
| 20 | [#209](https://github.com/McKalvan/GlossMark/issues/209) Plugin SDK | L | Phase 3; largest surface, least urgent. |
