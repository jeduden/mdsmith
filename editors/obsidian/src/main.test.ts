// Drives main.ts's vault-event wiring: the on-save re-lint filter and
// the fix-on-save filter + wrong-buffer guard.
//
// Obsidian fires `vault.on("modify")` for ANY file in the vault, not
// just the active note. Both the on-save re-lint (registerActiveFileCheck)
// and fix-on-save (configureFixOnSave) must therefore filter to the
// active MarkdownView's file, and fix-on-save must additionally refuse
// to fix a buffer the user has switched away from inside the debounce
// window. These tests exercise the REAL wiring methods against a focused
// fake App — no WASM runtime, no Obsidian host — by spying on the
// check/fix entry points the handlers call.

import { describe, expect, mock, test } from "bun:test";

import MdsmithPlugin from "./main";
import type { MdsmithSettings } from "./settings";

// FakeFile is the TAbstractFile slice the modify handler reads.
interface FakeFile {
  path: string;
}

// FakeView stands in for a MarkdownView: it carries a file path and an
// editor. The editor is opaque to these tests — checkActiveFile and
// fixFile are spied on, so the editor is never dereferenced.
interface FakeView {
  file: { path: string } | null;
  editor: unknown;
}

// makeApp builds the minimal App surface main.ts's event wiring touches:
// a vault with a modify-event registry + offref, and a workspace whose
// getActiveViewOfType returns the currently-active fake view (settable
// per test to simulate switching notes). workspace.on is a no-op
// registry — these tests fire only vault modify.
function makeApp(): {
  app: {
    vault: {
      on(event: string, cb: (f: FakeFile) => unknown): unknown;
      offref(ref: unknown): void;
    };
    workspace: {
      on(event: string, cb: () => unknown): unknown;
      getActiveViewOfType(_t: unknown): FakeView | null;
      getLeavesOfType(_t: string): unknown[];
    };
  };
  active: { view: FakeView | null };
  fireModify(path: string): void;
  modifyListenerCount(): number;
} {
  type Handler = (f: FakeFile) => unknown;
  const vaultHandlers: Record<string, Handler[]> = {};
  const active: { view: FakeView | null } = { view: null };
  return {
    app: {
      vault: {
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
      },
      workspace: {
        on(_event: string, _cb: () => unknown): unknown {
          return {};
        },
        getActiveViewOfType(_t: unknown): FakeView | null {
          return active.view;
        },
        getLeavesOfType(_t: string): unknown[] {
          return [];
        },
      },
    },
    active,
    fireModify(path: string): void {
      for (const cb of vaultHandlers["modify"] ?? []) cb({ path });
    },
    modifyListenerCount(): number {
      return vaultHandlers["modify"]?.length ?? 0;
    },
  };
}

// makePlugin wires a fresh plugin onto a fake App and replaces its
// settings, check, and fix entry points with spies so the wiring can be
// observed without a runtime. registerEvent is a no-op (the fake App's
// own registry tracks listeners). Returns the plugin plus the spies and
// the App harness.
function makePlugin(cfg: Partial<MdsmithSettings> = {}): {
  plugin: MdsmithPlugin;
  harness: ReturnType<typeof makeApp>;
  checkSpy: ReturnType<typeof mock>;
  fixSpy: ReturnType<typeof mock>;
} {
  const harness = makeApp();
  // The base Plugin constructor takes (app, manifest); the test-setup
  // stub ignores both. Pass the fake App and a bare manifest to satisfy
  // the type — the real App is installed structurally below.
  const PluginCtor = MdsmithPlugin as unknown as new (
    app: unknown,
    manifest: unknown,
  ) => MdsmithPlugin;
  const plugin = new PluginCtor(harness.app, { dir: "plugin" });
  const settings: MdsmithSettings = {
    configPath: "",
    runMode: "onSave",
    fixOnSave: false,
    ...cfg,
  };
  // Reach into the instance to install the fake App, settings, and
  // spies. These fields/methods are private on the class; the test owns
  // the wiring under test, so it drives them structurally.
  const internals = plugin as unknown as {
    app: unknown;
    cfg: MdsmithSettings;
    registerEvent(ref: unknown): void;
    checkActiveFile(): Promise<void>;
    fixFile(editor: unknown, view: unknown): Promise<void>;
  };
  internals.app = harness.app;
  internals.cfg = settings;
  internals.registerEvent = () => {};
  const checkSpy = mock(() => Promise.resolve());
  const fixSpy = mock(() => Promise.resolve());
  internals.checkActiveFile = checkSpy;
  internals.fixFile = fixSpy;
  return { plugin, harness, checkSpy, fixSpy };
}

