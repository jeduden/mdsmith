// Mocha bootstrap that runs inside the VS Code extension host. Discovers
// every *.itest.js in this directory and runs them. Exported `run` is the
// entry point @vscode/test-electron invokes as extensionTestsPath.
import * as path from "path";
import Mocha from "mocha";
import { glob } from "glob";

export async function run(): Promise<void> {
  const mocha = new Mocha({ ui: "tdd", color: true, timeout: 120000 });
  const testsRoot = __dirname;
  // Sort so suite execution order is deterministic across platforms and
  // filesystems — glob makes no ordering guarantee.
  const files = (await glob("**/*.itest.js", { cwd: testsRoot })).sort();
  for (const f of files) {
    mocha.addFile(path.resolve(testsRoot, f));
  }
  await new Promise<void>((resolve, reject) => {
    mocha.run((failures) => {
      if (failures > 0) {
        reject(new Error(`${failures} test(s) failed`));
      } else {
        resolve();
      }
    });
  });
}
