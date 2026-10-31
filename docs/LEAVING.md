<!-- @gloss documents gmi:Stripper scope=file -->
# Trying GlossMark safely, and leaving it

GlossMark writes into your source files, so it is fair to ask what it costs to back out. The short answer: one command, and a `git diff` you can read in a minute.

## What GlossMark adds to a repository

- **Markers** are comments: `# @gloss implements .Checkout`, in the file's own comment syntax, read by nothing but `gloss`. Your compiler, linter, formatter and test runner see a comment.
- **Files of its own**: `glossmark.yaml`, the ontology under `ontology/`, one `.gloss.yaml` manifest per boundary directory, `.glossmark/` (the built graph and the review ledger), `glossmark.lock` when you import a namespace, and a `foo.json.gloss` sidecar beside a JSON file you annotate.
- **Wiring you opted into**: a CI job, a `gloss` target in the Makefile or `package.json`, the agent hook in `.claude/settings.json`, the MCP server in `.mcp.json`, skills under `.claude/skills/`, and the vocabulary block `gloss agents --write` keeps in `AGENTS.md`.

Nothing is stored anywhere else. There is no service, no account and no database outside the tree; the graph is a JSON Lines file in git and `gloss export` writes it as SQLite or PROV-O whenever you like.

## Trying it

Run `gloss init` on a branch. Everything it writes is listed in its plan (`--dry-run`) and lands in the commit you make, so the trial is the branch: merge it or delete it. The markers an agent or `gloss suggest` proposes are `@gloss?` comments that a reviewer accepts or rejects; nothing is accepted on your behalf unless you set a policy for it.

## Leaving

```sh
gloss strip --dry-run   # what will be removed, what will be exported, what stays for you
gloss strip             # export, then remove; review with git diff
```

`gloss strip` is the reverse of `gloss init`. In order:

1. **A final export** goes to `glossmark-export/` (`--export dir` to choose, `--export -` for none): the graph as JSON Lines with its context (`graph.jsonl`, `context.jsonld`), as one JSON-LD document a standard processor loads (`graph.jsonld`), and as SQLite with the closure views (`graph.db`); the ledger as it is (`ledger.jsonl`) and as PROV-O (`ledger.prov.jsonld`); and copies of `glossmark.yaml`, the ontology files and every manifest. The vocabulary and the review trail survive the tool.
2. **Every marker goes.** `@gloss` and `@gloss?` lines in every comment style the scanner reads (`#`, `//`, `--`, `;`, `/* */`, `<!-- -->`, the SPDX-style `Gloss-Implements:` header, and the comment styles your `glossmark.yaml` adds), and JSON sidecars. The same scanner that builds the graph decides what a marker is, so a marker-shaped string literal, `@gloss` quoted in prose or a fenced code block in Markdown is left alone. A comment that held only the marker goes with its line (a trailing comment after code leaves the code); a comment with other text, or the opening line of a block comment, loses only the marker. The block between `<!-- gloss:begin -->` and `<!-- gloss:end -->` in `AGENTS.md` goes too.
3. **GlossMark's files go**: every `.gloss.yaml` manifest, `.glossmark/`, `glossmark.lock`, the sidecars, and `ontology/` and `glossmark.yaml` unless you pass `--keep-ontology`, which keeps the vocabulary readable in the tree while the markers leave.
4. **What stays is listed.** The hook, the MCP server, the skills, the CI job, the Makefile target and the `.gitignore` lines sit inside files that are not GlossMark's, so strip names each one with what to remove and leaves the file alone.

`exclude:` in `glossmark.yaml` and `generated:` in a manifest are honoured: a file the tool never read is a file it never edits. Afterwards `git diff` shows marker lines and GlossMark files removed and nothing else; `gloss strip --dry-run` on a committed tree, then `git diff --stat`, is the fastest way to see the scope before you decide. The export directory is new and untracked: commit it, move it out, or delete it.

## Coming back

The export is enough to resume. Put `glossmark.yaml` and `ontology/` back from `glossmark-export/`, the manifests from `glossmark-export/manifests/`, and `gloss suggest` proposes the markers again from the code, scored against `graph.jsonl` as labels (`gloss suggest --eval`); the ledger restores the review trail of every link whose anchor still matches.
