import * as vscode from "vscode";
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
  TransportKind
} from "vscode-languageclient/node";

import {
  buildClientOptions,
  buildServerOptions,
  collectFixAllEdits,
  startupErrorMessage
} from "./wiring";

let client: LanguageClient | undefined;

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  const cfg = vscode.workspace.getConfiguration("mdsmith");
  const binary = cfg.get<string>("path", "mdsmith");

  const serverOptions: ServerOptions = buildServerOptions(binary, TransportKind.stdio);
  const clientOptions: LanguageClientOptions = buildClientOptions(
    vscode.workspace.createFileSystemWatcher("**/.mdsmith.yml")
  );

  client = new LanguageClient("mdsmith", "mdsmith", serverOptions, clientOptions);

  try {
    await client.start();
  } catch (err) {
    const choice = await vscode.window.showErrorMessage(
      startupErrorMessage(err),
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
        ).then((actions) => collectFixAllEdits(actions, event.document.uri))
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
