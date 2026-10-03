---
id: 2610031420
title: Free the func when js.FuncOf's wrapper call throws
status: "🔲"
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

## Tasks

1. Write a failing js/wasm test that patches
   `Reflect.apply` to throw on `_makeFuncWrapper`,
   calls an async session method, and asserts the
   func-table size (counted through the `funcOf` seam)
   is unchanged.
2. Pick one of the fixes above and record why in this
   plan.
3. Make the test pass. The engine-api.md size budgets
   must still hold.
4. Update
   [engine-api.md](../docs/background/concepts/engine-api.md)
   with the hostile-global behavior that results.

## Acceptance Criteria

- [ ] A throwing `_makeFuncWrapper` call leaves no func
      registered after the call
- [ ] The session the call ran against is collectable
      after `dispose()`
- [ ] All tests pass: `go test ./...` and
      `go run ./cmd/mdsmith-release test-js-wasm ./cmd/mdsmith-wasm`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues, natively and with
      `GOOS=js GOARCH=wasm`
