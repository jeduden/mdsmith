---
id: 2610021849
title: Kill running build recipes when the CLI is interrupted
status: "🔳"
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

## Acceptance Criteria

- [x] Interrupting `mdsmith fix` during a recipe leaves no
      process from the recipe's group running (Unix test).
- [x] An interrupted build is reported as interrupted, not
      as timed out.
- [ ] `GOOS=plan9 go vet ./...` and
      `GOOS=js GOARCH=wasm go build ./...` still pass.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
