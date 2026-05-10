import { describe, expect, test } from "bun:test";
import { runMergeDriverInstall, type MergeDriverHandlerDeps } from "./merge-driver";

function makeDeps(overrides: Partial<MergeDriverHandlerDeps> = {}): {
  deps: MergeDriverHandlerDeps;
  infoMessages: Array<{ msg: string; buttons: string[] }>;
  errorMessages: string[];
  output: string[];
} {
  const infoMessages: Array<{ msg: string; buttons: string[] }> = [];
  const errorMessages: string[] = [];
  const output: string[] = [];
  const deps: MergeDriverHandlerDeps = {
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

describe("runMergeDriverInstall", () => {
  test("shows error when no workspace folder is open", async () => {
    let spawned = false;
    const { deps, errorMessages } = makeDeps({ workspaceRoot: undefined });
    deps.spawn = async () => { spawned = true; return { stdout: "", stderr: "", exitCode: 0 }; };
    await runMergeDriverInstall(deps);
    expect(errorMessages).toHaveLength(1);
    expect(errorMessages[0]).toContain("workspace folder");
    expect(spawned).toBe(false);
  });

  test("shows error when workspace is not trusted", async () => {
    let spawned = false;
    const { deps, errorMessages } = makeDeps({ isTrusted: () => false });
    deps.spawn = async () => { spawned = true; return { stdout: "", stderr: "", exitCode: 0 }; };
    await runMergeDriverInstall(deps);
    expect(errorMessages).toHaveLength(1);
    expect(errorMessages[0]).toContain("trusted workspace");
    expect(spawned).toBe(false);
  });

  test("returns without spawning when confirmation is declined", async () => {
    let spawned = false;
    const { deps } = makeDeps({ confirm: async () => false });
    deps.spawn = async () => { spawned = true; return { stdout: "", stderr: "", exitCode: 0 }; };
    await runMergeDriverInstall(deps);
    expect(spawned).toBe(false);
  });

  test("shows success notification on exit 0", async () => {
    const { deps, infoMessages } = makeDeps();
    deps.spawn = async () => ({ stdout: "", stderr: "installed\n", exitCode: 0 });
    await runMergeDriverInstall(deps);
    expect(infoMessages).toHaveLength(1);
    expect(infoMessages[0].msg).toContain("installed");
    expect(infoMessages[0].buttons).toContain("Show Output");
  });

  test("shows failure notification on non-zero exit", async () => {
    const { deps, infoMessages } = makeDeps();
    deps.spawn = async () => ({ stdout: "", stderr: "not a git repo\n", exitCode: 2 });
    await runMergeDriverInstall(deps);
    expect(infoMessages).toHaveLength(1);
    expect(infoMessages[0].msg).toContain("failed");
  });

  test("passes binary, merge-driver install args, and cwd to spawn", async () => {
    let bin = "", args: string[] = [], cwd: string | undefined;
    const { deps } = makeDeps();
    deps.spawn = async (b, a, c) => { bin = b; args = a; cwd = c; return { stdout: "", stderr: "", exitCode: 0 }; };
    await runMergeDriverInstall(deps);
    expect(bin).toBe("/usr/bin/mdsmith");
    expect(args).toEqual(["merge-driver", "install"]);
    expect(cwd).toBe("/repo");
  });
});
