<!-- @gloss documents gmi:MarkerSuggester scope=file -->
# ADR 0022: What suggest proposes, and from what evidence

Status: accepted, 2026-10-20. Closes issues #58 (F-POOR-1), #59 (F-POOR-2), #60 (F-POOR-3), #64 (F-POOR-7, the suggest half), #66 (F-CAL-2), #67 (F-CAL-3) and #68 (F-CAL-4).

## Context

`gloss suggest` matched a declaration's name, and its docstring only when the name scored low, against concept names through the search ranker. In the adoptions this went wrong in six ways:

- It found nothing in code whose names carry no domain words (#58).
- It matched programming words and fragments (`Handler` to `HazmatHandling`, `ent` to `Replenishment`), and scored them highest (#59).
- It never looked inside a class (#60).
- It was silent on an empty ontology (#64).
- It read `allowed: [derived, X]` literally, and its runs tripped GM131 (#66).
- It proposed four relations in source files only (#67), and proposed package clauses, fakes, SQL indexes and private helpers (#68).

## Decision

**Surfaces (#60, #68).** suggest proposes on a public surface, as MARKING.md defines one:

- a top-level declaration, or a method of a top-level class (`Manager.doIt`, `StringUtils.check`);
- a table, view, function or procedure in SQL;
- a Terraform resource or module;
- a test, which is a test function, a test class, or a Gherkin feature or scenario.

It skips:

- package clauses and Ruby namespace modules;
- Rust `impl` blocks;
- SQL indexes, types, triggers and `ALTER`s;
- Terraform data sources, variables and outputs;
- module-level variables, except an exported function value or component;
- language-private declarations: a leading `_`, Go lower case, Rust without `pub`, Java and C# `private`, TypeScript not exported;
- in a test path, everything that is not a test: fakes, stubs, fixtures and builders.

A method whose class already carries the same link is not proposed again.

**Evidence (#58, #69).** Each proposal names its evidence in `matched`. From strongest to weakest:

1. `named-in-doc`: a concept's doc names the declaration (`Service.Book: checks the slot...`).
2. `name-exact`, then `alias-exact`.
3. `calls`: a test calls a symbol that an implements link binds. The callee whose class or file the test names wins.
4. `name-contains` and `alias-contains`.
5. `topic`: a listener's or producer's topic names an event.
6. `doc`: the docstring or the comment block names the concept or an alias as a phrase.
7. `data`: the body names a table that a represents link binds to an entity, and exactly one process or capability in the ontology works on that entity. The tables touched are reported as `context` either way.
8. `name-partial` and `name-overlap`.

When the core concept wins, an implementation concept that realizes it and that the declaration also resembles is preferred. With several such concepts, only one the code calls or that names the declaration is preferred.

**Matching (#59).** Matching is on whole words after stemming. A symbol under four letters matches only an alias it equals. A word inside a concept-name word is no match: `ent` and `plen` do not match `Replenishment`. A built-in stoplist of programming words (handle, process, manage, run, do, data, util, helper, common, misc, get, set, service, ...) never carries a match on its own. `glossmark.yaml` extends it under `suggest: { stopwords: [...] }`. An alias is never filtered. Legacy table prefixes (`tbl_`, `tb_`, `t_`) are dropped before matching. The search tokenizer now stems twice, so `bookings` meets `Booking`. This also applies to `search_concepts`.

**Relations and files (#67).** Besides implements, represents, verifies and provisions, suggest proposes:

- `documents` on a Markdown heading;
- `deploys` on a Kubernetes workload (Deployment, StatefulSet, DaemonSet, Job, CronJob);
- `presents` on a page component, `hosts` on a layout or app shell;
- `subscribes` or `consumes` on a listener or consumer, `publishes` on a producer.

Provisions and deploys target only resource and system-type concepts. It reads `.md`, `.yaml`, `.yml` and `.sh` besides source files. It does not propose `configures`, `publishes-value`, `instructs`, `may-modify` or `produces`, or anything in sidecars, JSON, Dockerfiles or CI workflows ([ADR 0041](0041-surface-rules.md) later lets a surface rule propose `configures` on an admin page and `produces` on a document renderer). `gloss suggest -h` prints this coverage.

**Allowed set and runs (#66).** Every candidate is tested against the allowed set `gloss check` uses. This is `rollup.AllowedFor`, which expands the `derived` keyword, so check and suggest cannot disagree. After choosing, identical proposals are collapsed:

- When every proposal in a file is the same link, they become one `scope=file` marker at the top of the file. This takes three proposals, or two when they are tests.
- Otherwise, a run of three or more consecutive identical proposals becomes one `scope=block` marker above the first.

`suggest --write` output then passes `gloss check` without GM131, GM502 or GM503.

**Thin ontology (#64).** When an owned namespace has fewer than three concepts, suggest prints `<ns>: has N concepts; write the core tier first (adopt-glossmark step 3)` on stderr. For an implementation namespace the hint points to step 4. `--format json` prints `[]`, never `null`. `gloss suggest --concepts` clusters declarations by the tables they touch, and tables by their first domain word. It names each cluster after the domain words of its members' docstrings, else of its tables, and never after a stoplisted word. Each cluster is printed as an `ns:NEW:Name` candidate for `gloss propose --from-new`. Clusters with no domain word are marked as `internal: true` candidates. The `gloss init` half of #64, which is to skip utility directories, is left to the init work.

## Consequences

- Proposals on concept-poor code come from docstrings, tables and test calls. On the `warehouse-legacy` fixture all 18 default proposals are right, against 1 of 10 before.
- Fewer proposals are noise. Package clauses, fakes and indexes are gone, and the stoplist removes the `handl` family. Some honest matches are lost too: `tbl_loc` no longer matches `Location`, because `loc` is under four letters.
- The callee, owner and table maps make suggest two passes over the files in a fixed order: SQL and proto first, then code, then tests.
- `Decl` (internal/marker) gained `End`, `Parent` and `Decorators`. `Declarations` returns members after their class. suggest is its only caller.
