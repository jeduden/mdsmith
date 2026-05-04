// Build script for the mdsmith VS Code extension. Bundles src/extension.ts
// into dist/extension.js as a single CommonJS file consumed by VS Code.

const esbuild = require("esbuild");

const args = process.argv.slice(2);
const watch = args.includes("--watch");
const production = args.includes("--production");

const options = {
  entryPoints: ["src/extension.ts"],
  bundle: true,
  outfile: "dist/extension.js",
  external: ["vscode"],
  format: "cjs",
  platform: "node",
  target: ["node18"],
  sourcemap: !production,
  minify: production,
  logLevel: "info"
};

(async () => {
  if (watch) {
    const ctx = await esbuild.context(options);
    await ctx.watch();
    console.log("watching for changes…");
  } else {
    await esbuild.build(options);
  }
})().catch((err) => {
  console.error(err);
  process.exit(1);
});
