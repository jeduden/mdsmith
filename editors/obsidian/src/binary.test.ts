// Binary resolution mirrors editors/vscode/src/binary.ts: the plugin
// bundles a binary for every supported platform under dist/cli/, laid
// out exactly like the @mdsmith/cli npm package's node_modules tree,
// and selects the right one at runtime by re-using the shared shim
// resolver. The tests drive that reuse against a faked dist tree.

import { describe, expect, mock, test } from "bun:test";
import { join } from "node:path";
import { type CliShim, findBinaryCandidates, resolveBinary } from "./binary";

// The real, published resolver. Loaded by path (not a bare import) so
// tsc never pulls a cross-package .js into the plugin's rootDir and so
// the test exercises the exact file build.ts copies into the release
// zip.
const canonicalShimPath = join(
  __dirname,
  "..",
  "..",
  "..",
  "npm",
  "mdsmith",
  "bin",
  "mdsmith.js",
);
const canonicalShim = require(canonicalShimPath) as CliShim & {
  PLATFORM_PACKAGES: Record<string, string>;
};

const PLUGIN = "/plugin";
const cliDir = join(PLUGIN, "cli");

function bundledTree(targets: string[]): (p: string) => boolean {
  const present = new Set<string>([join(cliDir, "mdsmith.js")]);
  for (const t of targets) {
    const exe = t.startsWith("win32") ? "mdsmith.exe" : "mdsmith";
    present.add(join(cliDir, "@mdsmith", t, "bin", exe));
  }
  return (p: string) => present.has(p);
}

const allTargets = [
  "linux-x64",
  "linux-arm64",
  "darwin-x64",
  "darwin-arm64",
  "win32-x64",
];

const platformArch: Record<string, [string, string]> = {
  "linux-x64": ["linux", "x64"],
  "linux-arm64": ["linux", "arm64"],
  "darwin-x64": ["darwin", "x64"],
  "darwin-arm64": ["darwin", "arm64"],
  "win32-x64": ["win32", "x64"],
};

describe("resolveBinary — custom path", () => {
  test("honors a non-default absolute path unchanged", () => {
    const fileExists = mock(() => false);
    const loadShim = mock(() => canonicalShim);
    const r = resolveBinary("/custom/mdsmith", PLUGIN, {
      platform: "linux",
      arch: "x64",
      fileExists,
      loadShim,
    });
    expect(r).toBe("/custom/mdsmith");
    expect(fileExists).not.toHaveBeenCalled();
    expect(loadShim).not.toHaveBeenCalled();
  });

  test("trims surrounding whitespace from a custom path", () => {
    const r = resolveBinary("  /opt/mdsmith  ", PLUGIN, {
      platform: "linux",
      arch: "x64",
      fileExists: () => false,
      loadShim: () => canonicalShim,
    });
    expect(r).toBe("/opt/mdsmith");
  });
});

describe("resolveBinary — bundled selection via the shared shim", () => {
  for (const target of allTargets) {
    const [platform, arch] = platformArch[target];
    test(`${target} resolves to its bundled binary`, () => {
      const exe = platform === "win32" ? "mdsmith.exe" : "mdsmith";
      const expected = join(cliDir, "@mdsmith", target, "bin", exe);
      const made: string[] = [];
      const r = resolveBinary("", PLUGIN, {
        platform,
        arch,
        fileExists: bundledTree([target]),
        loadShim: () => canonicalShim,
        makeExecutable: (p) => made.push(p),
      });
      expect(r).toBe(expected);
      // The resolved binary is marked executable (zip extraction
      // drops the +x bit on Linux/macOS).
      expect(made).toEqual([expected]);
    });
  }

  test("empty binaryPath resolves to the bundled binary, never \"\"", () => {
    // Mirrors the same regression as the VS Code extension: an empty
    // binaryPath setting must fall through to the bundled binary
    // rather than handing the spawn an empty command.
    const r = resolveBinary("", PLUGIN, {
      platform: "linux",
      arch: "x64",
      fileExists: bundledTree(["linux-x64"]),
      loadShim: () => canonicalShim,
    });
    expect(r).toBe(join(cliDir, "@mdsmith", "linux-x64", "bin", "mdsmith"));
  });

  test("whitespace-only binaryPath behaves like the default", () => {
    const r = resolveBinary("   ", PLUGIN, {
      platform: "darwin",
      arch: "arm64",
      fileExists: bundledTree(["darwin-arm64"]),
      loadShim: () => canonicalShim,
    });
    expect(r).toBe(
      join(cliDir, "@mdsmith", "darwin-arm64", "bin", "mdsmith"),
    );
  });
});

