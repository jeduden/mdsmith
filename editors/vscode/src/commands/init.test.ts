import { describe, expect, test } from "bun:test";
import { runInit, type InitHandlerDeps } from "./init";

function makeDeps(overrides: Partial<InitHandlerDeps> = {}): {
  deps: InitHandlerDeps;
  infoMessages: Array<{ msg: string; buttons: string[] }>;
  errorMessages: string[];
  output: string[];
} {
  const infoMessages: Array<{ msg: string; buttons: string[] }> = [];
  const errorMessages: string[] = [];
  const output: string[] = [];
  const deps: InitHandlerDeps = {
    binary: "/usr/bin/mdsmith",
    workspaceRoot: "/repo",
    isTrusted: () => true,
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

describe("runInit", () => {
  test("shows error when no workspace folder is open", async () => {
    let spawned = false;
    const { deps, errorMessages } = makeDeps({ workspaceRoot: undefined });
    deps.spawn = async () => { spawned = true; return { stdout: "", stderr: "", exitCode: 0 }; };
    await runInit(deps);
    expect(errorMessages).toHaveLength(1);
    expect(errorMessages[0]).toContain("workspace folder");
    expect(spawned).toBe(false);
  });

  test("shows error when workspace is not trusted", async () => {
    let spawned = false;
    const { deps, errorMessages } = makeDeps({ isTrusted: () => false });
    deps.spawn = async () => { spawned = true; return { stdout: "", stderr: "", exitCode: 0 }; };
    await runInit(deps);
    expect(errorMessages).toHaveLength(1);
    expect(errorMessages[0]).toContain("trusted workspace");
    expect(spawned).toBe(false);
  });

  test("shows success notification on exit 0", async () => {
    const { deps, infoMessages } = makeDeps();
    deps.spawn = async () => ({ stdout: "created .mdsmith.yml\n", stderr: "", exitCode: 0 });
    await runInit(deps);
    expect(infoMessages).toHaveLength(1);
    expect(infoMessages[0].msg).toContain(".mdsmith.yml created");
    expect(infoMessages[0].buttons).toContain("Show Output");
  });

  test("shows failure notification on non-zero exit", async () => {
    const { deps, infoMessages } = makeDeps();
    deps.spawn = async () => ({ stdout: "", stderr: "error: already exists\n", exitCode: 1 });
    await runInit(deps);
    expect(infoMessages).toHaveLength(1);
    expect(infoMessages[0].msg).toContain("failed");
    expect(infoMessages[0].buttons).toContain("Show Output");
  });

  test("appends stderr to output channel", async () => {
    const { deps, output } = makeDeps();
    deps.spawn = async () => ({ stdout: "", stderr: "some warning\n", exitCode: 0 });
    await runInit(deps);
    expect(output.join("")).toContain("some warning");
  });

  test("passes binary, init args, and cwd to spawn", async () => {
    let bin = "", args: string[] = [], cwd: string | undefined;
    const { deps } = makeDeps();
    deps.spawn = async (b, a, c) => { bin = b; args = a; cwd = c; return { stdout: "", stderr: "", exitCode: 0 }; };
    await runInit(deps);
    expect(bin).toBe("/usr/bin/mdsmith");
    expect(args).toEqual(["init"]);
    expect(cwd).toBe("/repo");
  });
});
