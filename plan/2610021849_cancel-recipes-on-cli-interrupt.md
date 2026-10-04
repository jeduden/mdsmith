---
id: 2610021849
title: Kill running build recipes when the CLI is interrupted
status: "🔲"
model: opus
summary: >-
  `mdsmith fix` builds recipes under `context.Background()`, and
  each recipe runs in its own process group (Unix) or note group
  (plan9), so a terminal interrupt kills mdsmith but not the
  recipe tree. Cancel the build context on SIGINT/SIGTERM (an
  "interrupt" note on plan9) so the existing timeout kill path
  tears every recipe down before mdsmith exits. Also close two
  kill races in `internal/build`: a Windows child spawned before
  `afterStart` assigns the Job Object, and a Unix group signal
  sent after the leader is reaped.
---
# Kill running build recipes when the CLI is interrupted

## Goal

A user stops `mdsmith fix` with Ctrl-C, or DEL on plan9.
Every running `<?build?>` recipe dies with it. So do the
recipe's children. None run on as orphans.

## Background

Found by the code review of PR #881
([plan 2610020946](2610020946_plan9-recipe-note-group-kill.md)).
`configureProcessGroup` moves each recipe out of mdsmith's
process group (`Setpgid` on Unix) or note group (`RFNOTEG` on
plan9), so the terminal's interrupt no longer reaches it.
[buildpass.go](../cmd/mdsmith/buildpass.go) and
[builddiag.go](../cmd/mdsmith/builddiag.go) start builds from
`context.Background()`, and only
[lsp.go](../cmd/mdsmith/lsp.go) wires `signal.NotifyContext`.
`runRecipe` already kills the whole group when its context is
done, so only the cancellation is missing.

Round 3 of the code review on PR #906
([plan 2610021917](2610021917_per-recipe-group-killer.md))
found two more ways a timeout or interrupt kill misses its
target. Both are on main already. They live here, not in plans
of their own, because `PLAN.md` is at its 300-line limit.

- Windows: `afterStart` in
  [exec_windows.go](../internal/build/exec_windows.go) assigns
  the Job Object after `cmd.Start` returns. A child the recipe
  spawns in that gap is outside the job, so `terminateJob` and
  `KILL_ON_JOB_CLOSE` miss it. `syscall.SysProcAttr` takes no
  `PROC_THREAD_ATTRIBUTE_JOB_LIST`, and `os/exec` does not
  return the thread handle a `CREATE_SUSPENDED` start needs.
- Unix: when the leader exits but a survivor holds a captured
  pipe until the deadline, `runRecipe` in
  [exec.go](../internal/build/exec.go) has reaped the leader,
  yet `pgKiller.kill` still signals `-pgid`. If every member
  has left or exited, the pid can be reused as another group's
  pgid, and that group gets SIGTERM and SIGKILL.

## Tasks

1. Add a failing test that runs the fix build pass with a
   context that is cancelled mid-recipe and asserts the
   recipe's spawned child is gone afterwards (Unix-tagged,
   reusing the `exec_unix_test.go` helpers).
2. Derive the build context in the `fix` and `check` entry
   points from `signal.NotifyContext(ctx, os.Interrupt,
   syscall.SIGTERM)` (on plan9 `os.Interrupt` maps to the
   "interrupt" note) and pass it down to the build pass
   instead of `context.Background()`.
3. Make sure a cancelled build reports an interrupt error,
   not a timeout, and that mdsmith exits non-zero after
   the recipes are reaped.
4. Document the behavior next to the timeout paragraph in
   [build.md](../docs/guides/directives/build.md).
5. Windows: put the recipe in its job before it can spawn.
   Write a failing test where a recipe starts a sleeper at
   once and exits; assert the sleeper is dead after
   `runRecipe`. Then start suspended, assign the job, and
   resume the main thread, or create the process with a job
   list attribute through `x/sys/windows`.
6. Unix: keep the leader unreaped until the group kill. Learn
   that it exited with `waitid(P_PID, pid, WEXITED | WNOWAIT)`
   from `x/sys/unix`, kill the group if the deadline passed,
   then reap with `cmd.Wait`. Where `WNOWAIT` is missing, skip
   the group signal once the leader is reaped.

## Acceptance Criteria

- [ ] Interrupting `mdsmith fix` during a recipe leaves no
      process from the recipe's group running (Unix test).
- [ ] An interrupted build is reported as interrupted, not
      as timed out.
- [ ] `GOOS=plan9 go vet ./...` and
      `GOOS=js GOARCH=wasm go build ./...` still pass.
- [ ] A child a Windows recipe spawns right after start is
      killed by the timeout kill and by the job close.
- [ ] On Unix, no group signal is sent after the recipe's
      leader has been reaped.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
