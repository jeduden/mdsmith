// End-to-end verification of the in-vault runtime behavior behind plan
// 217 acceptance criteria 3 and 6.
//
// These criteria were previously only proxy-verified (unit tests +
// benchmark), which the PR draft flagged as not backed by a real run.
// This file closes that gap: it boots the REAL MdsmithPlugin via
// onload() against a focused fake App, drives the engine compiled to
// GOOS=js GOARCH=wasm (the same artifact the host ships), and exercises
// the real wiring in main.ts + diagnostics.ts + actions.ts end to end.
//
//   Criterion 3 — opening a file with an MDS001 violation shows a wavy
//   underline: file-open → plugin check → the CM6 editor receives a
//   setDiagnostics effect carrying the engine's diagnostics, and
//   decorationSetFor / buildDecorations yield a non-empty underline span
//   on the violation line (the underline substance).
//
//   Criterion 6 — fixOnSave runs Fix file after a save without a plugin
//   restart: a vault modify of the active file (a fixable MDS009
//   trailing-space violation) replaces the buffer with the engine's
//   fixed source after the debounce, with no restart. The A/B
//   corrections ride along: a modify of an unrelated file does NOT fix,
//   and switching notes inside the debounce window does NOT fix the
//   wrong buffer.
//
// Go IS required: the test builds the real wasm and skips only when the
// toolchain is absent (mirrors wasm-runtime.test.ts).

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import {
  existsSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { CONFIG_FILE_NAME } from "./config";
import { buildDecorations, decorationSetFor } from "./diagnostics";
import MdsmithPlugin from "./main";
import type { Diagnostic } from "./wasm-runtime";
import { __resetEngineForTests } from "./wasm-runtime";

function hasGo(): boolean {
  return !Bun.spawnSync(["go", "version"]).exitCode;
}

// wasmExecPath resolves the active toolchain's wasm_exec.js (moved to
// lib/wasm/ in recent Go releases), matching wasm-runtime.test.ts.
function wasmExecPath(): string {
  const out = Bun.spawnSync(["go", "env", "GOROOT"]);
  return join(out.stdout.toString().trim(), "lib", "wasm", "wasm_exec.js");
}

// buildWasm compiles cmd/mdsmith-wasm for GOOS=js GOARCH=wasm into dir.
function buildWasm(dir: string): string {
  const out = join(dir, "mdsmith.wasm");
  const res = Bun.spawnSync(["go", "build", "-o", out, "."], {
    cwd: join(import.meta.dir, "..", "..", "..", "cmd", "mdsmith-wasm"),
    env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
  });
  if (res.exitCode !== 0) {
    throw new Error(`go build wasm failed: ${res.stderr.toString()}`);
  }
  return out;
}

const skip = !hasGo();
let pluginDir = "";

beforeAll(() => {
  if (skip) return;
  // Lay out a plugin directory exactly as the host sees it: the WASM
  // artifact and wasm_exec.js side by side, read through the vault
  // adapter (makeAssetLoaders reads manifest.dir/{mdsmith.wasm,
  // wasm_exec.js}).
  pluginDir = mkdtempSync(join(tmpdir(), "mds-obsidian-e2e-"));
  buildWasm(pluginDir);
  const execSrc = readFileSync(wasmExecPath(), "utf8");
  writeFileSync(join(pluginDir, "wasm_exec.js"), execSrc);
}, 120_000); // a cold GOOS=js GOARCH=wasm build takes ~30 s.

afterAll(() => {
  if (pluginDir) rmSync(pluginDir, { recursive: true, force: true });
});

// FakeDoc is a CM6 Text stand-in backed by a mutable string. It is real
// enough that buildDecorations computes correct offsets: line(n) returns
// true start/end offsets derived from the buffer, and a whole-buffer
// replace change rewrites the string. dispatch records setDiagnostics
// effects so the test can read what the plugin pushed.
class FakeDoc {
  constructor(public text: string) {}

  get length(): number {
    return this.text.length;
  }

  toString(): string {
    return this.text;
  }

  // CM6 numbers lines from 1. line(n) returns the n-th line's start/end
  // document offsets (end excludes the trailing newline), matching the
  // surface DocLike reads.
  line(n: number): { from: number; to: number } {
    const lines = this.text.split("\n");
    if (n < 1 || n > lines.length) {
      throw new RangeError(`line ${n} out of range`);
    }
    let from = 0;
    for (let i = 0; i < n - 1; i++) from += lines[i]!.length + 1;
    return { from, to: from + lines[n - 1]!.length };
  }

  get lines(): number {
    return this.text.split("\n").length;
  }
}

// FakeEditorView is the CM6 EditorView slice cmOf() and editorAdapter()
// drive. dispatch handles two transaction shapes: { effects } records
// the pushed diagnostics; { changes: {from,to,insert} } applies a
// whole-buffer replace to the backing doc (the fix path).
class FakeEditorView {
  effects: Diagnostic[][] = [];
  state: { doc: FakeDoc };

  constructor(text: string) {
    this.state = { doc: new FakeDoc(text) };
  }

  dispatch(tr: {
    effects?: { value: Diagnostic[] };
    changes?: { from: number; to: number; insert: string };
  }): void {
    if (tr.effects) this.effects.push(tr.effects.value);
    if (tr.changes) {
      const { from, to, insert } = tr.changes;
      const doc = this.state.doc.text;
      this.state.doc.text = doc.slice(0, from) + insert + doc.slice(to);
    }
  }

  get lastDiagnostics(): Diagnostic[] | undefined {
    return this.effects[this.effects.length - 1];
  }
}

// FakeEditor is Obsidian's Editor slice the plugin calls: getValue() for
// the check source, and the undocumented `cm` field cmOf() reaches.
class FakeEditor {
  cm: FakeEditorView;

  constructor(text: string) {
    this.cm = new FakeEditorView(text);
  }

  getValue(): string {
    return this.cm.state.doc.text;
  }
}

// FakeMarkdownView carries a file path and an editor — the surface
// getActiveViewOfType(MarkdownView) returns and main.ts dereferences.
interface FakeMarkdownView {
  file: { path: string } | null;
  editor: FakeEditor;
}

// Harness is the focused fake App plus the controls the test drives.
interface Harness {
  plugin: MdsmithPlugin;
  setActive(view: FakeMarkdownView | null): void;
  fireModify(path: string): void;
  newView(path: string, text: string): FakeMarkdownView;
}

// bootPlugin builds the fake App, wires the real plugin onto it, and
// runs onload() — instantiating the real engine from pluginDir. The
// vault adapter serves the temp-dir WASM assets; the vault has a modify
// event registry; workspace.getActiveViewOfType returns the currently
// active fake view.
async function bootPlugin(
  files: Record<string, string>,
  cfg: { runMode?: string; fixOnSave?: boolean; configYAML?: string } = {},
): Promise<Harness> {
  type Handler = (f: { path: string }) => unknown;
  const vaultHandlers: Record<string, Handler[]> = {};
  const active: { view: FakeMarkdownView | null } = { view: null };
  const store = { ...files };

  const app = {
    vault: {
      adapter: {
        // The plugin reads its bundled assets through the adapter; serve
        // them from the temp plugin dir. Paths arrive as
        // `${manifest.dir}/wasm_exec.js` etc. The vault-root .mdsmith.yml
        // auto-discovery reads through the same adapter, so serve the
        // test's config (when given) for that vault-relative name.
        async read(p: string): Promise<string> {
          if (p === CONFIG_FILE_NAME) {
            if (cfg.configYAML === undefined) throw new Error(`ENOENT: ${p}`);
            return cfg.configYAML;
          }
          return readFileSync(p, "utf8");
        },
        async readBinary(p: string): Promise<ArrayBuffer> {
          const buf = readFileSync(p);
          return buf.buffer.slice(
            buf.byteOffset,
            buf.byteOffset + buf.byteLength,
          ) as ArrayBuffer;
        },
        async exists(p: string): Promise<boolean> {
          if (p === CONFIG_FILE_NAME) return cfg.configYAML !== undefined;
          return existsSync(p);
        },
      },
      getMarkdownFiles(): Array<{ path: string; extension: string }> {
        return Object.keys(store).map((path) => ({ path, extension: "md" }));
      },
      async cachedRead(file: { path: string }): Promise<string> {
        return store[file.path] ?? "";
      },
      on(event: string, cb: Handler): { event: string; cb: Handler } {
        (vaultHandlers[event] ??= []).push(cb);
        return { event, cb };
      },
      offref(ref: unknown): void {
        const r = ref as { event: string; cb: Handler };
        const list = vaultHandlers[r.event];
        if (!list) return;
        const i = list.indexOf(r.cb);
        if (i >= 0) list.splice(i, 1);
      },
      getFileByPath(_p: string): null {
        return null;
      },
    },
    workspace: {
      on(_event: string, _cb: () => unknown): unknown {
        return {};
      },
      getActiveViewOfType(_t: unknown): FakeMarkdownView | null {
        return active.view;
      },
      getLeavesOfType(_t: string): unknown[] {
        return [];
      },
      getRightLeaf(_split: boolean): null {
        return null;
      },
      revealLeaf(_leaf: unknown): void {},
    },
  };

  const PluginCtor = MdsmithPlugin as unknown as new (
    appArg: unknown,
    manifest: unknown,
  ) => MdsmithPlugin;
  const plugin = new PluginCtor(app, { dir: pluginDir });
  const internals = plugin as unknown as {
    app: unknown;
    manifest: unknown;
    registerEvent(ref: unknown): void;
    loadData(): Promise<unknown>;
    saveData(d: unknown): Promise<void>;
  };
  internals.app = app;
  internals.manifest = { dir: pluginDir };
  internals.registerEvent = () => {};
  // onload reads settings via loadData; feed the test's run mode /
  // fixOnSave so configureFixOnSave wires the right listener.
  internals.loadData = async () => ({
    configPath: "",
    runMode: cfg.runMode ?? "onSave",
    fixOnSave: cfg.fixOnSave ?? false,
  });
  internals.saveData = async () => {};

  await plugin.onload();

  return {
    plugin,
    setActive(view): void {
      active.view = view;
    },
    fireModify(path): void {
      for (const cb of vaultHandlers["modify"] ?? []) cb({ path });
    },
    newView(path, text): FakeMarkdownView {
      store[path] = text;
      return { file: { path }, editor: new FakeEditor(text) };
    },
  };
}

const sleep = (ms: number): Promise<void> =>
  new Promise((r) => setTimeout(r, ms));

const LONG_LINE =
  "This line is deliberately made to exceed the eighty character limit by adding extra words here now.";

describe.skipIf(skip)("plugin e2e (criteria 3 & 6)", () => {
  test("criterion 3: opening a file with an MDS001 violation pushes diagnostics and yields a wavy-underline span", async () => {
    // The engine memoizes; reset so this test's plugin loads the engine
    // cleanly and other suites cannot leak a stale one in.
    __resetEngineForTests();
    const path = "note.md";
    const source = `# Title\n\n${LONG_LINE}\n`;
    const harness = await bootPlugin({ [path]: source });
    const view = harness.newView(path, source);
    harness.setActive(view);

    // Drive the real check path the way active-leaf-change / file-open
    // does: checkActiveFile() lints the active buffer and pushes the
    // result into its CM6 editor via setDiagnostics.
    await (
      harness.plugin as unknown as { checkActiveFile(): Promise<void> }
    ).checkActiveFile();

    // (a) The CM6 editor received a setDiagnostics effect carrying the
    // engine's diagnostics, including the MDS001 the long line trips.
    const pushed = view.editor.cm.lastDiagnostics;
    expect(pushed).toBeDefined();
    expect((pushed ?? []).map((d) => d.rule)).toContain("MDS001");

    // (b) The underline substance: decorationSetFor / buildDecorations
    // yield a non-empty span on the violation line. The MDS001
    // diagnostic points at line 3 (1-based) — the long line.
    const mds001 = (pushed ?? []).find((d) => d.rule === "MDS001")!;
    const doc = view.editor.cm.state.doc;
    const specs = buildDecorations(doc, pushed ?? []);
    expect(specs.length).toBeGreaterThan(0);
    const lineRange = doc.line(mds001.line);
    const onViolationLine = specs.some(
      (s) => s.from >= lineRange.from && s.from <= lineRange.to,
    );
    expect(onViolationLine).toBe(true);
    // A non-degenerate (>= 1 char) underline, and decorationSetFor
    // produces a real DecorationSet (not the empty sentinel).
    expect(specs[0]!.to).toBeGreaterThan(specs[0]!.from);
    expect(decorationSetFor(doc, pushed ?? [])).not.toBe(
      decorationSetFor(doc, []),
    );
  }, 120_000);

  test("auto-discovers a vault-root .mdsmith.yml and compiles it into the session (no Config path set)", async () => {
    // Criterion 3 proves the same LONG_LINE trips MDS001 under the
    // defaults. Here a vault-root .mdsmith.yml turns line-length off;
    // with discovery wired, the engine compiles that config and the
    // long line no longer reports MDS001 — proving the config was found
    // AND applied end to end, not merely read.
    __resetEngineForTests();
    const path = "note.md";
    const source = `# Title\n\n${LONG_LINE}\n`;
    const harness = await bootPlugin(
      { [path]: source },
      { configYAML: "rules:\n  line-length: false\n" },
    );
    const view = harness.newView(path, source);
    harness.setActive(view);

    await (
      harness.plugin as unknown as { checkActiveFile(): Promise<void> }
    ).checkActiveFile();

    const pushed = view.editor.cm.lastDiagnostics ?? [];
    expect(pushed.map((d) => d.rule)).not.toContain("MDS001");
  }, 120_000);

  test("criterion 6: a save fixes the active buffer in place with no restart", async () => {
    __resetEngineForTests();
    const path = "trail.md";
    // MDS009 trailing spaces — fixable. The buffer has a violation.
    const source = "# Title\n\nText with trailing.   \n";
    const harness = await bootPlugin(
      { [path]: source },
      { runMode: "onSave", fixOnSave: true },
    );
    const view = harness.newView(path, source);
    harness.setActive(view);

    // A save of the active file fires vault modify. After the 200 ms
    // debounce the buffer is replaced with the engine's fixed source —
    // the plugin instance is the same one onload() built (no restart).
    harness.fireModify(path);
    expect(view.editor.getValue()).toBe(source); // debounced, not yet
    await sleep(320);

    const fixed = view.editor.getValue();
    expect(fixed).not.toBe(source);
    expect(fixed).not.toContain("trailing.   ");
    expect(fixed).toContain("Text with trailing.");
  }, 120_000);

  test("criterion 6 (A/B guards): an unrelated save does not fix, and switching notes mid-debounce does not fix the wrong buffer", async () => {
    __resetEngineForTests();
    const aPath = "A.md";
    const bPath = "B.md";
    const aSource = "# A\n\nText with trailing.   \n";
    const bSource = "# B\n\nClean B body here.   \n";
    const harness = await bootPlugin(
      { [aPath]: aSource, [bPath]: bSource },
      { runMode: "onSave", fixOnSave: true },
    );
    const aView = harness.newView(aPath, aSource);
    const bView = harness.newView(bPath, bSource);

    // 1. An unrelated-file save never schedules a fix for the active
    // buffer.
    harness.setActive(aView);
    harness.fireModify("background.md");
    await sleep(320);
    expect(aView.editor.getValue()).toBe(aSource);

    // 2. Switching notes inside the debounce window: A saves while
    // active, then the user switches to B before the window elapses. The
    // fix must apply to NEITHER buffer (A is no longer active; B was not
    // the saved file).
    harness.fireModify(aPath);
    harness.setActive(bView);
    await sleep(320);
    expect(aView.editor.getValue()).toBe(aSource);
    expect(bView.editor.getValue()).toBe(bSource);
  }, 120_000);
});
