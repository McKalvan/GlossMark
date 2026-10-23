<!-- @gloss documents gmi:ReviewEngine scope=file -->
# ADR 0033: Review sessions, headless for agents and interactive for people

Status: accepted (experimental), 2026-10-22.

## Context

Reviewing meant three commands and a lot of copying. `gloss status` listed the proposed, stale and rejected links. You then opened each file, found what had changed since the last review in `git log`, looked up the concept's doc in an ontology file, and ran `gloss accept <id>` or `gloss reject <id>`. Drift findings had no actions at all: a waiver meant editing `glossmark.yaml` by hand. Agents had the same problem through MCP. The server could search and propose, but it could not list what waits on review or act on it, so an agent that wanted to help with review had to scrape `gloss status`.

## Decision

1. **One engine, three front ends.** `internal/review` builds a queue of review items from the pipeline (graph, ledger, markers, drift report). Each item carries the context a reviewer needs and the actions it allows. The engine applies decisions and keeps an undo journal. Three front ends share it:
   - `gloss review --format json` lists the queue, `gloss review show <id>` adds an item's detail, `gloss review apply [file|-]` applies decisions written as JSON Lines, and `gloss review undo` reverts the last one. These are for scripts, CI and any agent with a shell.
   - The MCP tools `review_queue`, `review_item`, `review_decide` and `review_undo`, plus `write_link` and `edit_concept_doc`.
   - `gloss review` on a terminal is a full-screen interface for people.
2. **Items.**
   - Proposed links (`@gloss?`).
   - Stale links.
   - Proposed concepts.
   - Drifted published values.
   - Drift findings: violations, gaps, rogues (the `ns:NEW:` decision requests), orphans, untested, presented-only, dangling, overdue, out-of-range and deprecated-use. Overlap is left out, as in the default text output (ADR 0019).

   An item's id is its ledger id (`m-…`, `concept:<curie>`, `value:<curie>`), or `finding:<id>` for a drift finding. Its detail holds:
   - the marker's context lines;
   - what changed in the file since the link was last reviewed (`git log -p --since`, then the uncommitted diff);
   - the doc and kind of each target concept;
   - each action's gate, with the reasons `gloss diff` gives;
   - any recommendation on it.
3. **Actions reuse the commands.** The engine runs the existing commands in-process: `accept`, `accept --stale` paths, `accept --concept`, `accept --value`, `reject [--remove]`, `reject --concept` and `propose --from-new`. Their output is captured, so `gloss accept` and a review decision cannot disagree. Three actions are new:
   - `waive` appends a `policy.waivers` entry to `glossmark.yaml` as text, so the file keeps its formatting.
   - `write-link` inserts a marker line in the file's comment style.
   - `edit-doc` sets a concept's `doc:` through `internal/ontoedit`.
4. **Authority follows the gate.** An actor whose name starts with `agent:` may take an action whose gate is AUTO or SOFT:
   - re-confirm a stale link;
   - accept or reject a SOFT proposal, or reject one whose own gate is SOFT;
   - write a link as a proposal (`@gloss?`);
   - propose a concept from a `ns:NEW:` marker.

   A HARD action stays a human decision. That covers a new or proposed concept, a doc edit, a waiver, a published value, and a link outside `allowed` or on the published seam. For those an agent can only **recommend**: the verdict, reason and parameters go into `.glossmark/recommendations.jsonl`, one row per item, which the terminal UI shows beside the item. The engine checks the authority rule. It is a convention, not a security boundary: an agent can still edit files or run `gloss accept` itself, and the CI gate (a HARD change needs the `ontology-approved` label) remains the control. A recommendation records the item's basis, the anchor fingerprint or the concept's ontology text. It reads as outdated once that basis changes, and it is removed when anyone decides the item.
5. **Everything can be undone.** Before a decision the engine reads every file a decision can touch: the ledger, `glossmark.yaml`, the owned ontology files, every file with a marker, and the file a new link goes into. After the decision it journals the earlier content of the files that changed, at `<git dir>/glossmark/undo/`, or under the user's cache directory outside git. The journal keeps the last 100 decisions. `undo` restores the newest one. It refuses when a file changed again since the decision, unless forced, because restoring would discard that edit.
6. **Pictures stay in the HTML viewer.** In the terminal, `v` runs `gloss view` and opens the page in a browser (`$BROWSER`, else `open`, `xdg-open` or the Windows URL handler) on the slice of the item's concept or boundary (`viewer.html#slice=<curie>`). Without a display it prints the URL. The terminal draws no diagrams of its own.
7. **The graph browser is read-only.** `gloss review node <ref>` (and the Browse tab, and the MCP tool `browse_node`) shows any node (concept, boundary or artifact) with its edges in and out, grouped by relation, and the drift findings on it. It lives in its own package, `internal/browse`, and changes nothing.
8. **The terminal UI uses no framework.** It is hand-rolled on `golang.org/x/term` (raw mode and size) and ANSI escapes, and it draws on the alternate screen. The state machine (`internal/tui`) is separate from terminal I/O, so tests drive it with keystrokes and compare rendered frames.

9. **A shipped skill drives the headless mode** ([#183](https://github.com/McKalvan/GlossMark/issues/183)). `review-glossmark` is installed by `gloss hook install --skill` with the other two:
   - It reads each item with `show` before deciding it.
   - It takes only the actions marked `agent_may`, and gives each a reason.
   - It records a recommendation on every HARD item.
   - It asks the owner about the HARD items in one stop-and-wait checkpoint, grouped by pattern.
   - It never applies the owner's answers itself: they go into a decisions file the owner applies as themselves, or onto the terminal UI as updated recommendations.

   `adopt-glossmark` hands proposal triage after `gloss suggest` to it.

## Walking it back

The feature is experimental and additive:
- No existing command changes behaviour.
- No file format changes. `recommendations.jsonl` and the undo journal are new files that nothing else reads.
- The only dependency added is `golang.org/x/term`.

To remove it, delete `internal/review`, `internal/tui`, `internal/browse`, `cmd/gloss/review*.go`, the review cases in `internal/mcp` and their concepts in `ontology/gmi.gloss.yaml`, then drop `golang.org/x/term` with `go mod tidy`. Each layer landed as its own commit, in this order: engine and headless CLI, MCP, terminal UI, browser. Each commit can be reverted alone, the newest first.

## Alternatives considered

- **Bubble Tea for the terminal UI.** Rejected for now: it adds a framework to a binary with two direct dependencies. The state machine is small enough to own.
- **Agents with full authority.** Rejected: it contradicts "humans own the core" ([README](../../README.md)). The gate already says which decisions are a human's.
- **Recommendations in the ledger.** Rejected: that would add a field to a format other tools read. A separate file can be deleted without a migration.
- **A subprocess per decision.** Rejected: about 0.3 s per action, and in-process capture gives the same reuse.
