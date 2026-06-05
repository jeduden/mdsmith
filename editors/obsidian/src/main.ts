// Entrypoint for the mdsmith Obsidian plugin.
//
// Obsidian loads dist/main.js and instantiates the default export as a
// Plugin subclass. This module is the thin wiring root: it builds one
// WASM runtime over the vault and connects it to the editor surfaces.
// Every piece of logic is delegated to a tested module (wasm-runtime,
// workspace, diagnostics, actions, settings, wiring); the methods here
// just orchestrate the Obsidian lifecycle.
//
// onload order (plan 217 §Lifecycle): read settings → load the WASM
// bundle → build the workspace snapshot → createRuntime once → register
// the CM6 extension, commands, diagnostics view, and vault listeners.
// onunload disposes the runtime, cancels listeners, and detaches the
// view. A "Restart session" command runs the same dispose + create
// flow a configPath change uses.

import {
  ItemView,
  MarkdownView,
  Notice,
  Plugin,
  WorkspaceLeaf,
  type App,
  type Editor,
  type EventRef,
  type PluginManifest,
} from "obsidian";

import {
  applyFixToEditor,
  buildLineCommands,
  debounce,
  type DebouncedFn,
  type LineCommand,
} from "./actions";
import { discoverConfigYAML } from "./config";
import {
  editorExtensions,
  setDiagnostics,
} from "./diagnostics";
import {
  classifyChange,
  DEFAULTS,
  MdsmithSettingTab,
  normalize,
  type MdsmithSettings,
  type SettingsHost,
} from "./settings";
import {
  createRuntime,
  type Diagnostic,
  type MdsmithRuntime,
} from "./wasm-runtime";
import {
  diagnosticRows,
  editorAdapter,
  makeAssetLoaders,
} from "./wiring";
import { snapshotVault, WorkspaceSync } from "./workspace";

export const DIAGNOSTICS_VIEW_TYPE = "mdsmith-diagnostics";

export default class MdsmithPlugin extends Plugin {
  // The base Plugin declares an optional public `settings`; use a
  // distinct field so our typed shape does not collide with it.
  private cfg: MdsmithSettings = { ...DEFAULTS };
  private runtime: MdsmithRuntime | undefined;
  private sync: WorkspaceSync | undefined;
  private fixOnSaveRef: EventRef | undefined;
  private debouncedFixOnSave: DebouncedFn<[string]> | undefined;
  private lineCommandIds: string[] = [];
  // The latest diagnostics per uri, for the workspace diagnostics view.
  private diagnosticsByUri = new Map<string, Diagnostic[]>();

  override async onload(): Promise<void> {
    this.cfg = normalize(await this.loadData());
    this.addSettingTab(new MdsmithSettingTab(this.app, this.asHost()));

    // The CM6 extension is registered once; it reads from the per-editor
    // diagnostics field the runtime feeds via setDiagnostics.
    this.registerEditorExtension(
      editorExtensions(
        (d) => void this.fixActiveFile(d),
        (loc) => this.navigateTo(loc.file, loc.line),
      ) as never,
    );

    this.registerView(
      DIAGNOSTICS_VIEW_TYPE,
      (leaf) => new DiagnosticsView(leaf, this),
    );
    this.addCommand({
      id: "mdsmith-open-diagnostics",
      name: "Open diagnostics panel",
      callback: () => void this.openDiagnosticsView(),
    });
    this.addCommand({
      id: "mdsmith-fix-file",
      name: "Fix file",
      editorCallback: (editor, ctx) => {
        if (ctx instanceof MarkdownView) void this.fixFile(editor, ctx);
      },
    });
    this.addCommand({
      id: "mdsmith-restart-session",
      name: "Restart session",
      callback: () => void this.restartRuntime(),
    });

    this.registerActiveFileCheck();
    this.registerCursorCommands();

    await this.startRuntime();
  }