const sleep = (ms: number): Promise<void> =>
  new Promise((r) => setTimeout(r, ms));

// callPrivate invokes a private method by name on the plugin instance.
function callPrivate(plugin: MdsmithPlugin, name: string): void {
  (plugin as unknown as Record<string, () => void>)[name]();
}

describe("registerActiveFileCheck — on-save re-lint filtering (thread A)", () => {
  test("a modify of the active file re-lints when runMode is onSave", () => {
    const { plugin, harness, checkSpy } = makePlugin({ runMode: "onSave" });
    harness.active.view = { file: { path: "active.md" }, editor: {} };
    callPrivate(plugin, "registerActiveFileCheck");

    harness.fireModify("active.md");
    expect(checkSpy).toHaveBeenCalledTimes(1);
  });

  test("a modify of a DIFFERENT file does not re-lint the active buffer", () => {
    const { plugin, harness, checkSpy } = makePlugin({ runMode: "onSave" });
    harness.active.view = { file: { path: "active.md" }, editor: {} };
    callPrivate(plugin, "registerActiveFileCheck");

    harness.fireModify("other.md");
    expect(checkSpy).not.toHaveBeenCalled();
  });

  test("a modify of the active file is ignored when runMode is off", () => {
    const { plugin, harness, checkSpy } = makePlugin({ runMode: "off" });
    harness.active.view = { file: { path: "active.md" }, editor: {} };
    callPrivate(plugin, "registerActiveFileCheck");

    harness.fireModify("active.md");
    expect(checkSpy).not.toHaveBeenCalled();
  });
});

describe("configureFixOnSave — filter + wrong-buffer guard (thread B)", () => {
  test("a modify of the active file fixes that buffer after the debounce", async () => {
    const { plugin, harness, fixSpy } = makePlugin({
      runMode: "onSave",
      fixOnSave: true,
    });
    const view: FakeView = { file: { path: "active.md" }, editor: {} };
    harness.active.view = view;
    callPrivate(plugin, "configureFixOnSave");

    harness.fireModify("active.md");
    expect(fixSpy).not.toHaveBeenCalled(); // debounced
    await sleep(260);
    expect(fixSpy).toHaveBeenCalledTimes(1);
    expect(fixSpy.mock.calls[0]?.[1]).toBe(view);
  });

  test("a modify of an unrelated file never schedules a fix", async () => {
    const { plugin, harness, fixSpy } = makePlugin({
      runMode: "onSave",
      fixOnSave: true,
    });
    harness.active.view = { file: { path: "active.md" }, editor: {} };
    callPrivate(plugin, "configureFixOnSave");

    harness.fireModify("background.md");
    await sleep(260);
    expect(fixSpy).not.toHaveBeenCalled();
  });

  test("switching notes within the debounce window does not fix the wrong buffer", async () => {
    const { plugin, harness, fixSpy } = makePlugin({
      runMode: "onSave",
      fixOnSave: true,
    });
    // A save fires for note A while A is active; before the 200 ms
    // window elapses the user switches to note B. The fix must NOT apply
    // to B (nor to A — A is no longer active).
    harness.active.view = { file: { path: "A.md" }, editor: {} };
    callPrivate(plugin, "configureFixOnSave");
    harness.fireModify("A.md");

    harness.active.view = { file: { path: "B.md" }, editor: {} };
    await sleep(260);
    expect(fixSpy).not.toHaveBeenCalled();
  });

  test("fixOnSave off installs no modify listener", async () => {
    const { plugin, harness, fixSpy } = makePlugin({
      runMode: "onSave",
      fixOnSave: false,
    });
    harness.active.view = { file: { path: "active.md" }, editor: {} };
    callPrivate(plugin, "configureFixOnSave");

    harness.fireModify("active.md");
    await sleep(260);
    expect(fixSpy).not.toHaveBeenCalled();
  });

  test("the debounced fix-on-save is cancelable (teardown contract)", async () => {
    const { plugin, harness, fixSpy } = makePlugin({
      runMode: "onSave",
      fixOnSave: true,
    });
    harness.active.view = { file: { path: "active.md" }, editor: {} };
    callPrivate(plugin, "configureFixOnSave");
    harness.fireModify("active.md");

    // teardownRuntime cancels the pending trailing fix.
    (
      plugin as unknown as { debouncedFixOnSave?: { cancel(): void } }
    ).debouncedFixOnSave?.cancel();
    await sleep(260);
    expect(fixSpy).not.toHaveBeenCalled();
  });
});

