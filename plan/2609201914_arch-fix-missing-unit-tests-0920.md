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
  have no unit test named after them. The WASM tests go in a
  js && wasm test file that CI runs under Node.
---
# Add dedicated unit tests for AdvancePastLine and the WASM bridge helpers

## Goal

Give each function below its own unit test by name, the
`TestFoo` convention [tests.md][tests] requires.

## Background

The 2026-09-20 architecture audit flagged these functions.
That audit was in closed PR #843 and never reached main.
These have no test named after them on main:

- [internal/rules/astutil/astutil.go][astutil] —
  `AdvancePastLine`, the cursor helper that `SectionBodies`
  and the `overrepetition` rule call. Its doc comment states
  when a caller may thread `lo` across calls: `start` never
  decreases and the paragraphs are in ascending `Line` order.
  Only its callers' tests exercise it: `TestSectionBodies_*`,
  the `overrepetition` rule tests, and the `requiredmentions`
  and `requiredtextpatterns` rule tests, which reach it
  through `SectionBodies`.
- [cmd/mdsmith-wasm/main.go][wasm] — `resolveVersion`,
  `workspaceFromJS`, `uriAndSource`, and `allStrings`. The
  file carries the `js && wasm` build constraint, so a native
  `go test` never compiles these helpers. Two tests reach
  them through a built artifact. The Go smoke test
  `TestWASMCheckMatchesNative` in [smoke_test.go][wasm-smoke]
  runs [smoke.cjs][wasm-smoke-js], which calls
  `createSession`, `capabilities`, `check`, `dispose`, and
  `version`. The Obsidian plugin's
  [wasm-runtime.test.ts][obsidian-wasm-test] also calls
  `rename` and `move`, so it is the only test that reaches
  `allStrings`.

None of them is a `rule.Rule` method, an LSP capability
handler, or a CLI subcommand entry, so all are `tax`, not
`blocker`.

Neither exemption in [tests.md][tests-exemptions] applies.
None is generated code, and each branches, so none is a
trivial accessor.

## Out of scope

- `isWorkspaceRelativeTarget` in
  [cmd/mdsmith/backlinks.go][backlinks] has
  `TestIsWorkspaceRelativeTarget`, and `isAbsOrDriveOrUNC` now
  lives in `internal/pathutil` with its own
  `TestIsAbsOrDriveOrUNC` (both from PR #842).
- The other bridge functions: `createSession` and
  `newSessionProxy` in [main.go][wasm], and `newPromise`,
  `jsError`, and `toJS` in [jsbridge.go][wasm-jsbridge]. They
  also have no test by name, but the audit did not flag them.
  The task 2 test file is where their tests would go.

## Tasks

1. Add a table-driven `TestAdvancePastLine` to
   [astutil_test.go][astutil-test] that calls
   `AdvancePastLine` directly. Cover an empty slice and `lo`
   already at the end. Cover a prefix with `Line` below
   `start`, which is skipped. Cover an entry whose `Line`
   equals `start`, which is kept. Last, thread the returned
   index through calls with non-decreasing `start`.
2. Add a test file with the `//go:build js && wasm`
   constraint, such as `cmd/mdsmith-wasm/main_js_test.go`.
   Put `TestResolveVersion`, `TestWorkspaceFromJS`,
   `TestURIAndSource`, and `TestAllStrings` in it. Build
   inputs with `js.ValueOf`. Do not move or change the
   helpers.

  - `TestResolveVersion`: set the package-level `version`
     and restore it with `t.Cleanup`; the set value wins.
     Then, with `version` empty, the result is non-empty.
     Do not mark the test `t.Parallel`, since it writes a
     package variable. A `go test` binary always carries
     build info with `Main.Version` set to `(devel)`, so the
     final `(devel)` return is not reachable from a test.
     Do not add a seam to reach it.
  - `TestWorkspaceFromJS`: a non-object gives `nil`; an
     object keeps its string entries and drops a non-string
     entry.
  - `TestURIAndSource`: too few args and a non-string arg
     each return `ok == false`; two strings return both.
  - `TestAllStrings`: no args and all strings return `true`;
     one non-string returns `false`.

3. Run the task 2 tests in CI. Add a step to the `wasm` job
   in [ci.yml][ci], which already installs Node, next to the
   existing "Vet the WASM bridge" step:

   ```bash
   env -i PATH="$PATH" HOME="$HOME" \
     GOCACHE="$(go env GOCACHE)" GOMODCACHE="$(go env GOMODCACHE)" \
     GOOS=js GOARCH=wasm go test -v \
     -exec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec" \
     -run '^Test(ResolveVersion|WorkspaceFromJS|URIAndSource|AllStrings)$' \
     ./cmd/mdsmith-wasm/ > wasm-bridge.log || { cat wasm-bridge.log; exit 1; }
   cat wasm-bridge.log
   test "$(grep -c '^--- PASS: Test' wasm-bridge.log)" -eq 4
   ```

   Each part of the command is there for a reason:

  - `env -i`: `wasm_exec.js` caps arguments plus environment
     at about 8 KB. With a full shell environment the test
     binary exits with "total length of command line and
     environment variables exceeds limit".
  - The `grep -c` check: a `-run` filter that matches no
     test prints `[no tests to run]` and exits 0. The check
     fails the step unless all four tests ran and passed.
  - The `go_js_wasm_exec` path: [go.mod][gomod] pins Go
     1.25.11, and the job's `setup-go` reads it. Go 1.24
     moved the script from `misc/wasm` to `lib/wasm`.
  - The `-run` filter: [methods_test.go][wasm-methods],
     [size_test.go][wasm-size], and
     [smoke_test.go][wasm-smoke] have no build constraint, so
     they also compile under `js/wasm`. The filter keeps them
     from running there; the size and smoke tests shell out
     to `go` and `node`.

4. Do not delete or duplicate the tests named in the
   Background section. They cover the public contract and
   stay as they are.
5. `go build ./...`, `go vet ./...`, and `go test ./...`
   pass. The wasm job's `GOOS=js GOARCH=wasm go vet` step
   also vets the new test file; `golangci-lint` runs natively
   and skips it.

## Acceptance Criteria

- [ ] `AdvancePastLine` has a dedicated `TestAdvancePastLine`.
- [ ] `resolveVersion`, `workspaceFromJS`, `uriAndSource`,
      and `allStrings` each have a dedicated test in a
      `js && wasm` test file.
- [ ] The CI `wasm` job runs those four tests under Node and
      fails if any of them did not run.
- [ ] No production code changed.
- [ ] `go test ./...` is green.
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [ ] `mdsmith check .` is green.

[tests]: ../docs/development/architecture/tests.md
[tests-exemptions]: ../docs/development/architecture/tests.md#exemptions
[astutil]: ../internal/rules/astutil/astutil.go
[astutil-test]: ../internal/rules/astutil/astutil_test.go
[wasm]: ../cmd/mdsmith-wasm/main.go
[wasm-jsbridge]: ../cmd/mdsmith-wasm/jsbridge.go
[wasm-methods]: ../cmd/mdsmith-wasm/methods_test.go
[wasm-size]: ../cmd/mdsmith-wasm/size_test.go
[wasm-smoke]: ../cmd/mdsmith-wasm/smoke_test.go
[wasm-smoke-js]: ../cmd/mdsmith-wasm/testdata/smoke.cjs
[obsidian-wasm-test]: ../editors/obsidian/src/wasm-runtime.test.ts
[ci]: ../.github/workflows/ci.yml
[gomod]: ../go.mod
[backlinks]: ../cmd/mdsmith/backlinks.go
