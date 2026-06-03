// WASM runtime facade for the Obsidian plugin.
//
// This is the ONLY module that touches the plan-215 WebAssembly engine.
// The rest of the plugin imports the typed surface below and never the
// raw module, so the WASM details (Go's wasm_exec.js glue, the
// globalThis.mdsmith factory, the Promise-returning methods) stay here.
//
// The WASM engine loads ONCE per module (memoized below): Go's main
// blocks forever on select{}, so re-instantiating it per runtime would
// leak an immortal Go runtime on every Restart / config change. Each
// MdsmithRuntime then owns one mdsmith.Session over that shared engine.
// Construct a runtime with the workspace snapshot + config YAML; a vault
// edit pushes new bytes through invalidate(); a config change or unload
// calls dispose(), which tears down only the session — the engine stays
// up, ready for the next createRuntime, exactly as the engine-API model
// prescribes (a config change is dispose-session + createSession).
//
// The engine API and the WASM binding contract live in
// docs/background/concepts/engine-api.md.

// Diagnostic is one lint finding. The shape matches the engine's
// `--format json` / Session.Check wire format (snake_case keys), so
// diagnostics.ts decodes it without a second schema. Column is a
// 1-based UTF-16 code-unit offset (LSP-native), measured on the Go
// side.
export interface Diagnostic {
  file: string;
  line: number;
  column: number;
  rule: string;
  name: string;
  // The engine reports severity as a string: "error" | "warning".
  severity: string;
  message: string;
  source_lines?: string[];
  source_start_line?: number;
  related_locations?: RelatedLocation[];
}

// RelatedLocation is a secondary source location attached to a
// diagnostic — for an MDS020 schema violation, the line of the schema
// constraint. File may differ from the owning diagnostic's file.
export interface RelatedLocation {
  file?: string;
  line?: number;
  column?: number;
  message: string;
}

// FixResult is the outcome of fix(): the rewritten source, a changed
// flag, and the diagnostics that survive the fix.
export interface FixResult {
  source: string;
  changed: boolean;
  diagnostics: Diagnostic[];
}

// MdsmithRuntime is the typed facade over one Session. Methods mirror
// the Go Session one-to-one; check and fix are async because the WASM
// bindings return Promises.
export interface MdsmithRuntime {
  check(uri: string, source: string): Promise<Diagnostic[]>;
  fix(uri: string, source: string): Promise<FixResult>;
  // invalidate pushes a vault change into the session. With content it
  // rewrites uri's bytes; without content it drops the file (deleted).
  invalidate(uri: string, content?: string): void;
  capabilities(): string[];
  dispose(): void;
}

// RuntimeOptions configure a runtime. workspace is the flat snapshot of
// the vault (slash-separated path → file text). loadWasmExec returns
// the Go wasm_exec.js glue source; loadWasmBytes returns the .wasm
// bytes. The host wires both to read the plugin directory; tests wire
// them to a prebuilt artifact.
export interface RuntimeOptions {
  workspace: Record<string, string>;
  configYAML?: string;
  loadWasmExec: () => string | Promise<string>;
  loadWasmBytes: () => Uint8Array | ArrayBuffer | Promise<Uint8Array | ArrayBuffer>;
}

// The shape of the globalThis.mdsmith factory the WASM module
// registers. Mirrors cmd/mdsmith-wasm/main.go.
interface MdsmithFactory {
  createSession(opts: {
    workspace: Record<string, string>;
    configYAML: string;
  }): Promise<WasmSession>;
  version: string;
}

// WasmSession is the JS proxy the factory returns. Method names match
// the Go Session exactly (see cmd/mdsmith-wasm/methods.go).
interface WasmSession {
  check(uri: string, source: string): Promise<Diagnostic[]>;
  fix(uri: string, source: string): Promise<FixResult>;
  capabilities(): string[];
  invalidate(uri: string, content?: string): void;
  dispose(): void;
}

