<!-- @gloss documents gmi:DriftComparator, gmi:Linter scope=file -->
# ADR 0028: What an orphan is, deprecated use in drift, and a stable drift trailer

Status: accepted, 2026-10-21. Refs issue #160 (rows FF12, L27, E24; the other rows are fixes that need no decision).

## Context

Adopting GlossMark in the glossmark-examples showcases turned up three places where `gloss drift` said something other than what a reader took from it:

- **Orphan (FF12).** shipment-core's `StatusProjector` subscribes to `frt:ShipmentPickedUp` in the ontology, and code implements `StatusProjector`. Drift still called the event an orphan, "no artifact links to it". The rule follows `produces` and `publishes` from a concept code reaches, but not `consumes` or `subscribes`. An inline `@gloss subscribes` on the listener counts, because any artifact link but `documents`, `instructs` and `may-modify` counts. The same fact written once in the ontology did not. The message was also wrong: a `documents` link is an artifact link and does not count.
- **Deprecated use (L27).** Code on a deprecated concept showed up in `gloss check` (GM222) and `gloss impact`, and in `gloss landscape` as `deprecated-use`. `gloss drift` and `gloss export --status` did not show it.
- **The trailer (E24).** The text trailer printed file coverage (`Files 65% (58/89)`). Every file added or deleted moves that number, Markdown included, so a committed `expected-drift.txt` changed with every commit.

## Decision

1. **Orphan.** A concept is reached, and not an orphan, when a reached concept `produces`, `publishes`, `consumes` or `subscribes` to it in the ontology. This is the same as the marker form. The orphan message now says "no code links to it (a documents or instructs link does not count)". We kept the count rule and fixed the message, rather than only renaming what it counts, because the inline and ontology forms of one fact must agree. Making `subscribes` count while the ontology form did not would have left the inconsistency in place.
2. **Deprecated use in drift.** `gloss drift`, MCP `drift` and `gloss export --status` add the `deprecated-use` findings that `gloss landscape` already reported. `policy.waivers` applies to them, so `gloss check` does not report a waiver of one as unmatched (GM611). The kind keeps its landscape name, `deprecated-use`, so one name works in `--fail-on` everywhere. It does not fail by default: a deprecation is a migration under way, and `overdue` enforces a boundary's deadline. A status row carries `deprecated: true` and `replaced-by`. The findings are appended after `rollup.Compare` (`rollup.AddDeprecatedUse`) instead of inside it, so views, slices and gates that read the report are unchanged.
3. **The trailer.** The text and Markdown trailers leave file coverage out. Concept coverage is the headline, and file coverage was already secondary (one marker per helper raises it without adding knowledge). `gloss coverage` and `--format json` still carry it, and `--fail-under-files` still gates on it.

## Consequences

- A repository whose events are only subscribed to locally, and published by a sibling, sees fewer orphans. An event nothing publishes anywhere is still visible in `gloss landscape`. Within one repository it is no longer an orphan once a built concept subscribes to it.
- `gloss drift` output gains a "Deprecated in use" section where a repository still uses a deprecated concept. Committed drift outputs change once for this, and once for the shorter trailer.
- Findings now have stable ids in the text output on request: `gloss drift --ids` prints the key a waiver names.
