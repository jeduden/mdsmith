---
id: 2610020046
title: >-
  Harden WASM bridge workspace input and version lookup
status: "✅"
model: sonnet
summary: >-
  workspaceFromJS in cmd/mdsmith-wasm/main.go accepts a JS
  array as a workspace and names the files "0", "1", and so
  on. A test also can't tell resolveVersion's build-info
  branch apart from its "(devel)" fallback. Reject arrays
  and add a build-info seam so both cases are pinned by
  js && wasm unit tests.
---
# Harden WASM bridge workspace input and version lookup

## Goal

Make `createSession` reject a JS array as `workspace`,
and let a unit test drive each branch of
`resolveVersion` on its own.

## Background

[Plan 2609201914][p1914] added
[bridge_test.go][bt] without changing production code.
Review of PR #873 found two gaps that need production
changes, so they are left to this plan.

- **Arrays.** In [main.go][main], `workspaceFromJS`
  checks only `v.Type() != js.TypeObject`. A JS array
  is a `TypeObject`, so `Object.keys` returns its
  indices. `createSession({workspace: ["# A"]})` then
  builds a one-file workspace whose path is `"0"`
  instead of failing. The `workspace` type in
  [engine-api.md][api] is `Record<string, string>`, so
  an array is a caller bug.
- **Build-info branch.** `resolveVersion` reads
  `debug.ReadBuildInfo()` directly. In a test binary
  `Main.Version` is `"(devel)"`, the same string as the
  last fallback. Deleting the build-info branch would
  therefore leave `TestResolveVersion` green.

## Tasks

1. Red: in [bridge_test.go][bt], add a
   `TestWorkspaceFromJS` subtest that passes
   `js.ValueOf([]any{"# A"})` and expects `nil`. Add a
   `createSession` test that expects the Promise to
   reject with an "options object" style error when
   `workspace` is an array.
2. Green: in `workspaceFromJS`, return `nil` when
   `js.Global().Get("Array").Call("isArray", v)` is
   true. Have `createSession` reject a non-object or
   array `workspace` that is present, and leave an
   absent `workspace` meaning an empty workspace.
3. Red: add `resolveVersion` subtests that swap a
   package-level `readBuildInfo = debug.ReadBuildInfo`
   variable. Cover `Main.Version: "v1.2.3"` (expect
   `v1.2.3`), an empty `Main.Version` (expect
   `(devel)`), and `ok == false` (expect `(devel)`).
4. Green: add the `readBuildInfo` variable to
   [main.go][main] and call it from `resolveVersion`.
5. Document the array rejection next to the
   `createSession` options in [engine-api.md][api].

Implemented as planned, with two additions. An `isRecord`
helper (non-null, non-array object) backs both checks,
with its own `TestIsRecord`. It also makes `createSession`
reject an array or `null` as the options object itself.
A present `null` workspace rejects too; only an absent
(`undefined`) workspace means an empty one.

## Acceptance Criteria

- [x] `createSession({workspace: []})` and
      `createSession({workspace: ["# A"]})` reject the
      Promise.
- [x] Deleting the build-info branch of
      `resolveVersion` makes a js && wasm unit test
      fail.
- [x] The bridge tests pass under Node in the `wasm`
      CI job.
- [x] All tests pass: `go test ./...`
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues

[p1914]: 2609201914_arch-fix-missing-unit-tests-0920.md
[bt]: ../cmd/mdsmith-wasm/bridge_test.go
[main]: ../cmd/mdsmith-wasm/main.go
[api]: ../docs/background/concepts/engine-api.md