// Go is the runtime class wasm_exec.js assigns onto globalThis. We type
// only the surface we call.
interface GoRuntime {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

// evalWasmExec runs the wasm_exec.js source for its side effect of
// defining globalThis.Go, then returns the Go constructor.
//
// wasm_exec.js is a plain script (an IIFE that assigns globalThis.Go),
// not a module. We evaluate it with the Function constructor rather
// than eval so it runs in its own scope but still sees the real
// globalThis. The Obsidian renderer and the bun test runtime both
// provide the globals the script references (TextEncoder/Decoder,
// crypto, performance); Bun additionally needs no globalThis.fs shim
// because the engine's JS bindings never write to stdout.
function loadGoConstructor(source: string): new () => GoRuntime {
  const g = globalThis as unknown as { Go?: new () => GoRuntime };
  // Run the script. It defines globalThis.Go as a side effect.
  new Function(source)();
  if (typeof g.Go !== "function") {
    throw new Error("wasm_exec.js did not define globalThis.Go");
  }
  return g.Go;
}

// enginePromise memoizes the one-time engine load (eval wasm_exec.js +
// WebAssembly.instantiate + grab the globalThis.mdsmith factory). It is
// module-scoped so every createRuntime call after the first reuses the
// same long-lived Go runtime instead of instantiating another.
//
// The Go WASM main blocks forever on select{} to keep the exported
// callbacks alive (cmd/mdsmith-wasm/main.go), so there is no per-runtime
// shutdown — only session.dispose(). Re-running go.run() per createRuntime
// would leak one immortal Go runtime on every Restart / configPath
// change. The engine-API model is: a config change is dispose-session +
// createSession, NOT a new WASM instance (docs/background/concepts/
// engine-api.md — compiled config is built once at NewSession). So the
// engine loads once and stays up; createRuntime only spins sessions.
//
// Holding a Promise (not the resolved factory) dedupes concurrent first
// calls: a second createRuntime that races the first awaits the same
// in-flight load rather than starting a second instantiate.
let enginePromise: Promise<MdsmithFactory> | undefined;

// loadEngine performs the one-time engine bring-up and returns the
// factory. The loaders run only on the first call; later calls await the
// cached promise and never re-read the assets.
function loadEngine(opts: RuntimeOptions): Promise<MdsmithFactory> {
  if (!enginePromise) {
    enginePromise = instantiateEngine(opts).catch((err: unknown) => {
      // A failed bring-up must not poison the cache: clear it so a later
      // createRuntime (e.g. after the user fixes a missing asset) can
      // retry rather than re-rejecting the same stale promise forever.
      enginePromise = undefined;
      throw err;
    });
  }
  return enginePromise;
}

// instantiateEngine evaluates wasm_exec.js, instantiates the module,
// starts the Go runtime, and returns the registered factory.
async function instantiateEngine(
  opts: RuntimeOptions,
): Promise<MdsmithFactory> {
  const execSource = await opts.loadWasmExec();
  const Go = loadGoConstructor(execSource);
  const bytes = await opts.loadWasmBytes();

  const go = new Go();
  const { instance } = await WebAssembly.instantiate(
    bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes),
    go.importObject,
  );

  // go.run never resolves: Go's main blocks on select{} to keep the
  // exported callbacks alive. It registers globalThis.mdsmith
  // synchronously during startup, so we grab the factory reference
  // immediately after. The engine loads exactly once per module, so
  // there is no second instance to race the global. The returned
  // Promise can still reject if Go panics; guard it so a panic
  // surfaces as a logged error instead of an unhandled rejection that
  // would crash Bun/Electron.
  void go.run(instance).catch((err: unknown) => {
    console.error("mdsmith: the WASM runtime exited unexpectedly", err);
  });

  const factory = (globalThis as unknown as { mdsmith?: MdsmithFactory })
    .mdsmith;
  if (!factory || typeof factory.createSession !== "function") {
    throw new Error(
      "the WASM module did not register globalThis.mdsmith.createSession",
    );
  }
  return factory;
}

// createRuntime builds one Session over the supplied workspace + config
// and resolves once the session is ready. The first call loads the WASM
// engine (memoized); every later call — a Restart, a configPath change —
// reuses that engine and only creates a fresh session. dispose() tears
// down the session alone; the engine stays up.
export async function createRuntime(
  opts: RuntimeOptions,
): Promise<MdsmithRuntime> {
  const factory = await loadEngine(opts);
  const session = await factory.createSession({
    workspace: opts.workspace,
    configYAML: opts.configYAML ?? "",
  });
  return new SessionRuntime(session);
}

// __resetEngineForTests drops the memoized engine so a test can force a
// fresh bring-up. Production code never calls it — the engine is meant
// to live for the whole plugin session. It does NOT tear down the Go
// runtime (there is no shutdown hook); a test that resets and reloads
// simply re-evaluates wasm_exec.js and re-instantiates.
export function __resetEngineForTests(): void {
  enginePromise = undefined;
}

// SessionRuntime adapts a WasmSession to the MdsmithRuntime facade. It
// is a thin pass-through — the engine does the work — plus a disposed
// guard so a call after dispose() throws a clear error rather than
// reaching into a torn-down session.
class SessionRuntime implements MdsmithRuntime {
  private disposed = false;

  constructor(private readonly session: WasmSession) {}

  private assertLive(): void {
    if (this.disposed) {
      throw new Error("mdsmith runtime used after dispose()");
    }
  }

  check(uri: string, source: string): Promise<Diagnostic[]> {
    this.assertLive();
    return this.session.check(uri, source);
  }

  fix(uri: string, source: string): Promise<FixResult> {
    this.assertLive();
    return this.session.fix(uri, source);
  }

  invalidate(uri: string, content?: string): void {
    this.assertLive();
    // The WASM binding reads a second string arg as the new content and
    // a one-arg call as a delete; forward exactly that contract.
    if (content === undefined) {
      this.session.invalidate(uri);
    } else {
      this.session.invalidate(uri, content);
    }
  }

  capabilities(): string[] {
    this.assertLive();
    return this.session.capabilities();
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.session.dispose();
  }
}
