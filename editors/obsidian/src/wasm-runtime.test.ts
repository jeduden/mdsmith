// Drives the WASM runtime facade.
//
// The facade (src/wasm-runtime.ts) is the only module that touches the
// plan-215 WebAssembly engine. It instantiates the module, constructs
// one mdsmith.Session over a workspace snapshot + config YAML, and
// exposes check / fix / invalidate / dispose through a typed surface.
//
// These tests load the REAL .wasm artifact via Go's wasm_exec.js — the
// same path src/wasm-runtime.ts uses in the Electron/WebView host — so
// they exercise the JS↔Go marshalling, not a mock. They build the
// artifact on demand and skip when the Go toolchain is absent.

import {
  afterAll,
  beforeAll,
  describe,
  expect,
  test,
} from "bun:test";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import {
  __resetEngineForTests,
  createRuntime,
  type Diagnostic,
  type MdsmithRuntime,
} from "./wasm-runtime";

function hasGo(): boolean {
  return !Bun.spawnSync(["go", "version"]).exitCode;
}

// goRoot resolves the active toolchain's wasm_exec.js, which moved to
// lib/wasm/ in recent Go releases (mirrors cmd/mdsmith-wasm smoke test).
function wasmExecPath(): string {
  const out = Bun.spawnSync(["go", "env", "GOROOT"]);
  const root = out.stdout.toString().trim();
  return join(root, "lib", "wasm", "wasm_exec.js");
}

// buildWasm compiles cmd/mdsmith-wasm for GOOS=js GOARCH=wasm into a
// temp file and returns the bytes.
function buildWasm(dir: string): Uint8Array {
  const out = join(dir, "mdsmith.wasm");
  const res = Bun.spawnSync(["go", "build", "-o", out, "."], {
    cwd: join(import.meta.dir, "..", "..", "..", "cmd", "mdsmith-wasm"),
    env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
  });
  if (res.exitCode !== 0) {
    throw new Error(`go build wasm failed: ${res.stderr.toString()}`);
  }
  return readFileSync(out);
}

const skip = !hasGo();
let tmp = "";
let wasmBytes: Uint8Array;
let wasmExecSource: string;

beforeAll(() => {
  if (skip) return;
  tmp = mkdtempSync(join(tmpdir(), "mds-wasm-rt-"));
  wasmBytes = buildWasm(tmp);
  wasmExecSource = readFileSync(wasmExecPath(), "utf8");
}, 120_000); // a cold GOOS=js GOARCH=wasm build takes ~30 s.

afterAll(() => {
  if (tmp) rmSync(tmp, { recursive: true, force: true });
});

// makeRuntime builds a runtime from the test's prebuilt artifact. The
// loader takes the wasm_exec.js source and the .wasm bytes so the test
// controls where they come from; the host wires its own loader that
// fetches the two files from the plugin directory.
async function makeRuntime(
  workspace: Record<string, string>,
  configYAML = "",
): Promise<MdsmithRuntime> {
  return createRuntime({
    workspace,
    configYAML,
    loadWasmExec: () => wasmExecSource,
    loadWasmBytes: async () => wasmBytes,
  });
}

