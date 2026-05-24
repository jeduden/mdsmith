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
import { type CliShim, findBinaryCandidates, resolveBinary } from "./binary";

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

describe("findBinaryCandidates", () => {
  // Used by the LSP-start failure path to tell the user where else
  // mdsmith was found on this machine, so a stale custom mdsmith.path
  // is recoverable without guessing.

  test("reports the bundled host binary when staged", () => {
    const candidates = findBinaryCandidates(EXT, {
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
    const candidates = findBinaryCandidates(EXT, {
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
    const candidates = findBinaryCandidates(EXT, {
      platform: "darwin",
      arch: "arm64",
      fileExists: (p) => present.has(p),
      loadShim: () => canonicalShim,
      pathEnv: "/opt/homebrew/bin:/usr/local/bin",
    });
    expect(candidates).toEqual([{ kind: "path", path: onPath }]);
  });

  test("returns [] when neither bundled nor PATH yields a hit", () => {
    const candidates = findBinaryCandidates(EXT, {
      platform: "linux",
      arch: "x64",
      fileExists: () => false,
      loadShim: () => canonicalShim,
      pathEnv: "/usr/bin:/usr/local/bin",
    });
    expect(candidates).toEqual([]);
  });

  test("splits PATH on ';' and tries PATHEXT extensions on win32", () => {
    // On Windows, mdsmith.exe lives next to PATH entries and the
    // resolver must append PATHEXT to bare "mdsmith" to find it.
    const exe = "C:\\tools\\mdsmith.exe";
    const present = new Set<string>([exe]);
    const candidates = findBinaryCandidates(EXT, {
      platform: "win32",
      arch: "x64",
      fileExists: (p) => present.has(p),
      loadShim: () => canonicalShim,
      pathEnv: "C:\\Windows;C:\\tools",
      // PATHEXT case is preserved through the join, so we test the
      // lowercase form to mirror what shows up in real registries.
      pathExt: ".com;.exe;.bat",
    });
    expect(candidates).toEqual([{ kind: "path", path: exe }]);
  });

  test("ignores PATH entirely when the env var is empty", () => {
    // An empty PATH is a real case in restricted CI runners; the
    // resolver must not invent candidates from nothing.
    const candidates = findBinaryCandidates(EXT, {
      platform: "linux",
      arch: "x64",
      fileExists: () => true,
      loadShim: () => canonicalShim,
      pathEnv: "",
    });
    // fileExists returns true for everything, so the bundled lookup
    // succeeds; the PATH branch must contribute nothing here.
    expect(candidates).toEqual([
      {
        kind: "bundled",
        path: join(cliDir, "@mdsmith", "linux-x64", "bin", "mdsmith"),
      },
    ]);
  });

  test("drops the bundled entry when this platform's slot is empty", () => {
    // Shim staged but only darwin-arm64 binary present while we run
    // as linux-x64. The shim invokes the resolver callback, which
    // throws because the linux-x64 file is missing; the helper must
    // swallow that into a no-op rather than handing back a stale
    // path. Mirrors the existing resolveBinary fallback test.
    const candidates = findBinaryCandidates(EXT, {
      platform: "linux",
      arch: "x64",
      fileExists: bundledTree(["darwin-arm64"]),
      loadShim: () => canonicalShim,
      pathEnv: "",
    });
    expect(candidates).toEqual([]);
  });

  test("survives a corrupt shim and still returns the PATH hit", () => {
    const onPath = "/usr/bin/mdsmith";
    const present = new Set<string>([
      join(cliDir, "mdsmith.js"),
      onPath,
    ]);
    const candidates = findBinaryCandidates(EXT, {
      platform: "linux",
      arch: "x64",
      fileExists: (p) => present.has(p),
      loadShim: () => {
        throw new Error("corrupt shim");
      },
      pathEnv: "/usr/bin",
    });
    expect(candidates).toEqual([{ kind: "path", path: onPath }]);
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
