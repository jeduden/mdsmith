// Unit tests for binary resolution logic.
// The extension bundles a cross-platform mdsmith binary that works on
// all platforms from a single .vsix install; these tests verify the
// fallback behavior when bundling fails.

import { afterEach, beforeEach, describe, expect, mock, test } from "bun:test";
import { existsSync } from "node:fs";
import { join } from "node:path";
import { resolveBinary } from "./binary";

// Mock existsSync so tests can inject arbitrary "file exists" results
// without touching the real filesystem. beforeEach saves the original,
// tests replace it with a mock, afterEach restores it.
const originalExistsSync = existsSync;
let mockExistsSync: typeof existsSync;

beforeEach(() => {
  mockExistsSync = mock(() => false);
  (global as any).existsSync = mockExistsSync;
});

afterEach(() => {
  (global as any).existsSync = originalExistsSync;
});

describe("resolveBinary", () => {
  test("returns custom path unchanged when user specifies non-default", () => {
    const result = resolveBinary("/custom/path/to/mdsmith", "/ext");
    expect(result).toBe("/custom/path/to/mdsmith");
    // Should not even attempt to check for bundled binary
    expect(mockExistsSync).not.toHaveBeenCalled();
  });

  test("returns bundled binary when default path and bundled exists (Unix)", () => {
    const originalPlatform = process.platform;
    Object.defineProperty(process, "platform", { value: "linux", writable: true });

    const extensionPath = "/ext";
    const bundledPath = join(extensionPath, "node_modules", ".bin", "mdsmith");

    // Mock: bundled binary exists
    mockExistsSync = mock((path) => path === bundledPath) as any;
    (global as any).existsSync = mockExistsSync;

    const result = resolveBinary("mdsmith", extensionPath);
    expect(result).toBe(bundledPath);
    expect(mockExistsSync).toHaveBeenCalledWith(bundledPath);

    Object.defineProperty(process, "platform", { value: originalPlatform, writable: true });
  });

  test("returns bundled binary when default path and bundled exists (Windows)", () => {
    const originalPlatform = process.platform;
    Object.defineProperty(process, "platform", { value: "win32", writable: true });

    const extensionPath = "/ext";
    const bundledPath = join(extensionPath, "node_modules", ".bin", "mdsmith.cmd");

    // Mock: bundled binary exists
    mockExistsSync = mock((path) => path === bundledPath) as any;
    (global as any).existsSync = mockExistsSync;

    const result = resolveBinary("mdsmith", extensionPath);
    expect(result).toBe(bundledPath);
    expect(mockExistsSync).toHaveBeenCalledWith(bundledPath);

    Object.defineProperty(process, "platform", { value: originalPlatform, writable: true });
  });

  test("falls back to default path when bundled binary does not exist", () => {
    // Mock: no bundled binary
    mockExistsSync = mock(() => false);
    (global as any).existsSync = mockExistsSync;

    const result = resolveBinary("mdsmith", "/ext");
    expect(result).toBe("mdsmith");
    // Should have checked for bundled binary
    expect(mockExistsSync).toHaveBeenCalled();
  });

  test("returns custom bare name unchanged", () => {
    const result = resolveBinary("my-mdsmith-fork", "/ext");
    expect(result).toBe("my-mdsmith-fork");
    // Should not check for bundled binary when not the default
    expect(mockExistsSync).not.toHaveBeenCalled();
  });
});