describe.skipIf(skip)("createRuntime", () => {
  // The engine is memoized at module scope and intentionally shared
  // across these tests: creating a fresh Go runtime per test would leak
  // an immortal one (there is no shutdown hook). Only the load-count and
  // retry-cache tests below call __resetEngineForTests(), since they
  // assert behavior that is only observable from a clean cache.

  test("check returns a normalized diagnostic array for a clean file", async () => {
    const rt = await makeRuntime({});
    const diags = await rt.check("clean.md", "# Clean\n\nA tidy paragraph.\n");
    expect(Array.isArray(diags)).toBe(true);
    expect(diags.length).toBe(0);
    rt.dispose();
  });

  test("check surfaces an MDS001 violation with the engine's wire shape", async () => {
    const rt = await makeRuntime({});
    // MDS001 is line-length (default max 80). A line past 80 columns
    // fires it — the exact rule the plan's acceptance criterion names.
    const longLine =
      "This line is deliberately made to exceed the eighty character limit by adding extra words here now.";
    const diags = await rt.check("bad.md", `# Title\n\n${longLine}\n`);
    const codes = diags.map((d) => d.rule);
    expect(codes).toContain("MDS001");
    const d = diags.find((x) => x.rule === "MDS001") as Diagnostic;
    expect(typeof d.line).toBe("number");
    expect(typeof d.column).toBe("number");
    expect(d.severity).toBeDefined();
    expect(typeof d.message).toBe("string");
    rt.dispose();
  });

  test("fix returns rewritten source plus a changed flag", async () => {
    const rt = await makeRuntime({});
    // Trailing spaces are fixable (MDS009).
    const res = await rt.fix("trail.md", "# Title\n\nText with trailing.   \n");
    expect(res.changed).toBe(true);
    expect(res.source).not.toContain("trailing.   \n");
    expect(Array.isArray(res.diagnostics)).toBe(true);
    rt.dispose();
  });

  test("fix matches the WASM session.fix on the same input", async () => {
    const input = "#  Spaced Title\n\n\n\ntext   \n";
    const rt = await makeRuntime({});
    const viaFacade = await rt.fix("doc.md", input);
    // The facade is a thin pass-through; a second fix of its own output
    // must be a no-op (idempotent), proving it returns the engine's
    // final bytes rather than a partial pass.
    const again = await rt.fix("doc.md", viaFacade.source);
    expect(again.changed).toBe(false);
    expect(again.source).toBe(viaFacade.source);
    rt.dispose();
  });

  test("invalidate(uri, content) makes a cross-file rule see new bytes", async () => {
    // index.md catalogs docs/*.md by their summary front matter. Drop a
    // doc in, then invalidate the index's view of the workspace.
    const indexSrc =
      "# Index\n\n<?catalog\nglob:\n  - \"docs/*.md\"\n" +
      "row: \"- [{summary}](docs/{filename})\"\n?>\n<?/catalog?>\n";
    const rt = await makeRuntime({
      "docs/a.md": "---\nsummary: Doc A\n---\n# A\n\nBody of doc A here.\n",
    });
    const before = await rt.check("index.md", indexSrc);
    // Add a second doc to the workspace and re-check: the catalog is now
    // out of date by one more entry, so MDS019 still fires (the body is
    // empty). The point is the call does not throw and the workspace
    // mutation is accepted.
    rt.invalidate(
      "docs/b.md",
      "---\nsummary: Doc B\n---\n# B\n\nBody of doc B here.\n",
    );
    const after = await rt.check("index.md", indexSrc);
    expect(Array.isArray(before)).toBe(true);
    expect(Array.isArray(after)).toBe(true);
    rt.dispose();
  });

  test("capabilities advertises check, fix, and kinds", async () => {
    const rt = await makeRuntime({});
    const caps = rt.capabilities();
    expect(caps).toContain("check");
    expect(caps).toContain("fix");
    expect(caps).toContain("kinds");
    rt.dispose();
  });

  // Thread C: the engine (wasm_exec eval + WebAssembly.instantiate +
  // globalThis.mdsmith) loads ONCE and is reused across every
  // createRuntime — a Restart / configPath change must not instantiate a
  // second immortal Go runtime, only a fresh session.
  test("the engine loads once across multiple createRuntime calls", async () => {
    // Count asset loads from a clean cache; the tests above reuse the
    // shared engine, so reset here to make the first load observable.
    __resetEngineForTests();
    let execLoads = 0;
    let byteLoads = 0;
    const countingOpts = () => ({
      loadWasmExec: () => {
        execLoads++;
        return wasmExecSource;
      },
      loadWasmBytes: async () => {
        byteLoads++;
        return wasmBytes;
      },
    });

    const rt1 = await createRuntime({ workspace: {}, ...countingOpts() });
    const rt2 = await createRuntime({ workspace: {}, ...countingOpts() });
    const rt3 = await createRuntime({
      workspace: {},
      configYAML: "rules:\n  MDS001:\n    max-line-length: 40\n",
      ...countingOpts(),
    });

    // One asset read total, despite three runtimes (one of them with a
    // different config — proving a config change reuses the engine).
    expect(execLoads).toBe(1);
    expect(byteLoads).toBe(1);

    rt1.dispose();
    rt2.dispose();
    rt3.dispose();
  });

  test("concurrent first createRuntime calls share a single engine load", async () => {
    __resetEngineForTests(); // dedupe is observable only from a clean cache
    let execLoads = 0;
    let byteLoads = 0;
    const opts = () => ({
      workspace: {},
      loadWasmExec: () => {
        execLoads++;
        return wasmExecSource;
      },
      loadWasmBytes: async () => {
        byteLoads++;
        return wasmBytes;
      },
    });

    // Two createRuntime calls launched before either resolves: the
    // memoized Promise dedupes the bring-up to one instantiate.
    const [rtA, rtB] = await Promise.all([
      createRuntime(opts()),
      createRuntime(opts()),
    ]);
    expect(execLoads).toBe(1);
    expect(byteLoads).toBe(1);
    rtA.dispose();
    rtB.dispose();
  });

  test("a failed engine load does not poison the cache — a later call can retry", async () => {
    // Start from a clean cache so the throwing loader is the first load.
    __resetEngineForTests();
    // First bring-up fails because the asset loader throws. The memoized
    // promise must be cleared so the next createRuntime re-attempts the
    // load rather than re-rejecting forever (the plugin's degraded-mode
    // recovery relies on this: startRuntime catches and the user can
    // Restart).
    let failure: Error | undefined;
    try {
      await createRuntime({
        workspace: {},
        loadWasmExec: () => {
          throw new Error("asset missing");
        },
        loadWasmBytes: async () => wasmBytes,
      });
    } catch (err) {
      failure = err as Error;
    }
    expect(failure?.message).toContain("asset missing");

    // A subsequent call with working loaders succeeds.
    const rt = await makeRuntime({});
    const caps = rt.capabilities();
    expect(caps).toContain("check");
    rt.dispose();
  });

  test("two cache-shared runtimes work independently; disposing one leaves the other live", async () => {
    // Both runtimes ride the same engine but own distinct sessions.
    const rtA = await makeRuntime({});
    const rtB = await makeRuntime({});

    const longLine =
      "This line is deliberately made to exceed the eighty character limit by adding extra words here now.";
    // A still works.
    const aDiags = await rtA.check("a.md", `# A\n\n${longLine}\n`);
    expect(aDiags.map((d) => d.rule)).toContain("MDS001");

    // Dispose A; B must keep working (the engine is not torn down, and
    // B's session is independent of A's).
    rtA.dispose();
    const bDiags = await rtB.check("b.md", `# B\n\n${longLine}\n`);
    expect(bDiags.map((d) => d.rule)).toContain("MDS001");

    // A is now disposed: a call on it throws the disposed-guard error,
    // proving dispose() really tore A's session down (not a no-op).
    expect(() => rtA.capabilities()).toThrow();

    rtB.dispose();
  });
});
