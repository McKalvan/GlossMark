<!-- @gloss documents gmi:SourcePlugin scope=file -->
# Plugins: link sources outside the binary

`gloss suggest` infers links from the specs a repository already has: an AsyncAPI contract, an OpenAPI document ([ADR 0049](adr/0049-links-inferred-from-specs.md)). Those two sources are built in. Every other format (GraphQL, Protobuf, Avro, a Terraform provider, the in-house registry) can be a **plugin**: an executable in any language that speaks a small JSON protocol over stdin and stdout, found on `PATH` or named in `glossmark.yaml` ([ADR 0050](adr/0050-external-link-sources.md)). This page is for the author of one.

A plugin answers two questions about a workspace, in plain data:

1. **Items**: which things does a spec file declare that a marker can anchor on (a channel, an operation, a type), with the relation a marker there means and the concept, or the names, it stands for?
2. **Hints**: which of those things does a code declaration name (a route the handler serves, a field the resolver resolves), and with what relation?

Everything after that is the core's: matching names against the vocabulary, the confidence from the plugin's own calibration bases, deduplication against hand-written markers and manifest `links:`, anchoring, the direction of a flow, marker rendering, the ledger. What a plugin returns is **proposals only**: `@gloss?` markers for a reviewer, never accepted links, and never auto-accepted until the repository has measured the plugin's precision.

## Install and run

| Where | How |
| --- | --- |
| `PATH` | an executable named `gloss-source-<name>` (`gloss-source-graphql`). `name` is lower-case letters, digits and hyphens. |
| `glossmark.yaml` | `sources: [{ name: graphql, command: [gloss-source-graphql] }]` or `command: "python3 tools/gql_source.py"` (split on spaces; a relative path is relative to the repository root). A config entry wins over `PATH` for the same name. |

`gloss source list` prints the sources a run in this repository uses and where each came from. `gloss suggest` then runs every plugin once per invocation, from the repository root, and says so on stderr (`gloss suggest: external sources: graphql`). Removing the plugin changes nothing but the absence of its proposals.

A plugin named like a built-in source (`asyncapi`, `openapi`) replaces it; `gloss source serve asyncapi` is the built-in run as a plugin, which is how the protocol is proved complete (the test `TestExternalAsyncAPIIsByteIdentical` checks `gloss suggest` prints the same patch either way). A plugin may not take a core source's name (`code`, `route`, `topic`, `schema`).

## The protocol (contract v1)

One JSON object per line, UTF-8, on stdin (from the core) and stdout (to the core). Stderr is the plugin's own: the core shows its last lines when the plugin fails. Nothing else is read from the environment, but `GLOSS_SOURCE_CONTRACT` holds the contract version the core speaks, for a plugin that wants to refuse early.

Every message is the same envelope with a `type` and the fields that type uses; unknown fields are ignored on both sides. The Go definition is `internal/infer/wire/wire.go`; the item, hint, declaration and concept shapes are those of `internal/infer/infer.go`.

```
core → plugin   index      contract, root, files[{path, dir, test}], vocabulary[concept]
plugin → core   read       paths[], globs[]            (optional, any number of times)
core → plugin   files      contents[{path, content}]
plugin → core   named      name, kinds[]               (optional: Vocabulary.Named)
core → plugin   concepts   concepts[concept]
plugin → core   ancestor   curies[]                    (optional: Vocabulary.CommonAncestor)
core → plugin   ancestor   curie
plugin → core   indexed    contract, name, items{path: [item]}, priors{basis: precision}
core → plugin   hints      decl                        (once per code declaration)
plugin → core   hints      hints[hint]
plugin → core   error      message                     (in place of indexed or hints)
```

The core closes stdin when it is done; the plugin exits. A plugin that does not exit within five seconds of that is killed.

### `index` (core → plugin)

