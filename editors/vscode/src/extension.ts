// @gloss implements gmi:EditorExtension scope=file

// The VS Code side of GlossMark: start `gloss lsp` for the workspace, let
// the language server do the work (diagnostics, completion, hover, code
// actions on every file type), show the current file's boundary in the
// status bar and run `gloss impact` on request. docs/IDE.md documents it.
import { spawn } from "child_process";
import * as path from "path";
import * as vscode from "vscode";
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
} from "vscode-languageclient/node";

let client: LanguageClient | undefined;
let status: vscode.StatusBarItem;
let output: vscode.OutputChannel;

interface FileInfo {
  path: string;
  boundary?: string;
  level?: string;
  manifest?: string;
  defaultNamespace?: string;
  criticality?: string;
  links: string[];
  proposed: number;
}

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  output = vscode.window.createOutputChannel("GlossMark");
  status = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 50);
  status.command = "glossmark.openManifest";
  context.subscriptions.push(output, status);

  context.subscriptions.push(
    vscode.commands.registerCommand("glossmark.showImpact", showImpact),
    vscode.commands.registerCommand("glossmark.restartServer", async () => {
      await stopClient();
      await startClient(context);
    }),
    // The server's "Go to ontology entry" code action runs this on the client.
    vscode.commands.registerCommand("glossmark.openLocation", async (uri: string, line: number) => {
      const doc = await vscode.workspace.openTextDocument(vscode.Uri.parse(uri));
      const pos = new vscode.Position(line ?? 0, 0);
      await vscode.window.showTextDocument(doc, { selection: new vscode.Range(pos, pos) });
    }),
    vscode.commands.registerCommand("glossmark.openManifest", async () => {
      const info = await fileInfo(vscode.window.activeTextEditor);
      const root = workspaceRoot();
      if (info?.manifest && root) {
        const doc = await vscode.workspace.openTextDocument(path.join(root, info.manifest));
        await vscode.window.showTextDocument(doc);
      }
    }),
    vscode.window.onDidChangeActiveTextEditor(() => void refreshStatus()),
    vscode.workspace.onDidChangeConfiguration(async (e) => {
      if (e.affectsConfiguration("glossmark.path")) {
        await stopClient();
        await startClient(context);
      }
    }),
  );
  await startClient(context);
}

export async function deactivate(): Promise<void> {
  await stopClient();
}

function glossPath(): string {
  return vscode.workspace.getConfiguration("glossmark").get<string>("path") || "gloss";
}

function workspaceRoot(): string | undefined {
  return vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
}

async function startClient(context: vscode.ExtensionContext): Promise<void> {
  const root = workspaceRoot();
  if (!root) {
    return;
  }
  const command = glossPath();
  if (!(await available(command))) {
    const pick = await vscode.window.showErrorMessage(
      `GlossMark: ${command} is not on PATH. Install gloss (https://github.com/McKalvan/GlossMark#install) or set glossmark.path.`,
      "Open settings",
    );
    if (pick) {
      await vscode.commands.executeCommand("workbench.action.openSettings", "glossmark.path");
    }
    return;
  }
  const serverOptions: ServerOptions = {
    command,
    args: ["lsp"],
    options: { cwd: root },
  };
  const clientOptions: LanguageClientOptions = {
    // Markers are comments and live in every file type the scanner reads;
    // the server answers only where a marker can be.
    documentSelector: [{ scheme: "file" }],
    synchronize: {
      // The graph is built from these too: a change to a manifest or an
      // ontology file changes what every open file may link to.
      fileEvents: [
        vscode.workspace.createFileSystemWatcher("**/glossmark.yaml"),
        vscode.workspace.createFileSystemWatcher("**/.gloss.yaml"),
        vscode.workspace.createFileSystemWatcher("**/*.gloss.yaml"),
        vscode.workspace.createFileSystemWatcher("**/ontology/**"),
        vscode.workspace.createFileSystemWatcher("**/.glossmark/ledger.jsonl"),
      ],
    },
    outputChannel: output,
    traceOutputChannel: output,
  };
  client = new LanguageClient("glossmark", "GlossMark", serverOptions, clientOptions);
  client.onNotification("glossmark/built", () => void refreshStatus());
  context.subscriptions.push(client);
  await client.start();
  await refreshStatus();
}

async function stopClient(): Promise<void> {
  if (client) {
    const c = client;
    client = undefined;
    await c.stop();
  }
  status.hide();
}

// available runs `gloss version` to tell a missing binary from a server
// that fails to start.
function available(command: string): Promise<boolean> {
  return new Promise((resolve) => {
    const p = spawn(command, ["version"]);
    p.on("error", () => resolve(false));
    p.on("exit", (code) => resolve(code === 0));
  });
}

async function fileInfo(editor: vscode.TextEditor | undefined): Promise<FileInfo | undefined> {
  if (!client || !editor || editor.document.uri.scheme !== "file") {
    return undefined;
  }
  try {
    const info = await client.sendRequest<FileInfo | null>("glossmark/fileInfo", {
      uri: editor.document.uri.toString(),
    });
    return info ?? undefined;
  } catch {
    return undefined;
  }
}

// refreshStatus shows the boundary the current file sits in and the links
// it carries: "clinicb:booking-api · 2 links, 1 proposed".
async function refreshStatus(): Promise<void> {
  const info = await fileInfo(vscode.window.activeTextEditor);
  if (!info || !info.boundary) {
    status.hide();
    return;
  }
  const parts: string[] = [];
  if (info.links.length > 0) {
    parts.push(`${info.links.length} link${info.links.length === 1 ? "" : "s"}`);
  }
  if (info.proposed > 0) {
    parts.push(`${info.proposed} proposed`);
  }
  status.text = `$(symbol-namespace) ${info.boundary}${parts.length ? " · " + parts.join(", ") : ""}`;
  const lines = [
    `Boundary ${info.boundary}${info.level ? ` (${info.level})` : ""}${info.criticality ? `, criticality ${info.criticality}` : ""}`,
    info.manifest ? `Manifest ${info.manifest}` : "",
    info.defaultNamespace ? `Default namespace ${info.defaultNamespace}` : "",
    ...info.links.map((l) => `  ${l}`),
    "Click to open the manifest",
  ].filter((l) => l !== "");
  status.tooltip = lines.join("\n");
  status.show();
}

// showImpact runs `gloss impact` for the current file (or, with
// glossmark.impactBase, for the working tree against that ref) and shows
// the report in the output channel.
async function showImpact(): Promise<void> {
  const root = workspaceRoot();
  if (!root) {
    return;
  }
  const args = ["impact"];
  const base = vscode.workspace.getConfiguration("glossmark").get<string>("impactBase");
  if (base) {
    args.push("--base", base);
  } else if (vscode.window.activeTextEditor?.document.uri.scheme === "file") {
    args.push(path.relative(root, vscode.window.activeTextEditor.document.uri.fsPath));
  }
  output.show(true);
  output.appendLine(`$ ${glossPath()} ${args.join(" ")}`);
  await new Promise<void>((resolve) => {
    const p = spawn(glossPath(), args, { cwd: root });
    p.stdout.on("data", (d) => output.append(d.toString()));
    p.stderr.on("data", (d) => output.append(d.toString()));
    p.on("error", (err) => {
      output.appendLine(`gloss impact failed to start: ${err.message}`);
      resolve();
    });
    p.on("exit", (code) => {
      if (code !== 0) {
        output.appendLine(`gloss impact exited with ${code}`);
      }
      resolve();
    });
  });
}
