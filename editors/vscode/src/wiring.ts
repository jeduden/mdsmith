// Wiring helpers extracted from extension.ts so the spawn/glob/error
// shapes can be unit-tested without booting a real VS Code host.
//
// These functions deliberately depend only on the types from
// `vscode-languageclient/node` plus a couple of structural shapes
// (FileSystemWatcher, Uri-like) so `bun test` can stand in lightweight
// fakes without pulling in the full `vscode` runtime.

import {
  LanguageClientOptions,
  ServerOptions,
  TransportKind
} from "vscode-languageclient/node";

import type { BinaryCandidate } from "./binary";

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

// shouldFixOnSave decides whether the willSave handler runs the
// whole-file fix. `mdsmith.fixOnSave` is subordinate to `mdsmith.run`:
// when run is "off" automatic linting is disabled, so a save must not
// rewrite the buffer even if fixOnSave was left on — matching the
// server, which publishes no diagnostics in off mode. onType and
// onSave both allow fix-on-save when fixOnSave is enabled.
export function shouldFixOnSave(run: string, fixOnSave: boolean): boolean {
  return fixOnSave && run !== RUN_OFF;
}

// Minimal shapes of the bits of vscode.CodeAction / WorkspaceEdit /
// Uri / TextEdit we touch when filtering fixAll edits. Defining them
// here lets tests drive the pure pipeline without importing `vscode`.
export interface UriLike {
  toString(): string;
}

export interface TextEditLike {
  // marker — we forward as-is.
}

export interface WorkspaceEditLike {
  entries(): readonly [UriLike, TextEditLike[]][];
}

export interface CodeActionLike {
  kind?: { value: string };
  edit?: WorkspaceEditLike;
}

// collectFixAllEdits filters the array returned by
// `vscode.executeCodeActionProvider` down to the TextEdits a
// fixAll-on-save handler should apply. We keep only edits whose
// CodeAction.kind is exactly "source.fixAll.mdsmith" and whose URI
// matches the document being saved; everything else (other kinds,
// other files, missing edits) is dropped.
export function collectFixAllEdits(
  actions: unknown,
  documentUri: UriLike
): TextEditLike[] {
  const list = (actions ?? []) as CodeActionLike[];
  const target = documentUri.toString();
  const edits: TextEditLike[] = [];
  for (const action of list) {
    if (action.kind?.value !== "source.fixAll.mdsmith") continue;
    if (!action.edit) continue;
    for (const [uri, items] of action.edit.entries()) {
      if (uri.toString() !== target) continue;
      for (const item of items) {
        edits.push(item);
      }
    }
  }
  return edits;
}
