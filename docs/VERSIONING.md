<!-- @gloss documents gmi:ImportLock, gmi:FormatVersions scope=file -->
# Versioning and stability

GlossMark has four version numbers. Each answers a different question, and `gloss version` (or `gloss --version`, `gloss -v`) prints all of them.

| Number | Answers | Where it lives |
| --- | --- | --- |
| Tool release `vX.Y.Z` | Which build of `gloss` am I running? | Git tag; injected at build time; `gloss version` |
| Grammar `N` | Which `@gloss` grammar is this repository written in? | `grammar: N` in `glossmark.yaml` (default 1) |
| File formats | Can this build read these `.glossmark/` files? | `"format": N` on the first line of `graph.jsonl` and `ledger.jsonl`; `format:` in `glossmark.lock` |
| Namespace `MAJOR.MINOR.PATCH` | Which version of this ontology am I importing? | `version:` per namespace; ranges under `imports:`; pins in `glossmark.lock` |

## Tool releases

Releases are git tags `vX.Y.Z` on this repository. The release workflow builds static binaries for Linux, macOS and Windows with the version injected, attaches them with sha256 checksums to a GitHub release, and `install.sh` downloads and verifies one. The same version is what `go install github.com/mckalvan/glossmark/cmd/gloss@vX.Y.Z` reports, because `gloss version` falls back to the Go module version when nothing was injected. The current release is `v0.2.0`.

Cadence: a release is cut whenever `main` carries a change an adopter needs (a MINOR for new commands, flags, finding kinds or file fields; a PATCH for fixes), and at least whenever a file format or the grammar changes, since those ship with the release that needs them. There is no fixed calendar.

Pin the tool where it runs; `@latest` and `@main` resolve to an unreleased build that reports `0.0.0-dev`:

- GitHub Action: `uses: mckalvan/glossmark@v0.2.0` (its `version:` input defaults to the same tag and downloads the binary).
- pre-commit: `rev: v0.2.0`.
- Shell: `curl -fsSL https://raw.githubusercontent.com/mckalvan/glossmark/main/install.sh | sh -s -- v0.2.0`.
- Local: `make build VERSION=v0.2.0` or `go install ...@v0.2.0`.

Tool semver means: a PATCH never changes any output for the same input; a MINOR may add commands, flags, finding kinds, MCP tools and file fields; a MAJOR may remove or rename any of those. Before 1.0 a MINOR may also remove, and the changelog says so.

## Grammar

The `@gloss` grammar is numbered independently of the tool, like Go's `go 1.24` directive. `grammar: 1` in `glossmark.yaml` is the current and default value. The guarantee:

- A marker that parses under grammar N parses identically under every tool release that lists N in `gloss version`.
- Additions to the grammar (a new relation, a new key) ship in a tool MINOR without bumping N, because they do not change the meaning of existing markers.
- A change that alters how an existing marker parses bumps N. The tool release that introduces N+1 keeps reading N, ships `gloss migrate --grammar N+1` to rewrite markers, and keeps N supported for at least two further MINOR releases. The deprecation is announced in the changelog and in `gloss check` as a warning.
- A tool refuses a repository whose `grammar:` it does not support, with the supported list in the message.

The reference regex and the conformance corpus in `corpus/` are the definition of grammar 1.

## File formats

`graph.jsonl` and `ledger.jsonl` start with a header line carrying `"format"`. `glossmark.lock` carries `format:`. The tool reads any format up to its own and refuses a newer one with a message naming the generator, so a committed graph never silently misparses after a tool upgrade. A format bump ships with the tool release that needs it, and `gloss build` rewrites the files; nothing else is required. Files without a header (written before format 1) are read as format 0.

## Namespaces

Every namespace carries semver in its ontology file (`version:` at the top of `<prefix>.gloss.yaml`). That file is what importers resolve and what `gloss diff` compares, so it is the only source. `owns.<prefix>.version` in `glossmark.yaml` may repeat it: when the two disagree `gloss check` fails (GM115), and a version given only in `glossmark.yaml` warns (GM114) because importers do not see it. The bump a change requires is classified by `gloss diff`:

| Bump | Change |
| --- | --- |
| MAJOR | a concept removed or retyped, or its is-a / part-of changed |
| MINOR | a concept or relation added; a concept deprecated or un-deprecated, or its `replaced-by` changed |
| PATCH | labels, docs, values |

A deprecation is MINOR because it changes what importers see (GM222 starts firing in their `check`), as semver 2.0.0 item 7 requires. A missing or undersized bump is HARD, like the change that needs it: importers' ranges rely on the bump, and a `~1.0.0` importer otherwise receives the change, or a GM107 failure, inside its range. A MAJOR bump is HARD. Removing a concept the base already deprecated is still MAJOR; the diff names its replacement instead of advising a deprecation. See [ADR 0018](adr/0018-gate-classification.md).

Importers state a range and get it enforced:

```yaml
imports:
  lib: { iri: https://onto.acme.dev/library/, version: "^1.0", source: ../circulation/ontology/lib.gloss.yaml }
```

Ranges accept `1.2.3`, `^1.2`, `~1.2.3`, `1.x`, `>=1.2 <2.0` and `*`. A resolved ontology outside the range fails `gloss check` (GM106); an imported ontology without a version warns (GM105).

`gloss lock` writes `glossmark.lock` with the resolved version, range, source and content hash of every import, and vendors each locked ontology to `.glossmark/imports/<prefix>@<version>.gloss.yaml`. Commit both. From then on every command resolves the import from that copy, so the lock is a pin: an upstream release does not change your build until you take it. `gloss check` warns (GM116) when the upstream now resolves to something else, naming the version and whether it is inside the range (`2.0.0 available, outside ~1.0.0`), or saying the content changed under the same version number (the upstream forgot to bump). Review it, then run `gloss lock --update` to move.

`gloss lock` refuses, exit 1 and the lockfile untouched, to record a version that is absent, not semver or outside the range; `--allow-out-of-range` overrides. A plain `gloss lock` keeps pinned imports where they are and pins new ones.

Without the vendored copy (a lock from an earlier release, or a deleted copy) the lock is a tripwire: `gloss check` reports (GM107) when the resolved version differs from the lock, when the content changed under the same version number, or when the lock is stale. See [ADR 0002](adr/0002-lock-pins-imports.md).

## Diagnostics

| Code | Meaning |
| --- | --- |
| GM105 | imported ontology has no version, or a non-semver one; the range cannot be checked |
| GM106 | resolved import version is outside the declared range |
| GM107 | lockfile disagrees with the resolved imports |
| GM108 | imports exist but no lockfile; run `gloss lock` |
| GM109 | lockfile unreadable or written by a newer tool |
| GM113 | imported namespace states no tier but has `published:` or `realizes:` concepts; it is read as core and the published seam is not enforced |
| GM114 | `owns.<prefix>.version` is set but the ontology file has no `version:` |
| GM115 | `owns.<prefix>.version` and the ontology file's `version:` disagree |
| GM116 | a pinned import's upstream now resolves to another version or content; run `gloss lock --update` after review |