  override onunload(): void {
    this.teardownRuntime();
    this.app.workspace
      .getLeavesOfType(DIAGNOSTICS_VIEW_TYPE)
      .forEach((leaf) => leaf.detach());
  }

  // startRuntime loads the WASM bundle, snapshots the vault, and builds
  // one session. A failure surfaces a Notice and leaves the plugin in a
  // degraded "engine down" state rather than crashing — the user can
  // fix config and restart.
  private async startRuntime(): Promise<boolean> {
    try {
      const loaders = makeAssetLoaders(
        this.app.vault.adapter,
        this.manifest.dir,
      );
      const workspace = await snapshotVault(this.app.vault);
      const configYAML = await this.loadConfigYAML();
      this.runtime = await createRuntime({
        workspace,
        configYAML,
        loadWasmExec: loaders.loadWasmExec,
        loadWasmBytes: loaders.loadWasmBytes,
      });
    } catch (err) {
      new Notice(
        `mdsmith: failed to start the engine: ${(err as Error).message}`,
      );
      return false;
    }

    // Push vault edits into the session, debounced 200 ms per file.
    // WorkspaceSync owns its vault subscriptions: start() subscribes and
    // stop() (run from teardownRuntime on both unload and restart)
    // unsubscribes, so a restart never leaks listeners onto the disposed
    // runtime.
    this.sync = new WorkspaceSync(this.app.vault, this.runtime);
    this.sync.start();

    this.configureFixOnSave();
    // Lint whatever is already open.
    await this.checkActiveFile();
    return true;
  }

  // teardownRuntime disposes the session, unsubscribes the fix-on-save
  // vault listener, cancels the pending fix, and clears any diagnostics
  // already shown. Safe to call when the runtime never started.
  private teardownRuntime(): void {
    this.debouncedFixOnSave?.cancel();
    this.debouncedFixOnSave = undefined;
    if (this.fixOnSaveRef) {
      this.app.vault.offref(this.fixOnSaveRef);
      this.fixOnSaveRef = undefined;
    }
    this.sync?.stop();
    this.sync = undefined;
    this.runtime?.dispose();
    this.runtime = undefined;
    // Drop diagnostics the engine already painted so a disposed or
    // failed-restart engine never leaves stale underlines, tooltips, or
    // a populated panel behind.
    const view = this.app.workspace.getActiveViewOfType(MarkdownView);
    if (view) this.pushDiagnostics(view, []);
    this.diagnosticsByUri.clear();
    this.refreshDiagnosticsView();
  }

  // restartRuntime is the dispose + recreate flow shared by the
  // "Restart session" command and a configPath change. Plan 215 exposes
  // no in-place reconfigure, so a config change rebuilds the session.
  private async restartRuntime(): Promise<void> {
    this.teardownRuntime();
    if (await this.startRuntime()) {
      new Notice("mdsmith: session restarted");
    }
  }

  // loadConfigYAML resolves the YAML the engine compiles. An explicit
  // Config path wins: it is read through the vault adapter (so it works
  // on mobile too) and a read failure surfaces a Notice. With no path
  // set, the plugin auto-discovers a .mdsmith.yml at the vault root; an
  // absent file falls back to "" (the engine defaults).
  private async loadConfigYAML(): Promise<string> {
    const p = this.cfg.configPath.trim();
    if (p) {
      try {
        return await this.app.vault.adapter.read(p);
      } catch (err) {
        new Notice(
          `mdsmith: could not read config ${p}: ${(err as Error).message}`,
        );
        return "";
      }
    }
    return discoverConfigYAML(this.app.vault.adapter);
  }

