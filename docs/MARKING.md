<!-- @gloss documents gm:Link scope=file -->
# When to mark, and with what

Five relations cover almost every marker in a real repository. Learn these first; the rest are on the [advanced page](ADVANCED.md).

| Write | On | Meaning |
| --- | --- | --- |
| `@gloss implements X` | the function, class or module that does the work | this code is the mechanism behind concept X (a process, capability, system-type, event, constraint or measure) |
| `@gloss represents X` | a model class, table, schema, record type | this is the data shape of entity X |
| `@gloss verifies X` | a test, a feature file, a contract test | this test checks X behaves as the ontology says |
| `@gloss documents X` | a Markdown heading or page | this prose explains X |
| `@gloss instructs X` | an agent file (`CLAUDE.md`, `.claude/agents/*.md`, `AGENTS.md`) | these instructions concern X |

Add `scope=file` when the whole file is about one concept and nothing inside it deserves its own link:

```python
# @gloss implements impl:ReportRenderer scope=file
```

## One marker per public surface

Mark the thing a reader would look for: the handler, the pipeline entry point, the model, the table, the test file. Do not mark every helper. Five consecutive helpers that each say `implements .AmountObfuscation` add five edges and no knowledge; `gloss check` reports any run of three or more (GM131) and suggests one marker with `scope=block` above it, or one on the public function that calls them.

Coverage counts concepts, not markers: an implementation concept is covered when at least one artifact links to it and at least one test verifies it (`gloss coverage`). Adding markers to raise the file percentage is the wrong lever; adding a test that `verifies` the concept is the right one.

## A file marker or symbol markers, not both

Either the file is about one concept (`scope=file`, one line in the header comment) or its declarations are about several (one marker per declaration). A file-level marker plus a symbol-level marker with the same relation and target state the same fact twice; `gloss check` reports the second one (GM130).

## Where the marker goes

Directly above the declaration, above any decorators:

```python
# @gloss implements impl:FoalingPipeline
@app.route("/foal", methods=["POST"])
def foal():
    ...
```

The declaration detector skips the decorator stack, including multi-line arguments, and anchors to `foal`. Formatters leave the comment there. Inside a block comment or a docstring is fine as long as the text is a comment; a marker inside a string literal is ignored.

## Core or implementation target?

Target the implementation concept when the artifact is a mechanism (`implements impl:AuthorizeHandler`), and the core concept directly when the artifact *is* the domain thing (`represents pay:Payment` on the Payment model, `verifies pay:Authorization` on the test of the business rule). The implementation concept's `realizes:` line carries the link to core; you do not repeat it in the marker.

## Pages and shells

A page, screen or report that *shows* a concept is not its mechanism. Model the page as `presents` (`pfi:FirePage: { kind: process, presents: pf:FirePlanning }`, or `// @gloss presents pf:FirePlanning` on the component) and the app frame as `hosts`. Neither makes the boundary realize the concept or inherit its criticality, so the engine's tests and the page's tests are counted apart ([ADR 0001](adr/0001-presents-and-hosts.md)).

## Proposals

Write `@gloss?` when you are not sure, and `conf=0.8` with an agent's confidence. On a proposal `gloss suggest` wrote, `conf` is the share of proposals with the same kind of evidence that were right on the calibration set, and `basis` in `--format json` names that kind ([ADR 0021](adr/0021-suggest-conf-is-measured-precision.md)). A human accepts it by deleting the `?` (`gloss accept <id>`). When the concept does not exist yet, write `ns:NEW:Name` and let `gloss propose --from-new` lift it into the ontology file as a proposed entry; never invent a concept name inline without `NEW:`.

## What `gloss suggest` drafts, and what it leaves to you

`gloss suggest` proposes on public surfaces only: top-level declarations and the methods of classes, tables and views, Terraform resources, tests, Markdown headings, Kubernetes workloads, and page and layout components. It skips package clauses, `impl` blocks, indexes, module-level variables, private declarations, and fakes and fixtures in test files. Its evidence is the declaration's name, a concept doc that names it, its docstring or comment, the tables its body touches, and the code a test calls. Programming words such as handler, manager and process never match on their own (extend the list under `suggest: stopwords:`). It proposes implements, represents, verifies, provisions, documents, deploys, presents, hosts, subscribes, consumes and publishes, and collapses a run of identical proposals into one `scope=file` or `scope=block` marker. Write `configures`, `publishes-value`, `instructs`, `may-modify` and `produces` by hand, and the markers in sidecars, JSON, Dockerfiles and CI workflows ([ADR 0022](adr/0022-what-suggest-proposes.md)).

## What not to mark

- Generated code: exclude it with `generated:` in the manifest and mark the generator's input.
- Vendored code: `exclude:` in `glossmark.yaml`.
- Glue that implements nothing a domain expert would name: leave it unmarked. An orphan concept (`gloss drift`) is a better signal than a forced link.
