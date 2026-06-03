// build.ts produces the release layout.
//
// Acceptance criterion (plan 217): `bun run build.ts --production`
// emits dist/main.js, dist/mdsmith.wasm, dist/wasm_exec.js,
// manifest.json, and styles.css. This test runs the real build into a
// throwaway dist and asserts every file lands. It compiles the WASM
// artifact from cmd/mdsmith-wasm, so it skips when the Go toolchain is
// absent (keeping the suite green on TS-only hosts; CI has Go).

import { afterAll, describe, expect, test } from "bun:test";
import { existsSync, rmSync, statSync } from "node:fs";
import { join } from "node:path";

const pluginDir = join(import.meta.dir, "..");
const distDir = join(pluginDir, "dist");

function hasGo(): boolean {
  return !Bun.spawnSync(["go", "version"]).exitCode;
}

afterAll(() => {
  // Leave a clean tree behind regardless of outcome.
  rmSync(distDir, { recursive: true, force: true });
});

describe("build.ts --production", () => {
  test.skipIf(!hasGo())(
    "emits main.js, mdsmith.wasm, wasm_exec.js, manifest.json, styles.css",
    () => {
      rmSync(distDir, { recursive: true, force: true });
      const out = Bun.spawnSync(["bun", "run", "build.ts", "--production"], {
        cwd: pluginDir,
        env: { ...process.env },
      });
      expect(out.exitCode, out.stderr.toString()).toBe(0);

      for (const name of [
        "main.js",
        "mdsmith.wasm",
        "wasm_exec.js",
        "manifest.json",
        "styles.css",
      ]) {
        const p = join(distDir, name);
        expect(existsSync(p), `expected dist/${name}`).toBe(true);
        expect(statSync(p).size, `dist/${name} should be non-empty`).toBeGreaterThan(0);
      }
    },
    120_000,
  );
});
