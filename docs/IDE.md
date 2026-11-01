<!-- @gloss documents gmi:LanguageServer scope=file -->
# Editor support: `gloss lsp`

Markers are written and read in the editor, and until now the editor knew nothing about them: no completion, no hover, no squiggle until CI failed. `gloss lsp` is a [Language Server Protocol](https://microsoft.github.io/language-server-protocol/) server built into the `gloss` binary; the extension under [`editors/vscode`](../editors/vscode/README.md) is a thin client of it, and any other LSP client (Neovim, Helix, Emacs, JetBrains through its LSP support) can start it the same way. The design is [ADR 0051](adr/0051-language-server.md).

```sh
gloss lsp                                  # what an editor starts: LSP over stdio
gloss lsp --log /tmp/gloss-lsp.log         # with one line per event, to see what it does
```

The server reuses the loader, extractor, linter, comparator and ledger: what it shows is what `gloss check`, `gloss drift` and `gloss accept` would say, over the open buffers before they are saved. It adds no rule of its own.

## What the editor gets

| Feature | LSP method | What it does |
| --- | --- | --- |
| Diagnostics | `textDocument/publishDiagnostics` | `gloss check` over the workspace with the open buffers laid over the tree, re-run (debounced, 300 ms) on every edit, save, open and close, and when `glossmark.yaml`, a manifest or an ontology file changes on disk. Each diagnostic carries its `GM` code as the code and `gloss` as the source; errors and warnings keep their severity, a hint (GM250, the starter relation profile) is an LSP *Hint*, which editors render faded. Diagnostics on files that are not open (an ontology file, a manifest) are published too, so the Problems panel shows the whole `gloss check` |
| Completion | `textDocument/completion` | After `@gloss ` or `@gloss? `: the relations the repository's [relation profile](adr/0042-relation-profiles.md) allows inline, with their docs. After the relation: the concepts, ranked by the boundary's [allowed set](ADVANCED.md#allowed-lists) (those outside it are marked `HARD`, as `gloss check` would warn), the relation's range kinds first within each group, each with its kind, criticality and doc; a word starting with `.` offers the default namespace's concepts as `.Name`; after a comma, the next target. Then the `scope=`, `conf=` and `note=` keys |
| Hover | `textDocument/hover` | On a concept reference (`ns:Name`, or `.Local` on a marker line): kind, criticality, tier, owner, published name, the doc, its declared relations, where code implements, represents, verifies and documents it, the boundaries that declare it, its drift findings and the ontology entry. On `ns:NEW:Name`: that it is a rogue until `gloss propose --from-new`. On a marker line away from a concept: the link, its anchor and scope, and its ledger row (status, who wrote it, who accepted it, the gate, the marker id) |
| Go to definition | `textDocument/definition` | A concept reference jumps to its entry in the ontology file (an imported one too, in its resolved copy) |
| Code actions | `textDocument/codeAction` | **Accept proposal** on a `@gloss?` line runs `gloss accept path:line`: the one-character diff in the file and the ledger row, through the same code as the command, so the file must be saved first (an unsaved buffer is refused rather than raced). **Propose new concept** inserts `@gloss? implements ns:NEW:Name` (or `verifies` in a test file) above the declaration on the line, in the file's comment style and the boundary's default namespace, with the declaration's name for you to rename. **Go to ontology entry** for each concept on the line |
| Status bar | `glossmark/fileInfo` (custom request) | The boundary the file sits in, its level and criticality, its manifest and default namespace, the links the file carries and how many are proposed |
| Rebuilt | `glossmark/built` (custom notification) | Sent after every rebuild with the marker and diagnostic counts and the time it took, so a client knows the graph is current |

The server speaks LSP 3.17 over stdio with full document sync, and asks a client that supports dynamic registration to watch `**/glossmark.yaml`, `**/.gloss.yaml`, `**/*.gloss.yaml`, `**/ontology/**` and `.glossmark/ledger.jsonl`; the VS Code extension configures the same watchers statically. The workspace root is the nearest `glossmark.yaml` at or above the editor's workspace folder; without one the server answers nothing and logs why.

Two things are deliberately not in this slice: the gate class of unsaved changes in the status bar (it needs the base commit's graph, which `gloss diff` builds in a second or two; too slow for every keystroke) and CodeLens above declarations. Both are listed under "Remaining" in ADR 0051.

## VS Code

[`editors/vscode`](../editors/vscode/README.md) is the extension: it starts `gloss lsp` from `PATH` (or `glossmark.path`), registers the language features for every file type, shows the status bar item, registers the `glossmark.openLocation` command the *Go to ontology entry* action uses, and adds **GlossMark: Show impact**, which runs `gloss impact` for the current file (or the working tree against `glossmark.impactBase`) into the GlossMark output channel.

```sh
cd editors/vscode
npm ci && npm run compile       # what CI would run; node is not set up in the Go workflow, so run it locally
npx vsce package                # glossmark-<version>.vsix
code --install-extension glossmark-*.vsix
```

VS Code does not pop completions inside comments unless `"editor.quickSuggestions": { "comments": "on" }` is set; `Ctrl+Space` after `@gloss ` works regardless. The extension is not published to a marketplace yet.

## Other editors

Any LSP client that can start a command on stdio works; register `gloss lsp` for the file types you mark (or for all of them, as the VS Code extension does). The *Go to ontology entry* code action asks the client to run `glossmark.openLocation [uri, line]`; a client that does not register that command still has go-to-definition for the same jump. The *Accept proposal* action is `workspace/executeCommand` `gloss.accept [path, line]`, which the server executes.

Neovim (0.10+), as an example:

```lua
vim.api.nvim_create_autocmd("FileType", {
  callback = function(args)
    if vim.fs.root(args.buf, { "glossmark.yaml" }) then
      vim.lsp.start({ name = "gloss", cmd = { "gloss", "lsp" }, root_dir = vim.fs.root(args.buf, { "glossmark.yaml" }) })
    end
  end,
})
```

A JetBrains plugin is the natural follow-up: IntelliJ Ultimate (2023.2+) and the LSP4IJ plugin for the community editions both run a stdio language server; the server needs nothing more than it has for VS Code.
