import { describe, expect, test } from "bun:test";
import {
  parseRunStats,
  buildFixNotificationMessage,
  runFixWorkspace,
  type FixStats,
  type FixWorkspaceHandlerDeps
} from "./fix-workspace";

describe("parseRunStats", () => {
  test("parses a well-formed stats line", () => {
    const out = "stats: checked=200 fixed=12 failures=0 unfixed=188\n";
    expect(parseRunStats(out)).toEqual({
      checked: 200,
      fixed: 12,
      failures: 0,
      unfixed: 188,
    });
  });

  test("finds the stats line inside longer stderr output", () => {
    const out = [
      "some/file.md: fixed trailing space",
      "stats: checked=5 fixed=3 failures=0 unfixed=2",
      "",
    ].join("\n");
    expect(parseRunStats(out)).toEqual({
      checked: 5,
      fixed: 3,
      failures: 0,
      unfixed: 2,
    });
  });

  test("returns null when no stats line is present", () => {
    expect(parseRunStats("")).toBeNull();
    expect(parseRunStats("some random output")).toBeNull();
  });
});

describe("buildFixNotificationMessage", () => {
  test("shows fixed-of-checked when no failures", () => {
    const stats: FixStats = { checked: 200, fixed: 12, failures: 0, unfixed: 188 };
    expect(buildFixNotificationMessage(stats)).toBe("Fixed 12 of 200 files");
  });

  test("appends error count when failures > 0", () => {
    const stats: FixStats = { checked: 10, fixed: 3, failures: 2, unfixed: 5 };
    expect(buildFixNotificationMessage(stats)).toBe("Fixed 3 of 10 files (2 errors)");
  });

  test("uses singular 'error' for exactly one failure", () => {
    const stats: FixStats = { checked: 5, fixed: 1, failures: 1, unfixed: 3 };
    expect(buildFixNotificationMessage(stats)).toBe("Fixed 1 of 5 files (1 error)");
  });
});

describe("runFixWorkspace", () => {
  function makeDeps(overrides: Partial<FixWorkspaceHandlerDeps> = {}): {
    deps: FixWorkspaceHandlerDeps;
    infoMessages: Array<{ msg: string; buttons: string[] }>;
    errorMessages: string[];
    output: string[];
  } {
    const infoMessages: Array<{ msg: string; buttons: string[] }> = [];
    const errorMessages: string[] = [];
    const output: string[] = [];
    const deps: FixWorkspaceHandlerDeps = {
      binary: "/usr/bin/mdsmith",
      workspaceRoot: "/repo",
      isTrusted: () => true,
      confirm: async () => true,
      showInfo: async (msg, ...buttons) => {
        infoMessages.push({ msg, buttons });
        return undefined;
      },
      showError: async (msg) => { errorMessages.push(msg); },
      appendOutput: (text) => { output.push(text); },
      showOutput: () => {},
      ...overrides,
    };
    return { deps, infoMessages, errorMessages, output };
  }

  test("shows an error and returns when no workspace folder is open", async () => {
    let spawned = false;
    const { deps, errorMessages } = makeDeps({ workspaceRoot: undefined });
    deps.spawn = async () => { spawned = true; return { stdout: "", stderr: "", exitCode: 0 }; };
    await runFixWorkspace(deps);
    expect(errorMessages).toHaveLength(1);
    expect(errorMessages[0]).toContain("workspace folder");
    expect(spawned).toBe(false);
  });

  test("shows an error and returns when workspace is not trusted", async () => {
    const { deps, errorMessages } = makeDeps({ isTrusted: () => false });
    deps.spawn = async () => ({ stdout: "", stderr: "", exitCode: 0 });
    await runFixWorkspace(deps);
    expect(errorMessages).toHaveLength(1);
    expect(errorMessages[0]).toContain("trusted workspace");
  });

  test("returns without spawning when confirmation is declined", async () => {
    let spawned = false;
    const { deps } = makeDeps({ confirm: async () => false });
    deps.spawn = async () => { spawned = true; return { stdout: "", stderr: "", exitCode: 0 }; };
    await runFixWorkspace(deps);
    expect(spawned).toBe(false);
  });

  test("shows fixed-count notification on success with stats line", async () => {
    const { deps, infoMessages } = makeDeps();
    deps.spawn = async () => ({
      stdout: "",
      stderr: "stats: checked=50 fixed=5 failures=0 unfixed=45\n",
      exitCode: 0,
    });
    await runFixWorkspace(deps);
    expect(infoMessages).toHaveLength(1);
    expect(infoMessages[0].msg).toBe("Fixed 5 of 50 files");
    expect(infoMessages[0].buttons).toContain("Show Output");
  });

  test("falls back to generic message when no stats line", async () => {
    const { deps, infoMessages } = makeDeps();
    deps.spawn = async () => ({ stdout: "", stderr: "", exitCode: 0 });
    await runFixWorkspace(deps);
    expect(infoMessages[0].msg).toBe("mdsmith fix completed.");
  });

  test("shows failure notification and Show Output on non-zero exit", async () => {
    const { deps, infoMessages } = makeDeps();
    deps.spawn = async () => ({ stdout: "", stderr: "error: bad config\n", exitCode: 2 });
    await runFixWorkspace(deps);
    expect(infoMessages).toHaveLength(1);
    expect(infoMessages[0].msg).toContain("failed");
    expect(infoMessages[0].buttons).toContain("Show Output");
  });

  test("appends stderr to output channel", async () => {
    const { deps, output } = makeDeps();
    deps.spawn = async () => ({
      stdout: "",
      stderr: "stats: checked=1 fixed=0 failures=0 unfixed=1\n",
      exitCode: 0,
    });
    await runFixWorkspace(deps);
    expect(output.join("")).toContain("stats:");
  });

  test("passes binary and cwd to spawn", async () => {
    let capturedBinary = "";
    let capturedArgs: string[] = [];
    let capturedCwd: string | undefined;
    const { deps } = makeDeps();
    deps.spawn = async (bin, args, cwd) => {
      capturedBinary = bin;
      capturedArgs = args;
      capturedCwd = cwd;
      return { stdout: "", stderr: "stats: checked=0 fixed=0 failures=0 unfixed=0\n", exitCode: 0 };
    };
    await runFixWorkspace(deps);
    expect(capturedBinary).toBe("/usr/bin/mdsmith");
    expect(capturedArgs).toEqual(["fix", "."]);
    expect(capturedCwd).toBe("/repo");
  });
});
