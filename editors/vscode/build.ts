// Bun-based build script for the mdsmith VS Code extension.
// Bundles src/extension.ts into dist/extension.js as a single CJS
// file consumed by VS Code, marking `vscode` as external because
// the host supplies it at runtime.

const args = Bun.argv.slice(2);
const watch = args.includes("--watch");
const production = args.includes("--production");

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
    for await (const file of glob.scan({ cwd: import.meta.dir })) {
      const stat = await Bun.file(file).stat();
      const prev = seen.get(file);
      if (prev !== undefined && prev !== stat.mtimeMs) {
        changed = true;
      }
      seen.set(file, stat.mtimeMs);
    }
    if (changed) {
      await buildOnce();
    }
  }
} else {
  await buildOnce();
}
