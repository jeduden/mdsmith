// Wiring is the VS Code extension's composition root: it owns the
// LanguageClient lifecycle, the .mdsmith.yml watcher, the shared output
// channel, and every command registration. extension.ts shrinks to
// constructing one Wiring with the live `vscode` namespace and the
// LanguageClient ctor, then handing over.
//
// Crucially, this module imports `vscode` and `vscode-languageclient`
// only in *type* position — never as runtime values — so `bun test` can
// load it and drive the orchestration with lightweight fakes (a fake
// `VscodeApi` plus a `createClient` factory) without the editor runtime,
// which require()s `vscode` at load time. The concrete `vscode` and
// `LanguageClient` cross the boundary as injected parameters; the
// boundary between the live editor API and this logic is exactly the
// Wiring constructor.

import type {
  LanguageClientOptions,
  ServerOptions,
  TransportKind
} from "vscode-languageclient/node";

import type { BinaryCandidate } from "./binary";
import { resolveBinary as resolveBinaryImpl, findBinaryCandidates as findBinaryCandidatesImpl } from "./binary";
import { MdsmithErrorHandler } from "./commands/error-handler";
import { runFixWorkspace } from "./commands/fix-workspace";
import { runInit } from "./commands/init";
import { runMergeDriverInstall } from "./commands/merge-driver";
import { runKindsResolve, runKindsWhy, makeKindsContentProvider } from "./commands/kinds";
import { KINDS_SCHEME, kindsContentUri, parseKindsUri } from "./commands/virtual-doc";
import {
  RULE_SCHEME,
  OPEN_RULE_DOC_COMMAND,
  buildRuleDocUri,
  isRuleId,
  provideRuleDocContent,
  rewriteHoverMarkdown,
} from "./commands/rule-doc";

// FileSystemWatcherLike is the structural subset of
// vscode.FileSystemWatcher that LanguageClientOptions.synchronize.fileEvents
// actually consults. Tests can pass a bare object literal.
export interface FileSystemWatcherLike {
  ignoreCreateEvents?: boolean;
  ignoreChangeEvents?: boolean;
  ignoreDeleteEvents?: boolean;
}

export function buildServerOptions(
  binary: string,
  transport: TransportKind,
  cwd?: string
): ServerOptions {
  const command = (binary ?? "").trim();
  if (!command) {
    // vscode-languageclient rejects { command: "" } with the opaque
    // "Unsupported server configuration" error. resolveBinary already
    // guarantees a non-empty command (empty / whitespace mdsmith.path
    // falls back to the bundled binary or "mdsmith" on PATH), so this
    // is unreachable in normal flow — but fail loudly and actionably
    // rather than handing the LanguageClient an empty launch.
    throw new Error(
      'mdsmith: empty binary path. Set "mdsmith.path" to the mdsmith ' +
        "binary or reinstall the extension."
    );
  }
  // Anchor the spawned server at the workspace root when one is
  // available. mdsmith's lint pipeline now passes
  // workspace-relative paths into the engine (so config globs
  // match), but a handful of rules still call os.Stat on paths
  // derived from f.Path; without a stable CWD they would resolve
  // against whatever directory VS Code's extension host happens
  // to be running from, which drifts from CLI behavior.
  const options = cwd ? { cwd } : undefined;
  return {
    run: { command, args: ["lsp"], transport, options },
    debug: { command, args: ["lsp"], transport, options }
  };
}

// OutputChannelLike captures the OutputChannel methods that
// vscode-languageclient calls when LanguageClientOptions.outputChannel
// is set. Defined structurally so wiring.ts stays decoupled from the
// `vscode` runtime package while still rejecting unrelated objects.
export interface OutputChannelLike {
  readonly name: string;
  append(value: string): void;
  appendLine(value: string): void;
  clear(): void;
  show(preserveFocus?: boolean): void;
  hide(): void;
  dispose(): void;
}

export function buildClientOptions(
  configWatcher: FileSystemWatcherLike,
  outputChannel?: OutputChannelLike
): LanguageClientOptions {
  const opts: LanguageClientOptions = {
    documentSelector: [
      { scheme: "file", language: "markdown" }
    ],
    synchronize: {
      // Cast: LanguageClientOptions wants the full vscode interface,
      // but at runtime only the shape we expose matters.
      fileEvents: configWatcher as never
    }
  };
  if (outputChannel) {
    // Sharing one OutputChannel between palette commands and the LSP
    // client avoids two channels with the same name once the client
    // starts. Cast through never because LanguageClientOptions wants
    // the real vscode.OutputChannel type which we don't import here.
    opts.outputChannel = outputChannel as never;
  } else {
    opts.outputChannelName = "mdsmith";
  }
  return opts;
}

