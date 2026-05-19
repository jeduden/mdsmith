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

// ExecutableLaunchShape is the subset of vscode-languageclient's
// `Executable` we actually build in buildServerOptions. ServerOptions
// is a union type — TypeScript cannot index it with a string literal
// — so cast through this shape when reaching into opts.run/opts.debug
// from tests.
type ExecutableLaunchShape = {
  command: string;
  args: string[];
  transport: number;
  options?: { cwd?: string };
};
type RunDebug = { run: ExecutableLaunchShape; debug: ExecutableLaunchShape };

describe("buildServerOptions", () => {
  test("spawns the configured binary with the lsp subcommand on stdio", () => {
    const opts = buildServerOptions("/abs/path/mdsmith", TransportKindStdio) as RunDebug;
    // Both run + debug share the same launch shape so the same
    // server is used for normal launches and editor debug.
    for (const variant of ["run", "debug"] as const) {
      const exe = opts[variant];
      expect(exe).toBeDefined();
      expect(exe.command).toBe("/abs/path/mdsmith");
      expect(exe.args).toEqual(["lsp"]);
      expect(exe.transport).toBe(TransportKindStdio);
    }
  });

  test("preserves a bare binary name so $PATH resolves it", () => {
    const opts = buildServerOptions("mdsmith", TransportKindStdio) as RunDebug;
    expect(opts.run.command).toBe("mdsmith");
  });

  test("sets options.cwd on both run and debug when supplied", () => {
    const opts = buildServerOptions("mdsmith", TransportKindStdio, "/repo/root") as RunDebug;
    for (const variant of ["run", "debug"] as const) {
      expect(opts[variant].options?.cwd).toBe("/repo/root");
    }
  });

  test("omits options entirely when no cwd is supplied", () => {
    // Some clients reject an Executable.options that exists with all
    // undefined fields; passing nothing keeps the launch shape clean.
    const opts = buildServerOptions("mdsmith", TransportKindStdio) as RunDebug;
    expect(opts.run.options).toBeUndefined();
  });

  test("refuses an empty or whitespace command instead of spawning command:\"\"", () => {
    // Defense in depth for the reported crash: vscode-languageclient
    // rejects { command: "" } with the opaque "Unsupported server
    // configuration" error. resolveBinary already guarantees a
    // non-empty command, so no caller should reach here — but if one
    // does, fail loudly with an actionable message.
    for (const bad of ["", "   ", "\t\n"]) {
      expect(() => buildServerOptions(bad, TransportKindStdio)).toThrow(
        /mdsmith\.path/,
      );
    }
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
    // without us re-registering the `**/.mdsmith.yml` glob. The
    // structural FileSystemWatcherLike doesn't satisfy bun's
    // strictly-typed `toBe` overloads, so cast through `unknown` to
    // short-circuit the typecheck.
    expect(opts.synchronize?.fileEvents as unknown).toBe(watcher as unknown);
    expect(opts.outputChannelName).toBe("mdsmith");
    expect(opts.outputChannel).toBeUndefined();
  });

  test("forwards a shared OutputChannel and omits outputChannelName", () => {
    const watcher: FileSystemWatcherLike = {};
    const channel = {
      name: "mdsmith",
      append: () => {},
      appendLine: () => {},
      clear: () => {},
      show: () => {},
      hide: () => {},
      dispose: () => {},
    };
    const opts = buildClientOptions(watcher, channel);
    expect(opts.outputChannel as unknown).toBe(channel as unknown);
    expect(opts.outputChannelName).toBeUndefined();
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
