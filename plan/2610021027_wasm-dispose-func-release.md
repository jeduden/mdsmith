---
id: 2610021027
title: Release a disposed wasm session's dispose func
status: "✅"
model: sonnet
summary: >-
  `proxyDispose` in `cmd/mdsmith-wasm` never releases its
  own `js.Func`, so each created and disposed session
  leaves one entry in `syscall/js`'s handler table for
  the life of the engine. Release it on the first call and
  point `dispose` at a shared no-op, so a loop of create
  and dispose holds a fixed number of funcs.
---
# Release a disposed wasm session's dispose func

## Goal

Obsidian creates and disposes a session on every restart
or config change. Many such cycles must not grow the
number of registered `js.Func`s.

## Background

This came from the pre-merge review of PR #877 (plan
[2610020725](2610020725_build-exec-js-wasm-stub.md)).
`proxyDispose` in [main.go](../cmd/mdsmith-wasm/main.go)
releases every method func. It keeps its own func, so a
second `dispose()` is a no-op. It drops its references,
so the session and its bytes are freed. Still, each
session leaves one small closure and one table entry
behind.

`js.Func.Release` may be called while the function is
running. So `dispose` can release itself and set
`proxy.dispose` to a shared `disposedNoop`. The catch is
a reference taken before the first call: `const d =
session.dispose; d(); d()`. The second call would reach a
released func and log "call to released function". The
[engine API page](../docs/background/concepts/engine-api.md)
already says this for the other methods, so it needs to
say it for `dispose` too.

## Tasks

1. Add a test that counts funcs in the handler table, or
   uses a release hook seam, across N create/dispose
   cycles and fails while the count grows.
2. Release `dispose`'s own func on its first call and set
   `proxy.dispose` to a package-level no-op func.
3. Update the engine API page's dispose paragraph.

## Acceptance Criteria

- [x] N create/dispose cycles leave the func count the
      same as one cycle.
- [x] `session.dispose(); session.dispose()` stays a
      no-op.
- [x] `go run ./cmd/mdsmith-release test-js-wasm
      ./cmd/mdsmith-wasm` passes.
- [x] All tests pass: `go test ./...`
