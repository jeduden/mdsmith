import * as vscode from "vscode";
import {
  CloseAction,
  CloseHandlerResult,
  DidChangeConfigurationNotification,
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
  forwardMdsmithConfigChange,
  notifyConfigChangeToClient,
  shouldFixOnSave,
  startupErrorMessage,
  RUN_ON_TYPE
} from "./wiring";
import { findBinaryCandidates, resolveBinary } from "./binary";
import { runFixWorkspace } from "./commands/fix-workspace";
import { runInit } from "./commands/init";
import { runMergeDriverInstall } from "./commands/merge-driver";
import { runKindsResolve, runKindsWhy, makeKindsContentProvider } from "./commands/kinds";
import { KINDS_SCHEME, parseKindsUri } from "./commands/virtual-doc";
import {
  RULE_SCHEME,
  OPEN_RULE_DOC_COMMAND,
  buildRuleDocUri,
  fetchRuleDocContent,
  isRuleId,
  rewriteHoverMarkdown,
} from "./commands/rule-doc";

let client: LanguageClient | undefined;
// Track the .mdsmith.yml file watcher across the activate /
// startServer / restartServer / deactivate lifecycle. A new
// watcher is created on every server start; the old one must be
// disposed first or VS Code accumulates watchers and emits
// duplicate change events for every restart.
let configWatcher: vscode.FileSystemWatcher | undefined;

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  // Register commands first so they remain available even when the
  // server fails to start (the most useful one then is "Show Output
  // Channel" so the user can read the failure reason). Restart will
  // try a fresh start.
  context.subscriptions.push(
    vscode.commands.registerCommand("mdsmith.restartServer", () => restartServer(context)),
    vscode.commands.registerCommand("mdsmith.showOutput", () => showOutput())
  );

  registerPaletteCommands(context);

  // Wire fix-on-save once. The handler reads both settings on every
  // save so toggling either option does not require a restart.
  // fixOnSave is gated by mdsmith.run: when run is "off" mdsmith is
  // inert, so a save must not rewrite the buffer even if fixOnSave was
  // left on (shouldFixOnSave centralizes that rule).
  context.subscriptions.push(
    vscode.workspace.onWillSaveTextDocument((event) => {
      if (event.document.languageId !== "markdown") return;
      const cfg = vscode.workspace.getConfiguration("mdsmith");
      const run = cfg.get<string>("run", RUN_ON_TYPE);
      const fixOnSave = cfg.get<boolean>("fixOnSave", false);
      if (!shouldFixOnSave(run, fixOnSave)) return;
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

  // Forward mdsmith.* settings changes to the running server so it
  // re-pulls config (mdsmith.run, mdsmith.config, mdsmith.previewFix).
  // The server only reads these on initialize and on
  // workspace/didChangeConfiguration; vscode-languageclient does not
  // emit that notification by itself in the pull model, so without this
  // toggling e.g. mdsmith.previewFix would have no effect until a server
  // restart. See forwardMdsmithConfigChange for the full rationale.
  context.subscriptions.push(
    vscode.workspace.onDidChangeConfiguration((event) =>
      forwardMdsmithConfigChange(event, () => {
        notifyConfigChangeToClient(client, (c) =>
          c.sendNotification(DidChangeConfigurationNotification.type, {
            settings: null
          })
        );
      })
    )
  );

  await startServer(context);
}

// startServer creates a fresh LanguageClient and start()s it. On
// failure it surfaces a quick-fix dialog (Download / Open Settings)
// without throwing, because the commands registered in activate()
// must remain usable so the user can retry.
async function startServer(context: vscode.ExtensionContext): Promise<void> {
  const cfg = vscode.workspace.getConfiguration("mdsmith");
  const configuredPath = cfg.get<string>("path", "mdsmith");
  const binary = resolveBinary(configuredPath, context.extensionPath);
  const workspaceRoot = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;

  const serverOptions: ServerOptions = buildServerOptions(
    binary,
    TransportKind.stdio,
    workspaceRoot
  );
  // Replace any previous watcher before creating a new one.
  // restartServer disposes via disposeConfigWatcher, but
  // defensively dispose here too so a future caller of
  // startServer (other than restartServer) cannot accidentally
  // leak. context.subscriptions covers deactivate-time cleanup.
  disposeConfigWatcher();
  configWatcher = vscode.workspace.createFileSystemWatcher("**/.mdsmith.yml");
  context.subscriptions.push(configWatcher);
  const clientOptions: LanguageClientOptions = buildClientOptions(
    configWatcher,
    getOutputChannel()
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

  // Rewrite the server's public-website rule-docs links (in hovers) so
  // they open the README offline from the bundled binary instead of a
  // browser. The server keeps emitting https links so editors without
  // this extension still get a working link; VS Code intercepts them
  // here and points them at the mdsmith-rule: virtual document. See
  // commands/rule-doc.ts.
  clientOptions.middleware = {
    provideHover: async (document, position, token, next) => {
      const hover = await next(document, position, token);
      if (hover) {
        for (const block of hover.contents) {
          if (block instanceof vscode.MarkdownString) {
            rewriteHoverMarkdown(block);
          }
        }
      }
      return hover;
    },
  };

  client = new LanguageClient("mdsmith", "mdsmith", serverOptions, clientOptions);

  try {
    await client.start();
  } catch (err) {
    // start() rejected — leave the LanguageClient referenceable
    // briefly so the user can hit "Show Output" to read the
    // failure log, then drop the reference. Without this clear,
    // a partially-started client lingers and a subsequent
    // deactivate() / restart would call stop() on something that
    // never reached the running state, throwing inside vscode-
    // languageclient. Also tear down the watcher; startServer
    // will install a fresh one on next attempt.
    const candidates = findBinaryCandidates(context.extensionPath);
    const detail = startupErrorMessage(err, {
      configuredPath,
      resolvedCommand: binary,
      candidates,
    });
    // Mirror the full diagnostic to the Output channel so the user
    // can scroll, copy, and inspect every candidate path — VS Code
    // truncates long error notifications, but the channel does not.
    getOutputChannel().appendLine(detail);
    const choice = await vscode.window.showErrorMessage(
      detail,
      "Download mdsmith",
      "Open Settings",
      "Show Output"
    );
    if (choice === "Show Output") {
      showOutput();
    }
    client = undefined;
    disposeConfigWatcher();
    if (choice === "Download mdsmith") {
      await vscode.env.openExternal(
        vscode.Uri.parse("https://github.com/jeduden/mdsmith/releases")
      );
    } else if (choice === "Open Settings") {
      await vscode.commands.executeCommand("workbench.action.openSettings", "mdsmith");
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
  // Clean up the previous file watcher; startServer will install a
  // fresh one.
  disposeConfigWatcher();
  await startServer(context);
}

// disposeConfigWatcher releases the active .mdsmith.yml watcher
// so a new one can take over. Idempotent — calling it without an
// active watcher is a no-op.
function disposeConfigWatcher(): void {
  if (configWatcher) {
    configWatcher.dispose();
    configWatcher = undefined;
  }
}

// showOutput reveals the "mdsmith" output channel. Uses
// getOutputChannel() so the standalone channel created by palette
// commands is also reachable when the LSP client is not running.
function showOutput(): void {
  getOutputChannel().show(true);
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

// getOutputChannel returns the single "mdsmith" OutputChannel shared
// between palette commands and the LanguageClient. Created lazily on
// first use so we don't reserve a channel before anyone needs it; the
// same instance is passed into LanguageClientOptions.outputChannel so
// the LSP client doesn't register a second channel with the same name.
let outputChannel: vscode.OutputChannel | undefined;

function getOutputChannel(): vscode.OutputChannel {
  if (!outputChannel) {
    outputChannel = vscode.window.createOutputChannel("mdsmith");
  }
  return outputChannel;
}

// resolveActiveBinary reads `mdsmith.path` at call time so the palette
// commands pick up config edits without a window reload.
function resolveActiveBinary(extensionPath: string, scope?: vscode.Uri): string {
  const cfg = vscode.workspace.getConfiguration("mdsmith", scope);
  return resolveBinary(cfg.get<string>("path", "mdsmith"), extensionPath);
}

// registerPaletteCommands wires the five mdsmith.* palette commands and
// registers the mdsmith-kinds: virtual-document scheme. Called once from
// activate(). Trust-gated commands use the built-in isWorkspaceTrusted
// when condition, which VS Code re-evaluates automatically when trust
// is granted — no onDidGrantWorkspaceTrust subscription is required.
function registerPaletteCommands(context: vscode.ExtensionContext): void {
  const getActiveFileUri = (): vscode.Uri | undefined => {
    const uri = vscode.window.activeTextEditor?.document.uri;
    return uri?.scheme === "file" ? uri : undefined;
  };
  // In multi-root workspaces, prefer the folder containing the active
  // editor so file-modifying commands operate in the folder the user is
  // working in. Falls back to the first folder when there is no active
  // editor or it lives outside any workspace folder.
  const getWorkspaceRoot = (): string | undefined => {
    const folders = vscode.workspace.workspaceFolders;
    if (!folders || folders.length === 0) return undefined;
    if (folders.length > 1) {
      const activeUri = vscode.window.activeTextEditor?.document.uri;
      if (activeUri) {
        const folder = vscode.workspace.getWorkspaceFolder(activeUri);
        if (folder) return folder.uri.fsPath;
      }
    }
    return folders[0].uri.fsPath;
  };
  // Scope configuration lookups to the workspace folder being operated on
  // so per-folder mdsmith.path / mdsmith.config values are respected in
  // multi-root workspaces. Falls back to the active file URI when no
  // workspace folder is available (e.g. for the kinds virtual-doc commands).
  const getConfigScope = (): vscode.Uri | undefined => {
    const root = getWorkspaceRoot();
    return root ? vscode.Uri.file(root) : getActiveFileUri();
  };
  const getBinary = () => resolveActiveBinary(context.extensionPath, getConfigScope());
  const getConfigPath = (): string | undefined => {
    const v = vscode.workspace.getConfiguration("mdsmith", getConfigScope()).get<string>("config", "");
    return v || undefined;
  };
  const isTrusted = () => vscode.workspace.isTrusted;

  const outputDeps = () => ({
    appendOutput: (text: string) => {
      const ch = getOutputChannel();
      ch.append(text);
    },
    showOutput: () => getOutputChannel().show(true),
  });

  // showNotification routes failures (messages containing "failed" or
  // "could not start") to showWarningMessage so they surface as warnings
  // rather than appearing informational to the user.
  const showNotification = (msg: string, ...buttons: string[]): Promise<string | undefined> => {
    const isFailure = msg.includes("failed") || msg.includes("could not start");
    return Promise.resolve(
      isFailure
        ? vscode.window.showWarningMessage(msg, ...buttons)
        : vscode.window.showInformationMessage(msg, ...buttons)
    );
  };

  const confirmDestructive = (label: string) => async () => {
    const answer = await vscode.window.showWarningMessage(
      `Run \`${label}\` in the workspace? This will modify files.`,
      { modal: true },
      "Proceed"
    );
    return answer === "Proceed";
  };

  context.subscriptions.push(
    vscode.commands.registerCommand("mdsmith.init", async () => {
      const od = outputDeps();
      await runInit({
        binary: getBinary(),
        workspaceRoot: getWorkspaceRoot(),
        isTrusted,
        showInfo: (msg, ...buttons) =>
          showNotification(msg, ...buttons),
        showError: (msg) =>
          Promise.resolve(vscode.window.showErrorMessage(msg)).then(() => {}),
        ...od,
      });
    }),

    vscode.commands.registerCommand("mdsmith.mergeDriver.install", async () => {
      const od = outputDeps();
      await runMergeDriverInstall({
        binary: getBinary(),
        workspaceRoot: getWorkspaceRoot(),
        isTrusted,
        confirm: confirmDestructive("mdsmith merge-driver install"),
        showInfo: (msg, ...buttons) =>
          showNotification(msg, ...buttons),
        showError: (msg) =>
          Promise.resolve(vscode.window.showErrorMessage(msg)).then(() => {}),
        ...od,
      });
    }),

    vscode.commands.registerCommand("mdsmith.fixWorkspace", async () => {
      const od = outputDeps();
      await runFixWorkspace({
        binary: getBinary(),
        workspaceRoot: getWorkspaceRoot(),
        configPath: getConfigPath(),
        isTrusted,
        confirm: confirmDestructive("mdsmith fix ."),
        showInfo: (msg, ...buttons) =>
          showNotification(msg, ...buttons),
        showError: (msg) =>
          Promise.resolve(vscode.window.showErrorMessage(msg)).then(() => {}),
        ...od,
      });
    }),

    vscode.commands.registerCommand("mdsmith.kinds.resolve", async () => {
      await runKindsResolve({
        getActiveFilePath: () => {
          const editor = vscode.window.activeTextEditor;
          if (!editor || editor.document.uri.scheme !== "file") return undefined;
          if (editor.document.languageId !== "markdown") return undefined;
          return editor.document.uri.fsPath;
        },
        getDiagnostics: (filePath) =>
          vscode.languages.getDiagnostics(vscode.Uri.file(filePath)),
        openVirtualDoc: async (uri) => {
          const doc = await vscode.workspace.openTextDocument(
            vscode.Uri.parse(uri)
          );
          const mdDoc = await vscode.languages.setTextDocumentLanguage(doc, "markdown");
          await vscode.window.showTextDocument(mdDoc, {
            preview: true,
            viewColumn: vscode.ViewColumn.Beside,
          });
        },
        showError: (msg) =>
          Promise.resolve(vscode.window.showErrorMessage(msg)).then(() => {}),
      });
    }),

    vscode.commands.registerCommand("mdsmith.kinds.why", async () => {
      await runKindsWhy({
        getActiveFilePath: () => {
          const editor = vscode.window.activeTextEditor;
          if (!editor || editor.document.uri.scheme !== "file") return undefined;
          if (editor.document.languageId !== "markdown") return undefined;
          return editor.document.uri.fsPath;
        },
        getDiagnostics: (filePath) =>
          vscode.languages.getDiagnostics(vscode.Uri.file(filePath)),
        pickRule: async (rules) => {
          const items =
            rules.length > 0
              ? rules
              : await vscode.window.showInputBox({
                  prompt: "No active diagnostics. Enter a rule ID (e.g. MDS001)",
                  placeHolder: "MDS001",
                }).then((v) => (v ? [v] : []));
          if (!items || items.length === 0) return undefined;
          if (items.length === 1) return items[0];
          return vscode.window.showQuickPick(items, {
            placeHolder: "Pick a rule to explain",
          });
        },
        openVirtualDoc: async (uri) => {
          const doc = await vscode.workspace.openTextDocument(
            vscode.Uri.parse(uri)
          );
          const mdDoc = await vscode.languages.setTextDocumentLanguage(doc, "markdown");
          await vscode.window.showTextDocument(mdDoc, {
            preview: true,
            viewColumn: vscode.ViewColumn.Beside,
          });
        },
        showError: (msg) =>
          Promise.resolve(vscode.window.showErrorMessage(msg)).then(() => {}),
      });
    }),

    // Open a rule's embedded README in a read-only virtual document.
    // Invoked from hover links the middleware rewrote (and trusted via
    // isTrusted.enabledCommands), so id is normally a rule ID. But
    // registerCommand is globally callable, so enforce the same
    // MDS<digits> shape here and no-op on anything else, rather than
    // opening a tab that just reports a malformed URI.
    vscode.commands.registerCommand(OPEN_RULE_DOC_COMMAND, async (id?: string) => {
      if (!id || !isRuleId(id)) return;
      const doc = await vscode.workspace.openTextDocument(
        vscode.Uri.parse(buildRuleDocUri(id))
      );
      const mdDoc = await vscode.languages.setTextDocumentLanguage(doc, "markdown");
      await vscode.window.showTextDocument(mdDoc, {
        preview: true,
        viewColumn: vscode.ViewColumn.Beside,
      });
    }),

    // Register the virtual document provider for the mdsmith-kinds: scheme.
    vscode.workspace.registerTextDocumentContentProvider(
      KINDS_SCHEME,
      {
        provideTextDocumentContent: (uri: vscode.Uri) => {
          const uriStr = uri.toString();
          const parsed = parseKindsUri(uriStr);
          // Derive the workspace folder from the file encoded in the URI so
          // binary/config lookups use the correct per-folder settings even
          // when the active editor is the virtual doc itself.
          const fileFolder = parsed
            ? vscode.workspace.getWorkspaceFolder(vscode.Uri.file(parsed.file))
            : undefined;
          // When workspace folders are open, reject files outside the workspace
          // to prevent crafted URIs from analyzing arbitrary paths on disk.
          const folders = vscode.workspace.workspaceFolders;
          if (parsed && folders && folders.length > 0 && !fileFolder) {
            return Promise.resolve(
              `**mdsmith: file is outside the workspace**\n\n\`\`\`\n${parsed.file}\n\`\`\``
            );
          }
          const binaryScope = fileFolder?.uri ?? getActiveFileUri();
          const binary = resolveActiveBinary(context.extensionPath, binaryScope);
          const provider = makeKindsContentProvider(binary, fileFolder?.uri.fsPath);
          return provider.provideTextDocumentContent(uriStr);
        },
      }
    ),

    // Register the virtual document provider for the mdsmith-rule: scheme,
    // which renders `mdsmith help rule <id>` (the embedded README) offline
    // so the rewritten hover doc link opens without a browser or network.
    vscode.workspace.registerTextDocumentContentProvider(
      RULE_SCHEME,
      {
        provideTextDocumentContent: (uri: vscode.Uri) =>
          fetchRuleDocContent(uri.toString(), getBinary(), getWorkspaceRoot()),
      }
    ),

    // VS Code automatically re-evaluates the built-in `isWorkspaceTrusted`
    // context when trust is granted, so menu entries gated with
    // `when: isWorkspaceTrusted` appear without a reload — no explicit
    // handler needed here.
  );
}

export async function deactivate(): Promise<void> {
  if (client) {
    try {
      await client.stop();
    } catch {
      // A client whose start() failed (or that is still in the
      // "starting" state when the host shuts the extension down)
      // can throw from stop(); swallow so deactivate always
      // completes cleanly. Dropping the reference below is
      // enough to release the client object regardless.
    }
    client = undefined;
  }
  // The watcher is also pushed onto context.subscriptions in
  // startServer, but VS Code disposes those AFTER deactivate
  // returns; clear it explicitly so the dispose ordering is
  // tight and configWatcher does not survive into a subsequent
  // activation.
  disposeConfigWatcher();
  // Dispose the shared output channel. context.subscriptions is
  // flushed after deactivate returns, so dispose explicitly here
  // for tight ordering.
  if (outputChannel) {
    outputChannel.dispose();
    outputChannel = undefined;
  }
}
