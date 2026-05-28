// Bun-based build script for the mdsmith Obsidian plugin.
//
// Obsidian loads CommonJS from `<vault>/.obsidian/plugins/mdsmith/`.
// The release zip ships three top-level files (main.js, manifest.json,
// styles.css) plus a staged `cli/` directory carrying the bundled
// @mdsmith/cli shim and one binary per supported platform — the same
// layout the VS Code extension uses, so src/binary.ts can drive the
// shim's resolver against it.
//
// Binary source order matches editors/vscode/build.ts:
//   1. $MDSMITH_OBSIDIAN_PLATFORM_DIR (the release workflow points
//      this at `mdsmith-release build-npm` output — all five present).
//   2. node_modules/@mdsmith/<target> (a local `bun install`, host
//      platform only by npm's os/cpu rules).
// Missing platforms are skipped; at runtime the plugin falls back to
// $PATH for any host that was not staged.

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

// Targets must stay in lock-step with editors/vscode/build.ts and
// npm/mdsmith/bin/mdsmith.js's PLATFORM_PACKAGES. Drift would mean
// the plugin and the npm shim disagree on which binary to ship.
const PLATFORM_TARGETS = [
  { target: "linux-x64", exe: "mdsmith" },
  { target: "linux-arm64", exe: "mdsmith" },
  { target: "darwin-x64", exe: "mdsmith" },
  { target: "darwin-arm64", exe: "mdsmith" },
  { target: "win32-x64", exe: "mdsmith.exe" },
];

function stageCli(): void {
  const distDir = join(import.meta.dir, "dist");
  const cliDir = join(distDir, "cli");
  mkdirSync(cliDir, { recursive: true });

  const shimSrc = join(
    import.meta.dir,
    "..",
    "..",
    "npm",
    "mdsmith",
    "bin",
    "mdsmith.js",
  );
  if (!existsSync(shimSrc)) {
    // The shim is the resolver the plugin re-uses; without it
    // there is no bundled-binary path at all.
    throw new Error(`missing @mdsmith/cli shim: ${shimSrc}`);
  }
  copyFileSync(shimSrc, join(cliDir, "mdsmith.js"));

  const stageDir = process.env.MDSMITH_OBSIDIAN_PLATFORM_DIR;
  const nodeModules = join(import.meta.dir, "node_modules", "@mdsmith");

  for (const { target, exe } of PLATFORM_TARGETS) {
    const src = stageDir
      ? join(stageDir, target, "bin", exe)
      : join(nodeModules, target, "bin", exe);
    if (!existsSync(src)) continue;
    const destDir = join(cliDir, "@mdsmith", target, "bin");
    mkdirSync(destDir, { recursive: true });
    copyFileSync(src, join(destDir, exe));
  }

  const present = PLATFORM_TARGETS.filter(({ target, exe }) =>
    existsSync(join(cliDir, "@mdsmith", target, "bin", exe)),
  ).map(({ target }) => target);

  if (present.length === PLATFORM_TARGETS.length) {
    console.log(
      `staged @mdsmith/cli + ${present.length} platform binaries → dist/cli/`,
    );
  } else if (present.length > 0) {
    console.warn(
      `warning: staged only ${present.length}/${PLATFORM_TARGETS.length} ` +
        `platform binaries (${present.join(", ")}). The release zip will ` +
        "fall back to PATH on the missing platforms. Set " +
        "MDSMITH_OBSIDIAN_PLATFORM_DIR to a full build-npm output for an " +
        "all-platform zip.",
    );
  } else {
    console.warn(
      "warning: no platform binaries found; the plugin will fall back to " +
        "PATH resolution everywhere. Run `bun install` or set " +
        "MDSMITH_OBSIDIAN_PLATFORM_DIR.",
    );
  }
}

function stageStaticFiles(): void {
  // Obsidian loads main.js, manifest.json, and styles.css from the
  // plugin directory. Bundle main.js writes itself; manifest.json
  // and styles.css are static, so copy them next to the bundle.
  const distDir = join(import.meta.dir, "dist");
  mkdirSync(distDir, { recursive: true });
  for (const name of ["manifest.json", "styles.css"]) {
    const src = join(import.meta.dir, name);
    if (!existsSync(src)) {
      throw new Error(`missing static file: ${src}`);
    }
    copyFileSync(src, join(distDir, name));
  }
}

// Reset only the bundle output between runs; preserve any
// previously staged platform binaries from an earlier explicit build
// (mirrors editors/vscode/build.ts: a subsequent run from another
// process should not wipe a binary the explicit build placed).
const distDir = join(import.meta.dir, "dist");
rmSync(join(distDir, "main.js"), { force: true });
rmSync(join(distDir, "main.js.map"), { force: true });

stageStaticFiles();
stageCli();

const config: Parameters<typeof Bun.build>[0] = {
  entrypoints: ["src/main.ts"],
  outdir: "dist",
  target: "node",
  format: "cjs",
  // Obsidian provides `obsidian` and the CM6 packages at runtime;
  // marking them external keeps the bundle small and reuses the
  // host's copy (mandatory for state-sharing with other plugins).
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
  // Bun's bundler does not expose a watch API; fall back to one-
  // second FS polling, the same pattern editors/vscode/build.ts uses.
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
    for await (const rel of glob.scan({ cwd: import.meta.dir })) {
      const abs = join(import.meta.dir, rel);
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
