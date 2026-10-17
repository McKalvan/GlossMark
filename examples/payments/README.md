# Worked example: a payments platform

Two repositories share one core. `payments-api` owns the core namespace `pay:`
and its own implementation namespace `pa:`. `settlement-worker` imports `pay:`
and owns `sw:`. Together they exercise every mechanism in the design: tiers,
the published seam, extension, markers across file types, federation and
agent remits.

```
cd payments-api        && gloss check && gloss drift
cd ../settlement-worker && gloss check && gloss drift
```

What the tooling makes of it (see the design doc's table):

| Derived from code | Declared | Result |
| --- | --- | --- |
| `acme:payments-api` implements {Authorization, Capture, Refunding} | manifest realizes [Authorization, Capture] | Violation: Refunding handled but not declared |
| `sw:BatchSettler` subscribes `pay:CaptureConfirmed`; `pa:CaptureEmitter` publishes it | nothing links the repos by hand | Landscape edge, drawn automatically by `gloss landscape` |
| nothing verifies `pay:Capture` | Capture declared and implemented | Gap: implemented but untested |
| `sw:CutoffClock` value changed | published as `pay:SettlementCutoff` | HARD in `gloss diff`: business view changes |
| `acme:payments-platform` produces {Payment, Payout} | system-level declaration | Consistent after taxonomic lift |

`settlement-worker/glossmark.yaml` imports `pay:` from a local path for this
example; in a real deployment the source is a git ref or URL pinned to a version.
