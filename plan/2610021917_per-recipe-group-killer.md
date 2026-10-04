---
id: 2610021917
title: Replace the per-platform kill maps with a per-recipe group killer
status: "🔲"
model: sonnet
summary: >-
  `internal/build` passes each recipe's kill state from
  `afterStart` to `killGroup` through a mutex-guarded global
  map keyed by `*exec.Cmd`: `jobHandles` on Windows and
  `notePgs` on plan9. Have `afterStart` return a per-recipe
  value with `kill` and `close` methods instead, so neither
  platform keeps a global map and `runRecipe` owns the state.
  Also finish the interrupt teardown that plan 2610021849's
  last review left open: hook grace, after-hook FAIL lines,
  second-interrupt waits, and the `allFresh` scan.
---
# Replace the per-platform kill maps with a per-recipe group killer

## Goal

Each recipe's kill state lives in a value that
`runRecipe` owns, not in a global map. Then no platform
needs a mutex or a lookup keyed by `*exec.Cmd` to kill
its group.

## Background

Round 2 of the code review on PR #881 (plan
[2610020946](2610020946_plan9-recipe-note-group-kill.md))
found that the plan9 kill path copies a Windows
workaround. `afterStart` and `killGroup` share no
argument except the `*exec.Cmd`. So
[exec_windows.go](../internal/build/exec_windows.go)
keeps its Job Object handle in a global `jobHandles`
map, and
[exec_plan9.go](../internal/build/exec_plan9.go) keeps
the note group's `notepg` file and `noteid` in a global
`notePgs` map. Both maps need a mutex, because `Build`
may run recipes at the same time. Both need a cleanup
that deletes the entry.

The PR left this design alone. Fixing it changes the
seam that all four exec files and the
`afterStartFn`, `killGroupFn`, and `forceKillLeaderFn`
test hooks in [exec.go](../internal/build/exec.go) use.
No CI runner tests the Windows or plan9 kill path.

The PR's reap fallback also added `forceKillLeaderFn`.
It calls `forceKillLeader` on every platform when the
group kill left the leader running. That covers a Unix
leader that left its group and Windows without a Job
Object. plan9's `killGroup` already ends in the same
uncatchable leader kill, so there it repeats one that
failed.

## Interrupt follow-ups

PR #905 built plan
[2610021849](2610021849_cancel-recipes-on-cli-interrupt.md).
Its last review left these open. Each needs a design
call. The first and third go through the new killer.

- **Hooks get SIGKILL with no grace.** On cancel,
  `sharedGroup` hooks are killed through
  `forceKillLeaderFn`. A hook with `trap cleanup INT`
  ran its cleanup before PR #905, but does not now.
  After `kill -TERM` on mdsmith, the hook's `sleep`
  child is left running. A Ctrl-C already sends SIGINT
  to the terminal's process group, so a new SIGTERM can
  cut an INT-only cleanup short. Choose between
  forwarding the received signal and waiting a grace
  period first.
- **After-hooks that never started print FAIL lines.**
  `runBuildPass` in
  [buildpass.go](../cmd/mdsmith/buildpass.go) still
  passes every after-hook to `RunAfterHooks` with a
  cancelled context. Each one prints
  `hook X: FAIL (exit 1): context canceled (interrupted)`.
  The [build guide](../docs/guides/directives/build.md)
  says an interrupt skips the after hooks. The guide is
  at its MDS022 line budget.
- **A second interrupt still waits up to 5 s.** It only
  shortens the SIGTERM grace in `killGroupUntil`.
  Reaping the leader, the leader-kill retry, and the
  output drain in `timeoutResult` each still wait up to
  `reapWait`. An example is a recipe whose setsid daemon
  holds the stdout pipe.
- **`allFresh` ignores the context.** With
  `--build-skip-hooks-when-fresh`, the scan keeps
  hashing every input after a Ctrl-C.
- **Unreachable report branch.** In
  [builddiag.go](../cmd/mdsmith/builddiag.go), the
  `!res.TimedOut` ("before start") branch of
  `reportInterrupt` runs only for a mock builder, since
  `refusedByInterrupt` filters real refusals out first.

## Tasks

1. Define, in `exec.go`, a `groupKiller` interface with
   `kill()` and `close()` methods. Change `afterStart`
   to return it. A no-op or leader-only killer stands
   in where a platform has no group state.
2. On Windows, have the returned killer hold the Job
   Object handle. `kill` sends `CTRL_BREAK` and
   terminates the job; `close` closes the handle.
   Delete `jobHandles` and `jobHandlesMu`.
3. On plan9, have the returned killer hold the
   `noteGroup`. `kill` runs today's `killGroup` body;
   `close` closes the `notepg` file. Delete `notePgs`
   and `notePgsMu`.
4. On Unix and on the `exec_other.go` targets, return a
   killer that wraps today's `killGroup`.
5. Change `runRecipe` to call the killer's `kill` on
   timeout and `close` on return. Replace the
   `afterStartFn`/`killGroupFn` hooks with one hook
   that returns a stub killer, and port the tests that
   use them.
6. Move the reap fallback into the killer: give it a
   `forceLeader()` method that is the leader kill on
   Unix and Windows and a no-op on plan9, where `kill`
   already ends in it. Delete `forceKillLeaderFn` and
   the shared `exec_leader_kill.go`.
7. Decide how the killer signals a cancelled hook:
   forward the received signal, or wait a grace period,
   then SIGKILL. Add a test with an INT-trap hook whose
   cleanup must run.
8. Skip after-hooks that have not started once the
   context is cancelled, with no FAIL line for them.
9. Make the leader reap, the leader-kill retry, and the
   drain wait respect the force-kill channel.
10. Check `ctx.Err()` in the `allFresh` loop, and remove
    or use the unreachable "before start" branch.

## Acceptance Criteria

- [ ] No global map keyed by `*exec.Cmd` remains in
      `internal/build`.
- [ ] No `forceKillLeaderFn` hook remains, and the
      plan9 killer does not kill the leader twice.
- [ ] The Unix kill-path tests and the plan9 fake-`/proc`
      tests pass unchanged in what they assert.
- [ ] `GOOS=plan9 go vet ./...`, `GOOS=windows go vet
      ./...`, and `GOOS=js GOARCH=wasm go build ./...`
      pass.
- [ ] An after-hook that traps INT runs its cleanup on
      Ctrl-C, and no child of the hook outlives mdsmith.
- [ ] After an interrupt, no after-hook that never
      started prints a FAIL line.
- [ ] After a double Ctrl-C, mdsmith exits in under 1 s
      even when a daemon holds the recipe's pipe.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
