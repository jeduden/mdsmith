// Binary resolution for the mdsmith extension.
//
// The .vsix bundles a prebuilt mdsmith binary for every supported
// platform under dist/cli/, laid out exactly like the @mdsmith/cli
// npm package's node_modules tree:
//
//   dist/cli/mdsmith.js                       (the @mdsmith/cli shim)
//   dist/cli/@mdsmith/<target>/bin/mdsmith[.exe]
//
// Rather than re-implement which-binary-for-which-host, we load that
// bundled shim and call its exported resolveBinary() — the same code
// `npx @mdsmith/cli` execs. One source of truth for the platform
// matrix means the extension and the npm package cannot drift.
//
// build.ts copies npm/mdsmith/bin/mdsmith.js verbatim and stages all
// five platform binaries; see editors/vscode/build.ts.

import { chmodSync, existsSync } from "node:fs";
import { join } from "node:path";

// CliShim is the structural subset of npm/mdsmith/bin/mdsmith.js this
// module consumes. resolveBinary(platform, arch, resolve) maps the
// host to its @mdsmith/<target> package, then hands the package-
// relative path to `resolve`, which returns an absolute path or
// throws (npm's require.resolve, or our bundled-tree lookup).
export interface CliShim {
  resolveBinary(
    platform: string,
    arch: string,
    resolve: (id: string) => string,
  ): string;
}

// ResolveDeps are seams for tests; production uses the node defaults.
export interface ResolveDeps {
  platform?: string;
  arch?: string;
  fileExists?: (p: string) => boolean;
  loadShim?: (shimPath: string) => CliShim;
  makeExecutable?: (p: string) => void;
}

function loadShimFromDisk(shimPath: string): CliShim {
  // Indirect through a variable so the bundler treats this as a
  // runtime require of a file on disk (dist/cli/mdsmith.js) instead
  // of trying to inline a module that does not exist at bundle time.
  const req = require as unknown as (id: string) => unknown;
  return req(shimPath) as CliShim;
}

function chmodExecutable(p: string): void {
  try {
    chmodSync(p, 0o755);
  } catch {
    // win32 has no +x bit and a read-only extension dir is fine —
    // the binary is already executable in both cases.
  }
}

// resolveBinary returns the command the LanguageClient should spawn.
//
// A user-supplied mdsmith.path (anything other than the bare default
// "mdsmith", once trimmed) is honored verbatim. Empty string,
// whitespace, and the default all mean "use the bundled binary": we
// ask the shared shim for this host's binary and return its absolute
// path. If the shim is absent (dev build), this platform was not
// staged, or the host is unsupported, we fall back to the bare
// "mdsmith" so the LanguageClient resolves it against PATH.
//
// The return value is never empty — an empty command crashes
// vscode-languageclient with the opaque "Unsupported server
// configuration { command: \"\" }" error.
export function resolveBinary(
  configuredPath: string,
  extensionPath: string,
  deps: ResolveDeps = {},
): string {
  const platform = deps.platform ?? process.platform;
  const arch = deps.arch ?? process.arch;
  const fileExists = deps.fileExists ?? existsSync;
  const loadShim = deps.loadShim ?? loadShimFromDisk;
  const makeExecutable = deps.makeExecutable ?? chmodExecutable;

  const trimmed = (configuredPath ?? "").trim();
  if (trimmed && trimmed !== "mdsmith") {
    return trimmed;
  }

  const cliDir = join(extensionPath, "dist", "cli");
  const shimPath = join(cliDir, "mdsmith.js");
  if (fileExists(shimPath)) {
    try {
      const shim = loadShim(shimPath);
      const bundled = shim.resolveBinary(platform, arch, (id) => {
        const p = join(cliDir, id);
        if (!fileExists(p)) {
          // Mirror require.resolve's miss so the shim raises its
          // typed MDSMITH_PLATFORM_PACKAGE_MISSING rather than
          // handing back a path that is not there.
          throw new Error(`mdsmith: bundled binary not found: ${p}`);
        }
        return p;
      });
      makeExecutable(bundled);
      return bundled;
    } catch {
      // Unsupported host, missing platform, or a corrupt shim —
      // fall through to PATH resolution below.
    }
  }
  return "mdsmith";
}
