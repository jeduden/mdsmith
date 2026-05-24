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
import { join, posix as posixPath, win32 as win32Path } from "node:path";

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
  // pathEnv / pathExt are only consulted by findBinaryCandidates.
  // Defaulted to process.env so production code never has to plumb
  // them; tests pin them so the candidate set is deterministic.
  pathEnv?: string;
  pathExt?: string;
}

// BinaryCandidate is one location where the `mdsmith` executable
// actually exists on this host, alongside what kind of source it
// came from. The LSP-start failure path uses these to tell the user
// where else they could point mdsmith.path.
export interface BinaryCandidate {
  kind: "bundled" | "path";
  path: string;
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

// resolveBundledBinary returns the bundled binary path for `platform`
// + `arch` when the .vsix actually ships one, or undefined otherwise.
// Shared between resolveBinary's happy path and findBinaryCandidates;
// returning undefined (rather than throwing or falling back to PATH)
// lets each caller pick its own fallback.
function resolveBundledBinary(
  extensionPath: string,
  platform: string,
  arch: string,
  fileExists: (p: string) => boolean,
  loadShim: (shimPath: string) => CliShim,
): string | undefined {
  const cliDir = join(extensionPath, "dist", "cli");
  const shimPath = join(cliDir, "mdsmith.js");
  if (!fileExists(shimPath)) return undefined;
  try {
    const shim = loadShim(shimPath);
    // The resolver callback enforces existence before handing the
    // path back to the shim, so a successful return is guaranteed
    // to be a real file — no second fileExists check needed.
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
// stays short — the LSP only ever spawns one binary, and showing
// every shadowed copy would just be noise in the error message.
//
// Uses the platform-specific `path` join so a win32 lookup produces
// `C:\\dir\\mdsmith.exe` even when the test (or extension host) is
// running on posix, instead of mixing separators.
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

// findBinaryCandidates lists the mdsmith binaries that actually
// exist on this host: the .vsix-bundled one for the current
// platform (if staged) and the first `mdsmith` on PATH (if any).
// Returned in priority order — bundled first, then PATH — matching
// what resolveBinary would prefer if mdsmith.path were cleared.
//
// Used by the LSP-start failure path so a stale custom mdsmith.path
// surfaces alongside the alternatives the user could switch to.
export function findBinaryCandidates(
  extensionPath: string,
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
    extensionPath,
    platform,
    arch,
    fileExists,
    loadShim,
  );
  if (bundled) out.push({ kind: "bundled", path: bundled });

  const onPath = whichBinary("mdsmith", platform, pathEnv, pathExt, fileExists);
  if (onPath) out.push({ kind: "path", path: onPath });

  return out;
}
