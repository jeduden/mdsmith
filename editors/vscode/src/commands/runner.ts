// Shared helper for spawning mdsmith subcommands from palette command handlers.
// The SpawnFn type is injectable so tests can mock child process execution.

import { spawn as nodeSpawn } from "node:child_process";

export interface SpawnResult {
  stdout: string;
  stderr: string;
  exitCode: number;
}

export type SpawnFn = (
  binary: string,
  args: string[],
  cwd?: string
) => Promise<SpawnResult>;

// defaultSpawn runs the binary with args, captures stdout and stderr,
// and resolves with the result on any exit code. Rejects only when
// the process cannot be started (e.g. ENOENT).
export const defaultSpawn: SpawnFn = (binary, args, cwd) =>
  new Promise((resolve, reject) => {
    const proc = nodeSpawn(binary, args, cwd ? { cwd } : {});
    let stdout = "";
    let stderr = "";
    proc.stdout.on("data", (chunk: Buffer) => { stdout += chunk.toString(); });
    proc.stderr.on("data", (chunk: Buffer) => { stderr += chunk.toString(); });
    proc.on("error", reject);
    proc.on("close", (code: number | null) => {
      resolve({ stdout, stderr, exitCode: code ?? 1 });
    });
  });
