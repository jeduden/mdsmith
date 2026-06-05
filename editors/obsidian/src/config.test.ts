// Drives config.ts — the vault-root .mdsmith.yml auto-discovery the
// plugin runs when the Config path setting is empty.
//
// The CLI walks up from the working directory to the nearest
// .mdsmith.yml. An Obsidian vault has one root and one lint session over
// it, so the analog is a single config file at the vault root. Obsidian
// hides dotfiles from its vault file API, so discovery reads through the
// DataAdapter (exists + read), which sees them. These tests exercise
// discoverConfigYAML against a fake adapter — no Obsidian host.

import { describe, expect, test } from "bun:test";

import {
  CONFIG_FILE_NAME,
  discoverConfigYAML,
  type ConfigAdapter,
} from "./config";

// makeAdapter builds a ConfigAdapter over an in-memory file map and
// records every read so a test can assert discovery did (or did not)
// touch disk.
function makeAdapter(files: Record<string, string>): ConfigAdapter & {
  reads: string[];
} {
  const reads: string[] = [];
  return {
    reads,
    async exists(path: string): Promise<boolean> {
      return path in files;
    },
    async read(path: string): Promise<string> {
      reads.push(path);
      const v = files[path];
      if (v === undefined) throw new Error(`ENOENT: ${path}`);
      return v;
    },
  };
}

describe("discoverConfigYAML", () => {
  test("returns the .mdsmith.yml text when one sits at the vault root", async () => {
    const adapter = makeAdapter({
      ".mdsmith.yml": "rules:\n  line-length: false\n",
    });
    expect(await discoverConfigYAML(adapter)).toBe(
      "rules:\n  line-length: false\n",
    );
  });

  test('returns "" when the vault has no .mdsmith.yml', async () => {
    const adapter = makeAdapter({ "note.md": "# Hi\n" });
    expect(await discoverConfigYAML(adapter)).toBe("");
  });

  test("does not read when the file is absent (the exists gate)", async () => {
    const adapter = makeAdapter({});
    await discoverConfigYAML(adapter);
    expect(adapter.reads).toEqual([]);
  });

  test('degrades to "" when the file exists but the read fails', async () => {
    // exists() reports the file, but read() rejects — a race with a
    // delete, a permission error. Discovery must fall back to defaults
    // rather than crash the engine bring-up.
    const adapter: ConfigAdapter = {
      async exists(): Promise<boolean> {
        return true;
      },
      async read(): Promise<string> {
        throw new Error("EACCES");
      },
    };
    expect(await discoverConfigYAML(adapter)).toBe("");
  });

  test("honors an explicit candidate name", async () => {
    const adapter = makeAdapter({ "alt.yml": "rules:\n  no-bare-urls: false\n" });
    expect(await discoverConfigYAML(adapter, "alt.yml")).toBe(
      "rules:\n  no-bare-urls: false\n",
    );
  });

  test("looks for the same basename the CLI walks the tree for", () => {
    expect(CONFIG_FILE_NAME).toBe(".mdsmith.yml");
  });
});
