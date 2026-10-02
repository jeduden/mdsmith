---
id: 2610021556
title: Declare each wasm method's disposed result next to its impl
status: "✅"
model: sonnet
summary: >-
  `disposedResult` in the wasm entry point picks what a
  session method returns after `dispose()` by a name
  switch kept apart from `sharedMethodImpls`. A new
  synchronous method left out of the switch returns a
  rejecting Promise after dispose instead of its sync
  result, and no test catches that. Pair each method
  with its disposed result in one table.
---
# Declare each wasm method's disposed result next to its impl

## Goal

Each method in [main.go](../cmd/mdsmith-wasm/main.go)
states what it returns after `dispose()`. It does so in
the same table entry as its code. Then a new method
cannot get the async default by mistake.

## Background

`sharedMethodImpls` maps each method name to its
implementation. `disposedResult` is a separate `switch`
on the name: `capabilities` returns an empty list,
`invalidate` returns `undefined`, and every other name
returns a Promise that rejects with
`Error("session disposed")`. A synchronous method added
to `sharedMethodImpls` but not to the switch silently
gets the Promise branch. Found in the pre-merge review
of plan
[2610021237](2610021237_wasm-stale-dispose-reference.md).

## Tasks

1. Add a failing js/wasm test that, for every name in
   `sharedMethodImpls`, calls the method on a disposed
   session and checks that the result's shape (Promise,
   array, or other JS type) matches the live method's
   result.
2. Change `sharedMethodImpls` to map each name to a
   struct of the implementation and its disposed result,
   and delete the name switch in `disposedResult`. Build
   each entry with a constructor named for its result
   shape (`asyncMethod`, `stringListMethod`,
   `voidMethod`), so the disposed result always has the
   live shape. `methodTable` panics at package init,
   naming the entry, if any entry has a nil func. The async
   constructor also owns the Promise boilerplate the five
   async methods repeated. `newPromise` defers
   `rejectOnJSError` around every executor, so a JS
   exception raised in `createSession`, a live async
   method, or a disposed method's rejection rejects that
   Promise instead of ending the Go program.
3. Run `go run ./cmd/mdsmith-release test-js-wasm
   ./cmd/mdsmith-wasm` and the WASM size check in
   [size_test.go](../cmd/mdsmith-wasm/size_test.go).

## Acceptance Criteria

- [x] Each method's disposed result is declared in the
      same table entry as its implementation
- [x] A test fails when a disposed method's result shape
      differs from its live result shape
- [x] Building the table panics at package init,
      naming the entry, when it has a nil call or
      disposed func, and a test covers that check
- [x] All tests pass: `go test ./...` and the js/wasm
      suite
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues, natively and with
      `GOOS=js GOARCH=wasm`