- `contract`: the version the core speaks, `1`. Answer `indexed` with your own `contract`; if they differ the command fails naming both, so a plugin written for a later contract never produces a half-understood graph.
- `root`: the repository root, absolute. `files`: every scanned file, in workspace order, each with its repo-relative slash-separated `path`, the directory of its boundary manifest (`dir`, absent at the root) and `test: true` when it is a test path. A plugin may read files from `root` itself, or ask the core (`read`), which keeps it to files in the list.
- `vocabulary`: every concept the plugin may match against: `curie`, `name` (the local name), `kind` (`entity`, `event`, `process`, `capability`, `measure`, `constraint`, `resource`, `system-type`, ...), `aliases`, `is-a` (parent CURIEs), `realizes` (for an implementation-tier concept, the core concepts it realizes), `impl` (in an implementation tier), `deprecated`.

### `read`, `named`, `ancestor` (plugin → core, optional)

Questions the plugin may ask at any point before it answers, in any number. `read` names `paths` and/or `globs` (`**/*.graphql`; a glob without `/` matches the file name) and is answered with `files`, the content of every listed workspace file that matches, in workspace order. `named` is the core's exact-name lookup (`Appointments` finds `Appointment`; an alias counts; `kinds` restricts, core tier first), answered with `concepts`. `ancestor` asks for the nearest concept every one of `curies` is-a (itself included), answered with `curie` (empty when none). A plugin that only passes names on to the matcher needs neither lookup.

### `indexed` (plugin → core)

