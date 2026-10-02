---
id: 2610021028
title: Gate untagged internal/build tests under js/wasm in CI
status: "🔲"
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

1. List every `internal/build` test that fails under
   `GOOS=js GOARCH=wasm` and move each one into a
   `unix || windows` file. Keep the pure ones untagged.
2. Add a CI step, through `mdsmith-release` per
   [release-tooling.md](../docs/development/release-tooling.md),
   that runs the whole `internal/build` package under
   Node and fails on any failure.
3. Note in the test-fixtures docs that a test which
   spawns a process or opens a pipe needs the tag.

## Acceptance Criteria

- [ ] `GOOS=js GOARCH=wasm go test ./internal/build`
      passes under Node.
- [ ] CI runs that command and gates on it.
- [ ] Native `go test ./internal/build` still runs every
      moved test.
