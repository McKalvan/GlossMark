# GlossMark for VS Code

Completion, hover, diagnostics and review actions for `@gloss` markers in every file type, through the `gloss lsp` language server. The extension is a thin client: the server is the `gloss` binary you already run in CI, so what the editor says is what `gloss check` says.

## What you get

- **Diagnostics as you type.** `gloss check` runs over the workspace with your unsaved buffers laid over it (debounced); errors and warnings carry their `GM` code, a hint (the starter relation profile, GM250) shows faded.
- **Completion.** After `@gloss ` or `@gloss? `: the relations the repository's profile allows, each with its doc; after the relation: the concepts, the boundary's allowed set first (the rest are marked `HARD`), each with its kind, criticality and doc; `.` offers the default namespace's concepts in their local form; then `scope=`, `conf=`, `note=`.
- **Hover** on a concept: doc, kind, criticality, owner, where it is implemented and verified, its drift findings, the ontology entry. On a marker line: the link, its anchor and its ledger row (who wrote it, who accepted it).
- **Go to definition** on a concept opens its ontology entry.
- **Code actions**: *Accept proposal* (`@gloss?` → `@gloss` through `gloss accept`, with the ledger row; the file must be saved), *Propose new concept* (inserts a `@gloss? implements ns:NEW:Name` marker above the declaration, in the file's comment style and the boundary's default namespace), *Go to ontology entry*.
- **Status bar**: the boundary the current file sits in and the links it carries; click to open the manifest.
- **GlossMark: Show impact** (command palette): `gloss impact` for the current file, or for the working tree against `glossmark.impactBase`.

## Install

1. Install `gloss` ([instructions](https://github.com/McKalvan/GlossMark#install)) so that `gloss version` works in a terminal, or point `glossmark.path` at the binary.
2. Build the extension from this directory and install the package:

   ```sh
   cd editors/vscode
   npm ci
   npm run compile
   npx vsce package            # writes glossmark-<version>.vsix
   code --install-extension glossmark-*.vsix
   ```

   The extension is not published to a marketplace yet.
3. Open a repository with a `glossmark.yaml` (or a folder beneath one). The extension starts `gloss lsp` and the status bar shows the boundary of the file you are in.

VS Code does not pop completions inside comments by default. Press `Ctrl+Space` after `@gloss `, or turn them on:

```json
"editor.quickSuggestions": { "comments": "on" }
```

## Settings

| Setting | Default | What |
| --- | --- | --- |
| `glossmark.path` | `gloss` | The executable; a name on `PATH` or an absolute path |
| `glossmark.impactBase` | `` | Git ref for *Show impact*; empty runs it on the current file |
| `glossmark.trace.server` | `off` | Trace the protocol in the GlossMark output channel |

## Develop

```sh
npm ci && npm run watch     # compile on change
```

Press `F5` in VS Code with this folder open to run an Extension Development Host. `gloss lsp --log /tmp/gloss-lsp.log` is what the extension starts; the GlossMark output channel shows the server's stderr.
