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
  Wiring,
  RUN_OFF,
  RUN_ON_SAVE,
  RUN_ON_TYPE,
  type ClientLike,
  type FileSystemWatcherLike,
  type RestartPolicyState,
  type VscodeApi
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

// --- Wiring orchestrator -------------------------------------------------
//
// The Wiring class is the extension's composition root: it owns the
// LanguageClient lifecycle, the .mdsmith.yml watcher, and all command
// registrations. It reaches the editor only through an injected
// `VscodeApi` (the live `vscode` namespace in production) and a
// `createClient` factory, so the orchestration is driven here with fakes
// instead of a real VS Code host. extension.ts supplies the real
// `vscode` + LanguageClient and nothing else.

// FakeDisposable counts dispose() calls so watcher / subscription
// lifecycle assertions can confirm cleanup happened exactly once.
class FakeDisposable {
  disposed = 0;
  dispose() {
    this.disposed++;
  }
}

// FakeClient stands in for a vscode-languageclient LanguageClient. It
// records start/stop/sendNotification calls, lets a test force start()
// to reject, and captures the mdsmith/superseded notification handler
// so the supersede path can be exercised without a live server.
class FakeClient {
  started = 0;
  stopped = 0;
  notifications = 0;
  running = false;
  startRejection: Error | undefined;
  supersededHandler: (() => void) | undefined;
  constructor(public readonly clientOptions: ClientOptionsCapture) {}
  onNotification(method: string, handler: () => void): { dispose(): void } {
    if (method === "mdsmith/superseded") this.supersededHandler = handler;
    return { dispose() {} };
  }
  async start(): Promise<void> {
    this.started++;
    if (this.startRejection) {
      throw this.startRejection;
    }
    this.running = true;
  }
  async stop(): Promise<void> {
    this.stopped++;
    this.running = false;
  }
  isRunning(): boolean {
    return this.running;
  }
  async sendNotification(_type: unknown, _params?: unknown): Promise<void> {
    this.notifications++;
  }
}

// ClientOptionsCapture is the subset of LanguageClientOptions the Wiring
// tests inspect after construction (the error handler and the hover
// middleware). Typed loosely on purpose — the production type comes from
// vscode-languageclient and is not imported at runtime here.
interface ClientOptionsCapture {
  errorHandler?: { closed(): { action: number }; markSuperseded?(): void };
  middleware?: { provideHover?: unknown };
}

// makeFakeApi builds a minimal stand-in for the `vscode` namespace
// covering every member the Wiring class touches. Spies record what was
// registered / shown so tests can assert on the wiring without a host.
function makeFakeApi(overrides?: {
  configuration?: Record<string, unknown>;
  workspaceFolders?: Array<{ uri: { fsPath: string } }>;
  errorMessageChoice?: string;
  isTrusted?: boolean;
}) {
  const registeredCommands: string[] = [];
  const registeredSchemes: string[] = [];
  const contentProviders: Record<string, { provideTextDocumentContent: (uri: unknown) => Promise<string> }> = {};
  const createdWatchers: Array<{ glob: string; disposable: FakeDisposable }> = [];
  const createdChannels: FakeDisposable[] = [];
  const errorMessages: string[] = [];
  let configChangeListener: ((e: { affectsConfiguration(s: string): boolean }) => void) | undefined;

  const cfg = overrides?.configuration ?? {};
  const api = {
    commands: {
      registerCommand: (id: string) => {
        registeredCommands.push(id);
        return new FakeDisposable();
      },
      executeCommand: async () => undefined,
    },
    workspace: {
      workspaceFolders: overrides?.workspaceFolders,
      isTrusted: overrides?.isTrusted ?? true,
      getConfiguration: () => ({
        get: (key: string, dflt?: unknown) => (key in cfg ? cfg[key] : dflt),
      }),
      getWorkspaceFolder: () => undefined,
      createFileSystemWatcher: (glob: string) => {
        const disposable = new FakeDisposable();
        createdWatchers.push({ glob, disposable });
        return disposable;
      },
      onDidChangeConfiguration: (listener: (e: { affectsConfiguration(s: string): boolean }) => void) => {
        configChangeListener = listener;
        return new FakeDisposable();
      },
      registerTextDocumentContentProvider: (scheme: string, provider: { provideTextDocumentContent: (uri: unknown) => Promise<string> }) => {
        registeredSchemes.push(scheme);
        contentProviders[scheme] = provider;
        return new FakeDisposable();
      },
      openTextDocument: async () => ({}),
    },
    window: {
      activeTextEditor: undefined,
      createOutputChannel: () => {
        const channel = Object.assign(new FakeDisposable(), {
          name: "mdsmith",
          append() {},
          appendLine() {},
          clear() {},
          show() {},
          hide() {},
        });
        createdChannels.push(channel);
        return channel;
      },
      showErrorMessage: async (msg: string) => {
        errorMessages.push(msg);
        return overrides?.errorMessageChoice;
      },
      showWarningMessage: async () => undefined,
      showInformationMessage: async () => undefined,
      showInputBox: async () => undefined,
      showQuickPick: async () => undefined,
      showTextDocument: async () => ({}),
    },
    languages: {
      getDiagnostics: () => [],
      setTextDocumentLanguage: async (doc: unknown) => doc,
    },
    env: { openExternal: async () => true },
    Uri: {
      file: (p: string) => ({ fsPath: p, scheme: "file", toString: () => p }),
      parse: (s: string) => ({ toString: () => s }),
    },
    ViewColumn: { Beside: 2 },
    MarkdownString: class {},
  };
  return {
    api: api as unknown as VscodeApi,
    registeredCommands,
    registeredSchemes,
    contentProviders,
    createdWatchers,
    createdChannels,
    errorMessages,
    fireConfigChange: (affects: boolean) =>
      configChangeListener?.({ affectsConfiguration: () => affects }),
  };
}

