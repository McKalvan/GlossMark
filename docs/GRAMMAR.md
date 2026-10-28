<!-- @gloss documents gm:Link scope=file -->
# The `@gloss` marker grammar

A gloss mark is one line, inside whatever comment syntax the file already uses.

```
@gloss[?] <relation> <target>[, <target>...] [key=value ...]
```

| Part | Meaning |
| --- | --- |
| `?` | The link is a proposal nobody has accepted yet. Accepting is the one-character diff `@gloss?` → `@gloss` (`gloss accept`). |
| `relation` | kebab-case, one of the inline relations: `implements`, `represents`, `presents`, `hosts`, `verifies`, `provisions`, `configures`, `deploys`, `documents`, `instructs`, `may-modify`, `publishes-value`, `produces`, `consumes`, `publishes`, `subscribes`. `tests` is an accepted alias of `verifies` and means the same: it is stored in the graph as `verifies`, with `"alias":"tests"` in the edge provenance; `gloss check` asks for `verifies` (GM240). |
| `target` | A CURIE (`lib:Book`, `circ:MarcFieldMapper`), or `.Local` resolved against the enclosing manifest's `default-namespace`, or `ns:NEW:Name` to propose a concept that does not exist yet. `may-modify` targets may be globs (`circ:*`). |
| `key=value` | Only `scope` (`file`, `symbol`, `block`, `line`, `range`), `conf` (0–1), `id`, `note`. Values with spaces are double-quoted. Nothing else goes inline; provenance lives in the ledger. |

Targets may be in either tier. Use `circ:` for the mechanisms (handlers, pipelines, queues) that realize core processes; use a core concept directly when the artifact is the domain type (a `Book` model class represents `lib:Book`).

## Reference regex

Frozen by the design and shipped verbatim as `marker.ReferenceRegex`:

```
@gloss(\?)?\s+([a-z][a-z0-9-]*)\s+((?:[A-Za-z][\w-]*:|\.)[\w.-]+(?:\s*,\s*(?:[A-Za-z][\w-]*:|\.)[\w.-]+)*)((?:\s+[a-z]+=[^\s]+)*)
```

The parser uses an extended form that agrees with the reference on every plain marker and additionally accepts:

- the `NEW:` infix in a target (`circ:NEW:DamageCheck`), so a proposal of a new concept parses as one token;
- `*` in targets, for `may-modify` globs;
- double-quoted values (`note="inspecting returned items has no concept yet"`).

Any implementation in another language must pass the conformance corpus in `corpus/`.

## Rules

