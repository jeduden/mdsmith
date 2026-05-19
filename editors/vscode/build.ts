// Bun-based build script for the mdsmith VS Code extension.
// Bundles src/extension.ts into dist/extension.js as a single CJS
// file consumed by VS Code, marking `vscode` as external because
// the host supplies it at runtime.
//
// Also stages the @mdsmith/cli distribution into dist/cli/ so the
// .vsix carries a binary for every supported platform and the
// extension can pick the right one at runtime by re-using the npm
// package's own resolver (see src/binary.ts):
//
//   dist/cli/mdsmith.js                        (npm/mdsmith/bin/mdsmith.js)
//   dist/cli/@mdsmith/<target>/bin/mdsmith[.exe]
//
// Binary source, in order: $MDSMITH_VSIX_PLATFORM_DIR (the release
// workflow points this at `mdsmith-release build-npm` output, which
// holds all five), else node_modules/@mdsmith/<target> (a local
// `bun install`, which by npm os/cpu rules only fetches the host
// one). Missing platforms are skipped — at runtime the extension
// falls back to PATH for any host that was not staged.

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

// Targets must stay in lock-step with npm/mdsmith/bin/mdsmith.js's
// PLATFORM_PACKAGES and internal/release/buildnpm.go's matrix; the
// drift guard in src/binary.test.ts fails CI if they diverge.
const PLATFORM_TARGETS = [
  { target: "linux-x64", exe: "mdsmith" },
  { target: "linux-arm64", exe: "mdsmith" },
  { target: "darwin-x64", exe: "mdsmith" },
  { target: "darwin-arm64", exe: "mdsmith" },
  { target: "win32-x64", exe: "mdsmith.exe" },
];

// stageCli copies the canonical @mdsmith/cli shim and every available
// platform binary into dist/cli/, mirroring the npm package's
// node_modules layout so src/binary.ts can drive the shim's own
// resolver against it.
function stageCli() {
  const cliDir = join(import.meta.dir, "dist", "cli");
  // `vsce package` re-invokes this script through the
  // `vscode:prepublish` hook, and that re-run does not inherit the
  // explicit build's MDSMITH_VSIX_PLATFORM_DIR. So staging is
  // non-destructive: we (re)copy the shim and only overwrite a
  // platform binary when a fresh source exists. A binary already
  // staged by the preceding explicit build is left intact rather
  // than wiped, so the .vsix keeps every platform. Only the legacy
  // flat dist/bin/ layout is actively removed.
  rmSync(join(import.meta.dir, "dist", "bin"), {
    recursive: true,
    force: true,
  });
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
    // The shim is the resolver the extension re-uses; without it
    // there is no bundled-binary path at all.
    throw new Error(`missing @mdsmith/cli shim: ${shimSrc}`);
  }
  copyFileSync(shimSrc, join(cliDir, "mdsmith.js"));

  // Prefer an explicit stage dir (release: `mdsmith-release
  // build-npm` output, all five present). Fall back to a local
  // node_modules install (host platform only).
  const stageDir = process.env.MDSMITH_VSIX_PLATFORM_DIR;
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

  // Report on what the .vsix will actually ship — i.e. what is
  // present in dist/cli/ now — not just what this invocation copied.
  // The prepublish re-run copies nothing yet leaves the explicit
  // build's binaries in place; the summary must reflect that.
  const present = PLATFORM_TARGETS.filter(({ target, exe }) =>
    existsSync(join(cliDir, "@mdsmith", target, "bin", exe)),
  ).map(({ target }) => target);

  if (present.length === PLATFORM_TARGETS.length) {
    console.log(`staged @mdsmith/cli + ${present.length} platform binaries → dist/cli/`);
  } else if (present.length > 0) {
    console.warn(
      `warning: staged only ${present.length}/${PLATFORM_TARGETS.length} ` +
        `platform binaries (${present.join(", ")}). The .vsix will fall ` +
        "back to PATH on the missing platforms. Set " +
        "MDSMITH_VSIX_PLATFORM_DIR to a full build-npm output for an " +
        "all-platform .vsix.",
    );
  } else {
    console.warn(
      "warning: no platform binaries found; the extension will fall " +
        "back to PATH resolution everywhere. Run `bun install` or set " +
        "MDSMITH_VSIX_PLATFORM_DIR.",
    );
  }
}

stageCli();

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
