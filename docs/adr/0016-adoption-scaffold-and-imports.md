<!-- @gloss documents gmi:Scaffolder scope=file -->
# ADR 0016: What gloss init scaffolds, and where namespace IRIs come from

Status: accepted, 2026-10-20. Closes issues #35 (F-ADOPT-1), #36 (F-ADOPT-2) and #135.

## Context

The companion repository's adoptions (`glossmark-examples`) ran `gloss init` on eight layouts. It placed one manifest per top-level source directory, which in a monorepo is a grouping (`apps/`, `services/`) rather than the deployables under it, and gave a directory of Kubernetes YAML, Avro schemas or configuration none. It wrote `.github/workflows/gloss.yml` into a workspace below the git root, where GitHub never runs it, and `gloss doctor` then warned about CI that was correctly pinned at the git root. `--iri` was used as a namespace's literal parent (`<iri>core/`, `<iri>impl/`), so two sibling repositories initialised with the company's base IRI minted the same namespace IRIs, and `gloss landscape` merged `bc:` into `frt:` by IRI without a word. An importer had no way to say it imports the shared core rather than owning it.

## Decision

1. **Deployables, not groupings.** A top-level directory gets a container manifest when it holds source, schema (`.avsc`, `.proto`, `.graphql`, `.prisma`, ...), infrastructure (`.tf`, Kubernetes or Helm YAML) or configuration files. A top-level directory with no project file of its own (`package.json`, `go.mod`, `pyproject.toml`, `Cargo.toml`, `pom.xml`, `Dockerfile`, `Chart.yaml`, ...) and at least one child that has one is a grouping: each child with such files gets the manifest, named after the child (`apps/web` → `<b>:web`; a second `api` takes its path, `services-api`). The grouping itself gets none; its children are `part-of` the root.
2. **`--boundary-prefix`** sets the boundary prefix, which was always `<core>b`.
3. **The git root decides where CI goes.** When the workspace is below the root of its git repository, `init` writes no `.github/` and prints the job with `root: <workspace path>`. `gloss doctor` reads the git root's workflows as well as the workspace's, and counts one that names the workspace's path (a `root:` input or a matrix entry) as covering it.
4. **`--iri` is a base, per prefix.** Each owned namespace gets `<iri><prefix>/` and the repository's `repo-iri` is `<iri><name>/`. Without `--iri` the base is `https://onto.example.com/<name>/`, already per repository, and is the `repo-iri`; namespaces sit under it as `<base><prefix>/`.
5. **`gloss landscape` refuses two prefixes bound to one namespace IRI** across the repositories it unions, as it already refused one prefix owned twice.
6. **`--import <sibling>`** (repeatable) reads the sibling's `glossmark.yaml` and writes each core-tier namespace it owns under `imports:` (its IRI, `^<version>`, a `source:` path to its tier file). The imported namespace becomes the core prefix, no core tier file is written, the implementation and boundary prefixes come from this repository's own name, and `init` runs `gloss lock` before the first build.
7. **The plan lists everything `init` does**: the files, then `gloss lock`, `gloss hook install` (with the files it writes) and `gloss build`, which now writes `.glossmark/graph.jsonl` as QUICKSTART said it did. A `glossary.md` or `enums.yaml` in the tree is listed with the `gloss import` command that reads it, in the plan and in "Next:".

## Alternatives considered

- **Keep `<iri>core/` and `<iri>impl/` and only warn in landscape.** Rejected: the collision is minted at `init` and silent until a landscape run, by which time markers and imports already use the IRIs.
- **Write the workflow at the git root.** Rejected: `init` would edit outside the workspace it was pointed at, and a monorepo usually has one workflow with a matrix over workspaces; printing the job lets the owner add a matrix entry.

## Migration

None for existing workspaces: `init` never overwrites, and IRIs already written stay. A landscape of siblings that share a namespace IRI under two prefixes now fails; give each namespace its own IRI (and re-publish).
