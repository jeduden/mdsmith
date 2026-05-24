import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const pkg = JSON.parse(
  readFileSync(resolve(__dirname, "../package.json"), "utf-8")
) as {
  contributes?: {
    configuration?: {
      properties?: Record<string, { type?: string; default?: unknown }>;
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
});
