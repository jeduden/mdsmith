// Schema guard for editors/obsidian/manifest.json.
//
// Obsidian reads manifest.json at plugin install time. Required fields
// are spelled out in https://docs.obsidian.md/Reference/Manifest. The
// plugin is desktop-only (it spawns mdsmith via child_process), so
// `isDesktopOnly` must be true — otherwise mobile Obsidian would try to
// load the bundle and crash. The other required fields lock the plugin
// identity (id, name, author, …) so a release ships a manifest the
// catalog UI accepts.

import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const manifestPath = join(__dirname, "..", "manifest.json");
const manifest = JSON.parse(readFileSync(manifestPath, "utf8")) as Record<
  string,
  unknown
>;

describe("editors/obsidian/manifest.json", () => {
  test("declares every field Obsidian's loader requires", () => {
    // Required keys per the Obsidian manifest reference. Missing any
    // of these and the plugin is silently rejected from the catalog
    // load.
    for (const key of [
      "id",
      "name",
      "version",
      "minAppVersion",
      "description",
      "author",
      "isDesktopOnly",
    ]) {
      expect(manifest[key]).toBeDefined();
    }
  });

  test("pins the plugin id to mdsmith so the install path is stable", () => {
    // The plugin lives at <vault>/.obsidian/plugins/mdsmith/ — the id
    // is the directory name. Changing it later would orphan every
    // existing install, so it must be set deliberately from the
    // start.
    expect(manifest.id).toBe("mdsmith");
  });

  test("marks the plugin desktop-only", () => {
    // Mobile Obsidian has no Node and cannot spawn the LSP binary;
    // see plan/215 for the WASM successor. The desktop-only flag
    // stops mobile from loading this bundle and crashing.
    expect(manifest.isDesktopOnly).toBe(true);
  });

  test("uses a 0.0.0-dev version that the release stamp rewrites", () => {
    // The release workflow stamps the real semver before packaging,
    // matching the convention used by editors/vscode/package.json.
    expect(manifest.version).toBe("0.0.0-dev");
  });
});
