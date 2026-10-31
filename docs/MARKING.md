<!-- @gloss documents gm:Link scope=file -->
# When to mark, and with what

Five relations cover almost every marker in a real repository. Learn these first; the rest are on the [advanced page](ADVANCED.md). A repository `gloss init` set up starts on the starter relation profile (`relations: starter` in `glossmark.yaml`): `gloss check` hints at a relation outside `implements`, `represents`, `verifies`, `documents`, `produces`, `consumes` and `part-of` until the file says `relations: full` ([ADVANCED.md](ADVANCED.md#relation-profiles)).

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

## Agent files

Claude Code agent and skill files (`.claude/agents/*.md`, `SKILL.md`) need their YAML frontmatter on line 1. Put the markers right below it, with `scope=file`:

```markdown
---
name: cataloging-assistant
description: Suggests subject headings for new acquisitions.
---
<!-- @gloss instructs circ:SubjectSuggester scope=file -->
<!-- @gloss may-modify circ:Catalog*, circ:Subject*, lib:Book scope=file -->
# Cataloging assistant
```

Without `scope=file` a marker below the frontmatter binds to the next heading (`# Cataloging assistant`), not to the file. `CLAUDE.md` and `AGENTS.md` have no frontmatter, so their markers go on line 1, still with `scope=file`. A new or changed `instructs` or `may-modify` link is HARD in `gloss diff`.

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
| is a server-rendered view, a CLI, a BFF route or a document renderer | the [surface rule](#surfaces) | the lab settled which relation each surface takes |

With the inline form the surface is not a concept, so it is never untested and never counts in coverage. Do not give its test `verifies hp:Pricing` to have something to link to: that credits the core concept with a test of a form, and inflates its verifiers (a page's test of a critical rule it does not enforce is the worst case). If the page has a test, it has earned a page concept.

## Surfaces

Six ordinary surfaces are each markable four or five ways, and the choice moves drift, coverage, criticality and placement. The ambiguous-surfaces lab (glossmark-examples, `lab/ambiguous-surfaces`) tried every way and settled one rule per surface ([ADR 0046](adr/0046-surface-rules.md)). `gloss check` recognises each surface from the file's path, type and imports and prints a hint (never a warning or an error) when a marker departs from the rule, with the rewrite; `gloss suggest` and the MCP `propose_link` default to the rule.

| Surface | Recognised by | Rule | Hint |
| --- | --- | --- | --- |
| A server-rendered page (a Django view, a Rails controller, a template, a page component) | `views.py`, a `pages/`, `views/`, `templates/` or `controllers/` directory, a template extension, `render(request`, `render_template(` | The view that computes the concept `implements` it, with `presents` added when it also shows it. `presents` alone is for a view of what an engine elsewhere computes; a page with tests of its own is a page concept | GM255: `presents` (inline or through a page concept) of a concept nothing implements |
| An admin page or settings screen | the page above under an `admin/`, `settings/` or `console/` directory | A page that edits a constraint's value `configures` it (and the manifest declares `configures:`). As a page concept it declares its own `criticality:`, or the page rates `none` | GM258: `presents` or `implements` of a constraint an engine elsewhere enforces; a page concept presenting a constraint with no criticality |
| A CLI | `@click.command`, `argparse`, `typer`, `cobra`, `commander`, `yargs`, `clap`, a `cli/` directory or `cli.py` | The command that does the work `implements` it. The group `hosts` a system-type, and `hosts` is never the only marker on a process | GM255 on `presents` with no mechanism; GM256 on `hosts` of anything but a system-type |
| A BFF or API gateway route | a `bff/` or `gateway/` directory | The route `presents` the backend process it reshapes. The rule it re-applies is implemented on the declaration that applies it (`implements hp:CreditLimit` on the helper, `scope=block` on inline code). The BFF's own concept presents, never realizes, the backend's process | GM257: `implements` (directly or through the BFF's concept) of a process another boundary implements, in a file that presents nothing; GM256 on `hosts` |
| A GraphQL gateway | an import of `graphql`, `@apollo/*`, `graphene`, `strawberry`, `gqlgen`; a resolvers file; `.graphql` | The resolver `presents` the process it delegates; the server `hosts` a system-type; a schema type `represents` the entity and the holder's manifest says who owns it | GM257 on a resolver that `implements` the backend's process; GM256 on a server that `hosts` it |
| A document renderer (a PDF invoice, a statement, a regulatory file) | an import of `reportlab`, `fpdf`, `weasyprint`, `pdfkit`, `jspdf`, `gofpdf`, `prawn`; `page.pdf(`; `pdf` in the file name | The output is the product: the renderer `produces` the entity and the manifest declares `produces:`. Not `presents` (the file rates `none`), not `represents` (the entity moves to the root box), not an implementation concept (it takes the engine's tests) | GM259: `presents`, `represents` or `implements` on the renderer |

Each hint ends with `[surface rule: <lab case>]` and names the marker to write. In doubt, the lab's test is: delete the file; if a rule stops being enforced or a number stops being computed anywhere, the file `implements`.

## API and wire schemas

A GraphQL type, an OpenAPI schema, a protobuf message or a cache table is a data shape too, so `represents hp:Order` is right on it. Representing a concept claims no ownership: the boundary where the data lives says so with `holds: [hp:Order]` in its manifest, and only that places the entity in its box ([ADR 0037](adr/0037-holds-is-declared-ownership.md)). When a `represents` falls outside the allowed set and another boundary declares `holds` for the concept, `gloss check` suggests widening `allowed:` (GM503).

## Value editors

An admin page or a settings screen that edits a concept's value at runtime (a credit limit stored in a row, a feature threshold) is neither the mechanism that enforces it nor only a view of it. Mark it `@gloss configures hp:CreditLimit` and declare `configures: [hp:CreditLimit]` in its boundary's manifest. The constraint stays drawn in the engine's box, the page takes the constraint's criticality (a mistake on it changes the rule), its test may `verify` the concept without GM503, and `gloss drift` compares the declaration with the code ([ADR 0012](adr/0012-surfaces.md)). A value bound to a line of code is `value-from` or `publishes-value` instead.

## Proposals

Write `@gloss?` when you are not sure, and `conf=0.8` with an agent's confidence. On a proposal `gloss suggest` wrote, `conf` is the share of proposals with the same kind of evidence that were right on the calibration set, and `basis` in `--format json` names that kind ([ADR 0021](adr/0021-suggest-conf-is-measured-precision.md)). A human accepts it by deleting the `?` (`gloss accept <id>`). When the concept does not exist yet, write `ns:NEW:Name` and let `gloss propose --from-new` lift it into the ontology file as a proposed entry; never invent a concept name inline without `NEW:`.

## What `gloss suggest` drafts, and what it leaves to you

`gloss suggest` proposes on public surfaces only: top-level declarations and the methods of classes, tables, views and Prisma models (represents), SQL functions and procedures (implements, like any routine), Terraform resources other than the ones that only support another (subnet and parameter groups, IAM roles and policies, attachments, event targets, task definitions, lifecycle, versioning and encryption settings, keys, security groups, log groups, alarms; [ADR 0032](adr/0032-suggest-calibration-set-grows.md)), tests, Markdown headings, Kubernetes workloads, and page and layout components. It skips package clauses, `impl` blocks, indexes, module-level variables, private declarations, and fakes and fixtures in test files. Its evidence is the declaration's name, a concept doc that names it, its docstring or comment, the tables its body touches, and the code a test calls. Programming words such as handler, manager and process never match on their own (extend the list under `suggest: stopwords:`). It proposes implements, represents, verifies, provisions, documents, deploys, presents, hosts, subscribes, consumes and publishes, and collapses a run of identical proposals into one `scope=file` or `scope=block` marker. A deprecated concept is still a target, because brownfield code still says its name: the proposal carries `note="lib:Borrower is deprecated, replaced-by lib:Patron; ..."`, is never an auto-accept candidate (the gate classes a link to a deprecated concept HARD), and you choose between keeping it and linking the replacement. A `proposed: true` concept is not a target until it is accepted ([ADR 0030](adr/0030-suggest-targets-deprecated-not-proposed.md)). On a recognised [surface](#surfaces) the surface rule sets the relation: `produces` on a document renderer, `configures` on an admin page that names a constraint, `presents` on a page, route, resolver or command for a process an engine elsewhere already implements; and a surface names the core concept it shows rather than the engine's implementation concept ([ADR 0046](adr/0046-surface-rules.md)). Write `publishes-value`, `instructs` and `may-modify` by hand, and `configures` and `produces` anywhere but those surfaces, as well as the markers in sidecars, JSON, Dockerfiles and CI workflows ([ADR 0022](adr/0022-what-suggest-proposes.md)).

## What not to mark

- Generated code: exclude it with `generated:` in the manifest and mark the generator's input (a Prisma `schema.prisma`, not its `migrations/`). Neither `gloss build` nor `gloss suggest` reads a generated path.
- Vendored code: `exclude:` in `glossmark.yaml`.
- Glue that implements nothing a domain expert would name: leave it unmarked. An orphan concept (`gloss drift`) is a better signal than a forced link.
