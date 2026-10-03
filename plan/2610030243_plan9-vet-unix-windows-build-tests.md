---
id: 2610030243
title: Vet the internal/build spawn tests for plan9
status: "🔲"
model: sonnet
summary: >-
  Plan 2610021028 moved about 35 process-spawning
  `internal/build` tests into `unix || windows` files.
  CI's `GOOS=plan9 go vet ./...` step no longer
  type-checks them, so a plan9-only compile break in
  those files reaches main. Restore plan9 type-checking
  without making plan9 run `sh` recipes.
---
# Vet the internal/build spawn tests for plan9

## Goal

CI type-checks every spawn test file for plan9. A
name clash with a plan9-only test file then fails CI.

## Background

Before plan
[2610021028](2610021028_js-wasm-portable-test-gate.md),
the spawn tests sat in untagged files, which the plan9
vet step compiled. They now carry `//go:build unix ||
windows`, so plan9 drops them. The tag is right at run
time: the tests run `sh` recipes and plan9 has only
`rc`. The tests that plan9 runs live in
[exec_plan9_test.go](../internal/build/exec_plan9_test.go).

The gap is compile-time only. No plan9 build compiles
the spawn test files now. A later change that widens
the tag, or ports a test to `rc`, finds plan9 compile
errors only then.

## Tasks

1. Pick one approach and record it here. Option A:
   change the tag to `!js && !wasip1` and skip each
   `sh`-dependent test at run time on plan9. Option B:
   keep the tag and add a CI step that type-checks the
   files for plan9 with a build-constraint override.
2. Write a failing check first: a plan9-only compile
   error placed in a `_proc_test.go` file must fail
   CI.
3. Apply the approach to every `unix || windows` test
   file in [internal/build](../internal/build): the
   `_proc_test.go` files and `recipe_output_pipe_test.go`.
   Leave `exec_plan9_proc_test.go`, which is tagged
   `plan9`, as it is.

## Acceptance Criteria

- [ ] A plan9 compile error in a `_proc_test.go` file
      fails a CI step.
- [ ] Native and js/wasm runs of `internal/build` still
      pass.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
