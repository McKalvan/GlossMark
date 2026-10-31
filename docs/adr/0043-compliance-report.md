<!-- @gloss documents gm:Criticality, gm:Ledger scope=file -->
# ADR 0043: the compliance report reads the graph, the ledger and drift

Status: proposed, 2026-10-28; `gloss report compliance` built 2026-10-28 (issue #269).

## Context

Regulated teams (insurance, health, finance, food safety) are asked to show, for each rule, every piece of code that enforces it, the tests that verify it, and who approved each change. GlossMark already holds the answer in four places: the graph (links from artifacts to concepts, lifted through the concepts that realize a rule), the ledger (who proposed, accepted or rejected each link, when, and why), the criticality levels and the published values with their bindings, and the drift report (what is open, what is waived and until when). No command put them together for a reader outside engineering, and the pieces are in engineering terms: marker ids, IRIs, finding kinds.

## Decision

1. **One report per rule, assembled, never re-parsed.** `internal/report` takes the built graph, the workspace, the ledger entries and the drift report and produces one `Concept` record per selected concept. It parses no file: a marker's target, a value's binding and a finding's concept all come from the packages that already resolved them. The one exception is a ledger entry whose marker has left the tree, which no edge resolves; its target text is resolved the way the extractor would, through the default namespace of the manifest over its path, so a rejected or removed link keeps its place in the history.

2. **A rule's evidence includes its realizers.** A core constraint is enforced by the code linked to the implementation concepts that realize it or are published as it (`rollup.RealizesStar`), so `claim:AutoApprovalLimit` reports the `implements` link to `tide:AutoApprovalRule` with `via`. The same set scopes the ledger history, the published values, the findings and the waivers.

3. **The ledger is read as events.** An entry becomes a short sequence: *linked* or *proposed* at `created` by `actor` (proposed when it was reviewed later, is still waiting, or was rejected), *accepted* or *rejected* at `reviewed_at` by `reviewed_by` with `review_reason`, *stale* or *removed* at `last_seen`; a `value:` entry becomes *value-bound* and, when drifted, *value-drifted*. Events sort by time, so one history reads across markers. `--since` keeps the events on or after a date; the concept and its code stay.

4. **The default selection is what an auditor asks about.** Without `--concept` or `--criticality`, every constraint (an implementation constraint that stands for a core one is folded under it) and every `critical` concept. `--criticality <level>` is a floor over every owned concept; `--concept` names them, with a did-you-mean on a typo.

5. **Four forms, one record.** Markdown is the text form and the default. HTML is one self-contained page with no script and a print stylesheet, so a browser's print-to-PDF files it; a PDF writer in the binary would add a dependency for a format the browser already produces. CSV has one row per (concept, evidence item): the concept, each boundary, each artifact and test, each value, each decision, each finding, each waiver, with the marker or finding id in `record`, so a spreadsheet filter on `row` gives the audit evidence for one question. JSON is the record itself.

## Consequences

- The report is only as complete as the ledger: a repository that does not commit `.glossmark/ledger.jsonl` has no history to show, which the report says with "No ledger entries". The ledger belongs in git (QUICKSTART.md).
- Owners come from `owner:` on manifests and namespaces, so a report without them shows "—"; `gloss doctor` does not yet ask for owners on critical boundaries.
- The history is the ledger's, not git's. `gloss history` answers when a concept or boundary changed; this report answers who decided each link. Folding the two into one timeline is left open.
