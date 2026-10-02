---
id: 2610021237
title: Silence a stale wasm session dispose reference
status: "🔲"
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
2. Choose a design that keeps
   `TestNewSessionProxy_DisposeLeavesNoFuncs` green, or
   record why the current trade-off stays.
3. Update the engine API page's dispose paragraph.

## Acceptance Criteria

- [ ] `const d = session.dispose; d(); d()` logs nothing,
      or the engine API page states why it still does.
- [ ] N create/dispose cycles leave the func count the
      same as one cycle.
- [ ] `go run ./cmd/mdsmith-release test-js-wasm
      ./cmd/mdsmith-wasm` passes.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