  // checkActiveFile lints the active Markdown buffer and pushes the
  // result into its CM6 editor and the diagnostics view. A no-op when
  // the runtime is down, run mode is off, or no Markdown file is active.
  private async checkActiveFile(): Promise<void> {
    if (!this.runtime || this.cfg.runMode === "off") return;
    const view = this.app.workspace.getActiveViewOfType(MarkdownView);
    const file = view?.file;
    if (!view || !file) return;
    const uri = file.path;
    const source = view.editor.getValue();
    let diags: Diagnostic[];
    try {
      diags = await this.runtime.check(uri, source);
    } catch (err) {
      new Notice(`mdsmith: check failed: ${(err as Error).message}`);
      return;
    }
    this.diagnosticsByUri.set(uri, diags);
    this.pushDiagnostics(view, diags);
    this.refreshDiagnosticsView();
  }

  // pushDiagnostics dispatches the setDiagnostics effect into a view's
  // CM6 editor so the decoration provider and hover tooltip see the
  // current set.
  private pushDiagnostics(view: MarkdownView, diags: Diagnostic[]): void {
    const cm = cmOf(view.editor);
    if (!cm) return;
    cm.dispatch({ effects: setDiagnostics.of(diags) });
  }

  // isActiveFilePath reports whether path is the active Markdown view's
  // file. Obsidian's vault `modify` fires for ANY vault file (background
  // sync, other plugins), so the on-save re-lint and fix-on-save both
  // gate on this: a save to some other note must not touch the buffer
  // the user is editing.
  private isActiveFilePath(path: string): boolean {
    const view = this.app.workspace.getActiveViewOfType(MarkdownView);
    return view?.file?.path === path;
  }

  // registerActiveFileCheck re-lints on the trigger the run mode picks:
  // every change for onType, only on save (vault modify) for onSave. The
  // modify handler filters to the active file so a background write to a
  // different note never re-lints what the user is looking at.
  private registerActiveFileCheck(): void {
    this.registerEvent(
      this.app.workspace.on("active-leaf-change", () => {
        void this.checkActiveFile();
      }),
    );
    this.registerEvent(
      this.app.workspace.on("editor-change", () => {
        if (this.cfg.runMode === "onType") void this.checkActiveFile();
      }),
    );
    this.registerEvent(
      this.app.vault.on("modify", (file) => {
        if (this.cfg.runMode !== "onSave") return;
        if (!this.isActiveFilePath(file.path)) return;
        void this.checkActiveFile();
      }),
    );
  }

  // registerCursorCommands re-derives the per-line palette commands on
  // each cursor move: one transient "Fix — {rule}" per rule on the
  // cursor line. The previous set is removed first.
  private registerCursorCommands(): void {
    this.registerEvent(
      this.app.workspace.on("active-leaf-change", () =>
        this.refreshLineCommands(),
      ),
    );
    this.registerEvent(
      this.app.workspace.on("editor-change", () =>
        this.refreshLineCommands(),
      ),
    );
  }

  private refreshLineCommands(): void {
    for (const id of this.lineCommandIds) this.removeCommandById(id);
    this.lineCommandIds = [];
    const view = this.app.workspace.getActiveViewOfType(MarkdownView);
    const file = view?.file;
    if (!view || !file) return;
    const diags = this.diagnosticsByUri.get(file.path) ?? [];
    const cursorLine = view.editor.getCursor().line + 1; // 0-based → 1-based
    const cmds = buildLineCommands(diags, cursorLine);
    for (const cmd of cmds) this.addLineCommand(view, cmd);
  }

  private addLineCommand(view: MarkdownView, cmd: LineCommand): void {
    this.addCommand({
      id: cmd.id,
      name: cmd.name.replace(/^mdsmith:\s*/, ""),
      callback: () => void this.fixFile(view.editor, view),
    });
    this.lineCommandIds.push(`mdsmith:${cmd.id}`);
  }

  // removeCommandById removes a transient command. Obsidian's
  // removeCommand takes the fully-qualified id; the typings omit it, so
  // call it structurally.
  private removeCommandById(id: string): void {
    (this as unknown as { removeCommand?: (id: string) => void }).removeCommand?.(
      id,
    );
  }

