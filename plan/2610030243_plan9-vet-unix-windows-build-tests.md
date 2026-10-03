---
id: 2610030243
title: Vet the internal/build spawn tests for plan9
status: "✅"
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

1. Pick one approach and record it here. **Chosen:
   Option A.** Option B cannot work with a tag override:
   `go vet -tags unix` under `GOOS=plan9` also pulls in
   the stdlib's unix runtime files and fails to compile
   `runtime`. Option A needs no new CI step because the
   existing `GOOS=plan9 go vet ./...` step now covers
   the files. Option A:
   change the tag to `unix || windows || plan9` and skip each
   `sh`-dependent test at run time on plan9. Option B:
   keep the tag and add a CI step that type-checks the
   files for plan9 with a build-constraint override.
   The new tag is the exact complement of
   `exec_other.go`'s `!unix && !windows && !plan9`, not
   `!js && !wasip1`: a future port then compiles the
   stub tests and not these, which would fail there.
2. Write a failing check first: a plan9-only compile
   error placed in a `_proc_test.go` file must fail
   CI.
3. Apply the approach to every `unix || windows` test
   file in [internal/build](../internal/build): the
   `_proc_test.go` files and `recipe_output_pipe_test.go`.
   Leave `exec_plan9_proc_test.go`, which is tagged
   `plan9`, as it is. Skip each `sh` or recipe test
   through `skipWithoutPOSIXTools`. The `echo`/`touch`
   hook tests and the `recipeOutput` pipe tests need no
   `sh`, so they run on plan9 unskipped. The two
   release-tooling spawn files,
   `internal/release/jswasmtests_proc_test.go` and
   `cmd/mdsmith-release/testjswasm_proc_test.go`, take
   the same tag. They run only the go toolchain and
   `os.Pipe`, which plan9 has, so they need no skip.
   The "Test Fixtures" section of
   [docs/development/index.md](../docs/development/index.md)
   names the new tag and the two plan9 skip helpers.
4. Guard the tag in CI:
   [proctags_test.go](../internal/build/proctags_test.go)
   fails when a test file in `internal/build`,
   `internal/release`, or `cmd/mdsmith-release` builds
   on unix and windows but not on js or plan9. It matches
   tags with `go/build`, so release tags and file-name
   suffixes count. A spawn file that also builds on
   wasip1 or on a future port fails too. It also fails
   when a test, fuzz target, or benchmark in a file
   that builds on plan9 but not on js reaches `sh`
   before a top-level plan9 skip. Reaching `sh` means
   a string whose first word is `sh` or ends in
   `/sh`, such as an `sh` argv, an `sh -c` recipe, or
   `writeScript`'s `#!/bin/sh`. A function, method, or
   func-valued var that builds on plan9 counts the
   same, whether called or passed by name, in any
   declaration order. A skip is a helper that skips
   first, or an `if` or `switch` on `runtime.GOOS`
   that calls `Skip` for `"plan9"`, so the guard
   needs no package-specific helper names. A `cp`
   recipe is not an `sh` use: the guard leaves it to
   `skipWithoutPOSIXTools`, which the test still
   needs for Windows. Untagged files build on js, so
   the js/wasm gate covers them instead.
   [Plan 2610031920](2610031920_module-wide-plan9-spawn-guard.md)
   takes the guard module-wide.

## Acceptance Criteria

- [x] A plan9 compile error in a `_proc_test.go` file
      fails a CI step.
- [x] Retagging a spawn test file `unix || windows`, or
      dropping an `sh` test's plan9 skip, fails
      `TestProcTestFilesCoverPlan9`.
- [x] Native and js/wasm runs of `internal/build` still
      pass.
- [x] `docs/development/index.md` and its three
      included copies name `unix || windows || plan9`
      and the plan9 skip helpers.
- [x] `mdsmith check .` passes.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