// StartupErrorContext captures what the resolver knew at the moment
// the LanguageClient failed to spawn: the user's raw mdsmith.path,
// the command we actually tried, and every alternative binary
// findBinaryCandidates found on disk. Optional so the basic form
// (cause + settings hint) stays available when no resolver state is
// at hand.
export interface StartupErrorContext {
  configuredPath: string;
  resolvedCommand: string;
  candidates: ReadonlyArray<BinaryCandidate>;
}

export function startupErrorMessage(
  err: unknown,
  ctx?: StartupErrorContext,
): string {
  if (!ctx) {
    return (
      `Failed to start mdsmith Language Server: ${err}. ` +
      `Set the binary path with the "mdsmith.path" setting or download mdsmith.`
    );
  }
  const lines: string[] = [
    `Failed to start mdsmith Language Server: ${err}.`,
    "",
    `"mdsmith.path": ${formatConfiguredPath(ctx.configuredPath)}`,
  ];
  if (ctx.resolvedCommand !== ctx.configuredPath) {
    // Echo the resolved command whenever it differs from the raw
    // setting — that happens both when the resolver substituted the
    // bundled binary (empty / whitespace / bare "mdsmith") and when
    // it merely trimmed surrounding whitespace from a custom value.
    // Suppressing the line when they match keeps the error tight.
    lines.push(`resolved command: ${ctx.resolvedCommand}`);
  }
  lines.push("");
  if (ctx.candidates.length === 0) {
    lines.push("No other mdsmith binaries found on this system.");
    lines.push("");
    lines.push(
      `Install mdsmith and either set "mdsmith.path" to its absolute ` +
        `location or put it on $PATH, then run "mdsmith: Restart ` +
        `Language Server".`,
    );
  } else {
    lines.push("Other mdsmith binaries found on this system:");
    for (const c of ctx.candidates) {
      lines.push(`  - ${candidateLabel(c)}: ${c.path}`);
    }
    lines.push("");
    // Only offer the "clear it to use the bundled binary" shortcut
    // when the candidate list actually has a bundled entry; on a dev
    // build with no dist/cli/ or an unsupported host the shortcut
    // would send the user to a missing binary.
    const hasBundled = ctx.candidates.some((c) => c.kind === "bundled");
    const clearHint = hasBundled
      ? ` (or clear it to use the bundled binary)`
      : "";
    lines.push(
      `Set "mdsmith.path" to one of these${clearHint} and run ` +
        `"mdsmith: Restart Language Server".`,
    );
  }
  return lines.join("\n");
}

function formatConfiguredPath(p: string): string {
  // Empty / whitespace mdsmith.path is the default; calling it out
  // explicitly stops the user from chasing an empty-string setting
  // when the resolver actually fell through to the bundled binary.
  if (p.trim() === "") return "(unset, using bundled)";
  return `"${p}"`;
}

function candidateLabel(c: BinaryCandidate): string {
  switch (c.kind) {
    case "bundled":
      return "bundled with the extension";
    case "path":
      return "on $PATH";
  }
}

// ConfigChangeLike is the structural subset of
// vscode.ConfigurationChangeEvent the extension consults when deciding
// whether a settings edit is worth forwarding to the server. Defined
// here so the decision can be unit-tested without the `vscode` runtime.
export interface ConfigChangeLike {
  affectsConfiguration(section: string): boolean;
}

// forwardMdsmithConfigChange invokes `notify` exactly when a
// configuration-change event touches any `mdsmith.*` setting.
//
// The LSP server reads mdsmith.config / mdsmith.run / mdsmith.previewFix
// by pulling workspace/configuration on initialize and on every
// workspace/didChangeConfiguration notification. vscode-languageclient
// does NOT emit that notification on its own unless
// LanguageClientOptions.synchronize.configurationSection is set — and we
// deliberately leave it unset to stay on the pull model. Without an
// explicit nudge the server therefore keeps whatever settings it read at
// startup, so toggling e.g. mdsmith.previewFix has no effect until the
// server restarts. The caller wires `notify` to push the notification;
// gating on the mdsmith section keeps unrelated settings edits from
// triggering a config re-pull and a re-lint of every open buffer.
export function forwardMdsmithConfigChange(
  event: ConfigChangeLike,
  notify: () => void
): void {
  if (event.affectsConfiguration("mdsmith")) {
    notify();
  }
}

