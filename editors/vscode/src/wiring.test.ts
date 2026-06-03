import { describe, expect, mock, test } from "bun:test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

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
  decideClose,
  forwardMdsmithConfigChange,
  notifyConfigChangeToClient,
  startupErrorMessage,
  RUN_OFF,
  RUN_ON_SAVE,
  RUN_ON_TYPE,
  type FileSystemWatcherLike,
  type RestartPolicyState
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

  test("surfaces the configured mdsmith.path when supplied", () => {
    // The reported failure mode: a stale mdsmith.path pointing at a
    // dev-only file. Echo the exact configured value back to the user
    // so they can spot the mismatch without opening settings.
    const msg = startupErrorMessage(new Error("spawn /tmp/mdsmith-debug ENOENT"), {
      configuredPath: "/tmp/mdsmith-debug",
      resolvedCommand: "/tmp/mdsmith-debug",
      candidates: [],
    });
    expect(msg).toContain('"mdsmith.path": "/tmp/mdsmith-debug"');
  });

  test("labels an empty mdsmith.path as (unset) instead of empty quotes", () => {
    // Distinguish "no setting" from `mdsmith.path=""` so the user
    // does not think a stray whitespace value is what failed.
    const msg = startupErrorMessage(new Error("oops"), {
      configuredPath: "",
      resolvedCommand: "/ext/dist/cli/@mdsmith/linux-x64/bin/mdsmith",
      candidates: [],
    });
    expect(msg).toContain('"mdsmith.path": (unset, using bundled)');
  });

  test("lists discovered alternatives with kind labels", () => {
    const msg = startupErrorMessage(new Error("spawn /tmp/mdsmith-debug ENOENT"), {
      configuredPath: "/tmp/mdsmith-debug",
      resolvedCommand: "/tmp/mdsmith-debug",
      candidates: [
        { kind: "bundled", path: "/ext/dist/cli/@mdsmith/linux-x64/bin/mdsmith" },
        { kind: "path", path: "/usr/local/bin/mdsmith" },
      ],
    });
    expect(msg).toContain("Other mdsmith binaries found");
    expect(msg).toContain("bundled with the extension: /ext/dist/cli/@mdsmith/linux-x64/bin/mdsmith");
    expect(msg).toContain("on $PATH: /usr/local/bin/mdsmith");
  });

  test("offers the 'clear to use bundled' shortcut only when bundled is present", () => {
    // Dev build (no dist/cli/) or an unsupported platform produces a
    // candidate list of just PATH hits; suggesting "clear mdsmith.path
    // to use the bundled binary" in that case sends the user down a
    // dead end.
    const onlyPath = startupErrorMessage(new Error("spawn /tmp/mdsmith-debug ENOENT"), {
      configuredPath: "/tmp/mdsmith-debug",
      resolvedCommand: "/tmp/mdsmith-debug",
      candidates: [{ kind: "path", path: "/usr/local/bin/mdsmith" }],
    });
    expect(onlyPath).not.toContain("clear it");
    expect(onlyPath).toContain('Set "mdsmith.path" to one of these');

    const withBundled = startupErrorMessage(new Error("spawn /tmp/mdsmith-debug ENOENT"), {
      configuredPath: "/tmp/mdsmith-debug",
      resolvedCommand: "/tmp/mdsmith-debug",
      candidates: [
        { kind: "bundled", path: "/ext/dist/cli/@mdsmith/linux-x64/bin/mdsmith" },
      ],
    });
    expect(withBundled).toContain("clear it to use the bundled binary");
  });

  test("says no alternatives were found when the candidate list is empty", () => {
    // Otherwise the user is left wondering whether the resolver
    // looked at all — making the negative result explicit is what
    // tells them to install mdsmith or fix the path themselves.
    const msg = startupErrorMessage(new Error("spawn mdsmith ENOENT"), {
      configuredPath: "mdsmith",
      resolvedCommand: "mdsmith",
      candidates: [],
    });
    expect(msg).toContain("No other mdsmith binaries found on this system");
    // The "pick one of these" prompt doesn't apply when there are
    // no candidates; the trailing instructions must redirect the
    // user to install mdsmith instead.
    expect(msg).not.toContain("one of these");
    expect(msg).toContain("Install mdsmith");
  });

  test("does not duplicate the resolved command when it matches configured", () => {
    // configuredPath and resolvedCommand only diverge when the
    // configured value was empty/whitespace and the resolver
    // substituted the bundled path. Showing both lines when they are
    // equal just adds noise to an already busy error.
    const msg = startupErrorMessage(new Error("oops"), {
      configuredPath: "/tmp/mdsmith-debug",
      resolvedCommand: "/tmp/mdsmith-debug",
      candidates: [],
    });
    expect(msg).not.toContain("resolved command");
  });

  test("shows the resolved command when the resolver substituted it", () => {
    const msg = startupErrorMessage(new Error("oops"), {
      configuredPath: "",
      resolvedCommand: "/ext/dist/cli/@mdsmith/linux-x64/bin/mdsmith",
      candidates: [],
    });
    expect(msg).toContain(
      "resolved command: /ext/dist/cli/@mdsmith/linux-x64/bin/mdsmith",
    );
  });

  test("shows the resolved command when the resolver merely trimmed whitespace", () => {
    // resolveBinary trims surrounding whitespace from custom paths,
    // so a typo like "  /opt/mdsmith  " makes configured and resolved
    // diverge without falling back to the bundled binary. Surfacing
    // the trimmed form is what tells the user the literal value of
    // their setting is not what got spawned.
    const msg = startupErrorMessage(new Error("oops"), {
      configuredPath: "  /opt/mdsmith  ",
      resolvedCommand: "/opt/mdsmith",
      candidates: [],
    });
    expect(msg).toContain("resolved command: /opt/mdsmith");
  });
});

