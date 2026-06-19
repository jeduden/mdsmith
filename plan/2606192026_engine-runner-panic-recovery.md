---
id: 2606192026
title: "Add per-goroutine recover() to CLI engine runner worker goroutines"
status: "🔳"
summary: >-
  CLI engine worker goroutines in internal/engine/runner.go:353-370 have
  no defer recover(). A panic on adversarial Markdown crashes the whole
  process. Mirror the LSP's recoverPanic pattern so rule panics are
  caught per-file and reported as InternalError diagnostics. Closes S003
  (MEDIUM) from the 2026-06-19 full-repo audit.
model: sonnet
---
# Add per-goroutine recover() to CLI engine runner worker goroutines

## Goal

Close S003 (medium, CWE-390) from the [2026-06-19 full-repo security
audit](../docs/security/2026-06-19-full-repo-audit/report.md).

The parallel worker goroutines in `internal/engine/runner.go:353-370`
have no `defer recover()`. A rule panic on attacker-controlled Markdown
content kills the entire `mdsmith` process. No diagnostics are printed
for other files. The LSP server already handles this correctly via
`defer s.recoverPanic("lint " + uri)` in `server_diagnostics.go:246`.

Mirror that pattern in the CLI path. A hostile file should produce a
per-file `InternalError` diagnostic, not a crash.

## Tasks

1. Write a failing test that injects a panic-triggering stub rule into
   a multi-file runner. Assert the runner returns an `InternalError`
   diagnostic for the panicking file and completes the others.
2. In `internal/engine/runner.go:353-370`, wrap the goroutine body:

   ```go
   defer func() {
       if r := recover(); r != nil {
           outcomes[i] = panicOutcome(r)
       }
   }()
   ```

   `panicOutcome` converts the recovered value and a stack trace into
   an `InternalError` diagnostic on the file.
3. Confirm the new test passes and no existing tests regress.
4. Run `go test ./...` and `go tool golangci-lint run`.

## Acceptance Criteria

- [x] A rule panic on file `i` in a multi-file run produces an
  `InternalError` diagnostic for file `i` and does not affect the
  outcome of any other file.
- [x] `mdsmith check .` on a directory containing a panic-triggering
  file exits with a non-zero status (InternalError) but does not crash.
- [x] The recovered panic includes a stack trace in the diagnostic
  message so the bug can be reported.
- [x] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues (blocked: tools/go.mod
  requires Go 1.25.8+; environment has 1.25.0; `go vet ./...` passes)
