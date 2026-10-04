---
id: 2610021917
title: Replace the per-platform kill maps with a per-recipe group killer
status: "🔳"
model: sonnet
summary: >-
  `internal/build` passes each recipe's kill state from
  `afterStart` to `killGroup` through a mutex-guarded global
  map keyed by `*exec.Cmd`: `jobHandles` on Windows and
  `notePgs` on plan9. Have `afterStart` return a per-recipe
  value with `kill` and `close` methods instead, so neither
  platform keeps a global map and `runRecipe` owns the state.
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

## Tasks

1. , in `exec.go`, a `groupKiller` interface with
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

## Acceptance Criteria

- [x] No global map keyed by `*exec.Cmd` remains in
      `internal/build`.
- [x] No `forceKillLeaderFn` hook remains, and the
      plan9 killer does not kill the leader twice.
- [x] The Unix kill-path tests and the plan9 fake-`/proc`
      tests pass unchanged in what they assert.
- [x] `GOOS=plan9 go vet ./...`, `GOOS=windows go vet
      ./...`, and `GOOS=js GOARCH=wasm go build ./...`
      pass.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
