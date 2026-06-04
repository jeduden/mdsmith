---
id: 219
title: Route cmd/mdsmith and the LSP through pkg/mdsmith.Session
status: "✅"
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

## Footguns to resolve before wiring

A plan-215 code review surfaced four traps in
[`pkg/mdsmith`](../pkg/mdsmith/session.go). They are dormant today
because the session is WASM/test-only. Each becomes live the moment
the CLI or LSP constructs a `Session`. Fix each as part of the surface
it affects, not after:

1. **OSWorkspace path split.** `OSWorkspace.ReadFile` reads the raw
   path (CWD-relative). But `OSWorkspace.FS` is rooted at `Root`. With
   a non-empty `Root`, the same workspace-relative `uri` resolves to
   two different files. `Session.frontMatterFor` reads through
   `ReadFile`; the engine reads cross-file content through `FS`. No
   product code sets `Root` yet. Wiring the CLI (which will) must
   reconcile the two read paths — root both, or pass only absolute
   paths — with a test that reads one `uri` both ways and asserts a
   single file.
2. **Per-pass corpus clone.** [`MemWorkspace.FS`](../pkg/mdsmith/workspace.go)
   deep-clones every file on every call, and the engine fetches a
   fresh `FS` per lint pass — `O(corpus)` per single-file Check. The
   LSP's per-keystroke Check makes this a latency item; hold it under
   the p95 gate or switch to a copy-on-write snapshot.
3. **Invalidate boundary.** [`Session.Invalidate`](../pkg/mdsmith/session.go)
   type-asserts `*MemWorkspace` to mutate content; an `OSWorkspace`
   silently drops the content argument. The LSP edits in-memory
   buffers, so decide the contract: either put `Set`/`Delete` on the
   `Workspace` interface, or split buffer-overlay from cache-invalidate
   so the LSP's open-document bytes reach cross-file rules.
4. **Fix re-lint waste.** [`Session.Fix`](../pkg/mdsmith/session.go)
   re-lints with a fresh full runner even when the fix made no edit,
   doubling work on already-clean files. Short-circuit the re-lint when
   `Changed` is false (or reuse the fixer's own remaining diagnostics)
   so `mdsmith fix` on a clean tree does not pay twice.

## Tasks

1. Route `cmd/mdsmith`'s check, fix, and kinds subcommands through
   `NewSession`, keeping flags and output unchanged.
2. Route the LSP diagnostics and code-action paths through a
   per-workspace `Session`, invalidating on `didChange` and
   `didChangeWatchedFiles`.
3. Confirm no engine-content `os.ReadFile` survives outside
   `pkg/mdsmith` and `cmd/` once both surfaces use the session.
4. Resolve the four footguns above as part of the surface that makes
   each live (OSWorkspace path split and Fix re-lint with the CLI;
   corpus clone and Invalidate boundary with the LSP).
5. Re-run the CLI e2e suite and the LSP latency gate; hold both.

## Acceptance Criteria

- [x] `cmd/mdsmith` check, fix, and kinds construct a
      `pkg/mdsmith.Session`.
- [x] The LSP uses one `Session` per workspace and invalidates it on
      document and watched-file changes.
- [x] `OSWorkspace.ReadFile` and `OSWorkspace.FS` resolve the same
      `uri` to the same file, proven by a test.
- [x] `Session.Fix` does not re-lint when the fix made no edit.
- [x] The LSP's in-memory buffer bytes reach cross-file rules through
      the session (Invalidate carries open-document content).
- [x] The CLI end-to-end tests and the LSP p95 latency gate pass.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues.

## See also

- [Plan 215: engine API and WASM bindings](215_engine-api-wasm.md)
