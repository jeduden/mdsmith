// Binary resolution logic for the mdsmith extension.
// The extension bundles a cross-platform mdsmith binary from npm that
// works on all supported platforms (Linux, macOS, Windows) via a single
// .vsix install. This module resolves the bundled binary when the user
// leaves the default "mdsmith" path, falling back to PATH if bundling
// failed.

import { existsSync } from "node:fs";
import { join } from "node:path";

// resolveBinary returns the path to the mdsmith binary. When the
// configured path is the bare string "mdsmith", it first checks for
// the bundled binary at node_modules/.bin/mdsmith (Unix) or
// node_modules/.bin/mdsmith.cmd (Windows). The @mdsmith/cli npm package
// ships with platform-specific binaries as optional dependencies, so
// a single extension install works on all platforms. If the bundled
// binary exists, return its absolute path. Otherwise return the
// configured path unchanged so the LanguageClient resolves it against
// PATH (fallback for proxy/offline install failures).
//
// The extensionPath should be the vscode.ExtensionContext.extensionPath
// (the directory containing package.json and node_modules/).
export function resolveBinary(configuredPath: string, extensionPath: string): string {
  // If the user specified a custom path (not the bare "mdsmith"),
  // honor it exactly — they know what they want.
  if (configuredPath !== "mdsmith") {
    return configuredPath;
  }

  // The user left the default "mdsmith". Check for the bundled binary
  // from @mdsmith/cli. The npm package ships with platform-specific
  // binaries (linux-x64, darwin-arm64, win32-x64, etc.) as optional
  // dependencies; npm installs the correct one for the current platform
  // during extension install. The bin wrapper lives at
  // node_modules/.bin/mdsmith (Unix) or node_modules/.bin/mdsmith.cmd
  // (Windows). This works across all platforms with a single .vsix.
  const binDir = join(extensionPath, "node_modules", ".bin");
  const unixBin = join(binDir, "mdsmith");
  const winBin = join(binDir, "mdsmith.cmd");

  // Prefer the platform-appropriate wrapper if it exists.
  const candidate = process.platform === "win32" ? winBin : unixBin;
  if (existsSync(candidate)) {
    return candidate;
  }

  // The bundled binary does not exist (optional dependency install
  // failed, or this is a dev build without `bun install`). Fall back
  // to the bare "mdsmith" string so the LanguageClient resolves it
  // against the shell PATH (same as before bundling).
  return configuredPath;
}
