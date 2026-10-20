<!-- @gloss documents gmi:ImportLock scope=file -->
# ADR 0002: `gloss lock` pins a vendored copy and refuses an out-of-range version

Status: accepted, 2026-10-20. Closes issue #107 (F-VER-2).

## Context

`glossmark.lock` recorded the version and hash of each import, and `gloss check` reported GM107 when the upstream file differed. It was a tripwire, not a pin:

- `gloss lock` wrote whatever it resolved and exited 0, even a version the import's range forbids or none at all. Following GM107's "run gloss lock" turned GM106 + GM107 into GM106 alone, and the committed lock then claimed `2.0.0` under `~1.0.0`.
- With a local-path import, every command read the upstream file on disk. An importer locked at 1.0.1 was built, drifted and landscaped against 2.0.0. It could not stay on the version it chose; it could only fail or move.

## Decision

1. `gloss lock` copies each locked ontology to `.glossmark/imports/<prefix>@<version>.gloss.yaml` (`@none` when unversioned) and removes copies nothing pins any more. The copy is committed with the lock.
2. Every command resolves an import from its copy while the lock pins it from the same `source` and the copy's hash matches the lock. The upstream is still resolved and compared: when it differs, `gloss check` warns (GM116) with the version available and whether it is inside the range, for example `2.0.0 available, outside ~1.0.0`. When the upstream is unreachable, its error becomes a warning and the copy is used.
3. `gloss lock` keeps a pinned import at its copy; `gloss lock --update` moves every import to what its source resolves to now. An import that is new, whose `source` changed, or that was locked before copies existed takes the upstream either way.
4. `gloss lock` refuses, with exit 1 and the lockfile untouched, when a version it would record is absent, not semver, or outside the range. `--allow-out-of-range` records it anyway.
5. Without a matching copy (a lock written by an earlier release, or a deleted copy), the lock is a tripwire as before: GM106 and GM107.

The lockfile format does not change; the copy's path is derived from the prefix and the locked version.

## Alternatives considered

- **Refuse only, no copy.** Fixes the false lock but leaves the importer unable to stay on its version.
- **Store the ontology inside `glossmark.lock`.** One file instead of two, but the lock stops being a short, reviewable diff, and nothing can point a `source:` at it.
- **Pin by git revision of the sibling checkout.** Only works for git sources and needs the fetcher; the copy works for local paths now, and the git fetcher can write the same copy.

## Migration

Run `gloss lock` once in each importer and commit `glossmark.lock` with `.glossmark/imports/`. Until then nothing changes: the lock behaves as a tripwire. To take an upstream release, review GM116, then run `gloss lock --update`.
