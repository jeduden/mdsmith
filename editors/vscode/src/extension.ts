import * as vscode from "vscode";
import {
  CloseAction,
  CloseHandlerResult,
  ErrorAction,
  ErrorHandler,
  ErrorHandlerResult,
  LanguageClient,
  LanguageClientOptions,
  Message,
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
  // Register commands first so they remain available even when the
  // server fails to start (the most useful one then is "Show Output
  // Channel" so the user can read the failure reason). Restart will
  // try a fresh start.
  context.subscriptions.push(
    vscode.commands.registerCommand("mdsmith.restartServer", () => restartServer(context)),
    vscode.commands.registerCommand("mdsmith.showOutput", () => showOutput())
  );

  // Wire fix-on-save once. The handler reads the setting on every
  // save so toggling the option does not require a restart.
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
        ).then(
          // collectFixAllEdits is typed against the structural
          // `TextEditLike` so wiring.ts stays decoupled from the
          // `vscode` runtime package; cast back to `vscode.TextEdit[]`
          // here because that's what `event.waitUntil` expects from a
          // willSave handler. The runtime objects are real
          // `vscode.TextEdit` instances forwarded from
          // executeCodeActionProvider, so the cast is safe.
          (actions) =>
            collectFixAllEdits(actions, event.document.uri) as vscode.TextEdit[]
        )
      );
    })
  );

  await startServer(context);
}

// startServer creates a fresh LanguageClient and start()s it. On
// failure it surfaces a quick-fix dialog (Download / Open Settings)
// without throwing, because the commands registered in activate()
// must remain usable so the user can retry.
async function startServer(_context: vscode.ExtensionContext): Promise<void> {
  const cfg = vscode.workspace.getConfiguration("mdsmith");
  const binary = cfg.get<string>("path", "mdsmith");
  const workspaceRoot = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;

  const serverOptions: ServerOptions = buildServerOptions(
    binary,
    TransportKind.stdio,
    workspaceRoot
  );
  const clientOptions: LanguageClientOptions = buildClientOptions(
    vscode.workspace.createFileSystemWatcher("**/.mdsmith.yml")
  );
  // Replace the default ErrorHandler (DoNotRestart after 5 close
  // events in 3 minutes) with one that gives the user a clear
  // recovery path. We let the client keep restarting up to a
  // higher per-window threshold; once we hit that ceiling we
  // surface a notification with a "Restart Language Server" /
  // "Show Output" prompt instead of silently disabling the
  // extension. The mdsmith.restartServer command stays the
  // explicit manual recovery path either way.
  clientOptions.errorHandler = new MdsmithErrorHandler();

  client = new LanguageClient("mdsmith", "mdsmith", serverOptions, clientOptions);

  try {
    await client.start();
  } catch (err) {
    const choice = await vscode.window.showErrorMessage(
      startupErrorMessage(err),
      "Download mdsmith",
      "Open Settings",
      "Show Output"
    );
    if (choice === "Download mdsmith") {
      await vscode.env.openExternal(
        vscode.Uri.parse("https://github.com/jeduden/mdsmith/releases")
      );
    } else if (choice === "Open Settings") {
      await vscode.commands.executeCommand("workbench.action.openSettings", "mdsmith");
    } else if (choice === "Show Output") {
      showOutput();
    }
  }
}

// restartServer stops the running client (if any) and starts a
// fresh one. Useful when the user fixes `mdsmith.path`, rebuilds
// the binary, or otherwise wants to recover without reloading the
// VS Code window.
async function restartServer(context: vscode.ExtensionContext): Promise<void> {
  if (client) {
    try {
      await client.stop();
    } catch {
      // Ignore — a half-started client may refuse to stop, but
      // dropping the reference is enough to reclaim it.
    }
    client = undefined;
  }
  await startServer(context);
}

// showOutput reveals the "mdsmith" output channel where the
// language client logs RPC traffic and the server's stderr.
function showOutput(): void {
  // The LanguageClient registers an OutputChannel under
  // outputChannelName ("mdsmith"). Calling outputChannel.show on
  // the client's own handle is the safest way to reveal it without
  // importing internals.
  client?.outputChannel.show(true);
}

// MdsmithErrorHandler replaces vscode-languageclient's default
// ErrorHandler. The default's "5 closes in 180 seconds → stop"
// rule is hostile during local development (rebuild loops,
// editor reloads, transient ENOENT while iterating on the
// binary path) — once it trips, the only recovery is a window
// reload. This handler:
//
//  - Always returns ErrorAction.Continue on RPC errors. Errors
//    don't kill the process, so there's nothing useful to do
//    on them other than keep going.
//  - Allows up to maxRestarts close events per windowMs of
//    wallclock time before falling back to DoNotRestart, which
//    is significantly more permissive than the default.
//  - On the falling-back path, surfaces a notification with a
//    "Restart Language Server" / "Show Output" choice so the
//    user can recover with one click instead of reloading the
//    window.
class MdsmithErrorHandler implements ErrorHandler {
  private static readonly maxRestarts = 25;
  private static readonly windowMs = 3 * 60 * 1000;
  private restarts: number[] = [];

  error(_error: Error, _message: Message | undefined, _count: number | undefined): ErrorHandlerResult {
    return { action: ErrorAction.Continue };
  }

  closed(): CloseHandlerResult {
    const now = Date.now();
    this.restarts = this.restarts.filter((t) => now - t < MdsmithErrorHandler.windowMs);
    this.restarts.push(now);
    if (this.restarts.length > MdsmithErrorHandler.maxRestarts) {
      // Show the prompt asynchronously so we do not block the
      // close handler. The promise body decides whether to
      // restart based on the user's choice.
      void promptRestartAfterRepeatedFailures();
      return { action: CloseAction.DoNotRestart };
    }
    return { action: CloseAction.Restart };
  }
}

// promptRestartAfterRepeatedFailures runs after the error
// handler has decided to stop restarting. The user can pick
// one of the actionable buttons; "Restart" calls the same
// command users get from the palette so the recovery path is
// consistent.
async function promptRestartAfterRepeatedFailures(): Promise<void> {
  const choice = await vscode.window.showErrorMessage(
    "mdsmith server crashed too many times in a row. Linting is paused.",
    "Restart Language Server",
    "Show Output"
  );
  if (choice === "Restart Language Server") {
    await vscode.commands.executeCommand("mdsmith.restartServer");
  } else if (choice === "Show Output") {
    showOutput();
  }
}

export async function deactivate(): Promise<void> {
  if (client) {
    await client.stop();
    client = undefined;
  }
}
