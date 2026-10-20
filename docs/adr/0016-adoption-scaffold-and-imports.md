<!-- @gloss documents gmi:Scaffolder scope=file -->
# ADR 0016: What gloss init scaffolds, where namespace IRIs come from, and what an importer remembers

Status: accepted, 2026-10-20. Closes issues #35 (F-ADOPT-1), #36 (F-ADOPT-2), #44 (F-ADOPT-10), #135 and #136; extended for #64 (F-POOR-7).

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

### Importers remember what they wrote (issue #136)

A brownfield adoption imported a fifteen-year-old staff glossary and an enums file. Re-running an importer undid review work: it compared the source with the current file, so a doc improved by hand was restored, a concept deleted (a section heading imported by mistake) or renamed came back under its old name. Decision:

8. **`.glossmark/import-record.jsonl`** (committed, format 1) holds, per namespace and concept, the doc and aliases the last import read from the source. A re-run updates a doc or an alias list only when the source changed since that import; when only the ontology changed, the hand edit is kept and reported as `kept`. A recorded concept missing from the ontology was deleted or renamed by hand and is not added again; deleting its record line re-enables it. Without a record line the importer behaves as before (compare with the file), so existing workspaces need no migration.
9. **Glossary parsing.** A heading with no field line followed by a deeper heading is a section, not a concept. Without a `Kind:` line the kind is read from the definition (a leading gerund or "the process of" → process; "never", "cannot", "at most", "how long", ... → constraint), then from the name. A deprecated status that says "use X" or "replaced by X" sets `replaced-by: X`. The command prints each added name with its kind and where it came from.
10. **`--value-names prefixed`** names every enum value `<Enum><Value>`. The default stays `bare` (with `<Enum><Value>` on a collision) so re-runs of existing imports do not add a second copy of every value.

### Unmarked files under a floor in the export (issue #44)

`criticality-floor: true` raises every artifact beneath a boundary, and `gloss impact` reports the floor for an unmarked file, but the SQLite export listed only artifacts, so the governance query missed exactly the unmarked files a floor exists to catch (a migration granting roles on patient tables). Decision:

11. `gloss export --sqlite` adds a `node='file'` row (id `urn:gloss:file:<path>`, `path`, `boundary`, `criticality`, `criticality_from` = the floor boundary) for every in-scope file with no artifact node beneath a floor. `graph.jsonl` is unchanged: a file without a marker is not a graph node. The governance query in GOVERNANCE.md reads `node IN ('artifact','file')`.

An `import-ignore:` list was not added: with the record, deleting an unwanted concept is remembered, which is the same decision made where the concept is.

### Utility directories are not boundaries (issue #64)

On concept-poor legacy code (`opstool`), `init` planned `core/` (a package of god classes) and `utils/` (a utility folder) as containers; the adopter deleted both manifests. Decision:

12. A directory named `util`, `utils`, `utilities`, `helper`, `helpers`, `common`, `commons`, `core`, `lib`, `libs`, `shared` or `misc`, at the top level or inside a grouping, gets a manifest only when it holds an entry point: a project file (as in 1), a build file (`Makefile`, `CMakeLists.txt`, `BUILD`, `Procfile`), a `Dockerfile`, or a source file named `main`, `__main__`, `server`, `wsgi`, `asgi`, `routes` or `Program`. Otherwise the plan lists it as "not a boundary" and its code stays under the parent's manifest. A `common/` with a `pom.xml` is still a container. The name list, not a content heuristic, decides, so a utility folder named `toolkit/` still gets a manifest that the adopter can delete.

## Alternatives considered

- **A hash per entry in the ontology file.** Rejected: the record would live in hand-edited YAML and be lost or copied with the entry; the record is tool-maintained, like the ledger.
- **Keep `<iri>core/` and `<iri>impl/` and only warn in landscape.** Rejected: the collision is minted at `init` and silent until a landscape run, by which time markers and imports already use the IRIs.
- **Write the workflow at the git root.** Rejected: `init` would edit outside the workspace it was pointed at, and a monorepo usually has one workflow with a matrix over workspaces; printing the job lets the owner add a matrix entry.

## Migration

None for existing workspaces: `init` never overwrites, and IRIs already written stay. A landscape of siblings that share a namespace IRI under two prefixes now fails; give each namespace its own IRI (and re-publish).
