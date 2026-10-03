---
id: 2610031253
title: Free the session when a then getter rejects the create
status: "✅"
model: sonnet
summary: >-
  `createSession` resolves its Promise with the session
  object. A native resolve reads `then` on that object,
  and a page that defines a throwing `then` getter on
  `Object.prototype` makes resolve reject the create
  without throwing to Go. The session stays in
  `sessions` until the FinalizationRegistry collects
  its token, and forever on a host with no registry.
---
# Free the session when a then getter rejects the create

## Goal

A throwing `then` getter on `Object.prototype` never
strands a session. The create resolves with a session
object, and once the caller disposes it `sessions` is
the same size as before the create.

## Background

Review round 1 of PR #894 (plan
[2610021800](2610021800_wasm-js-exception-outside-guard.md))
found this path. `createSession` in
[main.go](../cmd/mdsmith-wasm/main.go) disposes the
session when resolve itself throws. A native Promise
resolve never throws. It reads `value.then`, and when
that read throws it rejects the Promise with the
exception. Go sees a resolve that returned normally,
so the session stays registered while the caller gets
a rejection and no session object.

Two fixes look possible. The session object could
carry its own non-enumerable `then: undefined`, so the
lookup never reaches `Object.prototype`. Or Go could
watch the create Promise and dispose the session when
it rejects. The first changes the object's shape that
[engine-api.md](../docs/background/concepts/engine-api.md)
documents. The second costs a func per create.

## Decision

Chose the own `then: undefined` property. Go watching the
Promise would register a func per create and still leave the
session registered until the rejection handler runs. The
property closes the path at the source, costs no func, and is
non-enumerable, so `Object.keys(session)` and
`TestRegisterSession_KeysMatchSessionMethodNames` are
unchanged. The create now resolves instead of rejecting, and
the caller owns a session it can dispose.

The descriptor has a null prototype. A page's
`Object.prototype.get` then cannot make the call throw. Its
`Object.prototype.enumerable` cannot list `then` in
`Object.keys`. The engine captures `Object.defineProperty`
and the descriptor once at load, beside `bind`, and freezes
the descriptor. A replacement installed later never runs and
never sees the session object.

A patched `Reflect.apply` sees the descriptor on every
create, but cannot change it for the sessions created after
it is removed. The session object and its token come from an
`Object` captured at load too, so a later global `Object`
cannot hand back a Proxy whose `then` trap throws. The JS
string `"then"` is also converted once at load, so a create
does no extra string conversion. A patched
`Reflect.construct` still sees each session object; like
`Reflect.apply`, it is a limit documented in engine-api.md,
per plan 2610021439.

The `mdsmith` global gets the same own `then`. The Obsidian
plugin's async engine load returns it, so the getter would
reject that load. The plugin then clears its cached load and
starts another Go runtime that never exits on each retry.
The plugin's `SessionRuntime`, which its async
`createRuntime` returns, gets its own `then` too, or its
resolve would strand the session the same way.

Review round 3 closed the same bug class in three more
places. Each method is added with the captured
`defineProperty`, not a Set, so an accessor or read-only
value of that name on `Object.prototype` cannot take it.
`Object.keys` and `isRecord`'s `toString` are captured at
load like `Object`. Each load-time capture is guarded, so
one that throws leaves its value undefined and makes every
create reject, rather than stopping the engine from
loading.

## Tasks

1. Write a failing js/wasm test that defines a
   throwing `then` getter on `Object.prototype`, calls
   `createSession`, and asserts that `sessions` is
   back to its size before the create.
2. Pick one of the two fixes above and record why in
   this plan.
3. Make the test pass; keep
   `TestRegisterSession_KeysMatchSessionMethodNames`
   green.
4. Update
   [engine-api.md](../docs/background/concepts/engine-api.md)
   if the session object's shape changes.

## Acceptance Criteria

- [x] A throwing `then` getter on `Object.prototype`
      leaves `sessions` the same size after a create
      and its dispose
- [x] No func stays registered after that create
- [x] All tests pass: `go test ./...` and
      `go run ./cmd/mdsmith-release test-js-wasm ./cmd/mdsmith-wasm`
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues, natively and with
      `GOOS=js GOARCH=wasm`
