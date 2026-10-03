---
id: 2610030244
title: Run internal/build under js/wasm once in CI
status: "🔳"
model: sonnet
summary: >-
  CI's wasm job compiles and runs the js/wasm-only
  `internal/build` tests twice: once in the
  `test-js-wasm` stubs step, which fails on a named
  skip, and again inside `test-js-wasm --all`, which
  allows skips. Fold the skip guarantee into one run so
  each CI build pays for one js/wasm compile of the
  package.
---
# Run internal/build under js/wasm once in CI

## Goal

[ci.yml](../.github/workflows/ci.yml) compiles and runs
[internal/build](../internal/build) under js/wasm once,
and a skipped js/wasm-only test still fails CI.

## Background

Plan
[2610021028](2610021028_js-wasm-portable-test-gate.md)
added `mdsmith-release test-js-wasm --all
./internal/build`. The earlier stubs step runs the
tests that only a js/wasm build compiles and fails when
one of them skips. `--all` allows skips, because
native-only tests skip under Node. So the stubs step
cannot be dropped as it stands. Each CI run pays for an
extra js/wasm compile and Node run of the package.

## Tasks

1. Add a unit test in
   [jswasmtests_test.go](../internal/release/jswasmtests_test.go)
   that fails while `--all` accepts a skipped test from
   the js/wasm-only set.
2. Teach `--all` to fail a skip of any test in that
   set. Reuse the default mode's test listing.
3. Drop `internal/build` from the stubs step in
   [ci.yml](../.github/workflows/ci.yml).

## Acceptance Criteria

- [ ] `test-js-wasm --all ./internal/build` fails when
      a js/wasm-only test skips.
- [ ] The wasm CI job runs `internal/build` under Node
      once.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
