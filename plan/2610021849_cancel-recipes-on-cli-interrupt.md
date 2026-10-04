---
id: 2610021849
title: Kill running build recipes when the CLI is interrupted
status: "✅"
model: sonnet
summary: >-
  `mdsmith fix` builds recipes under `context.Background()`, and
  each recipe runs in its own process group (Unix) or note group
  (plan9), so a terminal interrupt kills mdsmith but not the
  recipe tree. Cancel the build context on SIGINT/SIGTERM (an
  "interrupt" note on plan9) so the existing timeout kill path
  tears every recipe down before mdsmith exits.
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

## Tasks

1. Add a failing test that runs the fix build pass with a
   context that is cancelled mid-recipe and asserts the
   recipe's spawned child is gone afterwards (Unix-tagged;
   the `exec_unix_test.go` helpers live in `internal/build`,
   so `cmd/mdsmith` carries its own pid probe).
2. Derive the build context in `fix`, the only command
   that runs the build pass (`check` runs none), from a
   `signal.Notify` watcher (`watchInterrupts`) on
   `os.Interrupt` and `syscall.SIGTERM` (on plan9 both map
   to the "interrupt" note) that cancels it, and pass it
   down to the build pass instead of
   `context.Background()`. The watcher, not
   `signal.NotifyContext`, so it can also escalate on a
   second signal (task 5). Install the handler
   only around the dispatch of recipes and hooks, so the
   target scan, a pass with no target, the lint-fix pass,
   `--no-build`, and the dry-run, check-stale, and explain
   modes keep the default signal action. Skip a signal
   the process started with ignored (a background job's
   SIGINT), since `Notify` would un-ignore it. On Unix
   also catch `SIGHUP`, and on plan9 the "hangup" note, so
   a closed terminal or window, or a dropped SSH session,
   reaps the recipe groups.
3. Make sure a cancelled build reports an interrupt error,
   not a timeout, and that after the recipes are reaped
   and the output is written mdsmith re-raises the first
   signal with its default action, so a calling shell
   script stops too (exit 2 where it cannot: Windows,
   plan9). This covers a target not yet
   started, a `--build-verify` re-run, and a hook.
4. Make `runRecipe` start no recipe when its context is
   already done at entry, so every `BuildWithResult`
   caller (build pass and verify re-run) is covered.
   `BuildWithResult` refuses the same way before it
   stages the target or opens the action's log, so a
   refused run never truncates an earlier log.
5. Escalate on a second signal: the first cancels the
   build (SIGTERM plus grace), the second closes a
   `build.WithForceKill` channel so the Unix kill sends
   SIGKILL at once. A repeat within 250 ms of the first
   (`escalateAfter`) is a duplicated delivery of the same
   interrupt (`npm run` forwarding SIGINT, bash resending
   SIGHUP) and keeps the grace. mdsmith still waits for
   the reap, so no recipe is orphaned. From the start of
   dispatch mdsmith catches SIGPIPE (`holdBrokenPipe`), so
   a report or `--build-stream` line written to a reader
   the Ctrl-C also ended (`2>&1 | tee log`) cannot end it
   before every group is reaped, even when the write
   beats the watcher to the signal.
6. Run hooks through `runRecipe` (keeping mdsmith's
   environment), so a done context starts no hook and an
   interrupted hook reports as interrupted. Keep hooks in
   mdsmith's own process group (`sharedGroup`) and signal
   only the hook process on cancel: a dev server a
   before-hook backgrounds must outlive the hook (a Windows
   Job Object would kill it on close), get the terminal's
   Ctrl-C, and a hook may prompt on `/dev/tty`. On Unix
   the hook gets the recipe kill aimed at its leader
   (`killLeaderUntil`): `SIGTERM`, up to the grace period,
   then `SIGKILL`, so a `trap cleanup TERM` still runs; a
   second interrupt skips the grace.
7. Print the last stdout and stderr lines in the
   `INTERRUPTED` report, as the timeout report does.
8. Document the behavior next to the timeout paragraph in
   [build.md](../docs/guides/directives/build.md).
9. Skip, with no output, every after-hook an interrupt
   reached before it started (`RunAfterHooks` stops at a
   cancelled context); a hook it cut short still reports
   `FAIL`.
10. Once a second interrupt closes the force channel, end
    the leader reap, the wait after the leader-only kill,
    and the output drain `forcedReapWait` (100 ms) later
    instead of after `reapWait` (5 s).
11. Stop the `--build-skip-hooks-when-fresh` scan
    (`allFresh`) before the next target's inputs are
    hashed once the build context is cancelled.

## Acceptance Criteria

- [x] Interrupting `mdsmith fix` during a recipe leaves no
      process from the recipe's group running (Unix test).
- [x] An interrupted build is reported as interrupted, not
      as timed out.
- [x] Interrupting a hook kills the hook process; the
      children it backgrounds stay in mdsmith's process
      group, so the terminal's Ctrl-C reaches them
      (Unix test).
- [x] A cancelled hook that traps `TERM` runs its cleanup;
      one that ignores `TERM` is killed after the grace,
      or at once on a second interrupt (Unix tests).
- [x] After an interrupt, no after-hook that never started
      runs or prints a line.
- [x] After a second interrupt, `runRecipe` returns in
      under 1 s even when a `setsid` daemon holds the
      recipe's stdout pipe (Unix test).
- [x] `allFresh` returns false at once on a cancelled
      context instead of hashing every target.
- [x] A second Ctrl-C ends a run whose recipe ignores
      SIGTERM before the 5 s grace period runs out.
- [x] A broken stderr after the interrupt does not end
      mdsmith before a SIGTERM-ignoring recipe group is
      reaped (Unix e2e test, `--build-jobs 2`).
- [x] `GOOS=plan9 go vet ./...` and
      `GOOS=js GOARCH=wasm go build ./...` still pass.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues

## Deviations

- Hooks first got an immediate `SIGKILL` on cancel, which
  cut short a `TERM` or `INT` trap that ran its cleanup
  before this plan. The final review restored the cleanup
  with the grace kill in task 6. A Ctrl-C also delivers
  `SIGINT` to the hook from the terminal, so a hook that
  traps only `INT` can still be cut short by the
  `SIGTERM` that follows; trap `TERM` too.
- `reportInterrupt` lost its "before start" branch: with
  the real builder a refusal before start is always
  `outcomeNotStarted`, which `reportNotStarted` lists, so
  only a mock builder reached it.
