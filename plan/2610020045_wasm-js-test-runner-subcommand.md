---
id: 2610020045
title: >-
  Move the WASM bridge test-runner shell into mdsmith-release
status: "🔲"
model: sonnet
summary: >-
  The ci.yml wasm job step "Unit-test the WASM bridge helpers
  under Node" holds about 25 lines of inline shell. It finds
  the js/wasm-only test files, extracts the Test names, and
  checks the PASS count. Move that logic into a tested
  `mdsmith-release test-js-wasm` subcommand, as
  release-tooling.md requires.
---
# Move the WASM bridge test-runner shell into mdsmith-release

## Goal

Move the inline shell out of the WASM bridge test step in
the `wasm` job. The step then makes one call to a tested
`mdsmith-release` subcommand.

## Background

[Plan 2609201914][p1914] added the step to
[ci.yml][ci]. Review of PR #873 flagged that it breaks the
rule in [release-tooling.md][rt]: a workflow that needs
runtime logic runs `go run ./cmd/mdsmith-release
<subcommand>` and carries no inline shell. The step does
five things in shell:

1. It diffs `go list` test files under `GOOS=js
   GOARCH=wasm` against a native `go list` to find the
   js/wasm-only test files.
2. It greps those files for `func TestX(... *testing.T)`
   declarations. A regex can't see block comments or
   unusual formatting.
3. It builds a `-run` regex and the expected count.
4. It runs `go test -v` with `-exec` set to `env -i` and
   `go_js_wasm_exec`, because of the ~8 KB limit on
   arguments plus environment in `wasm_exec.js`.
5. It compares the `--- PASS: Test` lines in the log
   with the expected count.

None of this has a unit test today, and the plan copies
the snippet, so the two can drift apart.

## Tasks

1. Add `internal/release/jswasmtests.go` with two pure
   functions. `JSOnlyTestFiles(jsFiles, nativeFiles
   []string) []string` returns the set difference.
   `TestNames(src []byte) ([]string, error)` uses
   `go/parser` and `go/ast` to list the top-level
   `TestXxx(*testing.T)` functions, so commented-out
   tests and unnamed parameters are handled correctly.
   Write table-driven tests first (red), then the
   functions.
2. Add `CountPasses(log []byte) int` and a test that
   checks top-level `--- PASS: Test` lines are counted
   and indented subtest lines are not.
3. Add an orchestrator that takes the package path. It
   runs `go list` twice and `go test -v -exec ...`
   through an injectable command runner, and it returns
   an error that names the missing tests when the PASS
   count does not match. Test it with a fake runner.
4. Wire up `test-js-wasm <pkg>` in
   [main.go][rmain] using pflag with `ContinueOnError`
   and `reportError`, and add a row to the subcommand
   table in [release-tooling.md][rt].
5. Replace the step body in [ci.yml][ci] with
   `go run ./cmd/mdsmith-release test-js-wasm
   ./cmd/mdsmith-wasm`. Update the Task 3 snippet in
   [plan 2609201914][p1914] to point at the subcommand.

## Acceptance Criteria

- [ ] The `wasm` job step is a single
      `mdsmith-release test-js-wasm` call with no inline
      shell logic.
- [ ] A js/wasm-only test that is added, skipped, or
      commented out produces a pass, a named-test
      error, and no error respectively, as unit tests
      show.
- [ ] A failing js/wasm-only test fails the step.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues

[p1914]: 2609201914_arch-fix-missing-unit-tests-0920.md
[ci]: ../.github/workflows/ci.yml
[rt]: ../docs/development/release-tooling.md
[rmain]: ../cmd/mdsmith-release/main.go
