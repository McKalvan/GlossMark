# Versioning and stability

GlossMark has four version numbers. Each answers a different question, and `gloss version` prints all of them.

| Number | Answers | Where it lives |
| --- | --- | --- |
| Tool release `vX.Y.Z` | Which build of `gloss` am I running? | Git tag; injected at build time; `gloss version` |
| Grammar `N` | Which `@gloss` grammar is this repository written in? | `grammar: N` in `glossmark.yaml` (default 1) |
| File formats | Can this build read these `.glossmark/` files? | `"format": N` on the first line of `graph.jsonl` and `ledger.jsonl`; `format:` in `glossmark.lock` |
| Namespace `MAJOR.MINOR.PATCH` | Which version of this ontology am I importing? | `version:` per namespace; ranges under `imports:`; pins in `glossmark.lock` |

## Tool releases

Releases are git tags `vX.Y.Z` on this repository. The release workflow builds static binaries for Linux, macOS and Windows with the version injected and attaches them to a GitHub release. The same version is what `go install github.com/mckalvan/glossmark/cmd/gloss@vX.Y.Z` reports, because `gloss version` falls back to the Go module version when nothing was injected.

Pin the tool where it runs:

- GitHub Action: `uses: mckalvan/glossmark@v0.2.0` and `with: { version: v0.2.0 }`.
- pre-commit: `rev: v0.2.0`.
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

Every namespace carries semver, owned ones in `glossmark.yaml` (`owns.<prefix>.version`) or in the ontology file, imported ones from the file that is resolved. The bump a change requires is classified by `gloss diff`:

| Bump | Change |
| --- | --- |
| MAJOR | a concept removed or retyped, or its is-a / part-of changed |
| MINOR | a concept or relation added |
| PATCH | labels, docs, values |

A missing bump is reported as SOFT; a MAJOR bump is HARD.

Importers state a range and get it enforced:

```yaml
imports:
  pay: { iri: https://onto.acme.dev/pay/, version: "^1.2", source: ../payments-api/ontology/pay.gloss.yaml }
```

Ranges accept `1.2.3`, `^1.2`, `~1.2.3`, `1.x`, `>=1.2 <2.0` and `*`. A resolved ontology outside the range fails `gloss check` (GM106); an imported ontology without a version warns (GM105).

`gloss lock` writes `glossmark.lock` with the resolved version, range, source and content hash of every import. `gloss check` then reports (GM107) when the resolved version differs from the lock, when the content changed under the same version number (the upstream forgot to bump), or when the lock is stale. Commit the lockfile; run `gloss lock` after a deliberate upgrade.

## Diagnostics

| Code | Meaning |
| --- | --- |
| GM105 | imported ontology has no version, or a non-semver one; the range cannot be checked |
| GM106 | resolved import version is outside the declared range |
| GM107 | lockfile disagrees with the resolved imports |
| GM108 | imports exist but no lockfile; run `gloss lock` |
| GM109 | lockfile unreadable or written by a newer tool |