  // fixFile runs Fix file on a buffer: fix(uri, source) then replace the
  // whole buffer with the rewritten source, then re-lint.
  private async fixFile(editor: Editor, view: MarkdownView): Promise<void> {
    if (!this.runtime || !view.file) return;
    const uri = view.file.path;
    const source = editor.getValue();
    let result;
    try {
      result = await this.runtime.fix(uri, source);
    } catch (err) {
      new Notice(`mdsmith: fix failed: ${(err as Error).message}`);
      return;
    }
    const cm = cmOf(editor);
    if (cm) applyFixToEditor(editorAdapter(cm, uri), result);
    this.diagnosticsByUri.set(uri, result.diagnostics);
    this.pushDiagnostics(view, result.diagnostics);
    this.refreshDiagnosticsView();
  }

  // fixActiveFile is the hover-tooltip Fix link's entry point. The
  // tooltip fires for a specific diagnostic, but mdsmith fix applies
  // every fixable rule at once, so it routes to the same whole-file fix.
  private async fixActiveFile(_d: Diagnostic): Promise<void> {
    const view = this.app.workspace.getActiveViewOfType(MarkdownView);
    if (view) await this.fixFile(view.editor, view);
  }

  // configureFixOnSave wires (or clears) the debounced save→Fix file
  // listener based on the current settings. fixOnSave is subordinate to
  // run mode: when run mode is off, a save never rewrites the buffer.
  //
  // Two correctness guards, both keyed on the modified file's path:
  //   - The modify event filters to the active file, so a background
  //     write to another note never rewrites the open buffer.
  //   - The debounce carries the captured path, and the trailing
  //     callback only fixes when the active view STILL points at that
  //     path — so switching notes inside the 200 ms window never fixes
  //     the wrong buffer.
  private configureFixOnSave(): void {
    if (this.fixOnSaveRef) {
      this.app.vault.offref(this.fixOnSaveRef);
      this.fixOnSaveRef = undefined;
    }
    this.debouncedFixOnSave?.cancel();
    this.debouncedFixOnSave = undefined;
    if (!this.cfg.fixOnSave || this.cfg.runMode === "off") return;
    this.debouncedFixOnSave = debounce((path: string) => {
      // Re-read the active view at fire time: if the user switched notes
      // within the debounce window, the captured path no longer matches
      // and we skip rather than fix a different buffer.
      const view = this.app.workspace.getActiveViewOfType(MarkdownView);
      if (view?.file?.path !== path) return;
      void this.fixFile(view.editor, view);
    }, 200);
    this.fixOnSaveRef = this.app.vault.on("modify", (file) => {
      if (!this.isActiveFilePath(file.path)) return;
      this.debouncedFixOnSave?.(file.path);
    });
    this.registerEvent(this.fixOnSaveRef);
  }

  // navigateTo opens a related location (a schema constraint) at its
  // file and line. A missing file/line is a no-op.
  private navigateTo(file?: string, line?: number): void {
    if (!file) return;
    const target = this.app.vault.getFileByPath(file);
    if (!target) return;
    void this.app.workspace.getLeaf(false).openFile(target, {
      eState: line ? { line: line - 1 } : undefined,
    });
  }

  // openDiagnosticsView reveals the workspace diagnostics panel in the
  // right sidebar, creating it if needed.
  private async openDiagnosticsView(): Promise<void> {
    const existing =
      this.app.workspace.getLeavesOfType(DIAGNOSTICS_VIEW_TYPE)[0];
    const leaf = existing ?? this.app.workspace.getRightLeaf(false);
    if (!leaf) return;
    await leaf.setViewState({ type: DIAGNOSTICS_VIEW_TYPE, active: true });
    this.app.workspace.revealLeaf(leaf);
  }