describe("forwardMdsmithConfigChange", () => {
  test("notifies the server when the change touches mdsmith.*", () => {
    let calls = 0;
    forwardMdsmithConfigChange(
      { affectsConfiguration: (section) => section === "mdsmith" },
      () => {
        calls++;
      }
    );
    expect(calls).toBe(1);
  });

  test("stays quiet when the change does not touch mdsmith.*", () => {
    // An unrelated settings edit (e.g. editor.fontSize) must not trigger
    // a config re-pull + full re-lint of every open buffer.
    let calls = 0;
    forwardMdsmithConfigChange(
      { affectsConfiguration: () => false },
      () => {
        calls++;
      }
    );
    expect(calls).toBe(0);
  });

  test("asks specifically about the mdsmith section", () => {
    const asked: string[] = [];
    forwardMdsmithConfigChange(
      {
        affectsConfiguration: (section) => {
          asked.push(section);
          return false;
        }
      },
      () => {}
    );
    expect(asked).toEqual(["mdsmith"]);
  });
});

describe("notifyConfigChangeToClient", () => {
  // A client double exposing just isRunning(), plus a send spy that
  // stands in for the caller-supplied notification callback. Tracking
  // sends on the spy (not the client) mirrors the real design, where the
  // payload lives in the callback rather than in RunningClientLike.
  function fixture(opts: { running: boolean; onSend?: () => Promise<void> }) {
    let sends = 0;
    const client = { isRunning: () => opts.running };
    const send = (_c: typeof client): Promise<void> => {
      sends++;
      return opts.onSend ? opts.onSend() : Promise.resolve();
    };
    return { sends: () => sends, client, send };
  }

  test("sends when the client is running", () => {
    const f = fixture({ running: true });
    notifyConfigChangeToClient(f.client, f.send);
    expect(f.sends()).toBe(1);
  });

  test("does not send when the client is undefined", () => {
    // No client yet (config edit racing activation): must be a no-op,
    // never a throw. The send callback must not run.
    const f = fixture({ running: true });
    expect(() => notifyConfigChangeToClient(undefined, f.send)).not.toThrow();
    expect(f.sends()).toBe(0);
  });

  test("does not send when the client is not running", () => {
    // vscode-languageclient@9 rejects sendNotification with
    // ConnectionInactive when the client is Stopping/Stopped/
    // StartFailed; calling it would surface an unhandled rejection.
    // The isRunning() guard must prevent the send entirely.
    const f = fixture({ running: false });
    notifyConfigChangeToClient(f.client, f.send);
    expect(f.sends()).toBe(0);
  });

  test("swallows a rejected send without throwing", async () => {
    // Even past the guard, the client can transition to not-running
    // between the isRunning() check and the send, so the callback
    // returns a rejected promise. notifyConfigChangeToClient must attach
    // a .catch() so it never becomes an unhandledRejection.
    const f = fixture({
      running: true,
      onSend: () => Promise.reject(new Error("Client is not running"))
    });
    expect(() => notifyConfigChangeToClient(f.client, f.send)).not.toThrow();
    // Let the microtask queue drain; if the .catch() inside were missing
    // Bun would surface an unhandledRejection and fail the test here.
    await new Promise((r) => setTimeout(r, 0));
    expect(f.sends()).toBe(1);
  });
});

