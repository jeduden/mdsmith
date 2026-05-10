import { describe, expect, test } from "bun:test";
import { defaultSpawn } from "./runner";

describe("defaultSpawn", () => {
  // Use the current runtime (Node when running under Node, Bun under Bun)
  // so the test never assumes a "node" binary exists on PATH.
  const runtime = process.execPath;

  test("captures stdout, stderr, and exit code from a real process", async () => {
    const result = await defaultSpawn(runtime, [
      "-e",
      "process.stdout.write('out\\n'); process.stderr.write('err\\n'); process.exit(0);",
    ]);
    expect(result.stdout.trim()).toBe("out");
    expect(result.stderr.trim()).toBe("err");
    expect(result.exitCode).toBe(0);
  });

  test("captures a non-zero exit code", async () => {
    const result = await defaultSpawn(runtime, ["-e", "process.exit(2);"]);
    expect(result.exitCode).toBe(2);
  });

  test("rejects when the binary does not exist", async () => {
    await expect(
      defaultSpawn("__no_such_binary_mdsmith__", [])
    ).rejects.toMatchObject({ code: "ENOENT" });
  });
});
