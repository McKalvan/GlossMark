---
name: gm-release
description: Cut a GlossMark release - run make release-check, choose MINOR or PATCH by docs/VERSIONING.md, move the Unreleased changelog entries under the version, tag, and confirm the Action, install.sh, pre-commit and doctor pins match. Use only when a maintainer asks for a release or a release candidate.
---

<!-- @gloss instructs gmi:GlossCli scope=file -->

# Release

Only a maintainer decides to release. You prepare and check; tag and push only when they say so in this session.

1. **Green main.** On an up-to-date `main`, run `make release-check` (gofmt, `go vet ./...`, `go test ./...`, and the version it would embed). Also run the example commands from AGENTS.md, `./gloss check` and `./gloss drift` at the root, and read `./gloss doctor` (its "CI pins" warning is expected here, see AGENTS.md).
2. **Choose the number** from `CHANGELOG.md`'s Unreleased section and the tool-semver rules in `docs/VERSIONING.md`: PATCH when no output changes for the same input; MINOR for new commands, flags, finding kinds, MCP tools or file fields; MAJOR (or, before 1.0, a MINOR that says so) for anything removed or renamed. A grammar or format bump ships in the release that needs it and is named in the entry. `docs/AGENT-CONTRACT.md` says which agent-facing changes are MAJOR.
3. **Changelog.** Rename `## Unreleased` to `## vX.Y.Z - YYYY-MM-DD`, keep its Added / Changed / Fixed / Removed groups, and start a new empty `## Unreleased` above it. Deprecations get their own line.
4. **Pins.** When the version differs from the one the docs pin, update every pin in one commit. List them with `grep -rn "v0\.1\.0" --exclude-dir=.git --exclude-dir=.glossmark .` (substitute the old version): `action.yml` (`version:` default), `.pre-commit-hooks.yaml`, `hooks/pre-commit`, `install.sh`, `README.md`, `docs/QUICKSTART.md`, `docs/VERSIONING.md`, `internal/doctor/doctor.go` and its test, `internal/scaffold/scaffold.go`. Leave `docs/PLAN.md`, `docs/proposals/` and `docs/reviews/` as the historical record.
5. **Plugin.** If `.claude-plugin/plugin.json` exists, set its `version` to the new number and run `claude plugin validate .`.
6. **Tag** when the maintainer says so: `git tag vX.Y.Z && git push origin vX.Y.Z`. `.github/workflows/release.yml` runs the tests, builds the five binaries with the version injected, and attaches them with sha256 files to a GitHub release.
7. **Confirm** the release page has every binary, `curl -fsSL https://raw.githubusercontent.com/mckalvan/glossmark/main/install.sh | sh -s -- vX.Y.Z` installs a `gloss version` that prints the number, and a workflow with `uses: mckalvan/glossmark@vX.Y.Z` downloads it rather than falling back to `go install`.
