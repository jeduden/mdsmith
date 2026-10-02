---
id: 2610021439
title: Make wasm session method bindings unforgeable
status: "🔲"
model: sonnet
summary: >-
  Wasm session methods are shared funcs bound to a
  sequential integer session id. The engine captures
  `Function.prototype.bind` and `call` at load, but a
  script that patches either before the engine loads,
  or patches `Reflect.apply` at any time, can capture
  a raw shared func and call any live session by
  trying ids 0, 1, 2, and so on. Decide whether to key
  sessions by an unguessable token or close the gap
  another way.
---
# Make wasm session method bindings unforgeable

## Goal

A script in the same JS global scope that holds a raw shared
session func cannot reach a session whose object it was
never given.

## Background

The shared funcs come from plan
[2610021237](2610021237_wasm-stale-dispose-reference.md),
in [main.go](../cmd/mdsmith-wasm/main.go). Review round
1 of PR #879 made `main` capture
`Function.prototype.call.bind(Function.prototype.bind)`
before it exposes the API. A `bind` or `call` patch
made after load never sees a raw shared func. A patch
made before the engine loads still does. The ids it
can then pass are small sequential integers.

`Reflect.apply` is a wider gap. `wasm_exec.js` looks it
up on every Go-to-JS call, so a `Reflect.apply`
replaced after load sees each raw shared func and id
when a session is built. It also sees every session
object the engine resolves. A random id or a token
object crosses the same call, so neither approach
below hides it from that script. Closing it means a
`wasm_exec.js` that captures `Reflect` at load, or a
Goal that names this limit.

In Obsidian, every plugin shares one global scope and can
already reach the others' objects, so this is hardening,
not a privilege boundary. It is filed as out of scope
for the stale-dispose fix.

## Tasks

1. Pick an approach: random 53-bit ids drawn with
   `math/rand/v2` and retried on collision, or a
   per-session JS token object compared with
   `js.Value.Equal`. Weigh the WASM size budget in
   [engine-api.md](../docs/background/concepts/engine-api.md).
   Decide whether to also capture `Reflect` at load in
   the shipped `wasm_exec.js`.
2. Write a failing js/wasm test that calls a raw shared
   func with every id from 0 to `nextSessionID` and
   reaches a live session.
3. Implement the chosen approach and make the test pass.
4. Update the engine-api page if the binding contract
   changes.

## Acceptance Criteria

- [ ] Calling a raw shared func with a guessed id does
      not reach a live session
- [ ] A create/dispose loop still holds a fixed number
      of registered funcs and registry entries
- [ ] All tests pass: `go test ./...` and
      `go run ./cmd/mdsmith-release test-js-wasm ./cmd/mdsmith-wasm`
- [ ] `go tool golangci-lint run` reports no issues,
      on the host and with `GOOS=js GOARCH=wasm`
