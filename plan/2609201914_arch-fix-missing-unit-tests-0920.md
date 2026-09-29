---
id: 2609201914
title: >-
  Add dedicated unit tests for AdvancePastLine and the WASM
  bridge helpers
status: "🔲"
model: sonnet
summary: >-
  AdvancePastLine in internal/rules/astutil and four helpers
  in the js/wasm-only cmd/mdsmith-wasm/main.go
  (resolveVersion, workspaceFromJS, uriAndSource, allStrings)
  have no unit test named after them. Re-filed from closed PR
  #843 and re-checked against main on 2026-09-29.
---
# Add dedicated unit tests for AdvancePastLine and the WASM bridge helpers

## Goal

Give each function below its own unit test by name, the
`TestFoo` convention [tests.md][tests] requires.

## Background

Closed PR #843 first filed this plan with its 2026-09-20
architecture audit. That PR closed as a duplicate, so neither
the audit entry nor this plan reached main. This copy is
re-filed from it. Every item was re-checked against main on
2026-09-29, and these still have no test named after them:

- [internal/rules/astutil/astutil.go][astutil] —
  `AdvancePastLine`, the cursor helper that `SectionBodies`
  and the `overrepetition` rule call. Its doc comment states
  when a caller may thread `lo` across calls: `start` never
  decreases and the paragraphs are in ascending `Line` order.
  Only `TestSectionBodies_*` and the `overrepetition` rule
  tests exercise it, as a side effect.
- [cmd/mdsmith-wasm/main.go][wasm] — `resolveVersion`,
  `workspaceFromJS`, `uriAndSource`, and `allStrings`. The
  file carries the `js && wasm` build constraint, so a native
  `go test` never compiles these helpers. Only the Node smoke
  test `TestWASMCheckMatchesNative` in
  [smoke_test.go][wasm-smoke] reaches them, through the built
  artifact.

None of them is a `rule.Rule` method, an LSP capability
handler, or a CLI subcommand entry, so all are `tax`, not
`blocker`.

Neither exemption in [tests.md][tests-exemptions] applies.
None is generated code, and each of the four WASM helpers
branches, so none is a trivial accessor. An earlier draft
offered a "no test by design" comment for them; that is not
allowed, so this plan asks for real tests.

## Scope changes since PR #843

- `isWorkspaceRelativeTarget` and `isAbsOrDriveOrUNC` in
  [cmd/mdsmith/backlinks.go][backlinks] are left out. On main
  both still lack a test by name in `cmd/mdsmith`. Open PR
  #842 moves `isAbsOrDriveOrUNC` into a new `internal/pathutil`
  package with its own `TestIsAbsOrDriveOrUNC`, and adds
  `TestIsWorkspaceRelativeTarget`. If #842 closes without
  merging, add both back here.
- `allStrings` was also listed in
  [plan 2609061915][plan-0906]. It now lives only here, next
  to the other WASM bridge helpers.

## Tasks

1. Add a table-driven `TestAdvancePastLine` to
   [astutil_test.go][astutil-test] that calls
   `AdvancePastLine` directly. Cover an empty slice and `lo`
   already at the end. Cover a prefix with `Line` below
   `start`, which is skipped. Cover an entry whose `Line`
   equals `start`, which is kept. Last, thread the returned
   index through calls with non-decreasing `start`.
2. Move `resolveVersion` and the `version` variable it reads
   out of `main.go` into a file with no build constraint. It
   uses no `syscall/js`. The `-X main.version=...` ldflags in
   `build.sh` still apply, since the package stays `main`.
   Add a native `TestResolveVersion` for both branches: a set
   `version` wins, and an empty `version` falls back to a
   non-empty string. The build-info value differs between
   `go test` and a release build, so do not pin it exactly.
   Check that `golangci-lint` stays clean: in the native build
   only the test calls the function.
3. Add a test file with the `js && wasm` build constraint,
   such as `cmd/mdsmith-wasm/main_js_test.go`. Put
   `TestWorkspaceFromJS`, `TestURIAndSource`, and
   `TestAllStrings` in it, and build inputs with `js.ValueOf`.
   Cover each branch. For `workspaceFromJS`, pass a
   non-object and a non-string entry. For `uriAndSource`,
   pass too few args and a non-string arg. For `allStrings`,
   pass no args, all strings, and one non-string.
4. Run the task 3 tests in CI. Add a step to the `wasm` job
   in [ci.yml][ci], which already installs Node, next to the
   existing "Vet the WASM bridge" step:

   ```bash
   GOOS=js GOARCH=wasm go test \
     -exec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec" \
     -run '^(TestWorkspaceFromJS|TestURIAndSource|TestAllStrings)$' \
     ./cmd/mdsmith-wasm/
   ```

   This command form passed locally on 2026-09-29 with Go
   1.25 and Node 22. `wasm_exec.js` refuses to start when the
   environment is too large ("total length of command line
   and environment variables exceeds limit"). If CI hits that,
   run the step under `env -i`, keeping `PATH`, `HOME`, and
   the Go cache and module variables.
5. Do not delete or duplicate the behavior-level tests named
   above. They cover the public contract and stay as-is.
6. `go build ./...`, `go vet ./...`, and `go test ./...`
   pass.

## Acceptance Criteria

- [ ] `AdvancePastLine` has a dedicated `TestAdvancePastLine`.
- [ ] `resolveVersion` has a native `TestResolveVersion`.
- [ ] `workspaceFromJS`, `uriAndSource`, and `allStrings` each
      have a dedicated test in a `js && wasm` test file, and
      the CI `wasm` job runs that file under Node.
- [ ] `go test ./...` is green.
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [ ] `mdsmith check .` is green.

[tests]: ../docs/development/architecture/tests.md
[tests-exemptions]: ../docs/development/architecture/tests.md#exemptions
[astutil]: ../internal/rules/astutil/astutil.go
[astutil-test]: ../internal/rules/astutil/astutil_test.go
[wasm]: ../cmd/mdsmith-wasm/main.go
[wasm-smoke]: ../cmd/mdsmith-wasm/smoke_test.go
[ci]: ../.github/workflows/ci.yml
[backlinks]: ../cmd/mdsmith/backlinks.go
[plan-0906]: 2609061915_arch-fix-touched-set-unit-tests-0906.md
