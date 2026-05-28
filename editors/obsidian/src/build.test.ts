// Drives `build.ts` end-to-end. The script bundles src/main.ts to
// dist/main.js as one CommonJS file (Obsidian only loads CJS), stages
// the platform binaries the .vsix-style shim resolves from, and
// copies manifest.json plus styles.css into dist/ so the release zip
// can be assembled from a single directory.
//
// The build is deterministic given a clean working tree, so we shell
// out to `bun run build.ts` in the obsidian directory and inspect the
// output.

import { describe, expect, test, afterEach, beforeEach } from "bun:test";
import { existsSync, readFileSync, rmSync } from "node:fs";
import { join } from "node:path";

const root = join(__dirname, "..");
const distDir = join(root, "dist");

describe("build.ts", () => {
  beforeEach(() => {
    rmSync(distDir, { recursive: true, force: true });
  });
  afterEach(() => {
    // Leave the artifact in place between runs would be fine; clean
    // up here so a failed assertion does not pollute the next run.
    rmSync(distDir, { recursive: true, force: true });
  });

  test("bundles main.ts to dist/main.js as CommonJS", async () => {
    const result = await Bun.$`bun run build.ts`
      .cwd(root)
      .quiet()
      .nothrow();
    expect(result.exitCode).toBe(0);
    expect(existsSync(join(distDir, "main.js"))).toBe(true);

    const bundled = readFileSync(join(distDir, "main.js"), "utf8");
    // Obsidian's loader requires CommonJS. Bun emits `module.exports`
    // for CJS targets — pin that the right format reached disk.
    expect(bundled).toContain("module.exports");
  });

  test("copies manifest.json into dist for the release zip", async () => {
    const result = await Bun.$`bun run build.ts`
      .cwd(root)
      .quiet()
      .nothrow();
    expect(result.exitCode).toBe(0);
    expect(existsSync(join(distDir, "manifest.json"))).toBe(true);
    const manifest = JSON.parse(
      readFileSync(join(distDir, "manifest.json"), "utf8"),
    );
    // Manifest staged unmodified — the release zip ships exactly what
    // editors/obsidian/manifest.json declares.
    expect(manifest.id).toBe("mdsmith");
    expect(manifest.isDesktopOnly).toBe(true);
  });

  test("copies styles.css into dist for the release zip", async () => {
    const result = await Bun.$`bun run build.ts`
      .cwd(root)
      .quiet()
      .nothrow();
    expect(result.exitCode).toBe(0);
    expect(existsSync(join(distDir, "styles.css"))).toBe(true);
  });
});
