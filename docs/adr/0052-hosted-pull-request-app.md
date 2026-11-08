<!-- @gloss documents gmi:PullRequestApp scope=file -->
# ADR 0052: a hosted pull-request app beside the Action

Status: accepted, 2026-11-01; `gloss app serve` for GitHub built 2026-11-01 (#264). GitLab is not built.

## Context

The AUTO/SOFT/HARD gate on a pull request is the feature that sells GlossMark to reviewers, and getting it means installing the Action, wiring a workflow and committing it: the most friction at the moment of least conviction. A GitHub App that a repository installs in two clicks removes the workflow from the first trial. The gate itself must not change: whatever the app posts has to be what `gloss diff` prints, or the two paths would judge a PR differently.

## Decision

1. **A subcommand of the one binary, not a second one.** `gloss app serve` is registered like `gloss mcp` and `gloss view`: the repository ships one binary and the app is a mode of it, so a release, an install script and a Docker image stay one each, and the app calls `gloss diff`'s own code (`runDiff` in `cmd/gloss`) in process rather than a copy. The GitHub-specific code (webhook shape, App authentication, REST calls, owner mapping) lives in `internal/prapp` behind a `Runner` that `cmd/gloss` supplies, so a GitLab backend can sit beside it later without touching the gate.

2. **Stdlib only.** The App's JWT is RS256 over `crypto/rsa`; the webhook check is `crypto/hmac`; the API is `net/http` and `encoding/json`. `go.mod` gains nothing: a GitHub SDK would be the largest dependency in the module for five endpoints.

3. **What it posts is what the Action posts.** The comment body is `gloss diff --format markdown` under the same `<!-- glossmark-impact:<root> -->` key the Action writes, found with the same rule (first comment carrying the key or the legacy marker), so a repository that runs both updates one comment. The actor is the PR author as the Action derives it (#186). The base is the merge base of the head and the base branch; when the shallow history (200 commits each side) does not reach it, the base branch's tip as fetched, which is what the Action's `origin/<base>` is.

4. **HARD is `action_required`, approval is the label.** A check run named `GlossMark gate` completes `success` for AUTO and SOFT and `action_required` for HARD, with the comment as its details link. `action_required` shows in the PR as a check that needs attention rather than a red failure, blocks merging when the check is required, and GitHub offers *Re-run* on it, which arrives as `check_suite` `rerequested`. The decision is recorded as in the Action's workflow: a reviewer labels the PR `ontology-approved`, the `labeled` event re-runs the gate and the run completes `neutral` (not `success`: something was decided), and a push afterwards (`synchronize`) ignores the label so the new commits are judged. `failure` is kept for the app's own errors (a checkout or diff that did not run), so a red check always means "look at the log", never "a human decision is pending".

5. **Reviewers come from the ontology's owners, then CODEOWNERS.** The owners of a HARD change are the `owner:` of the namespace of each concept it names and of the boundary whose manifest encloses its path, which are the names the repository already uses. `app.owners` in `glossmark.yaml` maps each to GitHub logins or `@org/team`; an owner with no entry falls back to the CODEOWNERS rules for the HARD changes' paths, and with no `app:` block CODEOWNERS alone decides. The author and anyone already asked are left out; the check run's summary names who was asked and which owner nobody is known for, so a missing mapping is visible on the PR rather than silent.

6. **Idempotent on redelivery.** The handler answers 202 before any work, remembers the last thousand delivery ids, and serializes events per pull request; the comment and the check run are looked up before they are written. A redelivery therefore updates what the first delivery wrote, or does nothing.

7. **Alpine, not distroless/static, for the image.** `gloss diff` shells out to `git` for the base tree (`git archive`), the changed files and the history section, and the app clones with it, so the runtime image needs `git`; distroless/static cannot carry it. The image is Alpine with `git` and `ca-certificates`, the static binary, and an unprivileged user.

## Consequences

- A team sees the comment, the check and a review request on its first PR with nothing committed. The Action remains for repositories that want the tree-level checks (drift, coverage, the committed graph) or their own runners; both can run at once.
- The app judges the root workspace only. A repository with several workspaces (the Action's `root:` matrix) gets the root's comment; per-root runs are a follow-up that reuses the same key.
- The approval label is read from the event payload, not from the API, so a label put on between deliveries is seen on the next event; `Re-run` from the checks tab is enough.
- GitLab's merge-request webhooks, project access tokens and "pipeline status" would take a second backend behind the same `Runner`; nothing in `internal/prapp` beyond `github.go` and `webhook.go` assumes GitHub, and the owner mapping is host-neutral (`@group/subgroup` reads as a team). It is left open.
