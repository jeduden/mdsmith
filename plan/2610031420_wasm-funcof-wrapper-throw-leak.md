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

Fix 1, without a pool: `sharedExecutor` registers one Promise
executor func (`runPromiseCall`) at load, from `sharedMethods`, and
`newPromise` constructs every Promise with it. The per-call context
slot is a Go-side stack, `promiseCalls`: `newPromise` pushes its call,
constructs the Promise, and pops that same call (by identity, not
whatever is on top) in a defer. A spec Promise
runs its executor synchronously during construction, so the shared
func runs the call on top of the stack. No func is registered per
call, so no patched `Reflect.apply`, `Reflect.get`, or
`_makeFuncWrapper` can strand a func-table entry. The stack slot is
cleared on pop, so the stack's backing array does not keep a Session
reachable. The size budgets are unaffected.

The executor func is registered once, and a failed registration is not
retried, since each retry would strand an entry.

This replaced a first attempt (fix 2) that captured `Reflect.apply` at
load and refused, returning `undefined`, when it had changed. Review
round 1 found that design wanting. Reading `apply` with `Get` could end
the program, since that read runs outside a `try`. A throwing
`_makeFuncWrapper`, a one-shot `Reflect.get`, or a patch in place at
load still leaked. And a benign delegating `Reflect.apply` silently
turned off every async method. The shared executor needs no global
identity check, so all of these go away.

Review round 2 found a hole in the one shared executor. A script that
kept it could run another call. Called during a later call's
construction (a Node `async_hooks` init hook fires before the
executor), it ran that call with the script's own `resolve`.

So `newPromise` now hands the constructor the shared executor bound,
through the `bindTo` captured at load, to the call's number.
`runPromiseCall` runs only the call on top of the stack whose number
matches. The number is AES of a counter under the session-id key, not
the counter itself. A script that once saw the unbound executor and a
number, through a `Reflect.apply` patched for a while, cannot step to
the next call's number. A bound function is plain JS and registers no
Go func. When that bind fails, the call falls back to the unbound
executor, so its Promise still settles.

A call of an executor that a patched constructor kept runs nothing
once its own call is no longer on top. A nested run from inside
`resolve` still runs, as it did when each call had its own func. A
second run after the outermost one returned runs nothing, as a
released func would not.

## Tasks

1. [x] Write a failing js/wasm test that patches
   `Reflect.apply` to throw on `_makeFuncWrapper`,
   calls an async session method, and asserts no func
   is registered through the `funcOf` seam and the
   wrapper is never called. (Red: the per-call
   `FuncOf` panicked out of the test, then, under
   the Reflect.apply check, returned `undefined`.)
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
- [x] An executor a script kept from one call runs
      nothing when called during another call
- [x] The unbound executor run with the number one past
      a call's number runs nothing during the next call
- [x] All tests pass: `go test ./...` and
      `go run ./cmd/mdsmith-release test-js-wasm ./cmd/mdsmith-wasm`
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues, natively and with
      `GOOS=js GOARCH=wasm`
