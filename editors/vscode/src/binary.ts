// Binary resolution logic for the mdsmith extension.
// The extension bundles cross-platform mdsmith binaries from npm packages
// into dist/bin/ during the build step. This module resolves the correct
// platform binary when the user leaves the default "mdsmith" path, falling
// back to PATH if bundling failed or binaries are unavailable.

import { existsSync } from "node:fs";
import { join } from "node:path";

// resolveBinary returns the path to the mdsmith binary. When the
// configured path is the bare string "mdsmith", it first checks for
// bundled binaries in dist/bin/ (copied there by build.ts from the
// @mdsmith/* npm packages). Platform-specific binaries are named like
// "linux-x64-mdsmith", "win32-x64-mdsmith.exe", etc. If the bundled
// binary exists, return its absolute path. Otherwise return the
// configured path unchanged so the LanguageClient resolves it against
// PATH (fallback for dev builds or when optional deps failed to install).
//
// Cross-platform bundling: The build script copies binaries from ALL
// @mdsmith/* platform packages (linux-x64, darwin-arm64, win32-x64, etc.)
// into dist/bin/. This works even with `vsce package --no-dependencies`
// because dist/ is included in the .vsix. At runtime, this function
// selects the binary matching the user's OS+arch.
//
// The extensionPath should be the vscode.ExtensionContext.extensionPath
// (the directory containing package.json and dist/).
//
// The optional platform, arch, and fileExists parameters are for testing;
// in production they default to process.platform, process.arch, and fs.existsSync.
export function resolveBinary(
  configuredPath: string,
  extensionPath: string,
  platform: string = process.platform,
  arch: string = process.arch,
  fileExists: (path: string) => boolean = existsSync
): string {
  // If the user specified a custom path (not the bare "mdsmith"),
  // honor it exactly — they know what they want.
  if (configuredPath !== "mdsmith") {
    return configuredPath;
  }

  // The user left the default "mdsmith". Check for bundled binaries
  // in dist/bin/. The build script copies them there with names like
  // "linux-x64-mdsmith", "win32-x64-mdsmith.exe".

  // Map Node's process.platform and process.arch to our package names
  const platformArch = `${platform}-${arch}`;
  const binaryName = platform === "win32" ? "mdsmith.exe" : "mdsmith";
  const bundledBinary = join(
    extensionPath,
    "dist",
    "bin",
    `${platformArch}-${binaryName}`
  );

  if (fileExists(bundledBinary)) {
    return bundledBinary;
  }

  // The bundled binary does not exist (build step didn't run, or
  // optional dependencies weren't installed). Fall back to the bare
  // "mdsmith" string so the LanguageClient resolves it against PATH.
  return configuredPath;
}
