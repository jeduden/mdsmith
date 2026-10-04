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
   that runs the build pass (`check` runs none), from
   `signal.NotifyContext(ctx, os.Interrupt,
   syscall.SIGTERM)` (on plan9 `os.Interrupt` maps to the
   "interrupt" note) and pass it down to the build pass
   instead of `context.Background()`. Install the handler
   only around the dispatch of recipes and hooks, so the
   target scan, a pass with no target, the lint-fix pass,
   `--no-build`, and the dry-run, check-stale, and explain
   modes keep the default signal action. Skip a signal
   the process started with ignored (a background job's
   SIGINT), since `Notify` would un-ignore it. On Unix
   also catch `SIGHUP`, so a closed terminal or dropped
   SSH session reaps the recipe groups.
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
5. Escalate on a second signal: the first cancels the
   build (SIGTERM plus grace), the second closes a
   `build.WithForceKill` channel so the Unix kill sends
   SIGKILL at once. A repeat within 250 ms of the first
   (`escalateAfter`) is a duplicated delivery of the same
   interrupt (`npm run` forwarding SIGINT, bash resending
   SIGHUP) and keeps the grace. mdsmith still waits for
   the reap, so no recipe is orphaned. After the first
   signal mdsmith catches SIGPIPE (`holdBrokenPipe`), so
   a report written to a reader the Ctrl-C also ended
   (`2>&1 | tee log`) cannot end it before every group
   is reaped.
6. Run hooks through `runRecipe` (keeping mdsmith's
   environment), so a done context starts no hook and an
   interrupted hook reports as interrupted. Keep hooks in
   mdsmith's own process group (`sharedGroup`) and kill
   only the hook process on cancel: a dev server a
   before-hook backgrounds must outlive the hook (a Windows
   Job Object would kill it on close), get the terminal's
   Ctrl-C, and a hook may prompt on `/dev/tty`.
7. Print the last stdout and stderr lines in the
   `INTERRUPTED` report, as the timeout report does.
8. Document the behavior next to the timeout paragraph in
   [build.md](../docs/guides/directives/build.md).

## Acceptance Criteria

- [x] Interrupting `mdsmith fix` during a recipe leaves no
      process from the recipe's group running (Unix test).
- [x] An interrupted build is reported as interrupted, not
      as timed out.
- [x] Interrupting a hook kills the hook process; the
      children it backgrounds stay in mdsmith's process
      group, so the terminal's Ctrl-C reaches them
      (Unix test).
- [x] A second Ctrl-C ends a run whose recipe ignores
      SIGTERM before the 5 s grace period runs out.
- [x] A broken stderr after the interrupt does not end
      mdsmith before a SIGTERM-ignoring recipe group is
      reaped (Unix e2e test, `--build-jobs 2`).
- [x] `GOOS=plan9 go vet ./...` and
      `GOOS=js GOARCH=wasm go build ./...` still pass.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
