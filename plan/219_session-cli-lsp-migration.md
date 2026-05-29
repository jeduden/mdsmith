---
id: 219
title: Route cmd/mdsmith and the LSP through pkg/mdsmith.Session
status: "🔲"
model: opus
summary: >-
  Migrate the CLI's check/fix/kinds subcommands and the LSP server
  onto the public `pkg/mdsmith.Session` from plan 215, so the CLI,
  the LSP, and WASM share one engine entry point instead of each
  re-deriving `engine.Runner` and `internal/fix` plumbing.
depends-on: [215]
---
# Route cmd/mdsmith and the LSP through pkg/mdsmith.Session

## Goal

Make `cmd/mdsmith` and `internal/lsp` construct a
[`pkg/mdsmith.Session`](../pkg/mdsmith/session.go) for check, fix, and
kind resolution. One engine entry point then serves the CLI, the LSP,
and WASM alike.

## Background

[Plan 215](215_engine-api-wasm.md) added `pkg/mdsmith.Session` and
routed the LSP's file reads through the `Workspace` seam. But the CLI
still builds `engine.Runner` and calls `internal/fix` directly, and the
LSP does the same for diagnostics. The session already owns the parse
cache, the config merge, and the workspace abstraction, so both
surfaces can drop their parallel plumbing.

This was deferred from plan 215. It refactors the primary product
surfaces, so it must hold their existing gates: the CLI end-to-end
tests and the LSP p95 latency budget.

## Tasks

1. Route `cmd/mdsmith`'s check, fix, and kinds subcommands through
   `NewSession`, keeping flags and output unchanged.
2. Route the LSP diagnostics and code-action paths through a
   per-workspace `Session`, invalidating on `didChange` and
   `didChangeWatchedFiles`.
3. Confirm no engine-content `os.ReadFile` survives outside
   `pkg/mdsmith` and `cmd/` once both surfaces use the session.
4. Re-run the CLI e2e suite and the LSP latency gate; hold both.

## Acceptance Criteria

- [ ] `cmd/mdsmith` check, fix, and kinds construct a
      `pkg/mdsmith.Session`.
- [ ] The LSP uses one `Session` per workspace and invalidates it on
      document and watched-file changes.
- [ ] The CLI end-to-end tests and the LSP p95 latency gate pass.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues.

## See also

- [Plan 215: engine API and WASM bindings](215_engine-api-wasm.md)
