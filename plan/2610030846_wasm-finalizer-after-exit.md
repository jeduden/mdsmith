---
id: 2610030846
title: Silence wasm session finalizers after the Go program exits
status: "✅"
model: sonnet
summary: >-
  Once the wasm Go program exits (a panic in a
  `js.FuncOf` callback ends it), each session object
  that JS collects later runs the
  `FinalizationRegistry` callback, and `wasm_exec.js`
  throws "Go program has already exited" from inside
  the garbage collector's callback as an uncaught
  error. Make a collected session after exit produce
  no uncaught error, without `eval` or `Function`,
  which a strict CSP blocks.
---
# Silence wasm session finalizers after the Go program exits

## Goal

After the wasm Go program exits, collecting a dropped
session object raises no uncaught error in the host.

## Background

Plan
[2610021452](2610021452_wasm-collect-dropped-sessions.md)
registered each session's token with a
`FinalizationRegistry` whose callback was the Go func
`finalizeSession` in
[main.go](../cmd/mdsmith-wasm/main.go). A code review of
PR #891 found that this callback runs outside any
caller's `try`. Once the Go program has exited,
`wasm_exec.js` throws from every Go func call. Before
plan 2610021452, the error reached a host only when it
called a session method. Now each GC of a dropped
session raises it, in Obsidian for example, with no
call from the host.

A JS wrapper of the form `try { f(id) } catch {}` would
fix it, but building one at run time needs `Function`
or `eval`, which a strict Content Security Policy
blocks. Other options:

- Unregister every live token before the program
  exits. A Go panic gives no hook, so this needs plan
  [2610021800](2610021800_wasm-js-exception-outside-guard.md)
  to keep panics from ending the program first.
- Ship the wrapper in the JS loader that hosts already
  include (the Obsidian plugin and the npm wrapper),
  and pass it to the engine.
- Make the cleanup callback a native function that
  cannot reach Go: `Array.prototype.push` bound to a
  private queue. Go drains the queue at the start of
  each engine call.

Chosen: the native push queue. It needs no plan
2610021800 and no loader change. A collected session
is freed on the next engine call rather than at
collection time. Go's own collector runs only during
engine calls, and until it runs, Go holds the token
anyway, so most of the delay is one Go already imposes.
A session that JS collects after the last call, though,
stays in `sessions` until the host calls the engine
again; a Go func callback would have freed it at
collection time.

Every session method drains too, not only
`createSession` and `dispose`. A host that keeps one
session and drops others then still frees them. An
empty drain is one read of the queue's length: about
0.3 µs under Node, about 1% of `invalidate`, the
cheapest call (`BenchmarkDrainFinalizedEmpty`,
`BenchmarkInvalidate` in
[drain_bench_test.go](../cmd/mdsmith-wasm/drain_bench_test.go)).

## Tasks

1. [x] Write a failing js/wasm or Node harness test that
   exits the Go program, drops a session object, forces
   a JS collection, and asserts that no uncaught error
   is raised. Built as
   [after_exit.cjs](../cmd/mdsmith-wasm/testdata/after_exit.cjs),
   run by `TestWASMFinalizerAfterExit`. It puts
   `wasm_exec.js` in the state `runtime.wasmExit`
   leaves, after a V8 collection has queued the cleanup
   callbacks. It fails when no callback ran after the
   exit, so it cannot pass without exercising the path.
2. [x] Pick the approach (unregister on exit, a wrapper
   from the loader, or a native push queue), and record
   the choice in this plan. Chosen: the native push
   queue (see Background).
3. [x] Implement it so the test passes.
4. [x] Document the behavior after exit in
   [engine-api.md](../docs/background/concepts/engine-api.md).

## Acceptance Criteria

- [x] Collecting a dropped session after the Go program
      exits raises no uncaught error
- [x] The fix uses no `eval` or `Function` constructor
- [x] All tests pass: `go test ./...` and
      `go run ./cmd/mdsmith-release test-js-wasm ./cmd/mdsmith-wasm`
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues, natively and with
      `GOOS=js GOARCH=wasm`
