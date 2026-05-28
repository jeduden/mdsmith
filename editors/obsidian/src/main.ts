// Entrypoint for the mdsmith Obsidian plugin.
//
// Obsidian loads `dist/main.js` and instantiates the default export
// as a Plugin subclass. This module is the wiring root: it spawns
// `mdsmith lsp`, hands stdio to `Wiring`, registers the CM6
// diagnostics extension, palette commands, the settings tab, and a
// debounced fix-on-save listener. `onunload` shuts the server down,
// kills the child after a short grace period, and removes the
// extension's transient state.

import {
  Editor,
  MarkdownView,
  Notice,
  Plugin,
  type TFile,
} from "obsidian";
import { spawn, type ChildProcessByStdio } from "node:child_process";
import { join } from "node:path";

import {
  applyWorkspaceEdit,
  buildLineCommands,
  debounce,
  type DebouncedFn,
  type LineCommand,
  type LspDiagnostic,
  type WorkspaceEdit,
} from "./actions";
import { resolveBinary } from "./binary";
import {
  editorExtensions,
  setDiagnostics,
  type LspDiagnostic as DiagShape,
} from "./diagnostics";
import { LspClient } from "./lsp-client";
import {
  DEFAULTS,
  MdsmithSettingTab,
  type MdsmithSettings,
  classifyChange,
  normalize,
} from "./settings";
import { Wiring } from "./wiring";

// SpawnedChild is the structural subset of `child_process.ChildProcess`
// the plugin uses. `stdin` and `stdout` are required (the LSP client
// reads/writes through them); `kill()` ends the child if `exit` is
// ignored.
type SpawnedChild = ChildProcessByStdio<
  NodeJS.WritableStream,
  NodeJS.ReadableStream,
  NodeJS.ReadableStream
>;

export default class MdsmithPlugin extends Plugin {
  private settings: MdsmithSettings = { ...DEFAULTS };
  private child: SpawnedChild | undefined;
  private client: LspClient | undefined;
  private wiring: Wiring | undefined;
  private diagnosticsByUri = new Map<string, LspDiagnostic[]>();
  private lineCommands: LineCommand[] = [];
  private debouncedFixOnSave: DebouncedFn<[]> | undefined;

  override async onload(): Promise<void> {
    // 1. Settings → 2. server start → 3. CM6 extension + commands.
    // Each step that can fail surfaces a Notice rather than crashing
    // the plugin, so the user can fix and reload from settings.
    this.settings = normalize(await this.loadData());
    this.addSettingTab(new MdsmithSettingTab(this.app, this.asHost()));
    this.registerEditorExtension(
      editorExtensions((d) => void this.applyQuickFix(d)) as never,
    );
    this.registerPaletteCommands();
    this.registerVaultListeners();
    await this.startServer();
  }

  override async onunload(): Promise<void> {
    this.debouncedFixOnSave?.cancel();
    if (this.wiring) {
      try {
        await this.wiring.stop();
      } catch {
        // shutdown may reject if the server is already dead — fall
        // through to the kill below either way.
      }
    }
    if (this.child && !this.child.killed) {
      // Give the server 1 s to exit cleanly; if it does not, the
      // kill() inside wiring.stop has already fired, but a hung
      // process keeps the file handles open. SIGKILL guarantees we
      // release them before the plugin is reloaded.
      setTimeout(() => {
        if (this.child && !this.child.killed) {
          try {
            this.child.kill("SIGKILL");
          } catch {
            // Process already gone — nothing to do.
          }
        }
      }, 1000);
    }
    this.wiring = undefined;
    this.client = undefined;
    this.child = undefined;
  }

  // saveSettings round-trips the new shape to disk and reacts based
  // on the change classifier: restart for binaryPath / configPath,
  // reconfigure for the runtime knobs.
  private async saveSettings(next: MdsmithSettings): Promise<void> {
    const before = this.settings;
    this.settings = next;
    await this.saveData(next);
    const reaction = classifyChange(before, next);
    if (reaction === "restart") {
      await this.restartServer();
    } else if (reaction === "reconfigure") {
      this.registerVaultListeners(); // refresh fixOnSave wiring
    }
  }

  // asHost exposes the typed surface the settings tab needs without
  // letting it reach the rest of the plugin state.
  private asHost(): {
    app: typeof this.app;
    manifest: typeof this.manifest;
    settings: MdsmithSettings;
    saveSettings(next: MdsmithSettings): Promise<void>;
  } {
    return {
      app: this.app,
      manifest: this.manifest,
      get settings(): MdsmithSettings {
        // Read at access time so the tab always sees the latest
        // value, not a snapshot from when it was constructed.
        // eslint-disable-next-line @typescript-eslint/no-this-alias
        const self = this as unknown as { __plugin?: MdsmithPlugin };
        return self.__plugin ? self.__plugin.settings : DEFAULTS;
      },
      saveSettings: (next: MdsmithSettings) => this.saveSettings(next),
      __plugin: this,
    } as unknown as ReturnType<MdsmithPlugin["asHost"]>;
  }