- `name`: the source id. It must be the name the plugin was discovered under; it becomes `Proposal.Source`, a `--source` value and the namespace of the calibration bases.
- `items`: per spec file (repo-relative path), the anchorable things it declares. Each item:
  - `symbol`: how the item is named in a proposal (`Mutation.placeOrder`, `claimsFiled`).
  - `role`: the item's kind for calibration (`channel`, `operation`, `schema`, `type`, ...). The basis of a proposal on it is `<name>/<basis>/<role>`.
  - `relation`: what a marker on the item means: `documents` for an operation or a channel, `represents` for a schema or a type.
  - `line`, `end`: the 1-based first and last line of the item's block. The marker goes above `line` with `scope=block`; an item whose `end` is its `line` gets `scope=line`. `indent`: the item's indentation, copied onto the marker.
  - `doc`: a description to match words in, if any.
  - the **target**: either `concept` (a CURIE the plugin resolved, through `named` or `ancestor`) or `names` (strings the core's matcher resolves, split into words: `"place order"`) restricted to `kinds` (an implementation concept that realizes one of them counts). `basis` names the evidence for the calibration bucket (`messages`, `name`, `operation`, `type`); `terms` is what matched, shown to the reviewer.
- `priors`: optional. What precision the author expects of each basis, by `basis` or `basis/role` (`{"type": 0.7, "field/func": 0.6}`), in (0, 1]. See calibration below.

A spec file needs a comment style for the marker: GraphQL SDL is not in the built-in table, so a repository adds `comment-styles: { ".graphql": ["#"] }` to `glossmark.yaml`. A file in a language `gloss suggest` does not read becomes a spec file when a plugin declares items in it.

### `hints` (core → plugin → core)

For every code declaration the core would propose on (not for spec items, headings, workloads, tables or resources), the core sends `decl`: `path`, `dir` (its boundary), `symbol`, `role` (`func`, `member`, `type`, `page`, ...), `line`, `decorators` (the lines above it) and `body` (its block, decorators included). The plugin answers `hints`, possibly empty. Each hint is a target as above, plus:

- `relation`: the relation the hint fixes (`implements`, `represents`); empty derives it from the role.
- or a **flow**: `channel` (the address the declaration names) and `message` (the message on it, when the code tells the channel's messages apart). A flow hint carries no relation: whether the code publishes or subscribes is code analysis, done by the core the same way for every source ([ADR 0049](adr/0049-links-inferred-from-specs.md) §3). `via` is the identifier or literal that named it, `line` the file line that did, which is what the core reads the direction from.

### `error` and failures

A plugin that cannot go on answers `error` with a `message` and may exit. The core reports it as a GM700 warning naming the plugin (`source plugin graphql (gloss-source-graphql, from PATH) reported: ...; its proposals are left out [GM700]`), together with the last lines of the plugin's stderr, and the other sources still run. The same happens when the plugin exits early, prints something that is not the protocol, answers under another name, or declares an item with a bad line range or a prior outside (0, 1]. Only a **contract mismatch** fails the command.

## Calibration: what a plugin's proposals are worth

A proposal's conf is the precision its basis measured on labelled links ([ADR 0021](adr/0021-suggest-conf-is-measured-precision.md)); SOFT, auto-accepted, is what measured above the repository's target ([ADR 0024](adr/0024-auto-accept-precision.md)). A plugin's bases are namespaced under its name (`graphql/type/type`, `graphql/field/func`), so they never borrow another source's numbers, and the shipped calibration cannot have measured them. A plugin basis reads its conf, in order:

1. the **repository's own calibration** when it measured the basis (its row, or the plugin's `<name>/*` row): `gloss suggest --calibrate --write` strips the repository's markers in a copy, runs every source including the plugins, scores the proposals against the repository's links and writes the counts to `suggest.calibration`. From then on the plugin's conf is its measured precision, and it auto-accepts when that precision clears the target like any other basis;
2. else the **prior** the plugin declared for the basis, **capped just below the auto-accept threshold in force** (0.76 under the default rule that derives 0.77): a prior ranks proposals for the reviewer and never auto-accepts;
3. else the unmeasured floor, 0.3, which is the lowest conf `gloss suggest` proposes at and below every threshold.

`gloss suggest --eval labels.tsv` and `--calibrate` print precision and recall per source, so the plugin's row is visible beside `code`, `asyncapi` and the rest.

## Writing one

The reference plugin is [`examples/plugins/gloss-source-graphql`](../examples/plugins/gloss-source-graphql/): a Go module of its own, with no dependency and no import from this repository, that reads GraphQL SDL and proposes `represents` on object types, `documents` on `Query`/`Mutation`/`Subscription` fields and `implements` on code that resolves a field. Its `README.md` says how to build and try it on the `graphql` case of `glossmark-examples/lab/ambiguous-surfaces`. A plugin in another language is the same loop: read a line, decode, answer. Keep to these rules, which are what [ADR 0049](adr/0049-links-inferred-from-specs.md) fixes for every source:

- Hand the core names and kinds whenever you are not certain of the concept; the matcher's tokenization and the allowed set of each boundary then apply as they do to everything else. Name a concept directly only when you resolved it through `named` or `ancestor`.
- Never decide a flow's direction, never return a relation the code has to settle, never return `holds`, `consumes` or `produces`.
- Use a few bases with clear meanings (`type`, `field`, `operation`) so a repository can measure them in a handful of labels.
- Be quiet on stdout outside the protocol: logging goes to stderr.
- Exit when stdin ends.

## A full exchange

The GraphQL plugin on a marker-stripped copy of the lab case, abridged (the vocabulary lists every concept; the files list every scanned file; one `hints` pair per declaration). Each line is one message.

```jsonl
→ {"type":"index","contract":1,"root":"/work/graphql","files":[{"path":"gateway/schema.graphql","dir":"gateway"},{"path":"gateway/src/resolvers.ts","dir":"gateway"},{"path":"gateway/test/resolvers.test.ts","dir":"gateway","test":true},{"path":"services/orders/orders/models.py","dir":"services/orders"},{"path":"services/orders/orders/placement.py","dir":"services/orders"}],"vocabulary":[{"curie":"hp:Order","name":"Order","kind":"entity"},{"curie":"hp:OrderPlacement","name":"OrderPlacement","kind":"process"},{"curie":"hp:CreditLimit","name":"CreditLimit","kind":"constraint"},{"curie":"hpx:OrderIntake","name":"OrderIntake","kind":"process","realizes":["hp:OrderPlacement"],"impl":true},{"curie":"hpx:CreditCheck","name":"CreditCheck","kind":"constraint","realizes":["hp:CreditLimit"],"impl":true}]}
← {"type":"read","globs":["**/*.graphql","**/*.graphqls","**/*.gql"]}
→ {"type":"files","contents":[{"path":"gateway/schema.graphql","content":"type Query {\n  me: Customer!\n  order(id: ID!): Order\n}\n\ntype Mutation {\n  placeOrder(input: PlaceOrderInput!): Order!\n}\n\ntype Customer {\n  id: ID!\n  name: String!\n}\n\ntype Order {\n  id: ID!\n  placedAt: String!\n  lines: [OrderLine!]!\n}\n"}]}
← {"type":"indexed","contract":1,"name":"graphql","items":{"gateway/schema.graphql":[{"symbol":"Query.me","role":"operation","relation":"documents","line":2,"end":2,"indent":"  ","names":["me"],"kinds":["process","capability","entity","event","measure"],"basis":"operation","terms":"me"},{"symbol":"Query.order","role":"operation","relation":"documents","line":3,"end":3,"indent":"  ","names":["order"],"kinds":["process","capability","entity","event","measure"],"basis":"operation","terms":"order"},{"symbol":"Mutation.placeOrder","role":"operation","relation":"documents","line":7,"end":7,"indent":"  ","names":["place order"],"kinds":["process","capability"],"basis":"operation","terms":"placeOrder"},{"symbol":"Customer","role":"type","relation":"represents","line":10,"end":13,"names":["customer"],"kinds":["entity","event","measure"],"basis":"type","terms":"Customer"},{"symbol":"Order","role":"type","relation":"represents","line":15,"end":19,"names":["order"],"kinds":["entity","event","measure"],"basis":"type","terms":"Order"}]},"priors":{"field":0.6,"operation":0.6,"type":0.7}}
→ {"type":"hints","decl":{"path":"services/orders/orders/models.py","dir":"services/orders","symbol":"Order","role":"type","line":4,"body":"class Order(models.Model):\n    customer_id = models.CharField(max_length=40, db_index=True)\n    placed_at = models.DateTimeField(auto_now_add=True)"}}
← {"type":"hints","hints":[{"names":["order"],"kinds":["entity","event","measure"],"basis":"type","terms":"Order","relation":"represents","via":"Order","line":4}]}
→ {"type":"hints","decl":{"path":"services/orders/orders/placement.py","dir":"services/orders","symbol":"place_order","role":"func","line":20,"body":"def place_order(customer_id: str, lines: list[Line]) -> Order:\n    check_credit(customer_id, sum(l.cents for l in lines))\n    return Order.objects.create(customer_id=customer_id)"}}
← {"type":"hints","hints":[{"names":["place order"],"kinds":["process","capability"],"basis":"field","terms":"placeOrder","relation":"implements","via":"place_order","line":20}]}
→ {"type":"hints","decl":{"path":"gateway/src/server.ts","dir":"gateway","symbol":"start","role":"func","line":9,"body":"export async function start(port = 4000) {\n  const server = new ApolloServer({ typeDefs, resolvers });\n  return startStandaloneServer(server, { listen: { port } });\n}"}}
← {"type":"hints"}
(stdin closes; the plugin exits)
```

What the core made of it (`gloss suggest --format text --min-conf 0.01`): the `Order` type is outside the gateway's allowed set (the gateway manifest presents `hp:OrderPlacement` and holds nothing), so no proposal; `Mutation.placeOrder` documents `.OrderIntake` at conf 0.60, the capped prior, to review; `models.py:Order represents hp:Order` is proposed from the core's own name match, the stronger evidence, with the plugin's hint agreeing.

```
gateway/schema.graphql:7  Mutation.placeOrder  documents .OrderIntake scope=line  conf=0.60 review  [operation:placeOrder]
services/orders/orders/models.py:4  Order  represents hp:Order  conf=0.75 review  [name-exact:Order]
```

## Other extension points

This is step B of [#209](https://github.com/McKalvan/GlossMark/issues/209), the first extension point outside the binary. Vocabulary importers (`gloss import --from <plugin>`) and graph contributors are the next, on the same mechanism: an executable named `gloss-<kind>-<name>`, versioned JSON on stdin and stdout, a contract handshake that fails the command naming both versions.