  private refreshDiagnosticsView(): void {
    for (const leaf of this.app.workspace.getLeavesOfType(
      DIAGNOSTICS_VIEW_TYPE,
    )) {
      const view = leaf.view;
      if (view instanceof DiagnosticsView) view.render();
    }
  }

  // rows exposes the flattened, sorted diagnostic rows for the view.
  rows(): ReturnType<typeof diagnosticRows> {
    return diagnosticRows(this.diagnosticsByUri);
  }

  // openSource is the diagnostics-view row click target: jump to a
  // file/line.
  openSource(uri: string, line: number): void {
    this.navigateTo(uri, line);
  }

  // saveSettings persists the new shape and reacts via the classifier:
  // restart for configPath, reconfigure for the runtime knobs.
  private async saveSettings(next: MdsmithSettings): Promise<void> {
    const before = this.cfg;
    this.cfg = next;
    await this.saveData(next);
    const reaction = classifyChange(before, next);
    if (reaction === "restart") {
      await this.restartRuntime();
    } else if (reaction === "reconfigure") {
      this.configureFixOnSave();
      await this.checkActiveFile();
    }
  }

  // asHost exposes the typed surface the settings tab needs without
  // handing it the rest of the plugin state. The getter reads cfg at
  // access time so the tab always sees the latest value.
  private asHost(): SettingsHost & { app: App; manifest: PluginManifest } {
    const self = this;
    return {
      app: this.app,
      manifest: this.manifest,
      get settings(): MdsmithSettings {
        return self.cfg;
      },
      saveSettings: (next: MdsmithSettings) => self.saveSettings(next),
    };
  }
}

// cmOf returns the CodeMirror 6 EditorView backing an Obsidian Editor,
// or undefined. Obsidian exposes it as the undocumented `cm` field; the
// typings omit it, so reach it structurally.
function cmOf(editor: Editor):
  | {
      state: { doc: { length: number; toString(): string } };
      dispatch(tr: unknown): void;
    }
  | undefined {
  return (
    editor as unknown as {
      cm?: {
        state: { doc: { length: number; toString(): string } };
        dispatch(tr: unknown): void;
      };
    }
  ).cm;
}

// DiagnosticsView is the "mdsmith Diagnostics" sidebar panel: a sortable
// table of every workspace diagnostic. Clicking a row jumps to source.
class DiagnosticsView extends ItemView {
  constructor(
    leaf: WorkspaceLeaf,
    private readonly plugin: MdsmithPlugin,
  ) {
    super(leaf);
  }

  getViewType(): string {
    return DIAGNOSTICS_VIEW_TYPE;
  }

  getDisplayText(): string {
    return "mdsmith Diagnostics";
  }

  getIcon(): string {
    return "list-checks";
  }

  async onOpen(): Promise<void> {
    this.render();
  }

  // render rebuilds the table from the plugin's current diagnostics.
  render(): void {
    const body = this.containerEl.children[1] as unknown as {
      empty(): void;
      createEl(
        tag: string,
        opts?: { text?: string; cls?: string },
      ): HTMLElement;
    };
    body.empty();
    body.createEl("h4", { text: "mdsmith Diagnostics" });
    const rows = this.plugin.rows();
    if (rows.length === 0) {
      body.createEl("p", { text: "No diagnostics." });
      return;
    }
    const table = body.createEl("table", { cls: "mdsmith-diagnostics-table" });
    for (const row of rows) {
      const tr = (table as unknown as {
        createEl(tag: string): {
          createEl(tag: string, opts?: { text?: string }): HTMLElement;
        };
      }).createEl("tr");
      tr.createEl("td", { text: `${row.uri}:${row.line}` });
      tr.createEl("td", { text: row.rule });
      tr.createEl("td", { text: row.message });
      (tr as unknown as { addEventListener(e: string, cb: () => void): void })
        .addEventListener?.("click", () =>
          this.plugin.openSource(row.uri, row.line),
        );
    }
  }
}
