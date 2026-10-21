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
# @gloss implements circ:OverdueReport scope=file
```

## One marker per public surface

Mark the thing a reader would look for: the handler, the pipeline entry point, the model, the table, the test file. Do not mark every helper. Five consecutive helpers that each say `implements .AmountObfuscation` add five edges and no knowledge; `gloss check` reports any run of three or more (GM131) and suggests one marker that spans it: on the enclosing class for methods, `scope=file` for top-level functions (a `scope=block` marker covers only the next function), or one on the public function that calls them. Authentication, rate limits and other cross-cutting mechanisms on every handler belong in the ontology instead: `guards: [SubmitClaimHandler, ...]` on the mechanism's concept ([ADR 0015](adr/0015-guards.md)).

Coverage counts concepts, not markers: an implementation concept, or a core concept that code implements directly, is covered when at least one artifact links to it and at least one test verifies it (`gloss coverage`). With no such concept in scope coverage is `n/a`, and `--fail-under` does not fail ([ADR 0013](adr/0013-data-shapes-and-coverage.md)). Adding markers to raise the file percentage is the wrong lever; adding a test that `verifies` the concept is the right one.

## A file marker or symbol markers, not both

Either the file is about one concept (`scope=file`, one line in the header comment) or its declarations are about several (one marker per declaration). A file-level marker plus a symbol-level marker with the same relation and target state the same fact twice; `gloss check` reports the second one (GM130).

## Where the marker goes

Directly above the declaration, above any decorators:

```python
# @gloss implements circ:ReturnPipeline
@app.route("/returns", methods=["POST"])
def check_in():
    ...
```

The declaration detector skips the decorator stack, including multi-line arguments, and anchors to `check_in`. The stack is part of the anchor's fingerprint, so deleting or changing a decorator such as `@requires_role("approver")` stales the link and shows in `gloss diff` as `decorators changed` ([ADR 0014](adr/0014-decorator-stack-in-fingerprint.md)). Formatters leave the comment there. Inside a block comment or a docstring is fine as long as the text is a comment; a marker inside a string literal is ignored. The scanner reads line by line, so a heredoc or a multi-line string is not a string to it: a line in a bash heredoc that starts with `#` is a comment of the script, and `# @gloss ...` there is a marker of the script (see [GRAMMAR.md](GRAMMAR.md#rules)). Assemble such a line instead (`printf '# @%s ...' gloss`).

## Core or implementation target?

Target the implementation concept when the artifact is a mechanism (`implements circ:CheckoutHandler`), and the core concept directly when the artifact *is* the domain thing (`represents lib:Loan` on the loans table, `verifies lib:Checkout` on the test of the business rule). The implementation concept's `realizes:` line carries the link to core; you do not repeat it in the marker.

## Pages and shells

A page, screen or report that *shows* a concept is not its mechanism. Model the page as `presents` (`pfi:FirePage: { kind: process, presents: pf:FirePlanning }`, or `// @gloss presents pf:FirePlanning` on the component) and the app frame as `hosts`. Neither makes the boundary realize the concept or inherit its criticality, so the engine's tests and the page's tests are counted apart ([ADR 0001](adr/0001-presents-and-hosts.md)).

Which form to use:

| The page | Write | Why |
| --- | --- | --- |
| has tests or logic of its own (form handling, validation, formatting rules) | a page concept, `pfi:QuotePage: { kind: process, presents: hp:Pricing }`, the component `implements .QuotePage`, its test `verifies .QuotePage` | the page is a concept, so it counts in coverage and `gloss drift` reports it `untested` when it has no test |
| is a thin component with no tests or logic of its own | inline `// @gloss presents hp:Pricing` on the component | nothing to test; the concept is `presented-only` until an engine implements it |
| edits the concept's value at runtime | `configures`, below | |

With the inline form the surface is not a concept, so it is never untested and never counts in coverage. Do not give its test `verifies hp:Pricing` to have something to link to: that credits the core concept with a test of a form, and inflates its verifiers (a page's test of a critical rule it does not enforce is the worst case). If the page has a test, it has earned a page concept.

## API and wire schemas

A GraphQL type, an OpenAPI schema or a protobuf message in a gateway looks like a data shape, but when the entity is stored elsewhere the gateway does not hold it. Mark it `presents hp:Order` (or leave it unmarked), not `represents`: `represents` makes the boundary `hold` the entity. Keep `represents` for the table, model or file where the data lives. `gloss check` says so when a `represents` falls outside the allowed set and another boundary declares `holds` for the concept (GM503).

## Value editors

An admin page or a settings screen that edits a concept's value at runtime (a credit limit stored in a row, a feature threshold) is neither the mechanism that enforces it nor only a view of it. Mark it `@gloss configures hp:CreditLimit` and declare `configures: [hp:CreditLimit]` in its boundary's manifest. The constraint stays drawn in the engine's box, the page takes the constraint's criticality (a mistake on it changes the rule), its test may `verify` the concept without GM503, and `gloss drift` compares the declaration with the code ([ADR 0012](adr/0012-surfaces.md)). A value bound to a line of code is `value-from` or `publishes-value` instead.

## Proposals

Write `@gloss?` when you are not sure, and `conf=0.8` with an agent's confidence. On a proposal `gloss suggest` wrote, `conf` is the share of proposals with the same kind of evidence that were right on the calibration set, and `basis` in `--format json` names that kind ([ADR 0021](adr/0021-suggest-conf-is-measured-precision.md)). A human accepts it by deleting the `?` (`gloss accept <id>`). When the concept does not exist yet, write `ns:NEW:Name` and let `gloss propose --from-new` lift it into the ontology file as a proposed entry; never invent a concept name inline without `NEW:`.

## What `gloss suggest` drafts, and what it leaves to you

`gloss suggest` proposes on public surfaces only: top-level declarations and the methods of classes, tables, views and Prisma models, Terraform resources, tests, Markdown headings, Kubernetes workloads, and page and layout components. It skips package clauses, `impl` blocks, indexes, module-level variables, private declarations, and fakes and fixtures in test files. Its evidence is the declaration's name, a concept doc that names it, its docstring or comment, the tables its body touches, and the code a test calls. Programming words such as handler, manager and process never match on their own (extend the list under `suggest: stopwords:`). It proposes implements, represents, verifies, provisions, documents, deploys, presents, hosts, subscribes, consumes and publishes, and collapses a run of identical proposals into one `scope=file` or `scope=block` marker. A deprecated concept is still a target, because brownfield code still says its name: the proposal carries `note="lib:Borrower is deprecated, replaced-by lib:Patron; ..."`, is never an auto-accept candidate (the gate classes a link to a deprecated concept HARD), and you choose between keeping it and linking the replacement. A `proposed: true` concept is not a target until it is accepted ([ADR 0030](adr/0030-suggest-targets-deprecated-not-proposed.md)). Write `configures`, `publishes-value`, `instructs`, `may-modify` and `produces` by hand, and the markers in sidecars, JSON, Dockerfiles and CI workflows ([ADR 0022](adr/0022-what-suggest-proposes.md)).

## What not to mark

- Generated code: exclude it with `generated:` in the manifest and mark the generator's input (a Prisma `schema.prisma`, not its `migrations/`). Neither `gloss build` nor `gloss suggest` reads a generated path.
- Vendored code: `exclude:` in `glossmark.yaml`.
- Glue that implements nothing a domain expert would name: leave it unmarked. An orphan concept (`gloss drift`) is a better signal than a forced link.
