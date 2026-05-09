// Handler logic for mdsmith.mergeDriver.install: runs
// "mdsmith merge-driver install" after a confirmation dialog.
// The command is trust-gated; the handler re-checks isTrusted() in
// case the workspace trust state changed after registration.

import { SpawnFn, defaultSpawn } from "./runner";

export interface MergeDriverHandlerDeps {
  binary: string;
  workspaceRoot: string | undefined;
  isTrusted: () => boolean;
  confirm: () => Promise<boolean>;
  showInfo: (msg: string, ...buttons: string[]) => Promise<string | undefined>;
  showError: (msg: string) => Promise<void>;
  appendOutput: (text: string) => void;
  showOutput: () => void;
  spawn?: SpawnFn;
}

export async function runMergeDriverInstall(deps: MergeDriverHandlerDeps): Promise<void> {
  if (!deps.isTrusted()) {
    await deps.showError("mdsmith: Install Git merge driver requires a trusted workspace.");
    return;
  }

  const confirmed = await deps.confirm();
  if (!confirmed) return;

  const spawnFn = deps.spawn ?? defaultSpawn;
  const result = await spawnFn(
    deps.binary,
    ["merge-driver", "install"],
    deps.workspaceRoot
  );

  if (result.stderr) deps.appendOutput(result.stderr);
  if (result.stdout) deps.appendOutput(result.stdout);

  if (result.exitCode !== 0) {
    const choice = await deps.showInfo(
      `mdsmith merge-driver install failed (exit ${result.exitCode}). See output for details.`,
      "Show Output"
    );
    if (choice === "Show Output") deps.showOutput();
    return;
  }

  const choice = await deps.showInfo("mdsmith: Git merge driver installed.", "Show Output");
  if (choice === "Show Output") deps.showOutput();
}
