// Headless VS Code integration runner. Downloads a real VS Code, launches
// it with the built extension loaded against a temp markdown workspace, and
// runs the mocha suite *inside* the extension host (where the `vscode`
// module is live). This is the only harness that exercises the actual
// editor apply path — diagnostics, code actions, and whether previewFix
// routes a fix through the Refactor Preview — rather than just the server's
// JSON. Not part of the default CI gate: run it on demand with
// `bun run test:e2e` (headless under xvfb, e.g. `xvfb-run -a`).
import * as path from "path";
import * as os from "os";
import * as fs from "fs";
import { runTests } from "@vscode/test-electron";

async function main(): Promise<void> {
  try {
    // editors/vscode. runTest.js compiles to out-e2e/runTest.js, so the
    // extension root is one level up.
    const extensionDevelopmentPath = path.resolve(__dirname, "..");
    const extensionTestsPath = path.resolve(__dirname, "suite", "index.js");

    const workspace = fs.mkdtempSync(
      path.join(os.tmpdir(), "mdsmith-vscode-e2e-"),
    );
    const vscodeDir = path.join(workspace, ".vscode");
    fs.mkdirSync(vscodeDir, { recursive: true });

    // Point the extension at the freshly built binary. Per-test settings
    // (mdsmith.previewFix, editor.codeActionsOnSave, …) are set at runtime via
    // the configuration API inside the suite.
    const binary = process.env.MDSMITH_E2E_BINARY || "mdsmith";
    fs.writeFileSync(
      path.join(vscodeDir, "settings.json"),
      JSON.stringify(
        {
          "mdsmith.path": binary,
          "mdsmith.run": "onType",
        },
        null,
        2,
      ),
    );

    // Pin the VS Code build so the regression test is reproducible over time:
    // the default is the latest stable, which drifts and could change save or
    // code-action behaviour out from under the test. Derive it from the
    // extension's engines.vscode floor — the oldest version we support and the
    // minimum with changeAnnotationSupport for previewFix.
    const pkg = JSON.parse(
      fs.readFileSync(
        path.join(extensionDevelopmentPath, "package.json"),
        "utf8",
      ),
    ) as { engines?: { vscode?: string } };
    const vscodeVersion =
      pkg.engines?.vscode?.match(/\d+\.\d+\.\d+/)?.[0] ?? "1.85.0";

    await runTests({
      version: vscodeVersion,
      extensionDevelopmentPath,
      extensionTestsPath,
      launchArgs: [
        workspace,
        "--no-sandbox",
        "--disable-gpu",
        "--disable-updates",
        "--skip-welcome",
        "--skip-release-notes",
        "--disable-workspace-trust",
        "--user-data-dir",
        fs.mkdtempSync(path.join(os.tmpdir(), "mdsmith-vscode-ud-")),
      ],
    });
  } catch (err) {
    console.error("\n=== VS Code e2e failed ===\n", err);
    process.exit(1);
  }
}

void main();
