# gloss-source-graphql

A reference link source for `gloss suggest` ([docs/PLUGINS.md](../../../docs/PLUGINS.md)): a GraphQL schema read through the plugin protocol, contract v1. It is a Go module of its own with no dependency and no import from the `gloss` module; what it shares with the core is the JSON on stdin and stdout, so it stands in for a plugin written in any language.

What it proposes:

| In | Proposal | Basis |
| --- | --- | --- |
| `type Order { ... }` in a `.graphql`, `.graphqls` or `.gql` file | `@gloss? represents hp:Order scope=block` above the type | `graphql/type/type` |
| `placeOrder(input: ...): Order!` under `Query`, `Mutation` or `Subscription` | `@gloss? documents hp:OrderPlacement scope=line` above the field (a query may also document the entity it returns) | `graphql/operation/operation` |
| code that defines or resolves a field (`def place_order(`, `placeOrder:` in a resolver map, as the function's own declaration) | `implements` the process the field names | `graphql/field/<role>` |
| a declaration named like a type (`class Order`) | `represents` what the type represents | `graphql/type/<role>` |

The plugin never names a concept itself: it hands the core the field or type name split into words and the concept kinds it may match (`process`/`capability` for an operation, `entity`/`event`/`measure` for a type), and the core's matcher, each boundary's allowed set, calibration, dedupe and anchoring decide. Until a repository measures its bases (`gloss suggest --calibrate --write`), proposals carry the declared prior (0.7 for a type, 0.6 otherwise) capped below the auto-accept threshold, so nothing from it auto-accepts.

## Build and try

```sh
cd examples/plugins/gloss-source-graphql
go build -o "$HOME/bin/gloss-source-graphql" .      # anywhere on PATH
cd /path/to/glossmark-examples/lab/ambiguous-surfaces/graphql/a-implements
gloss source list                                  # graphql  external  PATH: gloss-source-graphql
gloss suggest --format text --min-conf 0.01        # the schema's operations, and the plugin's hints on the code
gloss suggest --source graphql --format json       # its proposals alone
```

GraphQL SDL is not in the built-in comment table, so the schema file needs `comment-styles: { ".graphql": ["#"] }` in `glossmark.yaml` for a marker to anchor in it (the lab cases have it). A resolver written as an object literal (`export const resolvers = { Mutation: { placeOrder: ... } }`) is not a declaration `gloss suggest` proposes on; resolvers written as functions are.

Instead of `PATH`, `glossmark.yaml` may name the command:

```yaml
sources:
  - { name: graphql, command: [gloss-source-graphql] }
```

`go test ./...` in this directory runs the parser and a full protocol exchange; the integration with `gloss suggest` is covered by the tests of `internal/infer/wire` and `internal/suggest` in the main module.
