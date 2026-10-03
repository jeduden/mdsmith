---
id: 2610021800
title: Contain wasm JS exceptions outside the executor guard
status: "✅"
model: sonnet
summary: >-
  `rejectOnJSError` covers only the Promise executor
  body. A JS exception thrown by `Promise.New` in
  `newPromise`, by a synchronous session method, or
  after `registerSession` registers a session still
  panics. The panic ends the Go program or leaves a
  session in `sessions` that no proxy can dispose.
  Decide what each path does on a JS exception, and
  release the session on a failed create.
---
# Contain wasm JS exceptions outside the executor guard

## Goal

A JS exception raised while a page calls the wasm
engine ends that one call. It does not end the Go
program, and it does not leave a session registered
that no proxy can reach.

## Background

Final review of PR #880 (plan
[2610021556](2610021556_wasm-disposed-result-table.md))
found two gaps in
[main.go](../cmd/mdsmith-wasm/main.go) and
[jsbridge.go](../cmd/mdsmith-wasm/jsbridge.go). Both
need a script that replaces a JS global the engine
calls through: `Promise`, `Reflect.construct`, or
`Reflect.apply`. `wasm_exec.js` looks up `Reflect.apply`
on every Go-to-JS call. Plan
[2610021439](2610021439_wasm-unforgeable-session-binding.md)
covers what such a script can reach. This plan covers
what its exceptions break.

- `registerSession` (then named `newSessionProxy`)
  stores the session in `sessions`
  before the create Promise's resolve runs. If resolve
  throws, `rejectOnJSError` rejects the create, but the
  session stays registered with no proxy to dispose it.
  Plan 2610021439 already moved the store after
  `bindMethods`, and
  `TestRegisterSession_BindThrowRegistersNoSession`
  covers a `bindTo` that throws.
- `newPromise` calls `Promise.New(handler)` outside the
  guard. A throwing `Promise` constructor panics inside
  the shared func and ends the program. If the
  constructor never calls the executor, the handler
  func is never released. The synchronous
  `capabilities` and `invalidate` paths have no guard
  at all.

## Tasks

1. Write a failing js/wasm test that makes the create
   Promise's resolve throw, then asserts that `sessions`
   is back to its size before the create.
2. Remove the session from `sessions` when create
   fails after it was registered.
3. Write a failing js/wasm test that replaces
   `globalThis.Promise` with a constructor that throws,
   then calls an async session method. Assert that the
   program keeps running and no func stays registered.
4. Decide what a synchronous method returns on a JS
   exception: rethrow it to the caller, or return its
   disposed value. Apply the choice to the
   `Promise.New` call and to the sync paths.
5. Update
   [engine-api.md](../docs/background/concepts/engine-api.md)
   if the error contract changes.

## Acceptance Criteria

- [x] A JS exception during create leaves `sessions`
      the same size as before the create (a throwing
      `then` getter rejects without throwing to Go;
      plan
      [2610031253](2610031253_wasm-create-then-getter-leak.md)
      covers that path)
- [x] A throwing `Promise` constructor or a throwing
      sync-path call does not end the Go program
- [x] No func stays registered after either failure
- [x] All tests pass: `go test ./...` and
      `go run ./cmd/mdsmith-release test-js-wasm ./cmd/mdsmith-wasm`
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues, natively and with
      `GOOS=js GOARCH=wasm`

## Review Round 1

Code review of PR #894 found five more ways a JS
failure could end the program. A `Promise` that is no
constructor raised a `*js.ValueError` that only
`recoverJS` catches. A `Promise` passed the executor
too few arguments, or a `reject` that threw. A
`finalizer.unregister` call in `dispose()` threw. A
disposed-value fallback failed the same way as the
call it replaced. Each now has a red/green test.

`drainFirst` recovers any JS failure as a last
resort, so every entry point is covered, not only the
guarded call sites. `dispose()` drops the session
before it calls `unregister`.
