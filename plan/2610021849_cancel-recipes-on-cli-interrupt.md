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
   only around a build pass that can start a recipe or
   hook, so the lint-fix pass, `--no-build`, and the
   dry-run, check-stale, and explain modes keep the
   default signal action.
3. Make sure a cancelled build reports an interrupt error,
   not a timeout, and that mdsmith exits 2 after the
   recipes are reaped. This covers a target not yet
   started, a `--build-verify` re-run, and a hook.
4. Make `runRecipe` start no recipe when its context is
   already done at entry, so every `BuildWithResult`
   caller (build pass and verify re-run) is covered.
5. Escalate on a second signal: the first cancels the
   build (SIGTERM plus grace), the second closes a
   `build.WithForceKill` channel so the Unix kill sends
   SIGKILL at once. mdsmith still waits for the reap, so
   no recipe is orphaned.
6. Print the last stdout and stderr lines in the
   `INTERRUPTED` report, as the timeout report does.
7. Document the behavior next to the timeout paragraph in
   [build.md](../docs/guides/directives/build.md).

## Acceptance Criteria

- [x] Interrupting `mdsmith fix` during a recipe leaves no
      process from the recipe's group running (Unix test).
- [x] An interrupted build is reported as interrupted, not
      as timed out.
- [x] A second Ctrl-C ends a run whose recipe ignores
      SIGTERM before the 5 s grace period runs out.
- [x] `GOOS=plan9 go vet ./...` and
      `GOOS=js GOARCH=wasm go build ./...` still pass.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