// RunningClientLike is the structural subset of LanguageClient that
// notifyConfigChangeToClient needs. Defined here so the not-running
// guard can be unit-tested without constructing a real LanguageClient.
export interface RunningClientLike {
  isRunning(): boolean;
}

// notifyConfigChangeToClient pushes the didChangeConfiguration nudge to
// the server, but only when the client is actually running.
//
// vscode-languageclient@9's LanguageClient.sendNotification returns a
// REJECTED promise (ResponseError ConnectionInactive, "Client is not
// running") whenever the client state is StartFailed, Stopping, or
// Stopped. The config listener can fire in exactly those windows — while
// the server is restarting, during deactivation, or after a spawn
// failure that has not yet nulled the reference — so an unguarded
// `void client?.sendNotification(...)` only guards `undefined` and lets
// those rejections escape as unhandledRejection. We therefore (1) skip
// the send unless isRunning(), and (2) attach a .catch() to absorb the
// residual race where the client stops between the check and the send.
//
// `send` is required and owns the actual notification payload: keeping
// it here (rather than constructing a DidChangeConfigurationNotification
// inside this helper) is what lets wiring.ts stay decoupled from the
// vscode-languageclient protocol types so this guard is unit-testable
// without the runtime. The generic preserves the caller's concrete
// client type, so `send` receives the real LanguageClient (with its
// typed sendNotification overloads) rather than the minimal
// RunningClientLike — no cast needed at the call site.
export function notifyConfigChangeToClient<T extends RunningClientLike>(
  client: T | undefined,
  send: (c: T) => Promise<void>
): void {
  if (!client || !client.isRunning()) {
    return;
  }
  // The send can still reject if the client stops in the gap between
  // isRunning() and here; swallow it so it never becomes an
  // unhandledRejection. Nothing actionable to do — the next initialize
  // (on the inevitable restart) re-pulls config anyway.
  void send(client).catch(() => {});
}

// RestartPolicyState is the mutable bookkeeping the LSP client's
// CloseHandler carries across close events: the recent restart
// timestamps (for rate-limiting respawns) and whether the server told
// us it was intentionally superseded by a newer instance for the same
// workspace (mdsmith/superseded). Defined here so the decision is
// unit-testable without the `vscode` runtime.
export interface RestartPolicyState {
  restarts: number[];
  superseded: boolean;
}

// CloseDecision is what decideClose returns: whether to restart the
// server, and whether the restart-rate cap was just exceeded (the
// caller surfaces the recovery prompt in that case).
export interface CloseDecision {
  restart: boolean;
  capExceeded: boolean;
}

// decideClose is the pure restart policy for a closed LSP connection.
//
//   - If the server announced it was superseded, the close is expected:
//     a newer mdsmith for this workspace has taken over. Restarting
//     would respawn a server the newer one immediately supersedes again
//     — the orphaned-host restart loop a leaked extension host caused.
//     So: never restart, and do not even count it against the cap.
//   - Otherwise count the close within a sliding window and restart
//     until more than `maxRestarts` happen inside `windowMs`, at which
//     point we stop and report capExceeded so the caller can prompt.
//
// It mutates `state.restarts` in place (the window prune + the new
// entry) so successive calls share the rolling history.
export function decideClose(
  state: RestartPolicyState,
  now: number,
  maxRestarts: number,
  windowMs: number
): CloseDecision {
  if (state.superseded) {
    return { restart: false, capExceeded: false };
  }
  state.restarts = state.restarts.filter((t) => now - t < windowMs);
  state.restarts.push(now);
  if (state.restarts.length > maxRestarts) {
    return { restart: false, capExceeded: true };
  }
  return { restart: true, capExceeded: false };
}

// Run modes for `mdsmith.run`. Mirror the enum declared in
// package.json and the runMode constants in the Go server
// (internal/lsp/server.go): onType lints live as you type, onSave
// lints only on save, and off stops automatic linting (no
// diagnostics; explicit code actions still work when invoked).
export const RUN_ON_TYPE = "onType";
export const RUN_ON_SAVE = "onSave";
export const RUN_OFF = "off";

// RESTART_SERVER_COMMAND is registered in activate() and re-invoked by
// the crash-recovery prompt; one constant keeps the two from drifting.
const RESTART_SERVER_COMMAND = "mdsmith.restartServer";

// TRANSPORT_STDIO_DEFAULT is the wire value of TransportKind.stdio. The
// Wiring uses it only as the default when extension.ts does not inject
// the real enum value; referencing the enum directly here would pull the
// vscode-languageclient runtime into bun test. The DEFAULT_DEPS that
// extension.ts supplies pass the genuine TransportKind.stdio.
const TRANSPORT_STDIO_DEFAULT = 0 as TransportKind;

