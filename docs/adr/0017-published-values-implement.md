<!-- @gloss documents gm:ValueBinding scope=file -->
# ADR 0017: A published value's code implements it, in as many places as it lives

Status: accepted, 2026-10-20. Closes issue #41 (F-ADOPT-7).

## Context

`@gloss publishes-value .X scope=line` above a constant binds X's published `value:` to that line: changing the line without the value is a HARD published-value drift. It did not lift to `realizes`, so the boundary that owns the setting could not declare X without a gap (`declares realizes lark:LeadTimeSetting but nothing under it implements it`) unless a second, redundant `implements .X` marker sat on the same line. GlossMark's own manifest had worked around it by putting `gm:Confidence` under `allowed:` instead of `realizes:`. Separately, `value-from` took one location, so a value duplicated in two languages (the 24-hour cancellation window in the Go API and in the TypeScript page) could bind only one copy, and a change to the other went unnoticed.

`gloss export --status` already counted `publishes-value` as implementing (issue #127); drift, `untested` and the boundary rollup did not.

## Decision

1. **`publishes-value` lifts as `implements`** for the concept it names: the boundary holding the line derives `realizes X` (and what X realizes, and X's intended relations), X counts as implemented for `untested`, and the line's artifact counts toward the boundary's implementations. The line is the code that holds the setting; it needs no second marker.
2. **`value-from` takes a list.** `value-from: [{ path: services/api/policy.go, symbol: CancellationWindow }, { path: apps/web/page.tsx, symbol: CANCELLATION_WINDOW_HOURS }]` binds every location. Several `publishes-value` markers on one concept do the same. The graph node's `value-from` lists the locations (`path:line, path:line`, in path order), `live-value` joins their lines with ` | `, and the fingerprint covers all of them, so a change to any one is a value drift; the ledger keeps one `value:<curie>` baseline. A single location is fingerprinted exactly as before, so existing baselines do not move.

"Every entry must agree with `value:`" is enforced the way a single binding is: the first build baselines the lines against the value, and from then on a change to a line without a change to `value:` is drift. GlossMark does not parse `24 * time.Hour` to compare it with `24 hours`.

## Alternatives considered

- **A separate relation for "holds the value and implements it".** Rejected: every published setting would need it, and `publishes-value` already says the line holds the concept.
- **One baseline per location.** Rejected: `gloss accept --value X` would need a location argument, and the drift is the concept's, whichever copy moved.

## Migration

A boundary whose code carries `publishes-value .X` now derives `realizes X`; where the manifest declares `realizes` without X (or X's core concept), `gloss drift` reports a violation: declare it (as GlossMark's own `internal/.gloss.yaml` now declares `gm:Confidence`). A redundant `implements .X` beside a `publishes-value .X` can be dropped. Importers that mark a copy of an upstream value with `publishes-value` still bind nothing, as before (the ledger skips imported concepts); a mirror marker for that case is left to a later change.
