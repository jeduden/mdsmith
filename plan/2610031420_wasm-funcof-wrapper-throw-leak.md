---
id: 2610031420
title: Free the func when js.FuncOf's wrapper call throws
status: "✅"
model: sonnet
summary: >-
  `syscall/js.FuncOf` stores the handler in its private
  func table, then calls `_makeFuncWrapper` through
  `Reflect.apply`. A page that patches `Reflect.apply`
  to throw makes that call panic after the entry is
  stored, and the panic drops the id. The entry, with
  its closure over the executor, the arguments, and the
  `*mdsmith.Session`, stays for good.
---
# Free the func when js.FuncOf's wrapper call throws

## Goal

An async method call or `createSession` call that fails
because building the JS wrapper func throws leaves no
entry in `syscall/js`'s func table.

## Background

Review round 3 of PR #894 (plan
[2610021800](2610021800_wasm-js-exception-outside-guard.md))
found this path. `newPromise` in
[jsbridge.go](../cmd/mdsmith-wasm/jsbridge.go) builds
its executor func through the `funcOf` seam in
[main.go](../cmd/mdsmith-wasm/main.go), which calls
`js.FuncOf`. In the standard library, `FuncOf` adds the
handler to the `funcs` map before it calls
`jsGo.Call("_makeFuncWrapper", id)`. If that call throws,
`Call` panics with a `js.Error`, `recoverJS` turns it
into an undefined result, and the id is lost. Nothing
outside `syscall/js` can call `Release` without the
`js.Func`, so this package cannot free the entry.

Possible fixes:

- Build every wrapper ahead of need, at a time the
  page cannot yet have patched `Reflect.apply`, and
  reuse them through a small pool with a per-call
  context slot. Each wrapper is bound once, like the
  shared session methods.
- Capture `Reflect.apply` at module start and check
  that it is unchanged before each `FuncOf`. If it has
  changed, refuse the call with a rejection instead of
  registering a func.
- Propose a fix upstream that makes `FuncOf` release
  the id when the wrapper call panics.

## Decision

Fix 2: capture `Reflect.apply` at load (`captureGlobals`) and
compare it with the current one before each `FuncOf`
(`reflectApplyIntact`, checked at the top of `newPromise`).
It is the smallest change: no pool, no per-call context
slot, no upstream dependency, and every per-call func goes
through `newPromise`, so one check covers it. The
size budgets are unaffected.

Deviation: the plan says to refuse with a rejection. A
rejection needs a Promise, and building or rejecting one
goes through the same patched `Reflect.apply` (`Call`) or a
second `FuncOf`, so `newPromise` returns `undefined`, as it
does for a patched `Promise` that fails.

Known limit: a `Reflect.apply` accessor that returns the
original on the first read and throws on the next passes the
check. Recorded in engine-api.md.

## Tasks

1. [x] Write a failing js/wasm test that patches
   `Reflect.apply` to throw on `_makeFuncWrapper`,
   calls an async session method, and asserts no func
   is registered through the `funcOf` seam and the
   wrapper is never called. (Red: the unguarded
   `FuncOf` panicked out of the test.)
2. [x] Pick one of the fixes above and record why in this
   plan.
3. [x] Make the test pass. The engine-api.md size budgets
   must still hold.
4. [x] Update
   [engine-api.md](../docs/background/concepts/engine-api.md)
   with the hostile-global behavior that results.

## Acceptance Criteria

- [x] A throwing `_makeFuncWrapper` call leaves no func
      registered after the call
- [x] The session the call ran against is collectable
      after `dispose()`
- [x] All tests pass: `go test ./...` and
      `go run ./cmd/mdsmith-release test-js-wasm ./cmd/mdsmith-wasm`
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues, natively and with
      `GOOS=js GOARCH=wasm`