// VscodeApi is the live `vscode` namespace, injected into Wiring rather
// than imported. The `import("vscode")` here is a *type* query — erased
// at compile time — so this module never triggers a runtime
// `require("vscode")` and stays loadable under `bun test`.
export type VscodeApi = typeof import("vscode");

// FileSystemWatcher / OutputChannel / Uri are referenced only in type
// position (again, erased) so Wiring's fields and helpers can name the
// concrete vscode shapes without importing the runtime.
type FileSystemWatcher = import("vscode").FileSystemWatcher;
type OutputChannel = import("vscode").OutputChannel;
type Uri = import("vscode").Uri;

// ClientLike is the structural subset of vscode-languageclient's
// LanguageClient that Wiring drives. extension.ts injects a factory that
// returns the real LanguageClient; tests inject a fake. Keeping it
// structural (rather than importing the class) is what lets this module
// stay off the languageclient runtime.
export interface ClientLike {
  onNotification(method: string, handler: () => void): { dispose(): void };
  start(): Promise<void>;
  stop(): Promise<void>;
  isRunning(): boolean;
  sendNotification(type: unknown, params?: unknown): Promise<void>;
}

// CreateClientFn constructs a language client from the same arguments
// the real `new LanguageClient(id, name, serverOptions, clientOptions)`
// takes. Injected so the construction — the one spot that needs the
// languageclient runtime — lives in extension.ts.
export type CreateClientFn = (
  id: string,
  name: string,
  serverOptions: ServerOptions,
  clientOptions: LanguageClientOptions
) => ClientLike;

// WiringDeps are the boundary objects extension.ts injects. Everything
// that needs the live editor or the languageclient runtime arrives here;
// the rest of Wiring is plain logic.
export interface WiringDeps {
  api: VscodeApi;
  createClient: CreateClientFn;
  // Optional seams; production uses the binary.ts implementations and
  // TransportKind.stdio. Tests stub them to avoid real binary lookups
  // and the languageclient enum import.
  resolveBinary?: (configuredPath: string, extensionPath: string) => string;
  findBinaryCandidates?: (extensionPath: string) => ReadonlyArray<BinaryCandidate>;
  stdioTransport?: TransportKind;
}

// ExtensionContextLike is the slice of vscode.ExtensionContext that
// Wiring consumes: the disposables array and the extension install path.
export interface ExtensionContextLike {
  subscriptions: Array<{ dispose(): void }>;
  extensionPath: string;
}

// DidChangeConfigurationNotificationType is the notification id Wiring
// pushes to nudge the server to re-pull configuration. The server keys
// off the method name "workspace/didChangeConfiguration"; passing the
// id as a string (rather than importing the typed
// DidChangeConfigurationNotification.type) keeps Wiring off the
// languageclient runtime. extension.ts no longer constructs the typed
// notification — the method string is the contract the server matches.
const DID_CHANGE_CONFIGURATION = "workspace/didChangeConfiguration";

export class Wiring {
  private readonly api: VscodeApi;
  private readonly createClient: CreateClientFn;
  private readonly resolveBinary: (configuredPath: string, extensionPath: string) => string;
  private readonly findBinaryCandidates: (extensionPath: string) => ReadonlyArray<BinaryCandidate>;
  private readonly stdioTransport: TransportKind;

  private client: ClientLike | undefined;
  // Track the .mdsmith.yml watcher across the activate / startServer /
  // restartServer / deactivate lifecycle. A new watcher is created on
  // every server start; the old one must be disposed first or VS Code
  // accumulates watchers and emits duplicate change events per restart.
  private configWatcher: FileSystemWatcher | undefined;
  private outputChannel: OutputChannel | undefined;

  constructor(deps: WiringDeps) {
    this.api = deps.api;
    this.createClient = deps.createClient;
    this.resolveBinary = deps.resolveBinary ?? resolveBinaryImpl;
    this.findBinaryCandidates = deps.findBinaryCandidates ?? findBinaryCandidatesImpl;
    this.stdioTransport = deps.stdioTransport ?? TRANSPORT_STDIO_DEFAULT;
  }