  // startServer resolves the binary, spawns `mdsmith lsp`, and hands
  // stdio to a fresh Wiring. Any failure surfaces a Notice and
  // leaves the plugin operating in a degraded "server down" state.
  private async startServer(): Promise<void> {
    const pluginPath = this.pluginPath();
    const command = resolveBinary(this.settings.binaryPath, pluginPath);
    const args = ["lsp"];
    if (this.settings.configPath) {
      args.push("-c", this.settings.configPath);
    }
    try {
      this.child = spawn(command, args, {
        stdio: ["pipe", "pipe", "pipe"],
        cwd: this.workspaceRoot(),
      });
    } catch (err) {
      new Notice(
        `mdsmith: failed to spawn ${command}: ${(err as Error).message}`,
      );
      return;
    }
    this.child.on("error", (err) => {
      new Notice(`mdsmith: server error: ${err.message}`);
    });
    this.child.stderr?.on("data", (b: Buffer) => {
      // Stash stderr in the console for debugging; surface a Notice
      // only if traceServer asks for it (mirrors VS Code's output
      // channel behavior).
      if (this.settings.traceServer !== "off") {
        console.warn(`mdsmith: ${b.toString("utf8")}`);
      }
    });

    this.client = new LspClient(this.child.stdin, this.child.stdout);
    this.wiring = new Wiring({
      client: this.client,
      killChild: () => this.child?.kill("SIGTERM"),
      rootUri: `file://${this.workspaceRoot() ?? ""}`,
      onPublishDiagnostics: (uri, diagnostics) =>
        this.handlePublishDiagnostics(uri, diagnostics),
      configPath: this.settings.configPath || undefined,
    });
    try {
      await this.wiring.start();
    } catch (err) {
      new Notice(
        `mdsmith: failed to initialize server: ${(err as Error).message}`,
      );
    }
  }

  private async restartServer(): Promise<void> {
    await this.onunload();
    await this.startServer();
  }

  // handlePublishDiagnostics caches the latest set per URI and
  // dispatches the matching CM6 effect into any open editor for
  // that URI.
  private handlePublishDiagnostics(
    uri: string,
    diagnostics: LspDiagnostic[],
  ): void {
    this.diagnosticsByUri.set(uri, diagnostics);
    const view = this.findMarkdownViewByUri(uri);
    if (!view) return;
    const editor = view.editor as unknown as {
      cm?: { dispatch(spec: unknown): void };
    };
    editor.cm?.dispatch({
      effects: setDiagnostics.of(diagnostics as DiagShape[]),
    });
  }

  private findMarkdownViewByUri(uri: string): MarkdownView | undefined {
    const leaves = this.app.workspace.getLeavesOfType("markdown");
    for (const leaf of leaves) {
      const view = leaf.view as unknown as MarkdownView & { file?: TFile };
      const file = (view as unknown as { file?: TFile }).file;
      if (!file) continue;
      const fileUri = this.fileUri(file);
      if (fileUri === uri) return view;
    }
    return undefined;
  }

  private fileUri(file: TFile): string {
    // Obsidian exposes the vault-relative path; build a file:// URI
    // anchored at the workspace root.
    const root = this.workspaceRoot();
    if (!root) return `file://${file.path}`;
    return `file://${join(root, file.path)}`;
  }

  private registerPaletteCommands(): void {
    this.addCommand({
      id: "mdsmith-fix-file",
      name: "mdsmith: Fix file",
      editorCallback: async (editor: Editor, view: MarkdownView) => {
        await this.fixFile(editor, view);
      },
    });
    this.addCommand({
      id: "mdsmith-restart-server",
      name: "mdsmith: Restart server",
      callback: async () => {
        await this.restartServer();
        new Notice("mdsmith: server restarted");
      },
    });
  }

