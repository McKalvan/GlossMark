# The `@gloss` marker grammar

A gloss mark is one line, inside whatever comment syntax the file already uses.

```
@gloss[?] <relation> <target>[, <target>...] [key=value ...]
```

| Part | Meaning |
| --- | --- |
| `?` | The link is a proposal nobody has accepted yet. Accepting is the one-character diff `@gloss?` → `@gloss` (`gloss accept`). |
| `relation` | kebab-case, one of the inline relations: `implements`, `represents`, `tests`, `verifies`, `provisions`, `configures`, `deploys`, `documents`, `instructs`, `may-modify`, `produces`, `consumes`, `publishes`, `subscribes`. |
| `target` | A CURIE (`zoo:Mare`, `impl:HoofStep`), or `.Local` resolved against the enclosing manifest's `default-namespace`, or `ns:NEW:Name` to propose a concept that does not exist yet. `may-modify` targets may be globs (`pa:*`). |
| `key=value` | Only `scope` (`file`, `symbol`, `block`, `line`, `range`), `conf` (0–1), `id`, `note`. Values with spaces are double-quoted. Nothing else goes inline; provenance lives in the ledger. |

Targets may be in either tier. Use `impl:` for the mechanisms (handlers, pipelines, queues) that realize core processes; use a core concept directly when the artifact is the domain type (a `Mare` model class represents `zoo:Mare`).

## Reference regex

Frozen by the design and shipped verbatim as `marker.ReferenceRegex`:

```
@gloss(\?)?\s+([a-z][a-z0-9-]*)\s+((?:[A-Za-z][\w-]*:|\.)[\w.-]+(?:\s*,\s*(?:[A-Za-z][\w-]*:|\.)[\w.-]+)*)((?:\s+[a-z]+=[^\s]+)*)
```

The parser uses an extended form that agrees with the reference on every plain marker and additionally accepts:

- the `NEW:` infix in a target (`impl:NEW:Gelding`), so a proposal of a new concept parses as one token;
- `*` in targets, for `may-modify` globs;
- double-quoted values (`note="castration step has no concept yet"`).

Any implementation in another language must pass the conformance corpus in `corpus/`.

## Rules

- Only text inside a comment is a marker. `"@gloss implements x"` in a string literal, a docstring or prose is ignored. Comment tokens come from a per-extension table (`#`, `//`, `/* */`, `--`, `<!-- -->`, `;`) that `comment-styles:` in `glossmark.yaml` can extend.
- Malformed markers are build warnings at `file:line:col` (codes `GM001`–`GM007`), never silent drops. Positions are physical columns.
- Multi-line YAML-in-comment blocks are not supported; one marker, one line.
- JSON has no comments: use a sidecar `foo.json.gloss` holding one marker per line, with JSON Pointers in `note=` and `scope=range`.
- SPDX-style headers are accepted as sugar and parsed into the same form with `scope=file`: `Gloss-Implements: impl:X`, `Gloss-May-Modify: pa:*`.
- Generated and vendored paths are excluded by the manifest's `generated:` globs or `exclude:` in `glossmark.yaml`; tag the generator input instead.

## Anchoring

A marker binds, in order of precedence, to an explicit `scope=`, else to the next declaration (function, class, resource block, YAML document, Gherkin Feature, SQL `CREATE`, Markdown heading), skipping any decorator or annotation stack above it (`@app.route(...)`, `@dataclass`, `@Override`, `#[derive]`, multi-line arguments included, so the marker goes above the decorators where formatters leave it), else to the file when it sits in the header comment, else to the next block. Markers never contain code identifiers; the anchor is derived and stored three ways:

1. path plus line range;
2. a symbol string (`breed_stallion`, `HorseBuilder.AttachHoof`, `resource.aws_sqs_queue.capture`, `Deployment/horse-factory`, `table.foals`, `feature:Stallion production`) — a SCIP symbol once an indexer is joined;
3. a content fingerprint of the normalized signature line (`sha256:` prefix).

A fingerprint change marks the link **stale** in the ledger until it is re-accepted. In Phase 1 the declaration detector is regex-based per language; tree-sitter replaces it in Phase 2 without changing the graph.

## Examples across file types

```python
# @gloss implements impl:StallionProduction
def breed_stallion(sire: Horse, dam: Horse) -> Stallion: ...
```
```ts
// @gloss? represents zoo:Mare conf=0.72
export interface Mare { id: string; foalingDate?: Date }
```
```go
// @gloss implements impl:HorseFactoryService scope=file
package horsefactory
```
```hcl
# @gloss provisions impl:FoalRegistry
resource "aws_dynamodb_table" "foals" { ... }
```
```yaml
# @gloss deploys impl:HorseFactoryService
apiVersion: apps/v1
kind: Deployment
```
```markdown
<!-- @gloss instructs impl:BreedingPlanner scope=file -->
<!-- @gloss may-modify impl:Breeding*, zoo:Foal scope=file -->
```
```sql
-- @gloss represents zoo:Foal
CREATE TABLE foals (id uuid PRIMARY KEY, dam_id uuid, sire_id uuid);
```
```gherkin
# @gloss verifies impl:StallionProduction
Feature: Stallion production
```

## Diagnostics

| Code | Meaning |
| --- | --- |
| GM001 | malformed marker |
| GM002 | unknown key (only scope, conf, id, note) |
| GM003 | duplicate key |
| GM004 | unexpected trailing text |
| GM005 | invalid scope |
| GM006 | conf outside [0,1] |
| GM007 | glob target on a relation other than may-modify |
| GM12x | target resolution (no default-namespace for `.Local`, sidecar target missing, symbol not detected) |
| GM130 | a symbol-level marker repeats a file-level marker's relation and target: keep one |
| GM131 | the same marker on consecutive declarations: one `scope=block` marker above the run says the same with one edge |