  // activate registers commands (first, so they stay usable even when
  // the server fails to start), wires the config-change forwarder, and
  // starts the language server.
  async activate(context: ExtensionContextLike): Promise<void> {
    // Register commands first so they remain available even when the
    // server fails to start (the most useful one then is "Show Output
    // Channel" so the user can read the failure reason). Restart will
    // try a fresh start.
    context.subscriptions.push(
      this.api.commands.registerCommand(RESTART_SERVER_COMMAND, () => this.restartServer(context)),
      this.api.commands.registerCommand("mdsmith.showOutput", () => this.showOutput())
    );

    this.registerPaletteCommands(context);

    // Fix-on-save uses VS Code's native editor.codeActionsOnSave with
    // source.fixAll.mdsmith — the same model ESLint uses with
    // source.fixAll.eslint (the deprecated mdsmith.fixOnSave setting
    // points users there). The extension contributes nothing beyond the
    // LSP code action: VS Code runs the action on save through the
    // bulk-edit service, which honours mdsmith.previewFix's
    // ChangeAnnotation and opens the Refactor Preview before writing. A
    // custom onWillSaveTextDocument handler cannot — its waitUntil drops
    // the annotation and times out rather than wait for a confirmation
    // UI. See docs/guides/editors/vscode.md.

    // Forward mdsmith.* settings changes to the running server so it
    // re-pulls config (mdsmith.run, mdsmith.config, mdsmith.previewFix).
    // The server only reads these on initialize and on
    // workspace/didChangeConfiguration; vscode-languageclient does not
    // emit that notification by itself in the pull model, so without this
    // toggling e.g. mdsmith.previewFix would have no effect until a
    // server restart. See forwardMdsmithConfigChange for the rationale.
    context.subscriptions.push(
      this.api.workspace.onDidChangeConfiguration((event) =>
        forwardMdsmithConfigChange(event, () => {
          notifyConfigChangeToClient(this.client, (c) =>
            c.sendNotification(DID_CHANGE_CONFIGURATION, { settings: null })
          );
        })
      )
    );

    await this.startServer(context);
  }

