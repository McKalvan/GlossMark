<!-- @gloss documents gmi:AdoptionLadder scope=file -->
# ADR 0044: staged adoption levels, each paying off on its own

Status: accepted, 2026-10-28. Refs issue #258 (tracking #256).

## Context

The docs and the defaults read as all-or-nothing: two tiers, manifests, markers, ledger, views and gate, with `gloss check`, `gloss drift` and `gloss diff` failing from the first PR on everything they can find. A team cannot tell how little it needs to write to get something back, or what each further step buys. A repository that has written only its glossary already serves agents a vocabulary over MCP and `AGENTS.md`, which is worth having, but the same repository fails `gloss drift` on every gap of the manifests it has not written yet.

Three things were asked for: a documented ladder with the payoff of each rung, a configuration switch that sets the strictness to match the rung, and `gloss doctor` saying where a repository is and what the next rung adds.

## Decision

1. **Four rungs, documented in QUICKSTART.** Level 0 is the vocabulary (`glossmark.yaml` and the core glossary); level 1 adds one manifest per service and `scope=file` links in its entry points; level 2 adds `criticality:` on the concepts and boundaries that matter; level 3 is markers on individual declarations, `value-from` bindings and imports. The quickstart's steps keep their numbers and anchors and each names its rung; the ladder table at the top says what each rung asks for, what it gives and what fails.

2. **`adoption:` in `glossmark.yaml` is a ceiling on enforcement, not a different set of checks.** `--fail-on` (and the Action's `drift-fail-on` and `fail-on` inputs) keep choosing which finding kinds or gate class count; the level caps how hard they bite:
   - level 0: `gloss check`, `gloss drift` and `gloss diff` report and exit 0 (`gloss check --strict` still fails, as an explicit ask);
   - level 1: `gloss check` errors fail; drift findings and the gate are reported only;
   - level 2: a drift finding fails only when the concept it names is classified `high` or `critical` (the concept decides, not the boundary around it; a finding on a boundary alone, such as `overdue`, takes the boundary's level); `gloss diff` fails only when a change at that criticality is HARD (or SOFT with `--fail-on SOFT`);
   - level 3, the default when the key is unset: everything as before, so no existing repository changes by upgrading.

   The report is the same at every level; only the exit code changes, and a command the level kept from failing says so on stderr with the line that raises the level. Making the level a ceiling rather than a replacement for `--fail-on` keeps one mental model (the kinds and classes documented in ADVANCED.md) and means the GitHub Action, which always passes `--fail-on`, needs no change. `gate.Impact` gains `enforced` (the gate after the level) and `advisory` (the one-line explanation); `gate` stays what the changes are.

3. **`gloss doctor` reads the rung from the tree** and compares it with the configured one. The observed rung is the highest whose every lower rung is present: a concept of the repository's own; a manifest and an inline link from code; a declared criticality (an imported concept's counts, since it governs here too); a marker on a declaration, a `value-from` binding or an import. The check never fails: a tree ahead of its level is an Info that suggests raising `adoption:`, a tree behind it (the usual case for a fresh adoption, since unset is level 3) suggests lowering it or names what the next rung asks for and gives.

## Consequences

- A repository can adopt one rung at a time and keep CI green at each, with the same workflow file throughout.
- The level is deliberately coarse: a team that wants, say, level 1 with `gloss check` warnings as errors combines `adoption: 1` with `--strict`; one that wants level 3 with fewer kinds narrows `--fail-on`. The ladder does not replace either knob.
- `gloss init` does not write `adoption:` yet; a scaffolded repository is at level 3 and `gloss doctor` says so with the line to add. Writing `adoption: 0` from `gloss init` is a follow-up once the first adoptions have run with the setting.
