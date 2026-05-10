// Handler logic for mdsmith.fixWorkspace: runs "mdsmith fix ." and
// shows a fixed-count notification. The handler re-checks
// workspace.isTrusted before running since the command menu entry is
// also gated with `when: isWorkspaceTrusted`.
//
// The spawn function is injectable so tests can mock it without
// starting a real child process.

import { SpawnFn, defaultSpawn } from "./runner";

export interface FixStats {
  checked: number;
  fixed: number;
  failures: number;
  unfixed: number;
}

// parseRunStats extracts counters from the "stats:" line that
// printRunStats writes to stderr.
// Format: "stats: checked=N fixed=N failures=N unfixed=N"
export function parseRunStats(output: string): FixStats | null {
  const match = output.match(
    /stats:\s+checked=(\d+)\s+fixed=(\d+)\s+failures=(\d+)\s+unfixed=(\d+)/
  );
  if (!match) return null;
  return {
    checked: parseInt(match[1], 10),
    fixed: parseInt(match[2], 10),
    failures: parseInt(match[3], 10),
    unfixed: parseInt(match[4], 10),
  };
}

// buildFixNotificationMessage returns the human-readable summary shown
// after a successful fix run. Exposed for testing.
export function buildFixNotificationMessage(stats: FixStats): string {
  if (stats.failures > 0) {
    return `Fixed ${stats.fixed} of ${stats.checked} files (${stats.failures} error${stats.failures !== 1 ? "s" : ""})`;
  }
  return `Fixed ${stats.fixed} of ${stats.checked} files`;
}

export interface FixWorkspaceHandlerDeps {
  binary: string;
  workspaceRoot: string | undefined;
  configPath?: string;
  isTrusted: () => boolean;
  confirm: () => Promise<boolean>;
  showInfo: (msg: string, ...buttons: string[]) => Promise<string | undefined>;
  showError: (msg: string) => Promise<void>;
  appendOutput: (text: string) => void;
  showOutput: () => void;
  spawn?: SpawnFn;
}

// runFixWorkspace executes the fix command and resolves when complete
// (or returns early when cancelled or on error). Extracted so tests
// can drive it without a VS Code host.
export async function runFixWorkspace(deps: FixWorkspaceHandlerDeps): Promise<void> {
  if (!deps.workspaceRoot) {
    await deps.showError("mdsmith: Fix all Markdown requires an open workspace folder.");
    return;
  }

  if (!deps.isTrusted()) {
    await deps.showError("mdsmith: Fix all Markdown requires a trusted workspace.");
    return;
  }

  const confirmed = await deps.confirm();
  if (!confirmed) return;

  const spawnFn = deps.spawn ?? defaultSpawn;
  const args = deps.configPath
    ? ["fix", ".", "--config", deps.configPath]
    : ["fix", "."];
  const result = await spawnFn(deps.binary, args, deps.workspaceRoot);

  deps.appendOutput(result.stderr);
  if (result.stdout) deps.appendOutput(result.stdout);

  // exit code >= 2 is a hard failure (bad config, binary error, etc.)
  if (result.exitCode >= 2) {
    const choice = await deps.showInfo(
      `mdsmith fix failed (exit ${result.exitCode}). See output for details.`,
      "Show Output"
    );
    if (choice === "Show Output") deps.showOutput();
    return;
  }

  // exit code 0 = all clean, exit code 1 = some issues remain — both show stats
  const stats = parseRunStats(result.stderr);
  const msg = stats
    ? buildFixNotificationMessage(stats)
    : "mdsmith fix completed.";

  const choice = await deps.showInfo(msg, "Show Output");
  if (choice === "Show Output") deps.showOutput();
}