  // startServer creates a fresh language client and start()s it. On
  // failure it surfaces a quick-fix dialog (Download / Open Settings)
  // without throwing, because the commands registered in activate() must
  // remain usable so the user can retry.
  private async startServer(context: ExtensionContextLike): Promise<void> {
    const cfg = this.api.workspace.getConfiguration("mdsmith");
    const configuredPath = cfg.get<string>("path", "mdsmith");
    const binary = this.resolveBinary(configuredPath, context.extensionPath);
    const workspaceRoot = this.api.workspace.workspaceFolders?.[0]?.uri.fsPath;

    const serverOptions = buildServerOptions(binary, this.stdioTransport, workspaceRoot);
    // Replace any previous watcher before creating a new one.
    // restartServer disposes via disposeConfigWatcher, but defensively
    // dispose here too so a future caller of startServer (other than
    // restartServer) cannot accidentally leak. context.subscriptions
    // covers deactivate-time cleanup.
    this.disposeConfigWatcher();
    this.configWatcher = this.api.workspace.createFileSystemWatcher("**/.mdsmith.yml");
    context.subscriptions.push(this.configWatcher);
    const clientOptions = buildClientOptions(this.configWatcher, this.getOutputChannel());
    // Replace the default ErrorHandler (DoNotRestart after 5 close
    // events in 3 minutes) with one that gives the user a clear recovery
    // path. We let the client keep restarting up to a higher per-window
    // threshold; once we hit that ceiling we surface a notification with
    // a "Restart Language Server" / "Show Output" prompt instead of
    // silently disabling the extension. The mdsmith.restartServer
    // command stays the explicit manual recovery path either way. The
    // handler surfaces the prompt via the injected callback once the cap
    // is exceeded; the decision logic lives in decideClose.
    const errorHandler = new MdsmithErrorHandler(() => {
      void this.promptRestartAfterRepeatedFailures();
    });
    clientOptions.errorHandler = errorHandler;

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
            if (block instanceof this.api.MarkdownString) {
              rewriteHoverMarkdown(block);
            }
          }
        }
        return hover;
      },
    };

    const client = this.createClient("mdsmith", "mdsmith", serverOptions, clientOptions);
    this.client = client;

    // Listen for the server announcing that a newer instance for this
    // workspace has taken over. Marking the error handler turns the
    // imminent connection close into a no-restart, so this (now orphaned)
    // editor host stops respawning a server the newer one immediately
    // supersedes again. See the newest-wins workspace singleton in
    // internal/lsp/singleton.go. Registered BEFORE start() so the
    // notification can never land in the gap between start() resolving
    // and handler registration — vscode-languageclient buffers pre-start
    // notification handlers.
    client.onNotification("mdsmith/superseded", () => {
      errorHandler.markSuperseded();
      this.getOutputChannel().appendLine(
        "mdsmith: a newer server has taken over this workspace; " +
          "stopping this instance (it will not be restarted)."
      );
    });

    try {
      await client.start();
    } catch (err) {
      // start() rejected — leave the client referenceable briefly so the
      // user can hit "Show Output" to read the failure log, then drop the
      // reference. Without this clear, a partially-started client lingers
      // and a subsequent deactivate() / restart would call stop() on
      // something that never reached the running state, throwing inside
      // vscode-languageclient. Also tear down the watcher; startServer
      // will install a fresh one on next attempt.
      const candidates = this.findBinaryCandidates(context.extensionPath);
      const detail = startupErrorMessage(err, {
        configuredPath,
        resolvedCommand: binary,
        candidates,
      });
      // Mirror the full diagnostic to the Output channel so the user can
      // scroll, copy, and inspect every candidate path — VS Code
      // truncates long error notifications, but the channel does not.
      this.getOutputChannel().appendLine(detail);
      const choice = await this.api.window.showErrorMessage(
        detail,
        "Download mdsmith",
        "Open Settings",
        "Show Output"
      );
      if (choice === "Show Output") {
        this.showOutput();
      }
      this.client = undefined;
      this.disposeConfigWatcher();
      if (choice === "Download mdsmith") {
        await this.api.env.openExternal(
          this.api.Uri.parse("https://github.com/jeduden/mdsmith/releases")
        );
      } else if (choice === "Open Settings") {
        await this.api.commands.executeCommand("workbench.action.openSettings", "mdsmith");
      }
    }
  }

  // restartServer stops the running client (if any) and starts a fresh
  // one. Useful when the user fixes `mdsmith.path`, rebuilds the binary,
  // or otherwise wants to recover without reloading the VS Code window.
  async restartServer(context: ExtensionContextLike): Promise<void> {
    if (this.client) {
      try {
        await this.client.stop();
      } catch {
        // Ignore — a half-started client may refuse to stop, but
        // dropping the reference is enough to reclaim it.
      }
      this.client = undefined;
    }
    // Clean up the previous file watcher; startServer will install a
    // fresh one.
    this.disposeConfigWatcher();
    await this.startServer(context);
  }

  // deactivate stops the client and disposes the watcher and output
  // channel for tight cleanup ordering (context.subscriptions is flushed
  // only after deactivate returns).
  async deactivate(): Promise<void> {
    if (this.client) {
      try {
        await this.client.stop();
      } catch {
        // A client whose start() failed (or that is still "starting"
        // when the host shuts the extension down) can throw from stop();
        // swallow so deactivate always completes cleanly. Dropping the
        // reference below releases the client object regardless.
      }
      this.client = undefined;
    }
    // The watcher is also pushed onto context.subscriptions in
    // startServer, but VS Code disposes those AFTER deactivate returns;
    // clear it explicitly so the dispose ordering is tight and
    // configWatcher does not survive into a subsequent activation.
    this.disposeConfigWatcher();
    // Dispose the shared output channel for the same tight-ordering
    // reason.
    if (this.outputChannel) {
      this.outputChannel.dispose();
      this.outputChannel = undefined;
    }
  }

  // disposeConfigWatcher releases the active .mdsmith.yml watcher so a
  // new one can take over. Idempotent — calling it without an active
  // watcher is a no-op.
  private disposeConfigWatcher(): void {
    if (this.configWatcher) {
      this.configWatcher.dispose();
      this.configWatcher = undefined;
    }
  }

  // showOutput reveals the "mdsmith" output channel. Uses
  // getOutputChannel() so the standalone channel created by palette
  // commands is also reachable when the LSP client is not running.
  private showOutput(): void {
    this.getOutputChannel().show(true);
  }

  // promptRestartAfterRepeatedFailures runs after the error handler has
  // decided to stop restarting. The user can pick one of the actionable
  // buttons; "Restart" calls the same command users get from the palette
  // so the recovery path is consistent.
  private async promptRestartAfterRepeatedFailures(): Promise<void> {
    const choice = await this.api.window.showErrorMessage(
      "mdsmith server crashed too many times in a row. Linting is paused.",
      "Restart Language Server",
      "Show Output"
    );
    if (choice === "Restart Language Server") {
      await this.api.commands.executeCommand(RESTART_SERVER_COMMAND);
    } else if (choice === "Show Output") {
      this.showOutput();
    }
  }

  // getOutputChannel returns the single "mdsmith" OutputChannel shared
  // between palette commands and the language client. Created lazily on
  // first use so we don't reserve a channel before anyone needs it; the
  // same instance is passed into LanguageClientOptions.outputChannel so
  // the LSP client doesn't register a second channel with the same name.
  private getOutputChannel(): OutputChannel {
    if (!this.outputChannel) {
      this.outputChannel = this.api.window.createOutputChannel("mdsmith");
    }
    return this.outputChannel;
  }

  // resolveActiveBinary reads `mdsmith.path` at call time so the palette
  // commands pick up config edits without a window reload.
  private resolveActiveBinary(extensionPath: string, scope?: Uri): string {
    const cfg = this.api.workspace.getConfiguration("mdsmith", scope);
    return this.resolveBinary(cfg.get<string>("path", "mdsmith"), extensionPath);
  }

  // registerPaletteCommands wires the mdsmith.* palette commands and the
  // two virtual-document schemes. Called once from activate(). Trust-
  // gated commands use the built-in isWorkspaceTrusted when condition,
  // which VS Code re-evaluates automatically when trust is granted — no
  // onDidGrantWorkspaceTrust subscription is required.
  private registerPaletteCommands(context: ExtensionContextLike): void {
    const api = this.api;
    const getActiveFileUri = (): Uri | undefined => {
      const uri = api.window.activeTextEditor?.document.uri;
      return uri?.scheme === "file" ? uri : undefined;
    };
    // In multi-root workspaces, prefer the folder containing the active
    // editor so file-modifying commands operate in the folder the user is
    // working in. Falls back to the first folder when there is no active
    // editor or it lives outside any workspace folder.
    const getWorkspaceRoot = (): string | undefined => {
      const folders = api.workspace.workspaceFolders;
      if (!folders || folders.length === 0) return undefined;
      if (folders.length > 1) {
        const activeUri = api.window.activeTextEditor?.document.uri;
        if (activeUri) {
          const folder = api.workspace.getWorkspaceFolder(activeUri);
          if (folder) return folder.uri.fsPath;
        }
      }
      return folders[0].uri.fsPath;
    };
    // Scope configuration lookups to the workspace folder being operated
    // on so per-folder mdsmith.path / mdsmith.config values are respected
    // in multi-root workspaces. Falls back to the active file URI when no
    // workspace folder is available (e.g. for the kinds virtual-doc
    // commands).
    const getConfigScope = (): Uri | undefined => {
      const root = getWorkspaceRoot();
      return root ? api.Uri.file(root) : getActiveFileUri();
    };
    const getBinary = () => this.resolveActiveBinary(context.extensionPath, getConfigScope());
    const getConfigPath = (): string | undefined => {
      const v = api.workspace.getConfiguration("mdsmith", getConfigScope()).get<string>("config", "");
      return v || undefined;
    };
    const isTrusted = () => api.workspace.isTrusted;

    const outputDeps = {
      appendOutput: (text: string) => {
        this.getOutputChannel().append(text);
      },
      showOutput: () => this.showOutput(),
    };

    // showNotification routes failures (messages containing "failed" or
    // "could not start") to showWarningMessage so they surface as
    // warnings rather than appearing informational to the user.
    const showNotification = (msg: string, ...buttons: string[]): Promise<string | undefined> => {
      const isFailure = msg.includes("failed") || msg.includes("could not start");
      return Promise.resolve(
        isFailure
          ? api.window.showWarningMessage(msg, ...buttons)
          : api.window.showInformationMessage(msg, ...buttons)
      );
    };

    // showError surfaces a command failure; the palette commands share
    // this one helper so the surfacing cannot drift between them.
    const showError = (msg: string): Promise<void> =>
      Promise.resolve(api.window.showErrorMessage(msg)).then(() => {});

    const getDiagnostics = (filePath: string) =>
      api.languages.getDiagnostics(api.Uri.file(filePath));

    const confirmDestructive = (label: string) => async () => {
      const answer = await api.window.showWarningMessage(
        `Run \`${label}\` in the workspace? This will modify files.`,
        { modal: true },
        "Proceed"
      );
      return answer === "Proceed";
    };

    const openVirtualDoc = async (uri: string) => {
      const doc = await api.workspace.openTextDocument(api.Uri.parse(uri));
      const mdDoc = await api.languages.setTextDocumentLanguage(doc, "markdown");
      await api.window.showTextDocument(mdDoc, {
        preview: true,
        viewColumn: api.ViewColumn.Beside,
      });
    };

    const getActiveMarkdownFilePath = (): string | undefined => {
      const editor = api.window.activeTextEditor;
      if (!editor || editor.document.uri.scheme !== "file") return undefined;
      if (editor.document.languageId !== "markdown") return undefined;
      return editor.document.uri.fsPath;
    };

    context.subscriptions.push(
      api.commands.registerCommand("mdsmith.init", async () => {
        await runInit({
          binary: getBinary(),
          workspaceRoot: getWorkspaceRoot(),
          isTrusted,
          showInfo: showNotification,
          showError,
          ...outputDeps,
        });
      }),

      api.commands.registerCommand("mdsmith.mergeDriver.install", async () => {
        await runMergeDriverInstall({
          binary: getBinary(),
          workspaceRoot: getWorkspaceRoot(),
          isTrusted,
          confirm: confirmDestructive("mdsmith merge-driver install"),
          showInfo: showNotification,
          showError,
          ...outputDeps,
        });
      }),

      api.commands.registerCommand("mdsmith.fixWorkspace", async () => {
        await runFixWorkspace({
          binary: getBinary(),
          workspaceRoot: getWorkspaceRoot(),
          configPath: getConfigPath(),
          isTrusted,
          confirm: confirmDestructive("mdsmith fix ."),
          showInfo: showNotification,
          showError,
          ...outputDeps,
        });
      }),

      api.commands.registerCommand("mdsmith.kinds.resolve", async () => {
        await runKindsResolve({
          getActiveFilePath: getActiveMarkdownFilePath,
          getDiagnostics,
          openVirtualDoc,
          showError,
          isTrusted,
        });
      }),

      api.commands.registerCommand("mdsmith.kinds.why", async () => {
        await runKindsWhy({
          getActiveFilePath: getActiveMarkdownFilePath,
          getDiagnostics,
          pickRule: async (rules) => {
            const items =
              rules.length > 0
                ? rules
                : await api.window
                    .showInputBox({
                      prompt: "No active diagnostics. Enter a rule ID (e.g. MDS001)",
                      placeHolder: "MDS001",
                    })
                    .then((v) => (v ? [v] : []));
            if (!items || items.length === 0) return undefined;
            if (items.length === 1) return items[0];
            return api.window.showQuickPick(items, {
              placeHolder: "Pick a rule to explain",
            });
          },
          openVirtualDoc,
          showError,
          isTrusted,
        });
      }),

      // Open a rule's embedded README in a read-only virtual document.
      // Invoked from hover links the middleware rewrote (and trusted via
      // isTrusted.enabledCommands), so id is normally a rule ID. But
      // registerCommand is globally callable, so enforce the same
      // MDS<digits> shape here and no-op on anything else, rather than
      // opening a tab that just reports a malformed URI.
      api.commands.registerCommand(OPEN_RULE_DOC_COMMAND, async (id?: string) => {
        if (!id || !isRuleId(id)) return;
        await openVirtualDoc(buildRuleDocUri(id));
      }),

      // Register the virtual document provider for the mdsmith-kinds:
      // scheme.
      api.workspace.registerTextDocumentContentProvider(KINDS_SCHEME, {
        provideTextDocumentContent: (uri) => {
          const uriStr = kindsContentUri(uri);
          const parsed = parseKindsUri(uriStr);
          // Derive the workspace folder from the file encoded in the URI
          // so binary/config lookups use the correct per-folder settings
          // even when the active editor is the virtual doc itself.
          const fileFolder = parsed
            ? api.workspace.getWorkspaceFolder(api.Uri.file(parsed.file))
            : undefined;
          // When workspace folders are open, reject files outside the
          // workspace to prevent crafted URIs from analyzing arbitrary
          // paths on disk.
          const folders = api.workspace.workspaceFolders;
          if (parsed && folders && folders.length > 0 && !fileFolder) {
            return Promise.resolve(
              `**mdsmith: file is outside the workspace**\n\n\`\`\`\n${parsed.file}\n\`\`\``
            );
          }
          const binaryScope = fileFolder?.uri ?? getActiveFileUri();
          const binary = this.resolveActiveBinary(context.extensionPath, binaryScope);
          const provider = makeKindsContentProvider(binary, fileFolder?.uri.fsPath);
          return provider.provideTextDocumentContent(uriStr);
        },
      }),

      // Register the virtual document provider for the mdsmith-rule:
      // scheme, which renders `mdsmith help rule <id>` (the embedded
      // README) offline so the rewritten hover doc link opens without a
      // browser or network.
      api.workspace.registerTextDocumentContentProvider(RULE_SCHEME, {
        provideTextDocumentContent: (uri) =>
          provideRuleDocContent(uri, getBinary(), getWorkspaceRoot(), undefined, isTrusted),
      })

      // VS Code automatically re-evaluates the built-in
      // `isWorkspaceTrusted` context when trust is granted, so menu
      // entries gated with `when: isWorkspaceTrusted` appear without a
      // reload — no explicit handler needed here.
    );
  }
}