- Only text inside a comment is a marker. `"@gloss implements x"` in a string literal, a docstring or prose is ignored. Comment tokens come from a per-extension table (`#`, `//`, `/* */`, `--`, `<!-- -->`, `;`) that `comment-styles:` in `glossmark.yaml` can extend, keyed by extension, file name or path glob (`crontab`, `**/cron.d/*`). GraphQL SDL (`.graphql`, `.graphqls`, `.gql`) uses `#`; Prisma schemas (`.prisma`) use `//`. A file whose type has no comment style is not scanned; when it contains text that reads as a marker, `gloss check` says so (GM117) instead of skipping it silently.
- Inside a comment, text between a pair of backticks is quoted prose, so a comment can talk about `` `@gloss` `` or quote a whole marker without being one. In Markdown the body of a fenced code block (```` ``` ```` or `~~~`) is quoted as well. This is a scanner rule; the reference regex is unchanged.
- Malformed markers are build warnings at `file:line:col` (codes `GM001`–`GM007`), never silent drops. Positions are physical columns.
- Multi-line YAML-in-comment blocks are not supported; one marker, one line.
- The scanner reads lines, not the language's string syntax beyond one line. A line inside a heredoc, a multi-line string or an embedded script that starts with the file's comment token is read as a comment: `# @gloss implements .OrderFlow` on its own line inside a bash `<<EOF` body is a marker of the `.sh` file. Build such a line so it does not read as one (`printf '# @%s implements ...' gloss`), or keep the embedded code in its own file and mark it there.
- JSON has no comments: use a sidecar `foo.json.gloss` holding one marker per line, with JSON Pointers in `note=` and `scope=range` (`@gloss configures .ReturnEventBus scope=range note="/bus"`). A sidecar marker anchors to the file it annotates, never to the sidecar: a range anchors to the lines of the value its pointer names and fingerprints that value, and any other marker fingerprints the whole annotated file. A pointer that names nothing is a warning (GM123).
- SPDX-style headers are accepted as sugar and parsed into the same form with `scope=file`: `Gloss-Implements: circ:X`, `Gloss-May-Modify: circ:*`.
- Generated and vendored paths are excluded by the manifest's `generated:` globs or `exclude:` in `glossmark.yaml`; tag the generator input instead.

## Anchoring

A marker binds, in order of precedence, to an explicit `scope=`, else to the next declaration (function, class, resource block, YAML document, Gherkin Feature, SQL `CREATE`, Prisma model, Perl `sub` or `package`, Markdown heading), skipping any decorator or annotation stack above it (`@app.route(...)`, `@dataclass`, `@Override`, `#[derive]`, multi-line arguments included, so the marker goes above the decorators where formatters leave it), else to the file when it sits in the header comment, else to the next block. Markers never contain code identifiers; the anchor is derived and stored three ways:

1. path plus line range;
2. a symbol string (`catalog_book`, `HoldQueue.Fulfil`, `resource.aws_dynamodb_table.loans`, `Deployment/circulation`, `table.loans`, `feature:Checkout`) — a SCIP symbol once an indexer is joined;
3. a content fingerprint of the normalized signature line (`sha256:` prefix), preceded by the decorator or annotation stack the detector skipped, so deleting `@requires_role(...)` stales the link like a signature change ([ADR 0014](adr/0014-decorator-stack-in-fingerprint.md)).

Go methods are always qualified by their receiver. In other languages a method keeps its bare name unless another marked declaration in the same file has the same name; then each is qualified by its enclosing classes (`InboundHandler.process`, `OutboundHandler.process`), and declarations that still share a name, such as overloads, take an ordinal in file order (`StringUtils.check`, `StringUtils.check#2`). A qualified symbol's fingerprint covers the qualified name, so identical signatures in two classes differ ([ADR 0008](adr/0008-same-named-declarations.md)).

The line range ends where the declaration does: at the closing brace in brace languages, after a parameter list that spans lines (a Kotlin primary constructor) when the body opens after it, and at the line that closes a dollar-quoted body (`$$ LANGUAGE plpgsql;`) for a SQL `CREATE FUNCTION` or `CREATE PROCEDURE`. Kotlin functions are found with any modifiers (`suspend`, `inline`, `operator`, ...) and backtick names, recorded without the backticks.

A fingerprint change marks the link **stale** in the ledger until it is re-confirmed with `gloss accept <id>`. A `scope=file` fingerprint covers the whole file except lines that carry `@gloss` and the `gloss:begin`…`gloss:end` block, so accepting a proposal or adding a marker does not stale it (ADR 0006). In Phase 1 the declaration detector is regex-based per language; tree-sitter replaces it in Phase 2 without changing the graph.

## Examples across file types

```python
# @gloss implements circ:BookIntake
def catalog_book(isbn: str, copies: int) -> Book: ...
```
```ts
// @gloss? represents lib:Book conf=0.72
export interface Book { isbn: string; dueOn?: Date }
```
```go
// @gloss implements circ:CirculationService scope=file
package circulation
```
```hcl
# @gloss provisions circ:LoanStore
resource "aws_dynamodb_table" "loans" { ... }
```
```yaml
# @gloss deploys circ:CirculationService
apiVersion: apps/v1
kind: Deployment
```
```markdown
---
name: cataloging-assistant
description: Suggests subject headings for new acquisitions.
---
<!-- @gloss instructs circ:SubjectSuggester scope=file -->
<!-- @gloss may-modify circ:Catalog*, circ:Subject*, lib:Book scope=file -->
# Cataloging assistant
```
An agent or skill file whose YAML frontmatter must stay on line 1 takes its markers below the frontmatter, with an explicit `scope=file`; without it a marker binds to the next heading.
```sql
-- @gloss represents lib:Loan
CREATE TABLE loans (id uuid PRIMARY KEY, item_id uuid, patron_id uuid REFERENCES patrons, due_on date);
```
```gherkin
# @gloss verifies lib:Checkout
Feature: Checkout
```

## Diagnostics

Severities are `error`, `warning` and `hint`. Errors fail `gloss check`, warnings fail it under `--strict`, and a hint (the surface rules, GM25x) never fails it.

| Code | Meaning |
| --- | --- |
| GM001 | malformed marker |
| GM002 | unknown key (only scope, conf, id, note) |
| GM003 | duplicate key |
| GM004 | unexpected trailing text |
| GM005 | invalid scope |
| GM006 | conf outside [0,1] |
| GM007 | glob target on a relation other than may-modify |
| GM12x | target resolution (no default-namespace for `.Local`, sidecar target missing, symbol not detected, JSON Pointer not resolved) |
| GM117 | a file contains a marker but its type has no comment style, so it is not scanned: add it under `comment-styles:` (JSON takes a sidecar) |
| GM118 | unknown key in `views.yaml` (at the top or in a view); the rest of the file still applies |
| GM119 | a file in a scanned language is not scanned: a symlink out of the repository, not a regular file (a FIFO, a device), unreadable, or larger than 4 MiB |
| GM130 | a symbol-level marker repeats a file-level marker's relation and target: keep one |
| GM131 | the same relation and target on three or more consecutive declarations, whatever else is marked on them (two in a row is fine). The hint names what spans the run: for methods, one marker on the enclosing class or block; for top-level functions, where `scope=block` covers only the next one, one `scope=file` marker at the top of the file, one on the shared entry point, or, for a cross-cutting mechanism, `guards:` on its concept ([ADR 0015](adr/0015-guards.md)) |
| GM132 | two markers with the same marker id (same relation, targets and anchor, or a reused `id=`): delete one; `gloss accept <id>` flips both |
| GM223 | `hosts` on a concept that is not a system-type (nor a page concept that presents or hosts one): write `implements` if the code does the work, `presents` if it shows it |
| GM224 | `presents` on a constraint: a page that edits the value `configures` it, code that enforces it `implements` it |
| GM225 | a manifest `publishes` or `subscribes` a concept that is not an event (often after an upstream retype) |
| GM250 | hint: a page, command, route or resolver `presents` a concept nothing implements (inline, or through a page concept): the surface that computes it `implements` it ([surface rules](MARKING.md#surfaces)) |
| GM251 | hint: `hosts` on a recognised surface targets something other than a system-type: the command, route or resolver `implements` or `presents` it; the group, server or app frame `hosts` a system-type |
| GM252 | hint: a BFF route or GraphQL resolver `implements` (directly or through its own concept) a process another boundary implements, and the file presents nothing: `presents` it |
| GM253 | hint: a page `presents` a constraint, `implements` one an engine elsewhere enforces, or implements a page concept that presents a constraint without a criticality: a value editor `configures` it (or its page concept declares `criticality:`) |
| GM254 | hint: a document renderer `presents`, `represents` or `implements`: its output is the product, so it `produces` the entity |
| GM240 | `tests` written where `verifies` is meant: one test relation, `verifies`; `tests` is an alias |
| GM360 | a `value-from` names a file, line or symbol that does not exist, or, in YAML, TOML or JSON, a key that ends several key paths (the message lists them) |
| GM502 / GM503 | link outside the manifest's written `allowed` list (error) / outside the set derived from what it declares (warning); both are HARD in the gate |
