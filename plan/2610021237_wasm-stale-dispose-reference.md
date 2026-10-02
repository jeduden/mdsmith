---
id: 2610021237
title: Silence a stale wasm session dispose reference
status: "✅"
model: sonnet
summary: >-
  Since plan 2610021027, a wasm session's `dispose`
  releases its own `js.Func`. A `dispose` reference taken
  before the first call (`const d = session.dispose; d();
  d()`) now logs "call to released function" on the second
  call, where it was a silent no-op before. Decide whether
  a second call through such a reference can stay silent
  without leaking one func per session.
---
# Silence a stale wasm session dispose reference

## Goal

A second call through a stored `dispose` reference stays
silent, and a create/dispose loop still holds a fixed
number of registered funcs.

## Background

The pre-merge review of PR #878 (plan
[2610021027](2610021027_wasm-dispose-func-release.md))
found this. `proxyDispose` in
[main.go](../cmd/mdsmith-wasm/main.go) releases its own
func on the first call and points `proxy.dispose` at the
shared `disposedNoop`. A call through the session object
reaches the no-op. A reference taken earlier reaches the
released func, which returns `undefined` but makes
`syscall/js` log "call to released function". The
[engine API page](../docs/background/concepts/engine-api.md)
documents this.

A frozen session object hits a related path. Think of
`Object.freeze(session)`, or a store that freezes its
state. There `proxy.Set` cannot swap in the stand-ins.
Plan 2610021027 keeps that dispose func registered. A
nil guard makes a second `dispose()` silent. The other
methods still reach released funcs.

One read-only method hits the same path. Take
`Object.defineProperty(session, "check", { writable:
false })`. `dispose` releases that func, but the swap
does not land. Then `session.check()` returns `undefined`,
not a rejecting `Promise`.

The simple alternatives each fail one goal:

- Keeping the per-session func unreleased leaks one
  handler-table entry per session.
- A single shared `dispose` that finds its session
  through `this` breaks `const d = session.dispose; d()`,
  because `this` is `undefined` there.

One option: a shared func plus a JS-side wrapper that
binds a session id, so the binding lives in JS and is
collected with the session object. Check its size cost
against the WASM budgets on the engine API page.

## Tasks

1. Add a js/wasm test that stores `session.dispose`,
   calls it twice, and fails if the second call reaches a
   released func (a release hook seam can record that).
   Add the same check for the other methods of a frozen
   session object, and for one read-only method.
2. Choose a design that keeps
   `TestNewSessionProxy_DisposeLeavesNoFuncs` green, or
   record why the current trade-off stays.
3. Update the engine API page's dispose paragraph.

Design chosen: the JS-side wrapper option. Shared method funcs
register once and take a session id first; each session holds
`Function.prototype.bind` of them (no `eval`, no per-session func).
`dispose` deletes the id from a Go-side registry, so every call
through any reference takes the disposed path. This supersedes the
release-and-swap design: nothing is released, so nothing can be
reached after release. The standard Go artifact is within
its budgets ([size_test.go](../cmd/mdsmith-wasm/size_test.go)
measures 13.3 MiB raw, 4.1 MiB gzip, against 14 and 4.25
MiB); the TinyGo budget is checked by the `tinygo-wasm` CI
job only.

The design trades two properties, each filed as a plan.
The Go session now stays registered until `dispose()`,
even once the JS object is collected (plan
[2610021452](2610021452_wasm-collect-dropped-sessions.md)).
A session id is a guessable integer, so a raw shared func
that leaks through a patched `Reflect.apply` can drive any
live session (plan
[2610021439](2610021439_wasm-unforgeable-session-binding.md)).

## Acceptance Criteria

- [x] `const d = session.dispose; d(); d()` logs nothing,
      or the engine API page states why it still does.
- [x] Calls to a frozen session's methods, or to one
      read-only method, after `dispose()` log nothing, or
      the engine API page states why they still do.
- [x] N create/dispose cycles leave the func count the
      same as one cycle.
- [x] `go run ./cmd/mdsmith-release test-js-wasm
      ./cmd/mdsmith-wasm` passes.
- [x] All tests pass: `go test ./...`
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues, natively and with
      `GOOS=js GOARCH=wasm`