// makeContext builds a fake ExtensionContext exposing only the
// subscriptions array and extensionPath the Wiring class uses.
function makeContext(): { subscriptions: Array<{ dispose(): void }>; extensionPath: string } {
  return { subscriptions: [], extensionPath: "/ext" };
}

// makeWiring constructs a Wiring with a fake api and a createClient
// factory that hands back FakeClient instances, returning the latest
// one for inspection. resolveBinary is stubbed so no real binary lookup
// runs.
function makeWiring(opts?: Parameters<typeof makeFakeApi>[0] & { startRejection?: Error }) {
  const fake = makeFakeApi(opts);
  const clients: FakeClient[] = [];
  const wiring = new Wiring({
    api: fake.api,
    createClient: (_id, _name, _server, clientOptions) => {
      const client = new FakeClient(clientOptions as ClientOptionsCapture);
      if (opts?.startRejection) client.startRejection = opts.startRejection;
      clients.push(client);
      return client as unknown as ClientLike;
    },
    resolveBinary: () => "/abs/mdsmith",
    findBinaryCandidates: () => [],
  });
  return { wiring, fake, clients, lastClient: () => clients[clients.length - 1] };
}

describe("Wiring.registerPaletteCommands", () => {
  test("registers every contributed mdsmith command id exactly once", async () => {
    // The command IDs are the extension's public surface (package.json
    // contributes.commands plus the two programmatic commands). Pin the
    // full set so a dropped or renamed registration fails loudly.
    const { wiring, fake } = makeWiring();
    await wiring.activate(makeContext());
    const expected = [
      "mdsmith.restartServer",
      "mdsmith.showOutput",
      "mdsmith.init",
      "mdsmith.mergeDriver.install",
      "mdsmith.fixWorkspace",
      "mdsmith.kinds.resolve",
      "mdsmith.kinds.why",
      "mdsmith.openRuleDoc",
    ];
    for (const id of expected) {
      expect(fake.registeredCommands).toContain(id);
    }
    // Each id is registered exactly once — no duplicate handlers.
    for (const id of expected) {
      expect(fake.registeredCommands.filter((c) => c === id)).toHaveLength(1);
    }
  });

  test("registers the kinds and rule virtual-document schemes", async () => {
    const { wiring, fake } = makeWiring();
    await wiring.activate(makeContext());
    expect(fake.registeredSchemes).toContain("mdsmith-kinds");
    expect(fake.registeredSchemes).toContain("mdsmith-rule");
  });

  test("mdsmith-kinds provider returns empty string in untrusted workspace", async () => {
    const { wiring, fake } = makeWiring({ isTrusted: false });
    await wiring.activate(makeContext());
    const provider = fake.contentProviders["mdsmith-kinds"];
    const fakeUri = { toString: () => "mdsmith-kinds://resolve?file=%2Frepo%2Fa.md" };
    const result = await provider.provideTextDocumentContent(fakeUri);
    expect(result).toBe("");
  });

  test("mdsmith-rule provider returns empty string in untrusted workspace", async () => {
    const { wiring, fake } = makeWiring({ isTrusted: false });
    await wiring.activate(makeContext());
    const provider = fake.contentProviders["mdsmith-rule"];
    const fakeUri = { toString: (_skip?: boolean) => "mdsmith-rule://doc?id=MDS001" };
    const result = await provider.provideTextDocumentContent(fakeUri);
    expect(result).toBe("");
  });
});

