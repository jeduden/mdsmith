---
id: 2610021026
title: Replace runRecipe's hand-rolled pipe reaping with Cmd.WaitDelay
status: "✅"
model: opus
summary: >-
  `runRecipe` in `internal/build` builds its own output
  pipes, output gate, bounded waits, and a fallback leader
  kill. Go's `exec.Cmd.Cancel` and `exec.Cmd.WaitDelay`
  cover most of that. Move to them where the timing is the
  same, and keep the leader's real exit code when a child
  holds the pipe past the deadline.
---
# Replace runRecipe's hand-rolled pipe reaping with Cmd.WaitDelay

## Goal

A timed-out `<?build?>` recipe is reaped by `os/exec`'s
own `Cancel` and `WaitDelay` hooks. This drops about 170
lines of platform-sensitive concurrency from
[exec.go](../internal/build/exec.go) and
[recipe_output.go](../internal/build/recipe_output.go).

## Background

The pre-merge review of PR #877 (plan
[2610020725](2610020725_build-exec-js-wasm-stub.md))
found two things it left out of scope:

- **Altitude.** `recipeOutput`, `outputGate`,
  `waitAtMost`, `timeoutResult`, and the direct leader
  kill re-implement `Cmd.Cancel` plus `Cmd.WaitDelay`
  (Go 1.20+). `WaitDelay` closes the parent pipe ends
  after a delay, kills a leader that is still running,
  and waits for the copy goroutines.
- **Lost exit code.** If the leader exits before the
  deadline but a child keeps the pipe open past it, the
  result is `timedOut=true` with `ExitCode -1`. The real
  status is already in `cmd.ProcessState`.
  `TestRunRecipe_LeaderExitedChildHoldsPipeTimesOut`
  pins the current -1.

`WaitDelay`'s timer also starts when the leader exits,
not only on cancel. That changes when output is cut off
for a recipe whose leader exits early, so this is a
behaviour change and needs its own plan.

## Decisions

Task 2 met three questions the tasks left open. The
coordinator settled them in favour of keeping today's
behaviour over deleting lines:

1. **Pipes stay ours (plan9).** Once `WaitDelay` runs
   out, `os/exec` closes its pipe ends and then waits
   for its copy goroutines. plan9 cannot end a pending
   read by closing the fd, so a survivor outside the note
   group (rc's `&`) would hang `cmd.Wait`. `recipeOutput`,
   the gate, and the bounded drain stay on every
   platform. `os/exec` gets `*os.File` ends and runs no
   copies, so `Cancel` and `WaitDelay` replace only the
   leader-reap waits and `forceLeader`.
2. **A second interrupt kills the leader at once.**
   `WaitDelay` is set once, before `Start`; the os/exec
   docs promise nothing about a change made inside
   `Cancel`. Instead, once the kill has returned and the
   `WithForceKill` channel is closed (before or after),
   `killLeaderOnForce` kills the leader directly, so a
   second interrupt never waits out `reapWait`. The
   output drain still shortens at any point.
3. **No user-visible timing change.** `os/exec` stops
   watching ctx once the leader exits. `runRecipe` keeps
   its own select on drain versus ctx for that case: at
   the deadline it kills the group, so a 300 ms timeout
   with an early leader exit still returns in about
   300 ms. A child's output keeps flowing until it
   closes the pipe or the deadline passes.

The scope is smaller than the Goal: `waitAtMost`,
`timeoutResult`, `recipeOutput`, and the gate stay.
What goes is the two leader waits, the `forceLeader`
fallback, and `forceLeader` on the `groupKiller`
interface (plan9's no-op among them).

Two other differences came with the move:

- After `WaitDelay`'s `Process.Kill`, `runRecipe` waits
  for the leader to exit. Before, it gave up after a
  second `reapWait`. An uncatchable kill (SIGKILL,
  TerminateProcess, plan9's ctl kill, which the group
  kill already sent) makes that wait short. On plan9
  `Process.Kill` also posts a "kill" note, which is
  harmless.
- `Start` on a `CommandContext` refuses to fork once
  ctx is done. A deadline that passes between the entry
  check and `Start` now reports `NotStartedError`, not
  a start and an immediate kill.

## Tasks

1. [x] Write a failing test: a recipe like `sleep 30 &
   exit 2` with captured output, timed out, must report
   exit code 2 and still say it timed out. A non-zero
   exit already kept its code (its `ExitError` reached
   `timeoutResult`), so the test also covers `exit 0`,
   which reported -1. `timeoutResult` now takes the code
   from `cmd.ProcessState`.
2. [x] Build the command with `exec.CommandContext`,
   setting `Cancel` to `killGroup` and `WaitDelay` to
   `reapWait`. Remove the parts of `recipeOutput` and the
   bounded waits that `WaitDelay` now handles. Keep the
   gate only if output after the return can still reach
   a caller's writer. Per decision 1 the pipes, gate,
   and drain stay; `Cancel` runs the group kill and
   `WaitDelay` replaces the leader waits and
   `forceLeader`. The red test was
   `TestRunRecipe_DeadlineBeforeStartSpawnsNothing`.
3. [x] Decide, and document in the `runRecipe` doc
   comment and
   [build.md](../docs/guides/directives/build.md), what
   happens to a child's output when the leader exits
   early. It keeps reaching the caller until the child
   closes the pipe or the deadline passes; the deadline
   kills the group and keeps the leader's exit code
   (decision 3).
4. [x] Run the Unix, Windows, and js/wasm tests, and
   `GOOS=plan9 go vet ./internal/build`. Linux ran
   `go test -race`, and Node ran `mdsmith-release
   test-js-wasm --all --require-js-only
   ./internal/build`. CI has no Windows test runner and
   this host cannot run one, so Windows and plan9 got
   `go vet ./...` only.

## Acceptance Criteria

- [x] A timed-out recipe whose leader has already exited
      reports the leader's exit code.
- [x] Timeout handling uses `Cmd.Cancel` and
      `Cmd.WaitDelay` rather than hand-rolled timers for
      the leader. The output drain keeps its bounded
      wait (decision 1).
- [x] A survivor that holds a pipe still cannot hang
      mdsmith past the documented bound.
- [x] All tests pass: `go test ./...`
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues
