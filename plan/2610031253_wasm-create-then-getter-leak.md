---
id: 2610031253
title: Free the session when a then getter rejects the create
status: "🔲"
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

A create that rejects because a `then` lookup on the
session object throws leaves `sessions` the same size
as before the create.

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

- [ ] A throwing `then` getter on `Object.prototype`
      leaves `sessions` the same size after a create
- [ ] No func stays registered after that create
- [ ] All tests pass: `go test ./...` and
      `go run ./cmd/mdsmith-release test-js-wasm ./cmd/mdsmith-wasm`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues, natively and with
      `GOOS=js GOARCH=wasm`