describe("Wiring config watcher", () => {
  test("creates the .mdsmith.yml watcher on activate", async () => {
    const { wiring, fake } = makeWiring();
    await wiring.activate(makeContext());
    expect(fake.createdWatchers).toHaveLength(1);
    expect(fake.createdWatchers[0].glob).toBe("**/.mdsmith.yml");
  });

  test("disposes the previous watcher before creating a new one on restart", async () => {
    // Each server start installs a fresh watcher; the prior one must be
    // disposed first or VS Code accumulates watchers and double-fires
    // change events. Restart goes through the same startServer path.
    const { wiring, fake } = makeWiring();
    const ctx = makeContext();
    await wiring.activate(ctx);
    await wiring.restartServer(ctx);
    expect(fake.createdWatchers).toHaveLength(2);
    // The first watcher is disposed; the second stays live.
    expect(fake.createdWatchers[0].disposable.disposed).toBe(1);
    expect(fake.createdWatchers[1].disposable.disposed).toBe(0);
  });

  test("disposes the watcher on deactivate", async () => {
    const { wiring, fake } = makeWiring();
    const ctx = makeContext();
    await wiring.activate(ctx);
    await wiring.deactivate();
    expect(fake.createdWatchers[0].disposable.disposed).toBe(1);
  });
});

describe("Wiring LSP client lifecycle", () => {
  test("constructs a client and starts it on activate", async () => {
    const { wiring, lastClient } = makeWiring();
    await wiring.activate(makeContext());
    expect(lastClient()).toBeDefined();
    expect(lastClient().started).toBe(1);
    expect(lastClient().isRunning()).toBe(true);
  });

  test("installs the MdsmithErrorHandler and the hover middleware", async () => {
    // The client must run with our custom restart policy (so a rebuild
    // loop doesn't permanently disable the server) and the hover-link
    // rewrite middleware (so rule-doc links open offline).
    const { wiring, lastClient } = makeWiring();
    await wiring.activate(makeContext());
    const opts = lastClient().clientOptions;
    expect(opts.errorHandler).toBeDefined();
    expect(typeof opts.errorHandler?.closed).toBe("function");
    expect(opts.middleware?.provideHover).toBeDefined();
  });

  test("restartServer stops the old client and starts a fresh one", async () => {
    const { wiring, clients } = makeWiring();
    const ctx = makeContext();
    await wiring.activate(ctx);
    const first = clients[0];
    await wiring.restartServer(ctx);
    expect(first.stopped).toBe(1);
    expect(clients).toHaveLength(2);
    expect(clients[1].started).toBe(1);
    expect(clients[1].isRunning()).toBe(true);
  });

  test("deactivate stops the running client and disposes the output channel", async () => {
    const { wiring, fake, lastClient } = makeWiring();
    await wiring.activate(makeContext());
    await wiring.deactivate();
    expect(lastClient().stopped).toBe(1);
    expect(fake.createdChannels[0].disposed).toBe(1);
  });

  test("a start() rejection surfaces an error, clears the client, and disposes the watcher", async () => {
    // When the binary cannot spawn, start() rejects. The extension must
    // not throw out of activate(); it surfaces an actionable error,
    // drops the client reference, and tears the watcher down so the next
    // restart installs a clean one.
    const { wiring, fake, lastClient } = makeWiring({
      startRejection: new Error("spawn /abs/mdsmith ENOENT"),
    });
    const ctx = makeContext();
    await wiring.activate(ctx);
    expect(fake.errorMessages.length).toBeGreaterThan(0);
    expect(fake.errorMessages[0]).toContain("Failed to start mdsmith Language Server");
    // The watcher created for this attempt is disposed after the failure.
    expect(fake.createdWatchers[0].disposable.disposed).toBe(1);
    // A subsequent deactivate must be a no-op on the cleared client
    // (stop() is never called a second time on the failed client).
    await wiring.deactivate();
    expect(lastClient().stopped).toBe(0);
  });

  test("the superseded notification flips the error handler to no-restart", async () => {
    // When the server announces a newer instance took over, the close
    // that follows must NOT restart — otherwise the orphaned host
    // respawns a server the newer one supersedes again.
    const { wiring, lastClient } = makeWiring();
    await wiring.activate(makeContext());
    const client = lastClient();
    expect(client.supersededHandler).toBeDefined();
    client.supersededHandler!();
    const decision = client.clientOptions.errorHandler!.closed();
    // CloseAction.DoNotRestart === 1.
    expect(decision.action).toBe(1);
  });

  test("forwards an mdsmith.* config change to the running client", async () => {
    // Toggling e.g. mdsmith.previewFix must nudge the server to re-pull
    // config. The change is forwarded only when it touches mdsmith.* and
    // only while the client is running.
    const { wiring, fake, lastClient } = makeWiring();
    await wiring.activate(makeContext());
    const client = lastClient();
    expect(client.isRunning()).toBe(true);
    fake.fireConfigChange(true);
    expect(client.notifications).toBe(1);
    // An unrelated change does not nudge the server.
    fake.fireConfigChange(false);
    expect(client.notifications).toBe(1);
  });
});