describe("run-mode constants", () => {
  test("match the mdsmith.run enum declared in package.json", () => {
    // wiring.ts claims to mirror package.json's mdsmith.run enum; pin
    // that here so the constants and the contributed setting cannot
    // drift apart — a drift would make the server and client compare
    // against a run value the other never sends.
    const pkg = JSON.parse(
      readFileSync(resolve(__dirname, "../package.json"), "utf-8")
    ) as {
      contributes: {
        configuration: { properties: Record<string, { enum?: string[] }> };
      };
    };
    const runEnum =
      pkg.contributes.configuration.properties["mdsmith.run"].enum;
    expect(runEnum).toEqual([RUN_ON_TYPE, RUN_ON_SAVE, RUN_OFF]);
  });
});

describe("decideClose", () => {
  const maxRestarts = 25;
  const windowMs = 3 * 60 * 1000;

  function fresh(): RestartPolicyState {
    return { restarts: [], superseded: false };
  }

  test("restarts a normal close and records the timestamp", () => {
    const state = fresh();
    // The first close is under the cap, so it must restart.
    const decision = decideClose(state, 1000, maxRestarts, windowMs);
    expect(decision.restart).toBe(true);
    expect(decision.capExceeded).toBe(false);
    expect(state.restarts).toEqual([1000]);
  });

  test("does not restart and does not count once superseded", () => {
    const state: RestartPolicyState = { restarts: [10, 20, 30], superseded: true };
    const decision = decideClose(state, 9_999, maxRestarts, windowMs);
    expect(decision.restart).toBe(false);
    expect(decision.capExceeded).toBe(false);
    // The superseded short-circuit must not touch the restart history.
    expect(state.restarts).toEqual([10, 20, 30]);
  });

  test("stops restarting and flags the cap once more than maxRestarts close in the window", () => {
    const state = fresh();
    let last = decideClose(state, 0, maxRestarts, windowMs);
    // 25 closes all restart; the 26th trips the cap.
    for (let i = 1; i <= maxRestarts; i++) {
      last = decideClose(state, i, maxRestarts, windowMs);
    }
    expect(last.restart).toBe(false);
    expect(last.capExceeded).toBe(true);
  });

  test("prunes close timestamps older than the window so a slow trickle keeps restarting", () => {
    const state = fresh();
    // Seed maxRestarts closes far in the past.
    for (let i = 0; i < maxRestarts; i++) {
      decideClose(state, i, maxRestarts, windowMs);
    }
    // A close well past the window prunes the stale entries, so it is
    // treated as the first recent close and restarts.
    const decision = decideClose(state, windowMs + 1_000, maxRestarts, windowMs);
    expect(decision.restart).toBe(true);
    expect(decision.capExceeded).toBe(false);
    expect(state.restarts).toEqual([windowMs + 1_000]);
  });
});
