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
   constraint, such as `cmd/mdsmith-wasm/bridge_test.go`.
   Avoid a `_js` or `_wasm` file-name suffix; it adds an
   implicit constraint on top of the build tag. Put
   `TestResolveVersion`, `TestWorkspaceFromJS`,
   `TestURIAndSource`, and `TestAllStrings` in it. Build
   inputs with `js.ValueOf`. Do not move or change the
   helpers.

   `TestResolveVersion` sets the package-level `version` and
   restores it with `t.Cleanup`; the set value wins. With
   `version` empty, the result must equal the
   `Main.Version` that `debug.ReadBuildInfo` reports. On Go
   1.25.11 that is `(devel)` in a test binary, both native
   and `js/wasm`, even inside this Git checkout. So the final
   `(devel)` fallback is not reachable from a test; do not
   add a seam for it. The test writes a package variable, so
   it must not call `t.Parallel`.

   `TestWorkspaceFromJS`: a non-object gives `nil`, and an
   object keeps its string entries and drops a non-string
   entry. `TestURIAndSource`: too few args and a non-string
   arg each return `ok == false`, and two strings return
   both. `TestAllStrings`: no args and all strings return
   `true`, and one non-string returns `false`.

3. Run the task 2 tests in CI. Add a step to the `wasm` job
   in [ci.yml][ci], which already installs Node, next to the
   existing "Vet the WASM bridge" step. The test names are
   listed once, and both the `-run` filter and the pass count
   come from that list:

   ```bash
   tests='ResolveVersion|WorkspaceFromJS|URIAndSource|AllStrings'
   env -i PATH="$PATH" HOME="$HOME" \
     GOCACHE="$(go env GOCACHE)" GOMODCACHE="$(go env GOMODCACHE)" \
     GOTOOLCHAIN="$(go env GOTOOLCHAIN)" GOFLAGS="$(go env GOFLAGS)" \
     GOOS=js GOARCH=wasm go test -v \
     -exec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec" \
     -run "^Test($tests)\$" \
     ./cmd/mdsmith-wasm/ > wasm-bridge.log || { cat wasm-bridge.log; exit 1; }
   cat wasm-bridge.log
   want=$(printf '%s\n' "$tests" | tr '|' '\n' | wc -l)
   test "$(grep -c '^--- PASS: Test' wasm-bridge.log)" -eq "$want"
   ```

   `env -i` is needed because `wasm_exec.js` caps arguments
   plus environment at about 8 KB. With a full shell
   environment the test binary exits with "total length of
   command line and environment variables exceeds limit". The
   step passes through only what `go test` needs: `PATH`,
   `HOME`, the two caches, `GOTOOLCHAIN` (so the job's
   `setup-go` toolchain is used, not a download), and
   `GOFLAGS`.

   The count check is there because a `-run` filter that
   matches no test prints `[no tests to run]` and exits 0.
   The step fails unless every listed test ran and passed.

   The `go_js_wasm_exec` path depends on the Go version.
   [go.mod][gomod] pins Go 1.25.11, and the job's `setup-go`
   reads it. Go 1.24 moved the script from `misc/wasm` to
   `lib/wasm`.

   The `-run` filter matters because
   [methods_test.go][wasm-methods], [size_test.go][wasm-size],
   and [smoke_test.go][wasm-smoke] have no build constraint,
   so they also compile under `js/wasm`. The filter keeps them
   from running there; the size and smoke tests shell out to
   `go` and `node`.

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
