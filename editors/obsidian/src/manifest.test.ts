// manifest.json contract.
//
// Obsidian reads manifest.json to register the plugin. Plan 217 fixes
// two hard requirements: the plugin must NOT set isDesktopOnly (mobile
// must load it, since the WASM runtime works in the sandboxed WebView),
// and the id/name must match the VS Code extension's so the two
// surfaces present as one product.

import { describe, expect, test } from "bun:test";
import { join } from "node:path";

const manifest = JSON.parse(
  await Bun.file(join(import.meta.dir, "..", "manifest.json")).text(),
) as Record<string, unknown>;

describe("manifest.json", () => {
  test("does not set isDesktopOnly (mobile must load the plugin)", () => {
    // The WASM runtime sidesteps the mobile sandbox's bans on
    // subprocess spawning and native binary loading, so the plugin
    // runs on iOS/iPadOS/Android. Setting isDesktopOnly would hide it
    // there — the one thing plan 217 forbids.
    expect(manifest.isDesktopOnly).toBeUndefined();
  });

  test("declares the required Obsidian manifest fields", () => {
    expect(manifest.id).toBe("mdsmith");
    expect(manifest.name).toBe("mdsmith");
    expect(typeof manifest.version).toBe("string");
    expect(typeof manifest.minAppVersion).toBe("string");
    expect(typeof manifest.description).toBe("string");
    expect(manifest.author).toBe("jeduden");
  });
});
