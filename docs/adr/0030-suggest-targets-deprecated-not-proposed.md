<!-- @gloss documents gmi:MarkerSuggester scope=file -->
# ADR 0030: suggest proposes deprecated concepts and skips proposed ones

Status: accepted, 2026-10-21. Amends [ADR 0022](0022-what-suggest-proposes.md). Closes issue #157 (L12, L13).

## Context

Two target-selection rules mattered in a brownfield adoption (`library-legacy` in glossmark-examples), where deprecated names are still in the code and planned terms are imported as proposed concepts:

- **L12.** suggest never proposed a deprecated concept. Code that still uses one was retargeted to whatever else matched: `class Borrower` became `represents` of `BorrowingLimit` through the stem *borrow*, although `lib:Borrower` (deprecated, `replaced-by: Patron`) is exactly what it is. The reviewer never saw the right candidate.
- **L13.** suggest did propose `proposed: true` concepts. `CirculationServiceTest` was proposed to verify `lib:TurnoverRate` through the alias *circulation per item*, which an agent wrote and nobody has reviewed.

The gate already classes a link to either kind of concept HARD (`target deprecated`, `target ... is a proposed concept awaiting acceptance`), whatever its conf.

## Decision

1. **A deprecated concept is a target.** It is matched like any other concept. The proposal carries `deprecated: true` and a note, which the marker writes as `note="lib:Borrower is deprecated, replaced-by lib:Patron; keep it, or link the replacement"` (without the `replaced-by` part when the concept names none). The reviewer chooses: legacy code that still is the old thing keeps the deprecated link, and `gloss check` reports it (GM222) until the code is migrated; code that already means the replacement is retargeted.
2. **A deprecated proposal is never an auto-accept candidate.** `gloss suggest` tags it `review`, and `auto_accept` is false in JSON, whatever its conf. This says what the gate does with it.
3. **Its conf is unchanged.** It comes from its basis like any other proposal, so conf still means measured precision (ADR 0021). No `/deprecated` basis was added: the calibration set has two such proposals, too few for a row of their own.
4. **A live concept wins a tie.** When a deprecated concept and another concept match with the same strength, the live one is chosen. A deprecated concept and its replacement are one choice, which the note puts to the reviewer, so matching both does not mark the proposal `/tie`.
5. **A proposed concept is not a target** until it is accepted (`gloss accept --concept`). The issue offered a second option, capping conf below the auto-accept threshold. It was not taken:
   - A cap would make conf a policy value rather than a measured precision, which ADR 0021 rules out.
   - The cap would change nothing at the gate, which already classes such a link HARD.
   - A proposed concept's names, aliases and doc are unreviewed (in the adoption, all were written by an agent), and they are the evidence suggest matches. Proposals built on them are noise in the first-pass patch, as L13 shows.
   - The code a proposed concept was made for already carries its marker: `gloss propose --from-new` rewrites the `ns:NEW:Name` marker that motivated it. suggest has nothing to add there, and skips marked declarations anyway.

   Once the concept is accepted, it becomes a target on the next run.

## Measured

On the calibration set (13 sets, `--min-conf 0.01`), pooled in sample:

- **Proposals.** 924 before this change, 923 after.
- **Correct.** 248 before, 250 after. The two gains are `Borrower.java#Borrower` and the `borrowers` view, both now `represents lib:Borrower` as labelled.
- **Wrong.** Proposals to `lib:FinePayment`, `pepper:MenuAvailability` and other proposed concepts are gone or retargeted. Three new unlabelled proposals to `lib:Borrower` (`BorrowerDAO`, `BorrowerActivityReport`, and the glossary heading) count as wrong under the harness's rule. A reviewer may keep some of them.

The shipped table stays within the sampling error `TestCalibrationTableIsFitted` allows, so it was not refitted.

## Consequences

- `gloss suggest --write` may now write markers that `gloss check` warns about (GM222, deprecated target). It is a warning, and the note on the marker says why the link was proposed.
- The calibration harness (`--eval`, `--calibrate`, and the threshold derivation) still counts a deprecated proposal at or above the threshold as auto-accepted, because it reads conf alone. This affects 2 of 923 proposals. Making it count only proposals the CLI tags `auto` is left to the calibration work (#65, #69).
