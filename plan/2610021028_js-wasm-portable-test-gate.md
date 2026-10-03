---
id: 2610021028
title: Gate untagged internal/build tests under js/wasm in CI
status: "✅"
model: sonnet
summary: >-
  CI's `test-js-wasm` step runs only tests that compile
  solely under js/wasm, so an untagged test that needs a
  real pipe or subprocess passes native CI but fails under
  `GOOS=js`. Tag or list the tests that can run on js/wasm
  and gate them in CI, so a mistagged test fails CI.
---
# Gate untagged internal/build tests under js/wasm in CI

## Goal

A test in [internal/build](../internal/build) that
cannot run on js/wasm fails CI unless it carries a
`unix || windows` tag.

## Background

The pre-merge review of PR #877 (plan
[2610020725](2610020725_build-exec-js-wasm-stub.md))
found `TestRecipeOutput_AttachSecondPipeFailsClosesFirst`
in an untagged file. Its first `pipeFn` call is the real
`os.Pipe`, so `GOOS=js GOARCH=wasm go test
./internal/build` failed. CI missed it because
`mdsmith-release test-js-wasm` runs only the tests that a
js/wasm build alone compiles, and the wasip1 and plan9
steps only vet. Today a full js/wasm run of the package
still fails about 25 tests that start real processes
(`TestBuild_*`, `TestRunHooks_*`, most of
`TestRunRecipe_*`). Those tests sit in untagged files.

## Tasks

1. [x] List every `internal/build` test that fails under
   `GOOS=js GOARCH=wasm` and move each one into a
   `unix || windows` file. Keep the pure ones untagged.
2. [x] Add a CI step, through `mdsmith-release` per
   [release-tooling.md](../docs/development/release-tooling.md),
   that runs the whole `internal/build` package under
   Node and fails on any failure.
3. [x] Note in the test-fixtures docs that a test which
   spawns a process or opens a pipe needs the tag.
4. [x] Also tag spawn tests that expect a failure, such
   as `TestBuild_Timeout` and the hook exit-code tests.
   They pass under Node only because the process never
   starts, so the gate cannot catch them.
5. [x] Make `test-js-wasm --all` fail when no test
   passed, so an empty or fully skipped package does
   not pass the gate.

Follow-ups from review: plan
[2610030243](2610030243_plan9-vet-unix-windows-build-tests.md)
(plan9 vet coverage) and plan
[2610030244](2610030244_dedupe-build-js-wasm-ci-run.md)
(one js/wasm run of `internal/build` in CI).

## Acceptance Criteria

- [x] `GOOS=js GOARCH=wasm go test ./internal/build`
      passes under Node.
- [x] CI runs that command and gates on it.
- [x] Native `go test ./internal/build` still runs every
      moved test.
