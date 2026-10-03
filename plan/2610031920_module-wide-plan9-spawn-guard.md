---
id: 2610031920
title: Take the plan9 spawn-test guard module-wide
status: "🔲"
model: sonnet
depends-on: [2610030243]
summary: >-
  `TestProcTestFilesCoverPlan9` lives in `internal/build`
  and walks two other packages by relative path, so a
  spawn test file anywhere else is never checked. Move
  the guard to a module-level test, give each package a
  stated plan9 contract, and run the POSIX-free recipe
  tests on plan9 instead of skipping them.
---
# Take the plan9 spawn-test guard module-wide

## Goal

Every package's process-spawning tests type-check
and run correctly on plan9, under one guard that
lives at module level.

## Background

Plan [2610030243](2610030243_plan9-vet-unix-windows-build-tests.md)
added a guard to
[proctags_test.go](../internal/build/proctags_test.go).
It checks `internal/build`. It also reaches
`../release` and `../../cmd/mdsmith-release` by
relative path. So the build package's tests depend on
how two other packages are laid out. And these
spawn-style test files go unchecked:

- `cmd/mdsmith/e2e_main_test.go` and
  `cmd/mdsmith/fileop_exec_test.go`
- `internal/refactor/fileop_exec_test.go`
- `internal/rules/externallink/probe_net_test.go`
- `internal/rules/recipesafety/register_test.go`

The tag rule in that guard is the complement of
`internal/build/exec_other.go`'s stubs. That rule
fits only `internal/build`, so other packages need a
contract of their own.

Two more gaps came out of the round-3 review:

- About 28 untagged tests in `internal/release` and
  `cmd/mdsmith-release` write fake `#!/bin/sh` tools
  and have no plan9 skip. No js/wasm gate runs those
  packages, so nothing stops them from running `sh`
  on plan9.
- The `cp` and `cat` recipe tests in `internal/build`
  skip on plan9 through `skipWithoutPOSIXTools`.
  Recipes run argv directly, with no shell, and plan9
  has `cp` and `cat`. So some of these tests could run
  there, but CI has no plan9 runner to show which.

## Tasks

1. Move the checker and its unit tests out of
   `internal/build` into a module-level test, such
   as one under `internal/integration`, that walks
   every package. Drop the relative paths.
2. Write down the plan9 tag contract for each
   package listed above. Keep the stub-complement
   rule for `internal/build` only.
3. Add a plan9 skip, or a `//go:build !plan9` tag,
   to the untagged release-tooling tests that run
   `sh`. Then extend the sh check to untagged files
   in packages that no js/wasm gate covers.
4. Find out whether a plan9 runner, such as a 9front
   VM in CI, is practical. If it is, split
   `skipWithoutPOSIXTools` so the `cp` and `cat`
   recipe tests run on plan9, and confirm they pass
   there.

## Acceptance Criteria

- [ ] A spawn test file in any package that breaks
      its package's plan9 contract fails the guard.
- [ ] No test in `internal/build` reads another
      package's directory.
- [ ] No untagged test runs `sh` on plan9 without a
      skip.
- [ ] The plan records whether the POSIX-free recipe
      tests run on plan9, and what shows it.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
