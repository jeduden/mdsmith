// Handler logic for mdsmith.init: runs "mdsmith init" in the workspace root.
// Trust-gated because it creates (or overwrites) .mdsmith.yml.

import { SpawnFn, defaultSpawn } from "./runner";

export interface InitHandlerDeps {
  binary: string;
  workspaceRoot: string | undefined;
  isTrusted: () => boolean;
  showInfo: (msg: string, ...buttons: string[]) => Promise<string | undefined>;
  showError: (msg: string) => Promise<void>;
  appendOutput: (text: string) => void;
  showOutput: () => void;
  spawn?: SpawnFn;
}

export async function runInit(deps: InitHandlerDeps): Promise<void> {
  if (!deps.workspaceRoot) {
    await deps.showError("mdsmith: Initialize config requires an open workspace folder.");
    return;
  }

  if (!deps.isTrusted()) {
    await deps.showError("mdsmith: Initialize config requires a trusted workspace.");
    return;
  }

  const spawnFn = deps.spawn ?? defaultSpawn;
  const result = await spawnFn(deps.binary, ["init"], deps.workspaceRoot);

  if (result.stderr) deps.appendOutput(result.stderr);
  if (result.stdout) deps.appendOutput(result.stdout);

  if (result.exitCode !== 0) {
    const choice = await deps.showInfo(
      `mdsmith init failed (exit ${result.exitCode}). See output for details.`,
      "Show Output"
    );
    if (choice === "Show Output") deps.showOutput();
    return;
  }

  const choice = await deps.showInfo("mdsmith: .mdsmith.yml created.", "Show Output");
  if (choice === "Show Output") deps.showOutput();
}
