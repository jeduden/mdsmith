import { describe, expect, mock, test } from "bun:test";

// `vscode-languageclient/node` does an unconditional `require("vscode")`
// at import time, but the `vscode` package is only available inside the
// VS Code host. Stub it out with the empty surface our wiring helpers
// touch (which is none). The mock has to land before we import the
// helpers under test or anything that transitively pulls in
// vscode-languageclient/node.
mock.module("vscode", () => ({}));

// TransportKind is a numeric enum in vscode-languageclient — pinning
// the wire value of stdio (0) keeps the test self-contained without
// importing the runtime package.
const TransportKindStdio = 0;

import {
  buildClientOptions,
  buildServerOptions,
  collectFixAllEdits,
  startupErrorMessage,
  type CodeActionLike,
  type FileSystemWatcherLike,
  type UriLike
} from "./wiring";

describe("buildServerOptions", () => {
  test("spawns the configured binary with the lsp subcommand on stdio", () => {
    const opts = buildServerOptions("/abs/path/mdsmith", TransportKindStdio);
    // Both run + debug share the same launch shape so the same
    // server is used for normal launches and editor debug.
    for (const variant of ["run", "debug"] as const) {
      const launch = opts[variant];
      expect(launch).toBeDefined();
      // ServerOptions's run/debug union has many shapes; cast to
      // the Executable variant we know we built.
      const exe = launch as { command: string; args: string[]; transport: number };
      expect(exe.command).toBe("/abs/path/mdsmith");
      expect(exe.args).toEqual(["lsp"]);
      expect(exe.transport).toBe(TransportKindStdio);
    }
  });

  test("preserves a bare binary name so $PATH resolves it", () => {
    const opts = buildServerOptions("mdsmith", TransportKindStdio);
    const exe = opts.run as { command: string };
    expect(exe.command).toBe("mdsmith");
  });
});

describe("buildClientOptions", () => {
  test("watches Markdown only and binds the supplied config watcher", () => {
    const watcher: FileSystemWatcherLike = {};
    const opts = buildClientOptions(watcher);
    expect(opts.documentSelector).toEqual([
      { scheme: "file", language: "markdown" }
    ]);
    // The same watcher object is forwarded so VS Code can reuse it
    // without us re-registering the `**/.mdsmith.yml` glob.
    expect(opts.synchronize?.fileEvents).toBe(watcher as unknown);
    expect(opts.outputChannelName).toBe("mdsmith");
  });
});

describe("startupErrorMessage", () => {
  test("includes the cause and the actionable settings hint", () => {
    const msg = startupErrorMessage(new Error("ENOENT: mdsmith"));
    expect(msg).toContain("Failed to start mdsmith Language Server");
    expect(msg).toContain("ENOENT: mdsmith");
    expect(msg).toContain("\"mdsmith.path\"");
    expect(msg).toContain("download mdsmith");
  });

  test("stringifies non-Error rejections", () => {
    const msg = startupErrorMessage("plain-string");
    expect(msg).toContain("plain-string");
  });
});

describe("collectFixAllEdits", () => {
  // Helpers: build a minimal Uri-like and code action so the
  // pipeline can filter without importing vscode.
  const uri = (value: string): UriLike => ({ toString: () => value });

  function action(
    kind: string | undefined,
    edits: readonly [UriLike, unknown[]][] | null
  ): CodeActionLike {
    return {
      kind: kind === undefined ? undefined : { value: kind },
      edit: edits === null
        ? undefined
        : {
            entries() {
              return edits as readonly [UriLike, never[]][];
            }
          }
    };
  }

  test("returns [] when the provider produced no actions", () => {
    expect(collectFixAllEdits(undefined, uri("file:///x.md"))).toEqual([]);
    expect(collectFixAllEdits(null, uri("file:///x.md"))).toEqual([]);
    expect(collectFixAllEdits([], uri("file:///x.md"))).toEqual([]);
  });

  test("keeps only source.fixAll.mdsmith actions targeting the document", () => {
    const target = uri("file:///x.md");
    const wantA = { tag: "wantA" };
    const wantB = { tag: "wantB" };
    const skip = { tag: "skip" };
    const actions: CodeActionLike[] = [
      // Wrong kind — must not contribute.
      action("source.fixAll.eslint", [[target, [skip]]]),
      // Right kind but a different file — must not contribute.
      action("source.fixAll.mdsmith", [[uri("file:///y.md"), [skip]]]),
      // Right kind, right file, two edits — both kept.
      action("source.fixAll.mdsmith", [[target, [wantA, wantB]]]),
      // Missing kind — must not contribute.
      action(undefined, [[target, [skip]]]),
      // Missing edit — must not contribute.
      action("source.fixAll.mdsmith", null)
    ];
    const edits = collectFixAllEdits(actions, target);
    expect(edits).toEqual([wantA, wantB]);
  });

  test("preserves edit order across multiple matching actions", () => {
    const target = uri("file:///x.md");
    const first = { id: 1 };
    const second = { id: 2 };
    const actions: CodeActionLike[] = [
      action("source.fixAll.mdsmith", [[target, [first]]]),
      action("source.fixAll.mdsmith", [[target, [second]]])
    ];
    expect(collectFixAllEdits(actions, target)).toEqual([first, second]);
  });
});
