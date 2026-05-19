// Unit tests for binary resolution.
//
// The extension bundles a binary for every supported platform into
// dist/cli/ and selects the right one at runtime by re-using the
// canonical @mdsmith/cli shim (npm/mdsmith/bin/mdsmith.js) — the same
// code the npm package execs. These tests drive that reuse with a
// faked dist tree and pin the cross-package platform matrix so the
// extension and the npm shim can never drift.

import { describe, expect, mock, test } from "bun:test";
import {
  chmodSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { type CliShim, resolveBinary } from "./binary";

// The real, published resolver. Loaded by path (not a bare import) so
// tsc never pulls a cross-package .js into the extension's rootDir and
// so the test exercises the exact file build.ts copies into the .vsix.
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

const EXT = "/ext";
const cliDir = join(EXT, "dist", "cli");

// bundledTree returns a fileExists fake that reports the shim plus the
// binaries for the given targets as present, everything else absent.
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
    const r = resolveBinary("/custom/mdsmith", EXT, {
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
    const r = resolveBinary("  /opt/mdsmith  ", EXT, {
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
      const r = resolveBinary("mdsmith", EXT, {
        platform,
        arch,
        fileExists: bundledTree([target]),
        loadShim: () => canonicalShim,
        makeExecutable: (p) => made.push(p),
      });
      expect(r).toBe(expected);
      // The resolved binary is marked executable (vsce's zip drops
      // the +x bit on extraction).
      expect(made).toEqual([expected]);
    });
  }

  test("empty mdsmith.path resolves to the bundled binary, never \"\"", () => {
    // Regression: a workspace settings.json with "mdsmith.path": ""
    // used to short-circuit to "" and crash the LanguageClient with
    // 'Unsupported server configuration { command: "" }'.
    const r = resolveBinary("", EXT, {
      platform: "linux",
      arch: "x64",
      fileExists: bundledTree(["linux-x64"]),
      loadShim: () => canonicalShim,
    });
    expect(r).toBe(join(cliDir, "@mdsmith", "linux-x64", "bin", "mdsmith"));
  });

  test("whitespace-only mdsmith.path behaves like the default", () => {
    const r = resolveBinary("   ", EXT, {
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
    const r = resolveBinary("mdsmith", EXT, {
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
    // Shim present, but only darwin bundled while we run on linux.
    const r = resolveBinary("", EXT, {
      platform: "linux",
      arch: "x64",
      fileExists: bundledTree(["darwin-arm64"]),
      loadShim: () => canonicalShim,
    });
    expect(r).toBe("mdsmith");
  });

  test("falls back to PATH on a platform the shim does not support", () => {
    const r = resolveBinary("mdsmith", EXT, {
      platform: "freebsd",
      arch: "x64",
      fileExists: bundledTree(allTargets),
      loadShim: () => canonicalShim,
    });
    expect(r).toBe("mdsmith");
  });

  test("falls back to PATH when the shim throws on load", () => {
    const r = resolveBinary("mdsmith", EXT, {
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
  test("the extension targets exactly the npm shim's platforms", () => {
    expect(Object.keys(canonicalShim.PLATFORM_PACKAGES).sort()).toEqual(
      [...allTargets].sort(),
    );
  });

  test("every npm platform resolves through the shared shim", () => {
    for (const target of Object.keys(canonicalShim.PLATFORM_PACKAGES)) {
      const [platform, arch] = platformArch[target];
      const exe = platform === "win32" ? "mdsmith.exe" : "mdsmith";
      const r = resolveBinary("mdsmith", EXT, {
        platform,
        arch,
        fileExists: bundledTree([target]),
        loadShim: () => canonicalShim,
      });
      expect(r).toBe(join(cliDir, "@mdsmith", target, "bin", exe));
    }
  });
});

describe("resolveBinary — production defaults (no injected seams)", () => {
  // Exercises the real loadShim (require off disk), real fileExists,
  // and real makeExecutable (chmod) against a temp dist/cli/ that
  // carries the canonical shim verbatim — the same tree build.ts
  // stages. Other tests inject all three seams, so this is the only
  // coverage of loadShimFromDisk / chmodExecutable / the
  // process.platform|arch fallbacks.
  test("loads the bundled shim from disk and resolves the host binary", () => {
    const host = canonicalShim.PLATFORM_PACKAGES[
      `${process.platform}-${process.arch}`
    ] as string | undefined;

    const ext = mkdtempSync(join(tmpdir(), "mdsmith-bin-"));
    try {
      const dist = join(ext, "dist", "cli");
      mkdirSync(dist, { recursive: true });
      writeFileSync(
        join(dist, "mdsmith.js"),
        readFileSync(canonicalShimPath),
      );

      if (host) {
        const exe = host.endsWith("win32-x64") ? "mdsmith.exe" : "mdsmith";
        const binDir = join(dist, host, "bin");
        mkdirSync(binDir, { recursive: true });
        const binPath = join(binDir, exe);
        writeFileSync(binPath, "#!/bin/sh\n");
        chmodSync(binPath, 0o644);

        // No deps: real require, real existsSync, real chmod, and
        // the process.platform/arch fallbacks.
        const r = resolveBinary("", ext);
        expect(r).toBe(binPath);
      } else {
        // Unsupported host: loadShimFromDisk still runs, the shim
        // throws, and we fall back to PATH (never "").
        const r = resolveBinary("mdsmith", ext);
        expect(r).toBe("mdsmith");
      }
    } finally {
      rmSync(ext, { recursive: true, force: true });
    }
  });
});
