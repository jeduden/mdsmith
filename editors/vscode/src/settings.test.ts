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
        {
          type?: string;
          default?: unknown;
          enum?: string[];
          deprecationMessage?: string;
          markdownDeprecationMessage?: string;
        }
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

  test("mdsmith.fixOnSave is deprecated, pointing to editor.codeActionsOnSave", () => {
    // Fix-on-save now uses VS Code's native editor.codeActionsOnSave (the
    // ESLint model). The setting is kept only as a deprecated alias so an
    // existing user sees an in-Settings hint — VS Code's idiom, the way
    // Prettier deprecates settings, with no runtime prompt — instead of a bare
    // "unknown setting" warning.
    const setting = props["mdsmith.fixOnSave"];
    expect(setting).toBeDefined();
    // Keep the shape stable while the deprecated alias exists, so a careless
    // package.json edit can't silently change the type/default users still see.
    expect(setting.type).toBe("boolean");
    expect(setting.default).toBe(false);
    expect(setting.deprecationMessage).toMatch(/editor\.codeActionsOnSave/);
    expect(setting.markdownDeprecationMessage).toMatch(/source\.fixAll\.mdsmith/);
  });
});
