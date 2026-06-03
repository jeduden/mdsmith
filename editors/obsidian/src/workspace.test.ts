// Drives the workspace snapshot + vault-change fan-out.
//
// src/workspace.ts has two jobs:
//   1. snapshotVault — read every Markdown file in the vault into the
//      flat Record<string,string> the runtime materializes into a
//      MemWorkspace at construction.
//   2. WorkspaceSync — subscribe to vault modify/create/delete events
//      and forward them to runtime.invalidate, debounced 200 ms per
//      file so a burst of saves collapses to one push.
//
// Both are exercised against a fake vault + a recording runtime — no
// Obsidian host needed.

import { describe, expect, test } from "bun:test";

import { snapshotVault, WorkspaceSync, type VaultLike } from "./workspace";

// FakeFile is the minimal TFile shape snapshotVault reads.
interface FakeFile {
  path: string;
  extension: string;
}

// makeVault builds a VaultLike over an in-memory file map. cachedRead
// returns the current bytes; the event registry lets a test fire
// modify/create/delete by hand.
function makeVault(files: Record<string, string>): VaultLike & {
  fire(event: "modify" | "create" | "delete", path: string): Promise<void>;
  set(path: string, content: string): void;
  listenerCount(event: "modify" | "create" | "delete"): number;
} {
  type Handler = (f: FakeFile) => unknown;
  const handlers: Record<string, Handler[]> = {};
  const store = { ...files };
  return {
    getMarkdownFiles(): FakeFile[] {
      return Object.keys(store).map((path) => ({ path, extension: "md" }));
    },
    async cachedRead(file: { path: string }): Promise<string> {
      return store[file.path] ?? "";
    },
    on(event: string, cb: Handler): { event: string; cb: Handler } {
      (handlers[event] ??= []).push(cb);
      return { event, cb };
    },
    // offref mirrors Obsidian's Events.offref: drop the exact handler the
    // matching on() returned, so stop() can unsubscribe what it started.
    offref(ref): void {
      const r = ref as { event: string; cb: Handler };
      const list = handlers[r.event];
      if (!list) return;
      const i = list.indexOf(r.cb);
      if (i >= 0) list.splice(i, 1);
    },
    async fire(event, path): Promise<void> {
      const file: FakeFile = { path, extension: "md" };
      for (const cb of handlers[event] ?? []) await cb(file);
    },
    set(path, content): void {
      store[path] = content;
    },
    listenerCount(event): number {
      return (handlers[event] ?? []).length;
    },
  };
}

// recordingRuntime captures every invalidate call so tests assert the
// fan-out without a live engine.
function recordingRuntime(): {
  calls: Array<{ uri: string; content?: string }>;
  invalidate(uri: string, content?: string): void;
} {
  const calls: Array<{ uri: string; content?: string }> = [];
  return {
    calls,
    invalidate(uri, content): void {
      calls.push({ uri, content });
    },
  };
}

const sleep = (ms: number): Promise<void> =>
  new Promise((r) => setTimeout(r, ms));

describe("snapshotVault", () => {
  test("reads every Markdown file into a flat path→text map", async () => {
    const vault = makeVault({
      "a.md": "# A\n",
      "dir/b.md": "# B\n",
    });
    const snap = await snapshotVault(vault);
    expect(snap).toEqual({ "a.md": "# A\n", "dir/b.md": "# B\n" });
  });

  test("returns an empty object for an empty vault", async () => {
    expect(await snapshotVault(makeVault({}))).toEqual({});
  });
});

describe("WorkspaceSync", () => {
  test("forwards modify with the new content after the debounce window", async () => {
    const vault = makeVault({ "a.md": "# A\n" });
    const rt = recordingRuntime();
    const sync = new WorkspaceSync(vault, rt, 50);
    sync.start();

    vault.set("a.md", "# A edited\n");
    await vault.fire("modify", "a.md");
    // Nothing fires synchronously — the push is debounced.
    expect(rt.calls.length).toBe(0);

    await sleep(80);
    expect(rt.calls).toEqual([{ uri: "a.md", content: "# A edited\n" }]);
    sync.stop();
  });

  test("forwards create with content and delete with no content", async () => {
    const vault = makeVault({});
    const rt = recordingRuntime();
    const sync = new WorkspaceSync(vault, rt, 50);
    sync.start();

    vault.set("new.md", "# New\n");
    await vault.fire("create", "new.md");
    await sleep(80);
    expect(rt.calls).toContainEqual({ uri: "new.md", content: "# New\n" });

    await vault.fire("delete", "new.md");
    await sleep(80);
    // delete forwards no content (drops the file from the workspace).
    expect(rt.calls).toContainEqual({ uri: "new.md", content: undefined });
    sync.stop();
  });

  test("collapses a burst of saves on one file to a single push", async () => {
    const vault = makeVault({ "a.md": "# A\n" });
    const rt = recordingRuntime();
    const sync = new WorkspaceSync(vault, rt, 50);
    sync.start();

    for (let i = 0; i < 5; i++) {
      vault.set("a.md", `# A v${i}\n`);
      await vault.fire("modify", "a.md");
    }
    await sleep(80);
    // Five rapid modifies → one trailing push with the latest bytes.
    expect(rt.calls.length).toBe(1);
    expect(rt.calls[0]).toEqual({ uri: "a.md", content: "# A v4\n" });
    sync.stop();
  });

  test("debounces per file — edits to two files each push once", async () => {
    const vault = makeVault({ "a.md": "# A\n", "b.md": "# B\n" });
    const rt = recordingRuntime();
    const sync = new WorkspaceSync(vault, rt, 50);
    sync.start();

    vault.set("a.md", "# A2\n");
    vault.set("b.md", "# B2\n");
    await vault.fire("modify", "a.md");
    await vault.fire("modify", "b.md");
    await sleep(80);

    expect(rt.calls.length).toBe(2);
    const byUri = Object.fromEntries(rt.calls.map((c) => [c.uri, c.content]));
    expect(byUri).toEqual({ "a.md": "# A2\n", "b.md": "# B2\n" });
    sync.stop();
  });

  test("stop() cancels a pending push", async () => {
    const vault = makeVault({ "a.md": "# A\n" });
    const rt = recordingRuntime();
    const sync = new WorkspaceSync(vault, rt, 50);
    sync.start();

    vault.set("a.md", "# A2\n");
    await vault.fire("modify", "a.md");
    sync.stop(); // before the window elapses
    await sleep(80);
    expect(rt.calls.length).toBe(0);
  });

  test("stop() unregisters the vault listeners so later events are ignored", async () => {
    const vault = makeVault({ "a.md": "# A\n" });
    const rt = recordingRuntime();
    const sync = new WorkspaceSync(vault, rt, 50);
    sync.start();
    sync.stop();

    // A change after stop() must never reach the (now disposed) runtime —
    // otherwise a session restart fires invalidate() on dead state.
    vault.set("a.md", "# A2\n");
    await vault.fire("modify", "a.md");
    await sleep(80);
    expect(rt.calls.length).toBe(0);
  });

  test("repeated start()/stop() cycles do not accumulate listeners", () => {
    const vault = makeVault({ "a.md": "# A\n" });
    const sync = new WorkspaceSync(vault, recordingRuntime(), 50);
    sync.start();
    sync.stop();
    sync.start();
    sync.stop();
    for (const event of ["modify", "create", "delete"] as const) {
      expect(vault.listenerCount(event)).toBe(0);
    }
  });
});
