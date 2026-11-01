<!-- @gloss documents gmi:PullRequestApp scope=file -->
# The hosted pull-request app

`gloss app serve` is the gate with no CI setup: a GitHub App that, once installed on a repository, judges every pull request and posts the result back. It is the same `gloss build` + `gloss diff` the [Action](QUICKSTART.md#7-wire-ci) runs, hosted once for every repository that installs it, so a team can see the impact comment on its first PR without touching a workflow file. The Action stays as the self-hosted path: a repository can run either, or both (they share one comment). GitLab is a follow-up ([ADR 0052](adr/0052-hosted-pull-request-app.md)).

On each pull request (opened, a push, reopened, marked ready, the `ontology-approved` label put on or taken off, and a re-requested check suite) the app:

1. checks out the PR head and its base branch, shallow, with the installation's token, and takes the merge base as `--base` (the base branch's tip when the shallow history does not reach it);
2. runs `gloss diff --base <merge-base> --actor <PR author>` in process, as the Action does (`human:<login>`, or `agent:<name>` for a bot account, so the agent-remit gate applies);
3. posts or updates **one impact comment**, keyed `<!-- glossmark-impact:. -->` exactly as the Action keys it, so a repository running both never gets two;
4. creates or updates a **check run named `GlossMark gate`** on the head commit: `success` for AUTO and SOFT, `action_required` for HARD, linking to the comment. Make the check required in branch protection and a HARD change cannot merge until a reviewer decides;
5. on HARD, **requests a review from the owners** of what the HARD changes touch: the `owner:` of the namespace of each concept (`owns.<prefix>.owner` in `glossmark.yaml`) and of the boundary whose manifest encloses the changed path, mapped to GitHub accounts through `app.owners`, with CODEOWNERS for the changed paths as the fallback.

A reviewer records the decision by labelling the PR `ontology-approved`: the check turns `neutral` with a summary saying who approved what. A push afterwards gates the new commits again (the label event is not re-read on `synchronize`), as in the Action's workflow. The comment itself is what [`gloss diff --format markdown`](ADVANCED.md#gates-gloss-diff) renders, recent history included.

A repository without `glossmark.yaml` at its root gets a `neutral` check saying so and nothing else, so the app can be installed organization-wide before every repository adopts. Several workspaces in one repository (the Action's `root:` matrix) are not handled yet: the app judges the root workspace.

## Mapping owners to reviewers

```yaml
# glossmark.yaml
owns:
  clinic: { iri: https://onto.larkspur.example/core/, tier: core, owner: clinical-ops }
app:
  owners:
    clinical-ops: ["@larkspur/clinical", "carol"]   # a team (@org/slug) and a login
    booking-platform: "@larkspur/booking"
```

`owner:` values are the team names a repository already uses in its manifests and namespaces; `app.owners` says who that is on GitHub. An owner with no entry falls back to the CODEOWNERS rules for the paths of the HARD changes (`.github/CODEOWNERS`, `CODEOWNERS` or `docs/CODEOWNERS`, last matching rule wins); with no `app:` block at all, CODEOWNERS alone decides. The PR author and anyone already asked are left out, and the check run's summary names who was asked and any owner nobody is known for. Requesting a team needs the App installed on the organization with the team visible to it.

## Registering the GitHub App

On GitHub, **Settings → Developer settings → GitHub Apps → New GitHub App** (for an organization, under its settings):

- **Webhook**: active, URL `https://<your host>/webhook`, a secret of your choosing (`WEBHOOK_SECRET`).
- **Repository permissions**: Contents *read* (to clone), Metadata *read*, Pull requests *write* (the comment and the review request), Checks *write* (the check run). Organization permission Members *read* when teams are to be requested.
- **Subscribe to events**: Pull request, Check suite.
- **Where can this App be installed**: your account, or any account to host it for others.

After creating it, note the **App ID** (`APP_ID`) and generate a **private key** (the `.pem` file; `PRIVATE_KEY_FILE`, or its contents in `PRIVATE_KEY`). Then **Install App** on the repositories to gate. A `ping` arrives at the webhook URL on registration; the server answers it with 200.

## Running it

```sh
APP_ID=123456 PRIVATE_KEY_FILE=app.pem WEBHOOK_SECRET=... gloss app serve
```

| Variable | Meaning |
|---|---|
| `APP_ID` | The App ID from the settings page. |
| `PRIVATE_KEY` / `PRIVATE_KEY_FILE` | The App's private key, PEM (PKCS#1 as GitHub hands it out, or PKCS#8); the value may carry `\n` for newlines. |
| `WEBHOOK_SECRET` | The webhook secret given at registration; every delivery's `X-Hub-Signature-256` is checked against it. |
| `LISTEN_ADDR` | Address to listen on; default `:8080`. `POST /webhook` receives deliveries, `GET /healthz` answers `ok`. |
| `WORKDIR` | Where checkouts are made (one temporary directory per pull request, removed afterwards); default the system temp directory. |
| `GITHUB_API_URL` | The REST API; default `https://api.github.com`, `https://<host>/api/v3` for GitHub Enterprise Server. |

The server needs `git` on its PATH and outbound HTTPS to GitHub. A delivery is answered `202` at once and the work runs in the background, one pull request at a time per PR, so a redelivery (GitHub retries on a timeout, and the webhook page has a *Redeliver* button) finds the comment and the check run already there and updates them rather than adding more; a delivery id seen before is acknowledged and skipped. Errors are logged to stderr with the repository, PR number and delivery id; a checkout or diff failure completes the check run as `failure` with the error in its summary.

### Docker

The [`Dockerfile`](../Dockerfile) builds a small Alpine image with the static `gloss` binary and `git`, running as an unprivileged user:

```sh
docker build -t glossmark-app .
docker run -p 8080:8080 -e APP_ID=123456 -e WEBHOOK_SECRET=... -v $PWD/app.pem:/run/secrets/app.pem:ro -e PRIVATE_KEY_FILE=/run/secrets/app.pem glossmark-app
```

[`docs/app/docker-compose.yml`](app/docker-compose.yml) is the same as a compose service, reading `APP_ID` and `WEBHOOK_SECRET` from `.env` and the key from `app.pem` beside it. Put a TLS-terminating proxy in front (GitHub delivers over HTTPS only) and point the App's webhook URL at it.

## What it posts, side by side with the Action

| | Action (`uses: mckalvan/glossmark@v0.3.0`) | App (`gloss app serve`) |
|---|---|---|
| Needs | a workflow file in each repository | installing the App once |
| Runs | on the repository's runners, with its secrets | on your host, with an installation token |
| Comment | `<!-- glossmark-impact:<root> -->`, one per `root:` | the same key, root workspace only |
| Gate | the job fails on `fail-on:` | the `GlossMark gate` check run: `action_required` on HARD |
| Approval | the `ontology-approved` label, with the workflow's `approval-reset` job | the same label, read on the `labeled` event; a push gates again |
| Reviewers | none | the owners of the HARD changes (`app.owners`, CODEOWNERS) |
| Also | `gloss check`, `gloss drift`, `gloss coverage`, `gloss build --check` | the gate only |

The Action's extra checks (drift, coverage, the committed graph) stay CI work: they judge the tree, not the change, and a team that wants them has already wired CI.
