---
id: 201
title: Rename internal/testutil to internal/testsymlink
status: "✅"
summary: >-
  Fix the anti-pattern package name. internal/testutil
  answers "a grab bag"; rename it to
  internal/testsymlink, which names the question
  its single symlink.go file answers.
model: ""
depends-on: []
---
# Rename internal/testutil to internal/testsymlink

## Goal

`internal/testutil` violated the SRP
naming rule from the architecture hub.
A package named `util` attracts unrelated
code. The package had one non-test file,
`symlink.go`, that creates temporary
symlinks for tests. It now lives at
[internal/testsymlink](../internal/testsymlink/symlink.go),
which signals the narrow scope.

## Tasks

1. Create `internal/testsymlink/`.
2. Move `symlink.go` and update its
   `package` declaration.
3. Update all imports referencing
   `internal/testutil`.
4. Delete `internal/testutil/`.
5. Run `go build ./...` and
   `go test ./...`.

## Acceptance Criteria

- [x] `internal/testutil/` is gone.
- [x] `internal/testsymlink/` exists.
- [x] `grep -r --include='*.go' 'internal/testutil'`
  returns no results.
- [x] `go build ./...` clean.
- [x] `go test ./...` passes.
- [x] `go tool golangci-lint run` clean.