describe("resolveBinary — fallbacks never yield an empty command", () => {
  test("falls back to PATH when the shim is not bundled", () => {
    const r = resolveBinary("", PLUGIN, {
      platform: "linux",
      arch: "x64",
      fileExists: () => false,
      loadShim: () => {
        throw new Error("shim must not load when absent");
      },
    });
    expect(r).toBe("mdsmith");
  });

  test("falls back to PATH when this platform's binary is missing", () => {
    const r = resolveBinary("", PLUGIN, {
      platform: "linux",
      arch: "x64",
      fileExists: bundledTree(["darwin-arm64"]),
      loadShim: () => canonicalShim,
    });
    expect(r).toBe("mdsmith");
  });

  test("falls back to PATH on a platform the shim does not support", () => {
    const r = resolveBinary("", PLUGIN, {
      platform: "freebsd",
      arch: "x64",
      fileExists: bundledTree(allTargets),
      loadShim: () => canonicalShim,
    });
    expect(r).toBe("mdsmith");
  });

  test("falls back to PATH when the shim throws on load", () => {
    const r = resolveBinary("", PLUGIN, {
      platform: "linux",
      arch: "x64",
      fileExists: bundledTree(["linux-x64"]),
      loadShim: () => {
        throw new Error("corrupt shim");
      },
    });
    expect(r).toBe("mdsmith");
  });
});

describe("cross-package platform matrix (drift guard)", () => {
  test("the plugin targets exactly the npm shim's platforms", () => {
    expect(Object.keys(canonicalShim.PLATFORM_PACKAGES).sort()).toEqual(
      [...allTargets].sort(),
    );
  });
});

describe("findBinaryCandidates", () => {
  test("reports the bundled host binary when staged", () => {
    const candidates = findBinaryCandidates(PLUGIN, {
      platform: "linux",
      arch: "x64",
      fileExists: bundledTree(["linux-x64"]),
      loadShim: () => canonicalShim,
      pathEnv: "",
    });
    expect(candidates).toEqual([
      {
        kind: "bundled",
        path: join(cliDir, "@mdsmith", "linux-x64", "bin", "mdsmith"),
      },
    ]);
  });

  test("finds an mdsmith on PATH alongside the bundled binary", () => {
    const onPath = "/usr/local/bin/mdsmith";
    const present = new Set<string>([
      join(cliDir, "mdsmith.js"),
      join(cliDir, "@mdsmith", "linux-x64", "bin", "mdsmith"),
      onPath,
    ]);
    const candidates = findBinaryCandidates(PLUGIN, {
      platform: "linux",
      arch: "x64",
      fileExists: (p) => present.has(p),
      loadShim: () => canonicalShim,
      pathEnv: "/usr/bin:/usr/local/bin",
    });
    expect(candidates).toEqual([
      {
        kind: "bundled",
        path: join(cliDir, "@mdsmith", "linux-x64", "bin", "mdsmith"),
      },
      { kind: "path", path: onPath },
    ]);
  });

  test("returns only the PATH hit when nothing is bundled", () => {
    const onPath = "/opt/homebrew/bin/mdsmith";
    const present = new Set<string>([onPath]);
    const candidates = findBinaryCandidates(PLUGIN, {
      platform: "darwin",
      arch: "arm64",
      fileExists: (p) => present.has(p),
      loadShim: () => canonicalShim,
      pathEnv: "/opt/homebrew/bin:/usr/local/bin",
    });
    expect(candidates).toEqual([{ kind: "path", path: onPath }]);
  });

  test("returns [] when neither bundled nor PATH yields a hit", () => {
    const candidates = findBinaryCandidates(PLUGIN, {
      platform: "linux",
      arch: "x64",
      fileExists: () => false,
      loadShim: () => canonicalShim,
      pathEnv: "/usr/bin:/usr/local/bin",
    });
    expect(candidates).toEqual([]);
  });

  test("splits PATH on ';' and tries PATHEXT extensions on win32", () => {
    const exe = "C:\\tools\\mdsmith.exe";
    const present = new Set<string>([exe]);
    const candidates = findBinaryCandidates(PLUGIN, {
      platform: "win32",
      arch: "x64",
      fileExists: (p) => present.has(p),
      loadShim: () => canonicalShim,
      pathEnv: "C:\\Windows;C:\\tools",
      pathExt: ".com;.exe;.bat",
    });
    expect(candidates).toEqual([{ kind: "path", path: exe }]);
  });
});
