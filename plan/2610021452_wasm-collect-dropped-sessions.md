---
id: 2610021452
title: Free wasm sessions dropped without dispose
status: "✅"
model: sonnet
summary: >-
  The wasm engine keeps each Go Session in a
  package-level map keyed by session id until
  `dispose()` runs. A host that drops a session
  object without calling `dispose()` leaks that
  Session, with its workspace and parse caches, for
  the life of the engine. Bind a per-session token
  into every method and register it with a
  `FinalizationRegistry` that disposes its id once the
  object and every method taken off it are collected.
---
# Free wasm sessions dropped without dispose

## Goal

A session that becomes unreachable without a `dispose()`
call has its Go Session removed from the `sessions` map
after JS collects it. A session is unreachable only once
its object and every method taken off it are, so
`const { check } = await createSession(opts)` keeps the
session live while `check` is.

## Background

Plan
[2610021237](2610021237_wasm-stale-dispose-reference.md)
moved the session methods in
[main.go](../cmd/mdsmith-wasm/main.go) to shared funcs
that are bound to a session id. Only the bound functions
live in JS. The Go Session is held by the `sessions`
map, and only `dispose()` deletes its entry. Review
round 2 of PR #879 corrected
[engine-api.md](../docs/background/concepts/engine-api.md)
to say that hosts must call `dispose()`. It filed the
automatic cleanup here because it changes behavior, and
the timing of garbage collection is hard to test
red/green.

## Tasks

1. Create one `FinalizationRegistry` at load whose
   callback disposes the id it is handed, and capture
   its `register` as `main` captures `bind`.
2. Write a failing js/wasm test. Create a session, drop
   it, run a forced GC (`node --expose-gc` and
   `globalThis.gc()`, if the test runner allows it),
   then assert the id has left `sessions`. If forced GC
   is not available, test the callback directly with a
   live id instead.
3. In `newSessionProxy`, create one token object per
   session, bind it into every method after the id, and
   register the token with its id. Do not register the
   session object: a method taken off it outlives it,
   and the callback would dispose a session that method
   still uses. Bound arguments keep the token alive
   while any method is reachable. Do not keep the token
   on the Go side as an unregister token: a `js.Value`
   held in Go pins it, so it is never collected.
   `proxyDispose` gets the bound id and the token, and
   calls `unregister(token)` on the registry before it
   drops the id. Ids never repeat (plan 2610021439), so a
   callback that still fired after an explicit dispose
   would find no session; unregister anyway so the
   registry drops the entry with the session.
4. Check the WASM size budgets with
   [size_test.go](../cmd/mdsmith-wasm/size_test.go), and
   update the engine-api page to describe the fallback.

## Acceptance Criteria

- [x] A session dropped without `dispose()` leaves
      `sessions` once its object is collected
- [x] A method taken off a session object keeps working
      after the object alone is collected
- [x] An explicit `dispose()` followed by collection
      disposes the Session only once
- [x] A create/dispose loop still holds a fixed number
      of registered funcs and registry entries
- [x] All tests pass: `go test ./...` and
      `go run ./cmd/mdsmith-release test-js-wasm ./cmd/mdsmith-wasm`
- [x] `go tool golangci-lint run` reports no issues,
      on the host and with `GOOS=js GOARCH=wasm`
