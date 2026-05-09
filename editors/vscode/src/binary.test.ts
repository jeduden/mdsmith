// Unit tests for binary resolution logic.
// The extension bundles cross-platform mdsmith binaries into dist/bin/
// during the build step; these tests verify the resolution logic and
// fallback behavior.

import { describe, expect, mock, test } from "bun:test";
import { join } from "node:path";
import { resolveBinary } from "./binary";

describe("resolveBinary", () => {
  test("returns custom path unchanged when user specifies non-default", () => {
    const fileExists = mock(() => false);
    const result = resolveBinary("/custom/path/to/mdsmith", "/ext", "linux", "x64", fileExists);
    expect(result).toBe("/custom/path/to/mdsmith");
    // Should not even attempt to check for bundled binary
    expect(fileExists).not.toHaveBeenCalled();
  });

  test("returns bundled binary when default path and bundled exists (Linux x64)", () => {
    const extensionPath = "/ext";
    const bundledPath = join(extensionPath, "dist", "bin", "linux-x64-mdsmith");

    // Mock: bundled binary exists
    const fileExists = mock((path) => path === bundledPath);

    const result = resolveBinary("mdsmith", extensionPath, "linux", "x64", fileExists);
    expect(result).toBe(bundledPath);
    expect(fileExists).toHaveBeenCalledWith(bundledPath);
  });

  test("returns bundled binary when default path and bundled exists (macOS arm64)", () => {
    const extensionPath = "/ext";
    const bundledPath = join(extensionPath, "dist", "bin", "darwin-arm64-mdsmith");

    // Mock: bundled binary exists
    const fileExists = mock((path) => path === bundledPath);

    const result = resolveBinary("mdsmith", extensionPath, "darwin", "arm64", fileExists);
    expect(result).toBe(bundledPath);
    expect(fileExists).toHaveBeenCalledWith(bundledPath);
  });

  test("returns bundled binary when default path and bundled exists (Windows x64)", () => {
    const extensionPath = "/ext";
    const bundledPath = join(extensionPath, "dist", "bin", "win32-x64-mdsmith.exe");

    // Mock: bundled binary exists
    const fileExists = mock((path) => path === bundledPath);

    const result = resolveBinary("mdsmith", extensionPath, "win32", "x64", fileExists);
    expect(result).toBe(bundledPath);
    expect(fileExists).toHaveBeenCalledWith(bundledPath);
  });

  test("falls back to default path when bundled binary does not exist", () => {
    // Mock: no bundled binary
    const fileExists = mock(() => false);

    const result = resolveBinary("mdsmith", "/ext", "linux", "x64", fileExists);
    expect(result).toBe("mdsmith");
    // Should have checked for bundled binary
    expect(fileExists).toHaveBeenCalled();
  });

  test("returns custom bare name unchanged", () => {
    const fileExists = mock(() => false);
    const result = resolveBinary("my-mdsmith-fork", "/ext", "linux", "x64", fileExists);
    expect(result).toBe("my-mdsmith-fork");
    // Should not check for bundled binary when not the default
    expect(fileExists).not.toHaveBeenCalled();
  });
});
