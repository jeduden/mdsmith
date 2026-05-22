---
id: 201
title: Rename internal/testutil to internal/testsymlink
status: "🔲"
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

[internal/testutil](../internal/testutil) violates the SRP
naming rule from the architecture hub.
A package named `util` attracts unrelated
code. The package has one non-test file,
[symlink.go](../internal/testutil/symlink.go), that creates temporary
symlinks for tests. Renaming to
`internal/testsymlink` signals the
narrow scope.

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

- [ ] `internal/testutil/` is gone.
- [ ] `internal/testsymlink/` exists.
- [ ] `grep -r --include='*.go' 'internal/testutil'`
  returns no results.
- [ ] `go build ./...` clean.
- [ ] `go test ./...` passes.
- [ ] `go tool golangci-lint run` clean.
