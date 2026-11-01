<!-- @gloss documents gmi:LanguageServer scope=file -->
# ADR 0051: an editor talks to gloss over the Language Server Protocol

Status: accepted, 2026-11-01; `gloss lsp` and the VS Code extension built 2026-11-01. The JetBrains plugin, CodeLens and the gate in the status bar are not built yet.

## Context

Markers are comments typed in an editor, and the editor knew nothing about them ([#265](https://github.com/McKalvan/GlossMark/issues/265)): the concept names had to be looked up in the ontology file, a dangling reference or a target outside the boundary's allowed set showed up in CI, and accepting a proposal meant a terminal. The agent surface (`gloss context`, MCP) gives an agent all of this while it writes; the human writing beside it had less.

The repository's constraint is one static binary with no cgo and no heavy dependency, and the acceptance criterion of the issue is that the server reuses the existing packages and duplicates no logic. Every editor worth supporting speaks the Language Server Protocol.

## Decision

1. **One language server in the binary, `gloss lsp`, over stdio.** The protocol is JSON-RPC 2.0 with `Content-Length` framing; `internal/lsp` implements the framing and the eleven methods it needs by hand on the standard library, as `internal/mcp` does for MCP. The Go LSP libraries (`go.lsp.dev/protocol`, `gopls`'s internal one, `tliron/glsp`) each pull in a protocol package of several thousand generated lines plus a JSON-RPC stack, for a dozen struct types; writing the types the server uses (`protocol.go`) is smaller than vendoring any of them and keeps `go.mod` as it is. If the feature set grows past hover, completion, definition, code actions and diagnostics (semantic tokens, rename, references), that trade-off is to be revisited.

2. **The server runs the pipeline, it does not reimplement it.** A `Backend` the command supplies builds the workspace (`load` → `extract` → `rollup.Derive` → `ledger.Sync` → `check.Run` → `rollup.Compare`, the same sequence as `gloss check` and `gloss drift`) and accepts a marker through the same function as `gloss accept`. Completion reads `Vocabulary.InlineNames` under the relation profile and `rollup.AllowedFor` for the ranking; hover reads the graph, the ledger and the drift report; definition reads the concept node's file and line. The only logic in `internal/lsp` is presentation: which items, in what order, in what words.

3. **Unsaved buffers are laid over the tree.** `load.Workspace` gains an `Overlay` (repo-relative path → contents) and a `ReadFile` that serves it before the disk; the extractor and the linter read files through it. The server keeps every open document and rebuilds the whole workspace, debounced, on each change: a rebuild of this repository takes about 300 ms, of the showcase repositories less, and a whole-workspace build is what makes the allowed-set warning, the duplicate-marker warning and the dangling reference correct, since each depends on files other than the one being edited. A per-file check would be a second, lesser linter. Files the walk does not list (an unsaved new file) are not extracted until saved; ontology files and manifests are read from disk, so an edit to them takes effect on save, which the file watchers report.

4. **Accept writes the file, not the buffer.** The *Accept proposal* action is `workspace/executeCommand` `gloss.accept [path, line]`, executed by the server with the same code as the command: the `@gloss?` → `@gloss` edit on disk and the ledger row, saved once. The server refuses when the open buffer differs from the disk, instead of applying a `WorkspaceEdit` to the buffer and writing the ledger for a file that is not yet in that state; the editor reloads a clean file on its own. One path to accept means one review trail (ADR 0027).

5. **A client-side command for the one jump that is not a definition.** *Go to ontology entry* returns the command `glossmark.openLocation [uri, line]`, which the client runs; the VS Code extension registers it. A client without it has `textDocument/definition` on the same reference, so nothing is lost, and the server does not depend on a VS Code-only command such as `editor.action.revealDefinition`.

6. **The VS Code extension is a thin client**, under `editors/vscode` in TypeScript on `vscode-languageclient`: start the server from `PATH` or a setting, register the features for every file type (markers live in any file the scanner reads), a status bar item fed by a custom `glossmark/fileInfo` request, and the `GlossMark: Show impact` command that runs `gloss impact`. It is built with `npm ci && npm run compile` and packaged with `vsce package`; it is not published to a marketplace by this change, and the Go CI workflow does not run it, since Node is not set up there. The README says how.

## Consequences

- The editor shows what CI will say, before the commit, and a reviewer can accept from where the proposal is read. The server has no rule of its own to drift from the command line.
- A rebuild per edit is simple and correct, and scales with the repository: a tree an order of magnitude larger than this one would want incremental extraction (re-scan the changed file, re-derive). The `Backend` boundary is where that goes; the protocol side does not change.
- Full document sync (the client sends the whole text on each change) costs bandwidth proportional to file size per keystroke; incremental sync is a contained change to `didChange` when it matters.
- The VS Code extension's `quickSuggestions` default hides completions in comments; the README documents the setting and `Ctrl+Space`. A dedicated marker language mode would avoid that but would claim files from their own language, which is not acceptable.

## Remaining

- JetBrains plugin: IntelliJ's LSP API (Ultimate 2023.2+) or LSP4IJ for the community editions; the server needs nothing new.
- CodeLens above a marked declaration ("realizes clinic:Booking (high)", "what does this change touch?").
- The gate class of the unsaved changes in the status bar, once a base graph can be cached between keystrokes.
- Publishing the extension to the marketplace and Open VSX, and a release step that builds the `.vsix` with the binary.
