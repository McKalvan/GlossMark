<!-- @gloss documents gmi:FileFormats scope=file -->
# GlossMark schemas

Machine-readable descriptions of every file GlossMark reads or writes. The JSON
Schemas are draft 2020-12 and each carries a stable `$id` under
`https://glossmark.dev/schemas/`. **The Go types in `internal/model`,
`internal/graph` and `internal/marker` are the source of truth**; these schemas
are derived from them (field names, enums, required fields and strict-key
behaviour) and should be updated whenever those structs change.

## Using the schemas

In an editor with the YAML language server (VS Code "YAML" extension and
friends), put a modeline at the top of a file to get completion and inline
validation, for example:

```yaml
# yaml-language-server: $schema=https://glossmark.dev/schemas/glossmark.json
```

or point at the checked-out file with a relative path such as
`$schema=../schemas/glossmark.schema.json`.

From the command line, [`check-jsonschema`](https://github.com/python-jsonschema/check-jsonschema)
validates YAML or JSON files directly:

```sh
check-jsonschema --schemafile schemas/glossmark.schema.json glossmark.yaml
check-jsonschema --schemafile schemas/ontology.schema.json ontology/*.gloss.yaml
check-jsonschema --schemafile schemas/manifest.schema.json $(git ls-files '*.gloss.yaml' | grep '/\.gloss\.yaml$\|^\.gloss\.yaml$')
check-jsonschema --schemafile schemas/views.schema.json ontology/views.yaml
```

The JSONL schemas describe one line at a time; validate them with a short loop
(`jq -c . file.jsonl | while read l; do echo "$l" | check-jsonschema --schemafile ... -; done`)
or with the `jsonschema` Python package.

## Files

### `glossmark.schema.json`

Validates the repo-level `glossmark.yaml` (`model.Config`): the `owns` and
`imports` namespace tables keyed by CURIE prefix, extra `prefixes`, the `views`
path, `exclude` and `test-paths` globs, `comment-styles` overrides and the gate
`policy` with its waivers and `criticality-gates`. Unknown keys are rejected, matching the Go decoder's
`KnownFields(true)`; `tier` must be `core` or `implementation`.

### `ontology.schema.json`

Validates a `*.gloss.yaml` ontology file (`model.Ontology`, `model.Concept`,
`model.Published`, `model.RelationDecl`). `concepts` maps PascalCase names to
concepts; the structural keys (`kind`, `label`, `doc`, `is-a`, `part-of`,
`has-part`, `realizes`, `published`, `internal`, `deprecated`, `replaced-by`,
`value`, `shapes`, `criticality`) are typed and every other kebab-case key is a relation whose
value is a target or a list of targets. `relations` declares or tunes relation
types. Note the Go loader is lenient about unknown top-level keys; the schema is
strict so that typos are caught in the editor.

### `manifest.schema.json`

Validates a per-directory `.gloss.yaml` boundary manifest (`model.Manifest`):
the required `boundary` CURIE, its `level`, the declared intent relations
(`part-of`, `realizes`, `produces`, `consumes`, `publishes`, `subscribes`),
`default-namespace`, the `allowed` CURIE globs, `generated` globs, `owner` and
`criticality`.
Unknown keys are rejected, as in Go.

### `views.schema.json`

Validates `views.yaml` (`model.Views`, `model.View`): named projections with
`include`/`exclude` CURIE patterns, `collapse` rules, `expand` depths,
`show-published`, `relations`, `lift`, `zoom` and `boundaries`. Unknown keys are
rejected, as in Go.

### `evidence.schema.json`

Validates one line of `gloss harvest` output or of a file passed to
`gloss harvest --ingest` (`harvest.Evidence`): the header
(`type: evidence`), then records with a `signal` from a closed list, the
`surface` as written and a `source` whose `family` the signal fixes, with
either a repository `path` or an external `uri`. A test keeps the signal and
family lists equal to the code's.

### `graph-record.schema.json`

Validates one line of `.glossmark/graph.jsonl`: a `oneOf` over the context
header (`type: context`), node records (`type: node`, `graph.Node`, with an
`anchor` shaped like `marker.Anchor` on artifacts) and edge records
(`type: edge`, `graph.Edge` with `prov` as `graph.Prov`). Keys are the exact
JSON tags from the Go structs.

### `ledger-entry.schema.json`

Validates one line of `.glossmark/ledger.jsonl` (`model.LedgerEntry`): the
marker id, path, relation and targets, who placed it and when, its review
`status` (`proposed`, `accepted`, `rejected`, `stale`, `removed`) and the anchor
fingerprint used to detect staleness.

### `backstage-export.schema.json`

Validates what `gloss export --backstage` and `gloss landscape --backstage`
write ([ADR 0053](../docs/adr/0053-portal-export.md)): a boundary document
(`kind: boundary`, `backstage.Document`), one per manifest as `<slug>.json`,
with the boundary's identity and criticality, its concepts with the code and
tests behind them, its coverage, its open drift findings and the last review
decisions under it; and the index (`kind: landscape`, `backstage.Landscape`),
`landscape.json`, listing every boundary across repositories with its drift
summary and, from `gloss landscape`, the context map. Both carry `format: 1`.
Validate a directory with
`check-jsonschema --schemafile schemas/backstage-export.schema.json glossmark/*.json`.

### `graph.linkml.yaml`

A [LinkML](https://linkml.io) schema for the graph metamodel behind
`graph.jsonl`: the abstract `Node` class with `Concept`, `Boundary`,
`Artifact`, `Namespace` and `Actor` subclasses, `Edge`, `Provenance` and
`Anchor`, plus the `NodeType`, `Tier`, `ConceptKind`, `BoundaryLevel`,
`AnchorScope`, `EdgeSource` and `RelationType` enumerations. Where a builtin
relation has an obvious standard counterpart it carries a `meaning`
(`is-a` → `rdfs:subClassOf`, `same-as` → `owl:sameAs`, `derived-from` →
`prov:wasDerivedFrom`). Use it with the LinkML toolchain, for example
`gen-json-schema schemas/graph.linkml.yaml` or `gen-owl`, to produce other
artefacts; the hand-written `graph-record.schema.json` remains the schema that
matches the JSONL byte for byte.
