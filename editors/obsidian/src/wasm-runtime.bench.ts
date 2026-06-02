// Benchmark for the WASM runtime: cold-start and steady-state check on
// a 1000-line fixture.
//
// Plan 217 §Budgets: cold-start check on a 1000-line file ≤ 1 s on
// desktop / ≤ 2 s on a modern iPad; steady-state check ≤ 150 ms
// everywhere. The numbers depend on hardware, so this file does two
// things:
//
//   1. Logs the measured cold-start and steady-state times so a human
//      (or CI log) can compare against the plan's budgets directly.
//   2. Asserts the steady-state check runs in well under half the
//      cold-start time — the same cache-effectiveness invariant the Go
//      session bench (pkg/mdsmith/bench_test.go) enforces. This is the
//      hardware-independent guard; the absolute ceiling below has wide
//      headroom so a slow CI runner does not flake.
//
// Runs under `bun test`. Skips when the Go toolchain is absent.

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { createRuntime, type MdsmithRuntime } from "./wasm-runtime";

function hasGo(): boolean {
  return !Bun.spawnSync(["go", "version"]).exitCode;
}

function wasmExecPath(): string {
  const out = Bun.spawnSync(["go", "env", "GOROOT"]);
  return join(out.stdout.toString().trim(), "lib", "wasm", "wasm_exec.js");
}

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

// thousandLineDoc builds a ~1000-line Markdown document. Every tenth
// line overruns the 80-column MDS001 limit so the linter does real
// work, not a trivial all-clean pass.
function thousandLineDoc(): string {
  const lines: string[] = ["# Benchmark fixture", ""];
  for (let i = 0; i < 1000; i++) {
    if (i % 10 === 0) {
      lines.push(
        `This sentence number ${i} is written long enough to push it well past the eighty column limit so MDS001 fires here.`,
      );
    } else {
      lines.push(`Short line ${i}.`);
    }
  }
  return lines.join("\n") + "\n";
}

const skip = !hasGo();
let tmp = "";
let wasmBytes: Uint8Array;
let wasmExecSource: string;
const doc = thousandLineDoc();

beforeAll(() => {
  if (skip) return;
  tmp = mkdtempSync(join(tmpdir(), "mds-wasm-bench-"));
  wasmBytes = buildWasm(tmp);
  wasmExecSource = readFileSync(wasmExecPath(), "utf8");
}, 120_000);

afterAll(() => {
  if (tmp) rmSync(tmp, { recursive: true, force: true });
});

async function makeRuntime(): Promise<MdsmithRuntime> {
  return createRuntime({
    workspace: {},
    configYAML: "",
    loadWasmExec: () => wasmExecSource,
    loadWasmBytes: async () => wasmBytes,
  });
}

describe.skipIf(skip)("wasm-runtime check budgets (1000-line fixture)", () => {
  test("steady-state is well under cold-start, and both are logged vs budget", async () => {
    const rt = await makeRuntime();

    // Cold: first check of the fixture (cache miss → full parse + lint).
    const coldStart = performance.now();
    const coldDiags = await rt.check("bench.md", doc);
    const coldMs = performance.now() - coldStart;
    expect(coldDiags.length).toBeGreaterThan(0); // MDS001 fired — real work.

    // Steady: re-check identical bytes (cache hit). Take the median of a
    // few runs to smooth out GC jitter.
    const samples: number[] = [];
    for (let i = 0; i < 7; i++) {
      const t = performance.now();
      await rt.check("bench.md", doc);
      samples.push(performance.now() - t);
    }
    samples.sort((a, b) => a - b);
    const steadyMs = samples[Math.floor(samples.length / 2)];

    // eslint-disable-next-line no-console
    console.log(
      `[wasm-runtime bench] cold=${coldMs.toFixed(1)}ms ` +
        `steady=${steadyMs.toFixed(1)}ms ` +
        `(plan budget: cold ≤ 1000ms desktop / 2000ms iPad, steady ≤ 150ms)`,
    );

    // Hardware-independent guard: the session cache must make a repeat
    // check far cheaper than the cold path.
    expect(steadyMs * 2).toBeLessThan(coldMs);

    // Absolute ceilings with wide CI headroom (5x the desktop budget)
    // so a slow runner logs the real number without flaking the suite.
    expect(coldMs).toBeLessThan(5000);
    expect(steadyMs).toBeLessThan(750);

    rt.dispose();
  }, 60_000);
});
