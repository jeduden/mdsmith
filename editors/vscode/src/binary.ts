// Binary resolution logic for the mdsmith extension.
// The extension bundles platform binaries from npm packages into dist/bin/
// during the build step when available. Due to npm os/cpu constraints on
// the @mdsmith/* packages, only the host platform binary is bundled (typically
// linux-x64 in CI). This module resolves the bundled binary when present,
// falling back to PATH for other platforms or when bundling failed.

import { existsSync } from "node:fs";
import { join } from "node:path";

// resolveBinary returns the path to the mdsmith binary. When the
// configured path is the bare string "mdsmith", it first checks for
// a bundled binary in dist/bin/ (copied there by build.ts from the
// @mdsmith/* npm packages). Platform-specific binaries are named like
// "linux-x64-mdsmith", "win32-x64-mdsmith.exe", etc. If the bundled
// binary exists, return its absolute path. Otherwise return the
// configured path unchanged so the LanguageClient resolves it against
// PATH (fallback for dev builds, non-host platforms, or when optional
// deps failed to install).
//
// Platform bundling limitation: The @mdsmith/* platform packages have
// os/cpu constraints, so npm only installs the package matching the
// build host. This means only one platform binary is bundled per .vsix
// (typically linux-x64 from CI). Other platforms fall back to PATH and
// require manual mdsmith installation.
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
