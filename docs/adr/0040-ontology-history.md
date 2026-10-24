<!-- @gloss documents gm:Gating scope=file -->
# ADR 0040: ontology history is the gate folded over commits

Status: proposed, 2026-10-24; `gloss history` with `--format json` and MCP `history_of` (decisions 1 to 6) built 2026-10-24; ledger transitions appear where the gate reports them (accepted, stale), not yet merged by date. The pull-request comment (decision 7) is next.

## Context

Every ontology file, manifest, marker and the compiled `.glossmark/graph.jsonl` is in git, so the history of what a system means is already recorded. Nothing reads it. `gloss diff --base <ref>` compares two points and the viewer overlays one such comparison (`gloss view --diff`), but a reviewer cannot ask when `lib:Hold` was added, which commit moved its realization from one boundary to another, or who accepted the link that made it critical. An agent about to edit a concept has no way to learn how it got to where it is.

The two readers are the reviewer of a pull request and an agent over MCP. Both want the past of the concepts and boundaries in front of them, in the terms the gate already uses.

Most of the parts exist:

- `gate.Classify` already types every change: `concept-added`, `concept-removed`, `concept-proposed`, `concept-accepted`, `boundary-added`, `boundary-changed`, `boundary-removed`, `ontology-edge-added`, `ontology-edge-removed`, `link-removed`, `realization-removed`, `planned-dropped`, `version-bump`, `major-bump`, `view-changed`, `published-value-drifted` and the rest, each with a gate class, an actor and a location.
- Identity is permanent. A rename is `deprecated` plus `replaced-by` (FILE-FORMATS.md), for concepts and for boundaries ([ADR 0007](0007-retiring-boundaries.md), [ADR 0019](0019-migration-overlap.md)), and a marker keeps its review row across rewrites ([ADR 0027](0027-review-trail-across-rewrites.md)). History never has to guess that two names are one thing; it follows the edge.
- The ledger records who proposed, accepted, rejected or re-confirmed each link.

What does not carry over is how `gloss diff` gets its base. It extracts the base tree and never reads the committed graph (`cmd/gloss/diff.go`), because a committed graph is repository content and could already hold a link its tree does not. Doing that at every commit of a long history means one extraction and build per commit; in this repository 145 of 270 commits change a 1.3 MB graph, and a monorepo's history is longer and its graph larger.

## Decision

1. **History is a sequence of gate results.** A step is a pair of adjacent commits; its record is what `gate.Classify` returns for that pair. One change model serves the pull-request gate and the history, so a change reads the same in both and every new change kind reaches history for free.

2. **History reads the committed graph.** Each step loads `.glossmark/graph.jsonl` as committed at both commits; nothing is extracted. History is a record for reading, not a gate: the pull-request gate keeps extracting its base, so its correctness does not depend on this. Three guards keep the record honest:
   - **Unbuilt commits.** A commit that changes an ontology file, a manifest or `glossmark.yaml` and leaves `graph.jsonl` as it was is marked `unbuilt`; its changes appear on the next commit that rebuilt the graph, and the record says so. A repository that runs `gloss build --check` in CI has none.
   - **Format skew.** A commit whose graph header names a format this release does not read is a `skipped` step with the reason, never a misread one.
   - **`--verify`.** Extracts and builds the selected commits from their trees, as `gloss diff` does, for when the committed record is in doubt.

3. **Each step is classified under the policy of its own commit.** `Classify` needs a workspace for manifests, vocabulary, views and policy. History loads one from that commit's ontology files, manifests, `glossmark.yaml` and views file, read with `git show`, without scanning code, as `gloss diff` already reads the base's hard-paths; the earlier commit's ledger and hard-paths go in as the step's base (`BaseLedger`, `BaseHardPaths`). A change therefore carries the gate class it had when it was made, not the one today's policy would give it. Fields that need the code tree (drift before and after, changed files) are left out of a history step.