  // registerVaultListeners attaches (or re-attaches) the file events
  // the plugin reacts to. Called on load and again after every
  // settings reconfigure so `fixOnSave` toggles take effect without
  // a full restart.
  private registerVaultListeners(): void {
    this.debouncedFixOnSave?.cancel();
    if (this.settings.fixOnSave) {
      this.debouncedFixOnSave = debounce(() => {
        const view = this.app.workspace.getActiveViewOfType(MarkdownView);
        if (!view) return;
        void this.fixFile(view.editor, view);
      }, 200);
    } else {
      this.debouncedFixOnSave = undefined;
    }
    this.registerEvent(
      this.app.vault.on("modify", (file) => {
        if (!file) return;
        // .mdsmith.yml edits trigger a workspace re-lint via
        // workspace/didChangeWatchedFiles so the server picks up
        // the new config without a plugin restart.
        if ((file as TFile).name === ".mdsmith.yml" && this.wiring) {
          this.wiring.notifyDidChangeWatchedFiles([
            { uri: this.fileUri(file as TFile), type: 2 },
          ]);
          return;
        }
        if ((file as TFile).extension !== "md") return;
        if (!this.wiring) return;
        const uri = this.fileUri(file as TFile);
        // Resync line palette commands on every modification so the
        // active set tracks the cursor's diagnostic landscape.
        this.refreshLineCommands();
        // fixOnSave debounces and triggers Fix file 200 ms after
        // the last modify event.
        this.debouncedFixOnSave?.();
        // Touch the local cache so subsequent didChange notifications
        // carry monotonic versions even before the LSP client wires
        // an editor-extension listener.
        this.diagnosticsByUri.get(uri);
      }),
    );
  }

  // refreshLineCommands rebuilds the transient per-line palette
  // commands so each active diagnostic on the cursor line appears as
  // its own `mdsmith: Fix — {code}` entry. The plan calls for this on
  // cursor move; we wire it to modify+activate events for now.
  private refreshLineCommands(): void {
    const view = this.app.workspace.getActiveViewOfType(MarkdownView);
    if (!view) {
      this.lineCommands = [];
      return;
    }
    const file = (view as unknown as { file?: TFile }).file;
    if (!file) return;
    const uri = this.fileUri(file);
    const diags = this.diagnosticsByUri.get(uri) ?? [];
    const cursor = view.editor.getCursor();
    this.lineCommands = buildLineCommands(diags, cursor.line);
    for (const cmd of this.lineCommands) {
      this.addCommand({
        id: cmd.id,
        name: cmd.name,
        editorCallback: async (editor: Editor, mv: MarkdownView) => {
          await this.applyQuickFixOnLine(editor, mv, cmd.diagnostic);
        },
      });
    }
  }

  private async fixFile(editor: Editor, view: MarkdownView): Promise<void> {
    if (!this.wiring) return;
    const file = (view as unknown as { file?: TFile }).file;
    if (!file) return;
    const uri = this.fileUri(file);
    const range = {
      start: { line: 0, character: 0 },
      end: { line: editor.lineCount(), character: 0 },
    };
    const actions = (await this.wiring.requestCodeAction(uri, range, [
      "source.fixAll.mdsmith",
    ])) as Array<{ edit?: WorkspaceEdit }>;
    for (const action of actions) {
      if (!action.edit) continue;
      this.dispatchWorkspaceEdit(uri, editor, action.edit);
    }
  }

  private async applyQuickFix(diagnostic: LspDiagnostic): Promise<void> {
    if (!this.wiring) return;
    const view = this.app.workspace.getActiveViewOfType(MarkdownView);
    if (!view) return;
    await this.applyQuickFixOnLine(view.editor, view, diagnostic);
  }

  private async applyQuickFixOnLine(
    editor: Editor,
    view: MarkdownView,
    diagnostic: LspDiagnostic,
  ): Promise<void> {
    if (!this.wiring) return;
    const file = (view as unknown as { file?: TFile }).file;
    if (!file) return;
    const uri = this.fileUri(file);
    const actions = (await this.wiring.requestCodeAction(
      uri,
      diagnostic.range,
    )) as Array<{ edit?: WorkspaceEdit; kind?: string }>;
    for (const action of actions) {
      if (!action.edit) continue;
      this.dispatchWorkspaceEdit(uri, editor, action.edit);
    }
  }

  private dispatchWorkspaceEdit(
    uri: string,
    editor: Editor,
    edit: WorkspaceEdit,
  ): void {
    const adapter = {
      uri,
      offsetAt: (pos: { line: number; character: number }) =>
        editor.posToOffset({ line: pos.line, ch: pos.character }),
      dispatch: (changes: Array<{ from: number; to: number; insert: string }>) => {
        // Editor exposes `replaceRange(text, from, to)`; loop bottom-
        // up so earlier offsets stay valid as later edits land.
        for (const c of changes) {
          editor.replaceRange(
            c.insert,
            editor.offsetToPos(c.from),
            editor.offsetToPos(c.to),
          );
        }
      },
    };
    applyWorkspaceEdit(adapter, edit);
  }

  private workspaceRoot(): string | undefined {
    const adapter = this.app.vault.adapter as unknown as { basePath?: string };
    return adapter.basePath;
  }

  private pluginPath(): string {
    // Obsidian places the plugin under
    // <vault>/.obsidian/plugins/<id>/. manifest.dir carries the
    // vault-relative form; resolve it against the vault root.
    const root = this.workspaceRoot();
    const dir = (this.manifest as unknown as { dir?: string }).dir;
    if (root && dir) return join(root, dir);
    return dir ?? "";
  }
}
