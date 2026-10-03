---
id: 2610031104
title: Build-tag the release tooling's process tests
status: "🔲"
model: haiku
summary: >-
  Many tests in `internal/release` and
  `cmd/mdsmith-release` start a real process or open
  an `os.Pipe` from files with no build tag. CLAUDE.md
  says such tests belong in a file tagged
  `//go:build unix || windows`. Move them into
  `_proc_test.go` files so every test file in the two
  packages follows the rule.
---
# Build-tag the release tooling's process tests

## Goal

Every test in [internal/release](../internal/release/)
and [cmd/mdsmith-release](../cmd/mdsmith-release/) that
starts a process (`go`, `git`, `exec.Command`) or opens
an `os.Pipe` lives in a file tagged
`//go:build unix || windows`. Both packages then follow
the CLAUDE.md rule, as `internal/build` does.

## Background

PR #892 tagged the `test-js-wasm` tests. They now
live in two files:

- [jswasmtests_proc_test.go](../internal/release/jswasmtests_proc_test.go)
- [testjswasm_proc_test.go](../cmd/mdsmith-release/testjswasm_proc_test.go)

Other files still start processes with no tag:

- `fs_test.go` in `internal/release`
- `pgo_test.go` in `internal/release`
- `sbom_test.go` in `internal/release`
- `buildwheels_test.go` in `internal/release`
- `main_test.go` in `cmd/mdsmith-release`, through
  `captureStderr`

## Tasks

1. List every untagged test in both packages that
   starts a process or opens a pipe: grep for
   `exec.Command`, `RunCommand`, `os.Pipe` and
   `captureStderr`, then confirm each hit.
2. Move each one, with the helpers only it uses, into
   a `<file>_proc_test.go` beside its source, tagged
   `//go:build unix || windows`.
3. Run `GOOS=js GOARCH=wasm go vet` on both packages
   to confirm the untagged files still compile there.

## Acceptance Criteria

- [ ] No untagged test file in either package starts a
      process or opens an `os.Pipe`
- [ ] `GOOS=js GOARCH=wasm go vet ./internal/release/
      ./cmd/mdsmith-release/` passes
- [ ] The moved tests run and pass on the host
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
