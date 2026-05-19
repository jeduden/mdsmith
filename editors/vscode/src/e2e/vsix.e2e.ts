// End-to-end: build a real .vsix and prove the @mdsmith/cli package
// and the VS Code extension work together on every supported
// platform.
//
// This is heavy (it runs `go build`, `build.ts`, and `vsce`), so it
// is gated behind MDSMITH_VSIX_E2E=1 and skipped by the fast unit
// `bun test` run. CI runs it in a dedicated step that has Go.
//
// It exercises the whole chain the user reported broken:
//   1. build.ts stages the canonical @mdsmith/cli shim verbatim plus
//      a binary slot for all five platforms into dist/cli/.
//   2. `vsce package` actually ships every one of those files.
//   3. The extension's resolveBinary(), re-using the bundled shim,
//      finds and runs this host's binary — and an empty mdsmith.path
//      no longer yields command:"" (the reported crash).

import { describe, expect, test } from "bun:test";
import { spawnSync } from "node:child_process";
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { resolveBinary } from "../binary";

const RUN = process.env.MDSMITH_VSIX_E2E === "1";

const extDir = join(__dirname, "..", "..");
const repoRoot = join(extDir, "..", "..");

const TARGETS = [
  { target: "linux-x64", exe: "mdsmith" },
  { target: "linux-arm64", exe: "mdsmith" },
  { target: "darwin-x64", exe: "mdsmith" },
  { target: "darwin-arm64", exe: "mdsmith" },
  { target: "win32-x64", exe: "mdsmith.exe" },
];

function hostTarget(): { target: string; exe: string } | undefined {
  const want = `${process.platform}-${process.arch}`;
  return TARGETS.find((t) => t.target === want);
}

function run(
  cmd: string,
  args: string[],
  cwd: string,
  env?: Record<string, string>,
) {
  const r = spawnSync(cmd, args, {
    cwd,
    encoding: "utf8",
    env: { ...process.env, ...env },
  });
  return r;
}

describe.skipIf(!RUN)("vsix e2e", () => {
  test(
    "a real .vsix bundles the shim + all platforms and the host binary runs",
    () => {
      const host = hostTarget();
      // Every CI runner this job uses is a supported host; bail
      // loudly rather than passing vacuously somewhere unexpected.
      expect(host).toBeDefined();
      if (!host) return;

      const work = mkdtempSync(join(tmpdir(), "mdsmith-vsix-"));
      try {
        // 1. Stage a full platform set: a real binary for this host,
        //    placeholders for the rest (the release builds all five
        //    via `mdsmith-release build-npm`; here we only need to
        //    prove the staging + packaging ships every slot).
        const stage = join(work, "platforms");
        for (const { target, exe } of TARGETS) {
          const dir = join(stage, target, "bin");
          mkdirSync(dir, { recursive: true });
          const dest = join(dir, exe);
          if (target === host.target) {
            const built = run(
              "go",
              ["build", "-o", dest, "./cmd/mdsmith"],
              repoRoot,
            );
            expect(built.status).toBe(0);
            chmodSync(dest, 0o755);
          } else {
            writeFileSync(dest, "placeholder\n");
          }
        }

        // 2. Build the extension against the full stage.
        const built = run("bun", ["run", "build.ts", "--production"], extDir, {
          MDSMITH_VSIX_PLATFORM_DIR: stage,
        });
        expect(built.status).toBe(0);

        const cliDir = join(extDir, "dist", "cli");

        // The shim is copied verbatim — same code `npx @mdsmith/cli`
        // execs, so the platform matrix cannot drift.
        const shippedShim = readFileSync(join(cliDir, "mdsmith.js"));
        const canonicalShim = readFileSync(
          join(repoRoot, "npm", "mdsmith", "bin", "mdsmith.js"),
        );
        expect(shippedShim.equals(canonicalShim)).toBe(true);

        for (const { target, exe } of TARGETS) {
          expect(
            existsSync(join(cliDir, "@mdsmith", target, "bin", exe)),
          ).toBe(true);
        }

        // 3. Package a real .vsix and confirm vsce actually ships
        //    every staged file (not just that they sit in dist/).
        const vsix = join(work, "mdsmith-e2e.vsix");
        const pkg = run(
          "bunx",
          [
            "--bun",
            "@vscode/vsce",
            "package",
            "--no-dependencies",
            "--out",
            vsix,
          ],
          extDir,
        );
        expect(pkg.status).toBe(0);
        expect(existsSync(vsix)).toBe(true);

        const ls = run(
          "bunx",
          ["--bun", "@vscode/vsce", "ls", "--no-dependencies"],
          extDir,
        );
        expect(ls.status).toBe(0);
        expect(ls.stdout).toContain("dist/cli/mdsmith.js");
        for (const { target, exe } of TARGETS) {
          expect(ls.stdout).toContain(
            `dist/cli/@mdsmith/${target}/bin/${exe}`,
          );
        }

        // 4. Re-use the bundled shim exactly like the extension does
        //    and run the resolved host binary.
        // eslint-disable-next-line @typescript-eslint/no-var-requires
        const shim = require(join(cliDir, "mdsmith.js")) as {
          resolveBinary(
            p: string,
            a: string,
            r: (id: string) => string,
          ): string;
        };
        const bin = shim.resolveBinary(
          process.platform,
          process.arch,
          (id) => join(cliDir, id),
        );
        const ver = run(bin, ["version"], repoRoot);
        expect(ver.status).toBe(0);
        expect(ver.stdout.toLowerCase()).toContain("mdsmith");

        // 5. The reported crash: an empty mdsmith.path must resolve
        //    to the bundled host binary, never command:"".
        const resolved = resolveBinary("", extDir);
        expect(resolved).toBe(
          join(cliDir, "@mdsmith", host.target, "bin", host.exe),
        );
      } finally {
        rmSync(work, { recursive: true, force: true });
      }
    },
    240_000,
  );
});
