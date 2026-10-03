---
id: 2610030846
title: Silence wasm session finalizers after the Go program exits
status: "🔲"
model: sonnet
depends-on: [2610021800]
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
registers each session's token with a
`FinalizationRegistry` whose callback is the Go func
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

## Tasks

1. Write a failing js/wasm or Node harness test that
   exits the Go program, drops a session object, forces
   a JS collection, and asserts that no uncaught error
   is raised.
2. Pick the approach (unregister on exit, or a wrapper
   from the loader), and record the choice in this plan.
3. Implement it so the test passes.
4. Document the behavior after exit in
   [engine-api.md](../docs/background/concepts/engine-api.md).

## Acceptance Criteria

- [ ] Collecting a dropped session after the Go program
      exits raises no uncaught error
- [ ] The fix uses no `eval` or `Function` constructor
- [ ] All tests pass: `go test ./...` and
      `go run ./cmd/mdsmith-release test-js-wasm ./cmd/mdsmith-wasm`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues, natively and with
      `GOOS=js GOARCH=wasm`
