// Bun-based build script for the mdsmith VS Code extension.
// Bundles src/extension.ts into dist/extension.js as a single CJS
// file consumed by VS Code, marking `vscode` as external because
// the host supplies it at runtime.

import { copyFileSync, existsSync } from "node:fs";
import { join } from "node:path";

const args = Bun.argv.slice(2);
const watch = args.includes("--watch");
const production = args.includes("--production");

// Stage the repo's MIT LICENSE inside the extension directory
// before packaging. vsce only ships LICENSE / LICENSE.md /
// LICENSE.txt that lives next to package.json, and warns
// "LICENSE, LICENSE.md, or LICENSE.txt not found" otherwise. The
// staged copy is git-ignored so the repo root remains the single
// source of truth.
const repoLicense = join(import.meta.dir, "..", "..", "LICENSE");
const stagedLicense = join(import.meta.dir, "LICENSE");
if (existsSync(repoLicense)) {
  copyFileSync(repoLicense, stagedLicense);
}

const config: Parameters<typeof Bun.build>[0] = {
  entrypoints: ["src/extension.ts"],
  outdir: "dist",
  target: "node",
  format: "cjs",
  external: ["vscode"],
  minify: production,
  sourcemap: production ? "none" : "external",
  // VS Code 1.85+ ships with Node 18; pin the same target so any
  // syntax we accidentally lower or polyfill against still works.
  // (Bun's `node` target maps to whatever the runtime supports.)
};

async function buildOnce() {
  const result = await Bun.build(config);
  if (!result.success) {
    for (const log of result.logs) {
      console.error(log);
    }
    process.exit(1);
  }
  console.log(`built ${result.outputs.length} file(s) → dist/`);
}

if (watch) {
  // Bun's bundler does not yet expose a watch API; fall back to
  // FS polling at one-second granularity. The extension is small
  // enough that a fresh build is fast.
  await buildOnce();
  const seen = new Map<string, number>();
  for await (const _ of (async function* () {
    while (true) {
      yield await new Promise((r) => setTimeout(r, 1000));
    }
  })()) {
    const glob = new Bun.Glob("src/**/*.ts");
    let changed = false;
    // Track which paths we observed this tick so we can detect
    // deletions after the scan finishes.
    const present = new Set<string>();
    // glob.scan returns paths relative to its cwd; resolve each one
    // against import.meta.dir so the subsequent Bun.file().stat()
    // calls do not depend on the process working directory (which
    // may differ from the script's directory under `bun run`).
    for await (const rel of glob.scan({ cwd: import.meta.dir })) {
      const abs = join(import.meta.dir, rel);
      // glob.scan yielded the path, but a delete/rename can race
      // between the yield and the stat call. Treat a stat failure
      // the same as "file vanished": skip this iteration so the
      // watch process keeps running. The deletion sweep below
      // (over `seen`) will pick the missing entry up next tick.
      let mtimeMs: number;
      try {
        mtimeMs = (await Bun.file(abs).stat()).mtimeMs;
      } catch {
        continue;
      }
      const prev = seen.get(abs);
      if (prev === undefined) {
        // Newly-appearing file — also a rebuild trigger.
        changed = true;
      } else if (prev !== mtimeMs) {
        changed = true;
      }
      seen.set(abs, mtimeMs);
      present.add(abs);
    }
    // Detect deletions: anything in `seen` that no longer shows
    // up in `present` was removed since the last tick.
    for (const abs of seen.keys()) {
      if (!present.has(abs)) {
        seen.delete(abs);
        changed = true;
      }
    }
    if (changed) {
      await buildOnce();
    }
  }
} else {
  await buildOnce();
}
