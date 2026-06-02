import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const pkg = JSON.parse(
  readFileSync(resolve(__dirname, "../package.json"), "utf-8")
) as {
  contributes?: {
    configuration?: {
      properties?: Record<
        string,
        { type?: string; default?: unknown; enum?: string[] }
      >;
    };
  };
};

const props = pkg.contributes?.configuration?.properties ?? {};

describe("package.json settings", () => {
  test("mdsmith.previewFix is a boolean defaulting to false", () => {
    const setting = props["mdsmith.previewFix"];
    expect(setting).toBeDefined();
    expect(setting.type).toBe("boolean");
    expect(setting.default).toBe(false);
  });

  test("mdsmith.run defaults to onType — lint as you type", () => {
    // The shipped default is live linting; onSave and off are opt-in.
    // Keep this in lockstep with the Go server's runMode default
    // (internal/lsp/server.go) so VS Code and bare LSP clients agree.
    const setting = props["mdsmith.run"];
    expect(setting).toBeDefined();
    expect(setting.type).toBe("string");
    expect(setting.default).toBe("onType");
  });

  test("mdsmith.run offers exactly onType, onSave, off", () => {
    const setting = props["mdsmith.run"];
    expect(setting.enum).toEqual(["onType", "onSave", "off"]);
  });

  test("mdsmith.fixOnSave is a boolean defaulting to false", () => {
    const setting = props["mdsmith.fixOnSave"];
    expect(setting).toBeDefined();
    expect(setting.type).toBe("boolean");
    expect(setting.default).toBe(false);
  });
});
