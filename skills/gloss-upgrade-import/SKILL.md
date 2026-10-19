---
name: gloss-upgrade-import
description: Upgrade a namespace this repository imports under glossmark.yaml imports - read the upstream's new version and changes, widen the version range, re-pin glossmark.lock, fix what the upgrade breaks, and list every MAJOR change it pulls in. Use when bumping an imported ontology, when an upstream repository released a new namespace version, or when gloss check reports GM105 to GM109.
---

<!-- @gloss instructs gm:Publication scope=file -->

# Upgrade an imported namespace

An import is another repository's ontology that this repository links to. `glossmark.yaml` states it under `imports:` with a version range, and `glossmark.lock` pins the version and content hash that was reviewed. The GlossMark rules in `AGENTS.md` apply. Editing `glossmark.yaml` is part of this task only because the human asked for the upgrade; say so in your summary.

```yaml
imports:
  pay: { iri: https://onto.acme.dev/pay/, version: "^1.2", source: ../payments-api/ontology/pay.gloss.yaml }
```

## 1. Read what changed upstream

1. Find the import's `source:`. It is a sibling checkout or a vendored copy of the upstream ontology file.
2. Read the new `version:` at the top of the upstream ontology file, and the old one from `glossmark.lock`.
3. Read the upstream's changelog or release notes for that range when it has them. Then diff the ontology file itself between the two versions: `git -C <upstream checkout> diff <old tag> <new tag> -- ontology/<ns>.gloss.yaml`, or the old vendored copy against the new one.
4. Classify each change by the namespace semver rules:

| Bump | Change |
| --- | --- |
| MAJOR | a concept removed or retyped, or its `is-a` or `part-of` changed |
| MINOR | a concept or relation added |
| PATCH | labels, docs, values |

5. Check that the upstream bumped its version to match. Changed content under the same version is what GM107 reports.

## 2. Widen the range and re-pin

1. Change only the `version:` range of this import in `glossmark.yaml`, for example `^1.2` to `^2.0`. Ranges accept `1.2.3`, `^1.2`, `~1.2.3`, `1.x`, `>=1.2 <2.0` and `*`.
2. When `source:` is a vendored copy, replace that file with the upstream's file at the new version, byte for byte. When it is a sibling checkout, check out the new version there. Never edit an imported ontology file to make a link resolve.
3. Run `gloss lock` and include `glossmark.lock` in the change.

## 3. Check

Run `gloss check`. The import codes and their fixes:

| Code | Meaning | Fix |
| --- | --- | --- |
| GM105 | The range cannot be checked: the upstream declares no semver `version:` | Ask the upstream to set one, or drop the range |
| GM106 | The resolved version is outside the range | Widen the range after review, or point `source:` at a version inside it |
| GM107 | The lock disagrees with what resolves | Review the upstream change, then `gloss lock` |
| GM108 | The repository imports namespaces but has no lock | `gloss lock` |
| GM109 | The lock cannot be parsed, or a newer gloss wrote it | Upgrade gloss, or delete the lock and `gloss lock` |

An upgrade can also break links. GM215 means a linked concept was removed or renamed. GM222 means it is deprecated: retarget to its `replaced-by:`. GM230 means it is no longer published. GM350 means the upstream shipped a `proposed: true` concept, which the upstream owner must accept or remove. For any other code, run `gloss explain <code>`.

Retarget a broken link to the upstream's replacement and keep it as a proposal, `@gloss? <relation> <target> conf=<0-1>`. When the upstream has no replacement and no owned concept fits, follow the proposal rule:

If no concept fits, write `@gloss? <relation> <ns>:NEW:<Name> note="why"`, then run `gloss propose --from-new --doc "what it is"` (MCP: propose_concept) so the concept ships in the same PR with `proposed: true`; a human accepts it with `gloss accept --concept <ns>:<Name>`. Never invent a concept name any other way, and never hand-edit an ontology file to make a link resolve.

A new concept can only be proposed in a namespace this repository owns (GM213). A concept the upstream should own is a request to the upstream's owner: name it in your summary.

## 4. Drift and landscape

1. Run `gloss drift`. A violation or gap the upgrade caused is part of this change; use the `gloss-resolve-drift` skill.
2. When the upstream and its other importers are checked out next to this repository, run `gloss landscape . <upstream> <other importers>` to see cross-repository drift and the context-map edges the upgrade changed.
3. Run `gloss build -q` and include `.glossmark/graph.jsonl` and `.glossmark/ledger.jsonl` in the change.

## 5. Report

End your summary with:

- the namespace, the old and new version, and the old and new range;
- every MAJOR change the upgrade pulls in, and what this repository changed because of it;
- every link you retargeted or proposed, and every request for the upstream's owner;
- what `gloss diff --base <base branch>` reports. MAJOR bumps and changes to cross-repository imports are HARD; a human reviews them in the PR.
