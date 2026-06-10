---
id: 2606092208
title: 'Recover from panics in the LSP lint pipeline'
status: "✅"
model: sonnet
summary: >-
  Wrap the LSP lint goroutine and dispatch loop in
  recover so a panic on hostile Markdown logs and drops
  one file's diagnostics instead of crashing the server.
  Closes finding S001 of the 2026-06-09 security audit.
---
# Recover from panics in the LSP lint pipeline

## Goal

The `mdsmith lsp` server runs each lint pass in a
`time.AfterFunc` goroutine
(`runLintIfCurrent` -> `runLint` -> `sess.CheckVersion`)
with no `recover()` in the chain. A panic while linting
an attacker-controlled file therefore takes down the
whole server. Every open editor session loses its
diagnostics. A still-open file can crash-loop the
restart. `dispatchRaw` has the same gap on the message
path.

This closes finding **S001** (medium, confirmed) of the
[2026-06-09 audit](../docs/security/2026-06-09-full-repo-audit/report.md),
location
[`internal/lsp/server_diagnostics.go`](../internal/lsp/server_diagnostics.go).

Recovery turns an unrecoverable crash into a logged
error: the server stays up and that document simply has
no diagnostics for the cycle. The underlying panic is a
separate bug — this plan only contains the blast radius.

## Tasks

1. [x] Add a failing test in `internal/lsp` that registers a
   rule whose `Check` panics, opens a document, and
   asserts the server stays running and publishes no
   diagnostics for that file (rather than the test
   process dying).
2. [x] Wrap the body of `runLint` (or `runLintIfCurrent`) in
   a deferred `recover()` that logs via `s.logger` and
   returns, leaving prior diagnostics untouched.
3. [x] Wrap `dispatchRaw` in a deferred `recover()` so a
   panic in one message handler does not kill the
   dispatch loop; log and continue.
4. [x] Add a test that a panic during dispatch of one
   request still lets the next request be served.

## Acceptance Criteria

- [x] A rule that panics on a given document no longer
      terminates the LSP server; the panic is logged and
      that document's diagnostics are skipped for the
      cycle.
- [x] A panic handling one LSP message does not stop the
      dispatch loop from serving the next message.
- [x] Recovery paths are covered by tests (driven
      red/green).
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
