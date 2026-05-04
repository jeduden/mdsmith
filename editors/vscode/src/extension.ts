import * as vscode from "vscode";
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
  TransportKind
} from "vscode-languageclient/node";

let client: LanguageClient | undefined;

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  const cfg = vscode.workspace.getConfiguration("mdsmith");
  const binary = cfg.get<string>("path", "mdsmith");

  const serverOptions: ServerOptions = {
    run: { command: binary, args: ["lsp"], transport: TransportKind.stdio },
    debug: { command: binary, args: ["lsp"], transport: TransportKind.stdio }
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [
      { scheme: "file", language: "markdown" }
    ],
    synchronize: {
      fileEvents: vscode.workspace.createFileSystemWatcher("**/.mdsmith.yml")
    },
    outputChannelName: "mdsmith"
  };

  client = new LanguageClient("mdsmith", "mdsmith", serverOptions, clientOptions);

  try {
    await client.start();
  } catch (err) {
    const message =
      `Failed to start mdsmith Language Server: ${err}. ` +
      `Set the binary path with the "mdsmith.path" setting or download mdsmith.`;
    const choice = await vscode.window.showErrorMessage(
      message,
      "Download mdsmith",
      "Open Settings"
    );
    if (choice === "Download mdsmith") {
      await vscode.env.openExternal(
        vscode.Uri.parse("https://github.com/jeduden/mdsmith/releases")
      );
    } else if (choice === "Open Settings") {
      await vscode.commands.executeCommand("workbench.action.openSettings", "mdsmith");
    }
    return;
  }

  // Wire fix-on-save when the user opted in.
  context.subscriptions.push(
    vscode.workspace.onWillSaveTextDocument((event) => {
      if (event.document.languageId !== "markdown") return;
      const fixOnSave = vscode.workspace.getConfiguration("mdsmith").get<boolean>("fixOnSave", false);
      if (!fixOnSave) return;
      event.waitUntil(
        vscode.commands.executeCommand(
          "vscode.executeCodeActionProvider",
          event.document.uri,
          new vscode.Range(0, 0, event.document.lineCount, 0),
          "source.fixAll.mdsmith"
        ).then(async (actions) => {
          const list = (actions ?? []) as vscode.CodeAction[];
          const edits: vscode.TextEdit[] = [];
          for (const action of list) {
            if (action.kind?.value !== "source.fixAll.mdsmith") continue;
            if (!action.edit) continue;
            for (const [uri, items] of action.edit.entries()) {
              if (uri.toString() !== event.document.uri.toString()) continue;
              for (const item of items) {
                edits.push(item as vscode.TextEdit);
              }
            }
          }
          return edits;
        })
      );
    })
  );
}

export async function deactivate(): Promise<void> {
  if (client) {
    await client.stop();
    client = undefined;
  }
}
