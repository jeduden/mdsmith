---
id: 2610021026
title: Replace runRecipe's hand-rolled pipe reaping with Cmd.WaitDelay
status: "🔳"
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

## Tasks

1. [x] Write a failing test: a recipe like `sleep 30 &
   exit 2` with captured output, timed out, must report
   exit code 2 and still say it timed out. A non-zero
   exit already kept its code (its `ExitError` reached
   `timeoutResult`), so the test also covers `exit 0`,
   which reported -1. `timeoutResult` now takes the code
   from `cmd.ProcessState`.
2. Build the command with `exec.CommandContext`, setting
   `Cancel` to `killGroup` and `WaitDelay` to
   `reapWait`. Remove the parts of `recipeOutput` and the
   bounded waits that `WaitDelay` now handles. Keep the
   gate only if output after the return can still reach
   a caller's writer.
3. Decide, and document in the `runRecipe` doc comment
   and [build.md](../docs/guides/directives/build.md),
   what happens to a child's output when the leader
   exits early.
4. Run the Unix, Windows, and js/wasm tests, and
   `GOOS=plan9 go vet ./internal/build`.

## Acceptance Criteria

- [x] A timed-out recipe whose leader has already exited
      reports the leader's exit code.
- [ ] Timeout handling uses `Cmd.Cancel` and
      `Cmd.WaitDelay` rather than hand-rolled timers.
- [ ] A survivor that holds a pipe still cannot hang
      mdsmith past the documented bound.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues
