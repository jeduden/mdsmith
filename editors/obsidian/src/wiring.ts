// Wiring helper that glues the LSP client to the rest of the plugin.
//
// `main.ts` instantiates one `Wiring` per plugin load, hands it the
// spawned-child seams (an `LspClient` plus a kill callback), and
// drives `start()` / `stop()` from `onload` / `onunload`. The wiring
// owns the LSP-side state (per-URI version counters, the
// publishDiagnostics fan-out) so it can be unit-tested without the
// Obsidian host.
//
// Wider integration (CM6 extension registration, palette commands,
// settings tab) lands in `main.ts` next to the `Plugin` subclass —
// those touch real Obsidian APIs that have no useful stub surface.

import { LspClient } from "./lsp-client";
import type { LspDiagnostic } from "./diagnostics";

// PublishHandler is the per-URI diagnostics fan-out callback. main.ts
// converts the LspDiagnostic[] into a CM6 effect and dispatches it
// against the matching editor.
export type PublishHandler = (
  uri: string,
  diagnostics: LspDiagnostic[],
) => void;

// WiringDeps is the seam bundle main.ts provides. `client` owns the
// JSON-RPC surface; `killChild` is the kill signal main.ts can fire
// after `exit` if the server fails to exit cleanly.
export interface WiringDeps {
  client: LspClient;
  killChild: () => void;
  rootUri: string;
  onPublishDiagnostics: PublishHandler;
  // Optional config path passed to the server as the `-c` arg in
  // ServerOptions; recorded here so future readers can correlate
  // it with the active settings.
  configPath?: string;
}

// PublishParams is the LSP notification shape; we keep the typing
// loose because the runtime payload from mdsmith is the only source
// of truth.
interface PublishParams {
  uri: string;
  diagnostics: LspDiagnostic[];
}

// Wiring runs the LSP handshake on start(), tears it down on stop(),
// tracks per-URI versions so didChange notifications carry the right
// monotonic counter, and routes publishDiagnostics into the supplied
// PublishHandler.
export class Wiring {
  private versions = new Map<string, number>();

  constructor(private readonly deps: WiringDeps) {
    // Hook the notification before initialize() runs so any
    // publishDiagnostics emitted during the handshake are not lost.
    this.deps.client.onNotification(
      "textDocument/publishDiagnostics",
      (params) => {
        const p = (params ?? {}) as Partial<PublishParams>;
        if (typeof p.uri !== "string") return;
        const diagnostics = Array.isArray(p.diagnostics) ? p.diagnostics : [];
        this.deps.onPublishDiagnostics(p.uri, diagnostics);
      },
    );
  }

  // start runs the LSP initialize handshake. The client also pushes
  // the `initialized` notification once initialize resolves.
  async start(): Promise<void> {
    await this.deps.client.initialize({
      processId: typeof process !== "undefined" ? process.pid : null,
      rootUri: this.deps.rootUri,
      capabilities: {
        textDocument: {
          publishDiagnostics: { versionSupport: true },
          codeAction: {
            codeActionLiteralSupport: {
              codeActionKind: {
                valueSet: ["source.fixAll.mdsmith", "quickfix"],
              },
            },
          },
        },
      },
      // workspaceFolders carries the same URI rootUri does; some
      // servers prefer one or the other, so we send both.
      workspaceFolders: [
        { uri: this.deps.rootUri, name: "vault" },
      ],
    });
  }

  // stop runs the shutdown handshake and signals the child. The kill
  // callback fires after `exit` is sent so main.ts can SIGKILL the
  // child after a short grace period if it does not exit on its own.
  async stop(): Promise<void> {
    try {
      await this.deps.client.shutdown();
    } finally {
      this.deps.killChild();
    }
  }

  // notifyDidOpen tells the server that a buffer has been opened.
  // Used when the user activates a Markdown file in the workspace.
  notifyDidOpen(uri: string, languageId: string, text: string): void {
    const version = 1;
    this.versions.set(uri, version);
    this.deps.client.notify("textDocument/didOpen", {
      textDocument: { uri, languageId, version, text },
    });
  }

  // notifyDidChange tells the server the buffer content changed.
  // The version monotonically increments per URI; the server uses
  // it to discard stale diagnostics.
  notifyDidChange(uri: string, text: string): void {
    const next = (this.versions.get(uri) ?? 0) + 1;
    this.versions.set(uri, next);
    this.deps.client.notify("textDocument/didChange", {
      textDocument: { uri, version: next },
      contentChanges: [{ text }],
    });
  }

  // notifyDidSave tells the server the buffer was saved. Triggers
  // fix-on-save behavior in the server when configured.
  notifyDidSave(uri: string, text?: string): void {
    this.deps.client.notify("textDocument/didSave", {
      textDocument: { uri },
      ...(text !== undefined ? { text } : {}),
    });
  }

  // notifyDidClose tells the server the buffer has been closed.
  // The version counter is dropped so a subsequent didOpen restarts
  // at 1.
  notifyDidClose(uri: string): void {
    this.versions.delete(uri);
    this.deps.client.notify("textDocument/didClose", {
      textDocument: { uri },
    });
  }

  // notifyDidChangeWatchedFiles signals an out-of-band change (e.g.
  // `.mdsmith.yml` edited). The server reacts by re-linting the
  // workspace.
  notifyDidChangeWatchedFiles(
    changes: Array<{ uri: string; type: number }>,
  ): void {
    this.deps.client.notify("workspace/didChangeWatchedFiles", { changes });
  }

  // requestCodeAction asks the server for the code actions
  // available on a range. The plugin uses this for both the hover
  // tooltip Fix link and the Fix file command.
  async requestCodeAction(
    uri: string,
    range: { start: { line: number; character: number }; end: { line: number; character: number } },
    only?: string[],
  ): Promise<unknown[]> {
    const params = {
      textDocument: { uri },
      range,
      context: {
        diagnostics: [],
        ...(only ? { only } : {}),
      },
    };
    const result = await this.deps.client.request(
      "textDocument/codeAction",
      params,
    );
    return Array.isArray(result) ? result : [];
  }
}
