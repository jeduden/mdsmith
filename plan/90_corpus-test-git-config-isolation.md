---
id: 90
title: Isolate corpus test git config from host signing
status: "🔲"
summary: >-
  Pin commit.gpgsign, tag.gpgsign, and gpg.format
  off for the temporary repositories built by
  corpus clone tests so host-level commit signing
  cannot make git commit fail inside CI and
  sandboxed development environments.
---
# Isolate corpus test git config from host signing

## Goal

Make `go test ./internal/corpus/...` pass on developer
machines and sandboxed CI where the host git config
forces commit signing through a program that cannot
run unattended.

## Context

Two tests in
[clone_test.go](../internal/corpus/clone_test.go)
call `makeBareRepo`, which runs `git commit -m seed`.
The commit reads the host git config. A global
`gpg.program` or `gpg.x509.program` fires a signing
helper that may fail unattended, and the commit
then aborts with exit status 128 before the test
body runs. The Claude Code sandbox hits this path:
`environment-runner code-sign` cannot sign without
prompting.

The other two tests in the file
(`TestResolveSource_InvalidRepository`,
`TestResolveSource_LocalPathOverrideSkipsGit`) do not
create commits and are unaffected.

## Tasks

1. Disable all commit and tag signing on the local
   test repository inside `makeBareRepo` before the
   `git commit` call. The minimum set is
   `commit.gpgsign=false`, `tag.gpgsign=false`, and
   `gpg.format=openpgp` (so a host-level
   `gpg.format=x509` pointing at an x509 signing
   helper is not inherited).
2. Confirm no other test in
   [`internal/corpus`](../internal/corpus) creates
   commits that would hit the same path; today only
   `clone_test.go` does, but a regression here would
   reproduce this bug.
3. Run `go test ./internal/corpus/...` and verify
   both tests pass on a machine whose global git
   config has `commit.gpgsign=true`.

## Acceptance Criteria

- [ ] `go test ./internal/corpus/...` passes on a
      host with `commit.gpgsign=true` globally set.
- [ ] `makeBareRepo` never reads
      `gpg.program` or `gpg.x509.program` from the
      global git config.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