describe("teardownRuntime — vault listener cleanup (Copilot review)", () => {
  test("unsubscribes the fix-on-save modify listener so a failed restart leaves none", () => {
    const { plugin, harness } = makePlugin({
      runMode: "onSave",
      fixOnSave: true,
    });
    harness.active.view = { file: { path: "active.md" }, editor: {} };
    callPrivate(plugin, "configureFixOnSave");
    expect(harness.modifyListenerCount()).toBe(1);

    // teardownRuntime must offref the fix-on-save subscription, not just
    // cancel the debounce: a restart whose startRuntime() fails early
    // never re-runs configureFixOnSave to clear it, so without the
    // offref a stale modify listener stays live for the rest of the
    // session.
    callPrivate(plugin, "teardownRuntime");
    expect(harness.modifyListenerCount()).toBe(0);
  });
});

describe("loadConfigYAML — config resolution + auto-discovery", () => {
  // makeConfigPlugin wires a plugin onto a fake App whose vault adapter
  // serves the given files through exists/read — the surface both the
  // explicit-path read and auto-discovery call. configPath seeds the
  // setting under test.
  function makeConfigPlugin(
    files: Record<string, string>,
    configPath = "",
  ): MdsmithPlugin {
    const adapter = {
      async exists(path: string): Promise<boolean> {
        return path in files;
      },
      async read(path: string): Promise<string> {
        const v = files[path];
        if (v === undefined) throw new Error(`ENOENT: ${path}`);
        return v;
      },
    };
    const app = { vault: { adapter } };
    const PluginCtor = MdsmithPlugin as unknown as new (
      a: unknown,
      m: unknown,
    ) => MdsmithPlugin;
    const plugin = new PluginCtor(app, { dir: "plugin" });
    const internals = plugin as unknown as {
      app: unknown;
      cfg: MdsmithSettings;
    };
    internals.app = app;
    internals.cfg = { configPath, runMode: "onSave", fixOnSave: false };
    return plugin;
  }

  const load = (plugin: MdsmithPlugin): Promise<string> =>
    (
      plugin as unknown as { loadConfigYAML(): Promise<string> }
    ).loadConfigYAML();

  test("auto-discovers the vault-root .mdsmith.yml when no Config path is set", async () => {
    const plugin = makeConfigPlugin({
      ".mdsmith.yml": "rules:\n  line-length: false\n",
    });
    expect(await load(plugin)).toBe("rules:\n  line-length: false\n");
  });

  test('falls back to "" when no path is set and the vault has no .mdsmith.yml', async () => {
    const plugin = makeConfigPlugin({ "note.md": "# Hi\n" });
    expect(await load(plugin)).toBe("");
  });

  test("an explicit Config path is read in preference to auto-discovery", async () => {
    const plugin = makeConfigPlugin(
      {
        ".mdsmith.yml": "rules:\n  line-length: false\n",
        "cfg/custom.yml": "rules:\n  no-bare-urls: false\n",
      },
      "cfg/custom.yml",
    );
    expect(await load(plugin)).toBe("rules:\n  no-bare-urls: false\n");
  });

  test('an unreadable explicit Config path degrades to "" (notice + defaults), not to discovery', async () => {
    // configPath names a file the adapter cannot read, so read() rejects.
    // The explicit-path branch surfaces a Notice (stubbed in tests) and
    // returns "" — it must NOT silently fall through to the vault-root
    // .mdsmith.yml, which would mask the user's broken path with an
    // unrelated config. The present .mdsmith.yml is the bait.
    const plugin = makeConfigPlugin(
      { ".mdsmith.yml": "rules:\n  line-length: false\n" },
      "cfg/missing.yml",
    );
    expect(await load(plugin)).toBe("");
  });
});

describe("engine-down / restart safety (Copilot review)", () => {
  test("teardownRuntime clears the active editor's diagnostics so none linger when the engine is down", () => {
    const { plugin, harness } = makePlugin({ runMode: "onSave" });
    const dispatched: Array<{ effects?: { value: unknown } }> = [];
    harness.active.view = {
      file: { path: "active.md" },
      editor: {
        cm: {
          dispatch: (tr: { effects?: { value: unknown } }) =>
            dispatched.push(tr),
        },
      },
    };
    callPrivate(plugin, "teardownRuntime");
    // Teardown pushes an empty diagnostics set into the editor so stale
    // underlines/tooltips do not survive a disposed engine.
    expect(dispatched[dispatched.length - 1]?.effects?.value).toEqual([]);
  });

  test("startRuntime reports failure (returns false) when the engine cannot load", async () => {
    // makePlugin's fake App has no vault adapter / getMarkdownFiles, so
    // the snapshot/load throws and startRuntime takes its catch branch.
    const { plugin } = makePlugin();
    const ok = await (
      plugin as unknown as { startRuntime(): Promise<boolean> }
    ).startRuntime();
    expect(ok).toBe(false);
  });
});
