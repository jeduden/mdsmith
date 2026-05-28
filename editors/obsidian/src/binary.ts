// Binary resolution for the Obsidian plugin.
//
// The release zip bundles a prebuilt mdsmith binary for every
// supported platform under `cli/` (laid out exactly like the @mdsmith/
// cli npm package's node_modules tree) plus the canonical shim:
//
//   <plugin>/cli/mdsmith.js                       (the @mdsmith/cli shim)
//   <plugin>/cli/@mdsmith/<target>/bin/mdsmith[.exe]
//
// Rather than re-implement which-binary-for-which-host, we load the
// bundled shim and call its exported resolveBinary() — the same code
// `npx @mdsmith/cli` execs and the same code the VS Code extension
// uses. One source of truth for the platform matrix means the
// extension, the npm package, and the Obsidian plugin cannot drift.
//
// `build.ts` copies npm/mdsmith/bin/mdsmith.js verbatim and stages
// every available platform binary; see editors/obsidian/build.ts.

import { chmodSync, existsSync } from "node:fs";
import { join, posix as posixPath, win32 as win32Path } from "node:path";

// CliShim is the structural subset of npm/mdsmith/bin/mdsmith.js this
// module consumes. resolveBinary(platform, arch, resolve) maps the
// host to its @mdsmith/<target> package, then hands the package-
// relative path to `resolve`, which returns an absolute path or
// throws.
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
  pathEnv?: string;
  pathExt?: string;
}

// BinaryCandidate is one location where the `mdsmith` executable
// actually exists on this host, alongside what kind of source it
// came from. The startup failure path uses these to tell the user
// where else they could point `binaryPath`.
export interface BinaryCandidate {
  kind: "bundled" | "path";
  path: string;
}

function loadShimFromDisk(shimPath: string): CliShim {
  // Indirect through a variable so the bundler treats this as a
  // runtime require of a file on disk (cli/mdsmith.js) instead of
  // trying to inline a module that does not exist at bundle time.
  const req = require as unknown as (id: string) => unknown;
  return req(shimPath) as CliShim;
}

function chmodExecutable(p: string): void {
  try {
    chmodSync(p, 0o755);
  } catch {
    // Windows has no +x bit and a read-only plugin dir is fine —
    // the binary is already executable in both cases.
  }
}

// resolveBinary returns the command the plugin should spawn.
//
// A non-empty `binaryPath` setting (once trimmed) is honored verbatim;
// the user pinned a specific build. An empty setting resolves through
// the bundled shim to the host's binary. If the shim is absent (dev
// build), this platform was not staged, or the host is unsupported,
// we fall back to the bare "mdsmith" so the spawn resolves it against
// $PATH. The return value is never empty.
export function resolveBinary(
  configuredPath: string,
  pluginPath: string,
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

  const cliDir = join(pluginPath, "cli");
  const shimPath = join(cliDir, "mdsmith.js");
  if (fileExists(shimPath)) {
    try {
      const shim = loadShim(shimPath);
      const bundled = shim.resolveBinary(platform, arch, (id) => {
        const p = join(cliDir, id);
        if (!fileExists(p)) {
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

// resolveBundledBinary returns the bundled binary path for `platform`
// + `arch` when the plugin actually ships one, or undefined otherwise.
// Shared between resolveBinary's happy path and findBinaryCandidates.
function resolveBundledBinary(
  pluginPath: string,
  platform: string,
  arch: string,
  fileExists: (p: string) => boolean,
  loadShim: (shimPath: string) => CliShim,
): string | undefined {
  const cliDir = join(pluginPath, "cli");
  const shimPath = join(cliDir, "mdsmith.js");
  if (!fileExists(shimPath)) return undefined;
  try {
    const shim = loadShim(shimPath);
    return shim.resolveBinary(platform, arch, (id) => {
      const p = join(cliDir, id);
      if (!fileExists(p)) {
        throw new Error(`mdsmith: bundled binary not found: ${p}`);
      }
      return p;
    });
  } catch {
    return undefined;
  }
}

// whichBinary mirrors `which mdsmith` / `where mdsmith` against a
// supplied PATH string. Returns the first hit so the candidate list
// stays short — the LSP only ever spawns one binary.
function whichBinary(
  name: string,
  platform: string,
  pathEnv: string,
  pathExt: string,
  fileExists: (p: string) => boolean,
): string | undefined {
  if (!pathEnv) return undefined;
  const isWin = platform === "win32";
  const sep = isWin ? ";" : ":";
  const joiner = isWin ? win32Path.join : posixPath.join;
  const exts = isWin
    ? pathExt.split(";").filter((e) => e.length > 0)
    : [""];
  for (const dir of pathEnv.split(sep)) {
    if (!dir) continue;
    for (const ext of exts) {
      const candidate = joiner(dir, name + ext);
      if (fileExists(candidate)) return candidate;
    }
  }
  return undefined;
}

// findBinaryCandidates lists the mdsmith binaries that actually exist
// on this host: the bundled one for the current platform (if staged)
// and the first `mdsmith` on PATH (if any). Returned in priority
// order — bundled first, then PATH — matching what resolveBinary would
// prefer with an empty `binaryPath`. Used by the startup failure
// notice so a stale `binaryPath` override surfaces alongside the
// alternatives the user could switch to.
export function findBinaryCandidates(
  pluginPath: string,
  deps: ResolveDeps = {},
): BinaryCandidate[] {
  const platform = deps.platform ?? process.platform;
  const arch = deps.arch ?? process.arch;
  const fileExists = deps.fileExists ?? existsSync;
  const loadShim = deps.loadShim ?? loadShimFromDisk;
  const pathEnv = deps.pathEnv ?? process.env.PATH ?? "";
  const pathExt =
    deps.pathExt ?? process.env.PATHEXT ?? ".COM;.EXE;.BAT;.CMD";

  const out: BinaryCandidate[] = [];
  const bundled = resolveBundledBinary(
    pluginPath,
    platform,
    arch,
    fileExists,
    loadShim,
  );
  if (bundled) out.push({ kind: "bundled", path: bundled });

  const onPath = whichBinary(
    "mdsmith",
    platform,
    pathEnv,
    pathExt,
    fileExists,
  );
  if (onPath) out.push({ kind: "path", path: onPath });

  return out;
}
