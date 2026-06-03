// Bun-based build script for the mdsmith Obsidian plugin.
//
// Obsidian loads CommonJS from `<vault>/.obsidian/plugins/mdsmith/`.
// The release zip ships five top-level files:
//
//   dist/main.js        the bundled plugin (this script bundles it)
//   dist/manifest.json  static, copied from the plugin root
//   dist/styles.css     static, copied from the plugin root
//   dist/mdsmith.wasm   the plan-215 engine compiled to WebAssembly
//   dist/wasm_exec.js   Go's WASM<->JS runtime glue
//
// Unlike the VS Code extension — which bundles a native binary per
// platform — the Obsidian plugin ships ONE WASM artifact that runs on
// desktop (Electron) and mobile (the sandboxed WebView) alike. There
// is no per-platform staging and no PATH fallback: the engine is the
// .wasm file in the zip.
//
// The WASM artifact comes from cmd/mdsmith-wasm/build.sh, which writes
// mdsmith.wasm + wasm_exec.js into its own dist/. We invoke it (unless
// MDSMITH_OBSIDIAN_WASM_DIR points at a prebuilt pair, as the release
// workflow does) and copy the pair next to the bundle.

import {
  copyFileSync,
  existsSync,
  mkdirSync,
  rmSync,
} from "node:fs";
import { join } from "node:path";

const args = Bun.argv.slice(2);
const watch = args.includes("--watch");
const production = args.includes("--production");

const pluginDir = import.meta.dir;
const distDir = join(pluginDir, "dist");
const wasmCmdDir = join(pluginDir, "..", "..", "cmd", "mdsmith-wasm");

// stageStaticFiles copies the two files Obsidian loads verbatim
// (manifest.json, styles.css) next to the bundle. main.js writes
// itself via Bun.build.
function stageStaticFiles(): void {
  mkdirSync(distDir, { recursive: true });
  for (const name of ["manifest.json", "styles.css"]) {
    const src = join(pluginDir, name);
    if (!existsSync(src)) {
      throw new Error(`missing static file: ${src}`);
    }
    copyFileSync(src, join(distDir, name));
  }
}

// stageWasm puts mdsmith.wasm + wasm_exec.js into dist/. Source order:
//   1. $MDSMITH_OBSIDIAN_WASM_DIR (release: a prebuilt, size-checked
//      pair — skip the in-build Go compile).
//   2. cmd/mdsmith-wasm/build.sh output (a local build: compile it
//      here so `bun run build.ts` is self-contained when Go is present).
function stageWasm(): void {
  mkdirSync(distDir, { recursive: true });
  const prebuilt = process.env.MDSMITH_OBSIDIAN_WASM_DIR;
  const wasmSrcDir = prebuilt ?? join(wasmCmdDir, "dist");

  if (!prebuilt) {
    // Compile the engine to WASM via the canonical build script so the
    // plugin ships the exact artifact cmd/mdsmith-wasm/size_test.go
    // budget-checks.
    const buildSh = join(wasmCmdDir, "build.sh");
    const res = Bun.spawnSync(["bash", buildSh], {
      cwd: wasmCmdDir,
      stdout: "inherit",
      stderr: "inherit",
    });
    if (res.exitCode !== 0) {
      throw new Error(
        `cmd/mdsmith-wasm/build.sh failed (exit ${res.exitCode}); is the Go ` +
          "toolchain installed? Set MDSMITH_OBSIDIAN_WASM_DIR to a prebuilt " +
          "mdsmith.wasm + wasm_exec.js pair to skip the in-build compile.",
      );
    }
  }

  for (const name of ["mdsmith.wasm", "wasm_exec.js"]) {
    const src = join(wasmSrcDir, name);
    if (!existsSync(src)) {
      throw new Error(
        `missing WASM artifact: ${src}. Build cmd/mdsmith-wasm first or set ` +
          "MDSMITH_OBSIDIAN_WASM_DIR.",
      );
    }
    copyFileSync(src, join(distDir, name));
  }
}

// Reset only the bundle output between runs; preserve any previously
// staged WASM/static files (mirrors editors/vscode/build.ts: a re-run
// from another process should not wipe what an explicit build placed).
rmSync(join(distDir, "main.js"), { force: true });
rmSync(join(distDir, "main.js.map"), { force: true });

stageStaticFiles();
stageWasm();

const config: Parameters<typeof Bun.build>[0] = {
  entrypoints: ["src/main.ts"],
  outdir: "dist",
  target: "browser",
  format: "cjs",
  // Obsidian provides `obsidian` and the CM6 packages at runtime;
  // marking them external keeps the bundle small and reuses the host's
  // copy (mandatory for state-sharing with other plugins). The WASM
  // module is loaded at runtime via fetch + WebAssembly.instantiate,
  // not bundled, so it is not an entrypoint.
  external: [
    "obsidian",
    "@codemirror/state",
    "@codemirror/view",
    "@codemirror/language",
  ],
  minify: production,
  sourcemap: production ? "none" : "external",
};

async function buildOnce(): Promise<void> {
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
  // Bun's bundler does not expose a watch API; fall back to one-second
  // FS polling, the same pattern editors/vscode/build.ts uses.
  await buildOnce();
  const seen = new Map<string, number>();
  for await (const _ of (async function* () {
    while (true) {
      yield await new Promise((r) => setTimeout(r, 1000));
    }
  })()) {
    const glob = new Bun.Glob("src/**/*.ts");
    let changed = false;
    const present = new Set<string>();
    for await (const rel of glob.scan({ cwd: pluginDir })) {
      const abs = join(pluginDir, rel);
      let mtimeMs: number;
      try {
        mtimeMs = (await Bun.file(abs).stat()).mtimeMs;
      } catch {
        continue;
      }
      const prev = seen.get(abs);
      if (prev === undefined) {
        changed = true;
      } else if (prev !== mtimeMs) {
        changed = true;
      }
      seen.set(abs, mtimeMs);
      present.add(abs);
    }
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