4. **Commits with no graph change cost nothing.** Under guard 2 a commit that leaves `graph.jsonl` unchanged and is not `unbuilt` has no ontology change, so history walks `git log -- .glossmark/graph.jsonl` plus the commits guard 2 must inspect, and diffs only those.

5. **The step is configurable; every commit by default.** `history.step` in `glossmark.yaml` takes `commit` (default), `first-parent` (merges to the main line only) or `version` (one step per namespace semver bump, the steps between two bumps folded into one). `--step` on the command line overrides it.

6. **A command, a JSON form and an MCP tool.**
   - `gloss history [<curie>...] [--range <a>..<b>] [--step ...] [--verify] [--all]` prints a timeline: commit, date, author, change kind, gate class, subject and location, and for a deprecated subject the `replaced-by` chain to its successor. With CURIEs it follows those concepts and boundaries, and the successors they were replaced by; without, every change.
   - Ledger transitions on the links of a followed subject (proposed, accepted, rejected, stale, re-confirmed) are merged into its timeline by date, so the reviewer sees who decided as well as what changed.
   - By default only steps with a SOFT or HARD change, or an edit under `policy.hard-paths`, are shown; `--all` shows AUTO-only steps too.
   - `--format json` emits the steps as a list, each an impact in the shape of `gloss diff --format json` with the step's commits.
   - MCP `history_of(curies, since?, step?, all?, limit?)` returns those steps compacted for an agent: without per-view deltas, without AUTO changes unless `all`, at most `limit` steps (default 20). `since` defaults to the merge base with the default branch on a branch, else the last version bump of the subject's namespace.

7. **The pull-request comment shows recent history.** For each concept and boundary the pull request touches, the Action's comment adds its last few history steps (default 3) under the impact, so the reviewer reads the change beside how the subject got here.

8. **Net and step views differ.** A range can be shown step by step or as its net change (the gate of its two ends). A concept added and removed inside the range is in the step view and absent from the net view.

## Consequences

- A reviewer of a change to `lib:Hold` sees, in the comment, that it was added in one commit, made critical in another and moved from `acme:circulation-svc` to `acme:loans-svc` in a third, with who did each.
- An agent asks `history_of` before editing and learns that a concept is the successor of a deprecated one, or that its realization moved recently, without reading the git log.
- In a repository without `gloss build --check`, history is as good as its committed graphs. `unbuilt` steps say where it is not, and `--verify` repairs a range on demand.
- History needs the commits. The Action's checkout must fetch enough history (`fetch-depth: 0`, or a depth that reaches the range); on a shallow clone `gloss history` reports the range as truncated instead of starting history at the shallow boundary.
- `glossmark.yaml` gains a `history:` block; `schemas/` and FILE-FORMATS.md list it, and ADVANCED.md documents the command.
- Out of scope here: a timeline scrubber in the viewer. The JSON of decision 6 is a list of the impacts `gloss view --diff` already overlays, so the viewer can step through it later without a new format.
- Also out of scope: overlaying declared dates (`planned.by`, `retire-by`, waiver `expires`) on the commit timeline to show plan slippage ([ADR 0034](0034-ontology-first-increments.md), [ADR 0038](0038-planned-in-views.md)).

## Alternatives considered

- **Extract every commit's tree, cached by tree hash under `.glossmark/cache/`.** As correct as `gloss diff`, but the first run over a long history costs one build per commit, and CI starts without the cache. Kept as `--verify`.
- **A committed append-only `history.jsonl`.** Cheap to read, but it is a second log beside git that can disagree with it, and a rebased or squashed branch leaves it describing commits that no longer exist.
- **Classify every step under today's workspace.** Simpler and faster, but an old change would show the gate today's policy gives it, not the one it passed; a reviewer asking how a HARD change got in needs the latter.
- **Only `version` steps by default.** Readable for importers, but hides which commit within a release changed a concept, which is what the pull-request reviewer and the agent ask.

## Migration

None. `gloss history`, `history_of` and the `history:` block are new; `gloss diff` and the gate are unchanged. The comment's history section appears once the Action's checkout fetches history.
