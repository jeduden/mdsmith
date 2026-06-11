---
id: 2606111050
title: Guard schema.chmodFile with a mutex like fix.chmodFile
status: "🔲"
summary: >-
  internal/schema has an injectable chmodFile var
  with no mutex, unlike internal/fix which added
  chmodFileMu in plan 247. Any concurrent test
  injection races on the schema var.
model: ""
depends-on: [247]
---
# Guard schema.chmodFile with a mutex like fix.chmodFile

## Goal

Add a `chmodFileMu sync.Mutex` to
`internal/schema` to protect the injectable
`chmodFile` var, matching the pattern already
used in `internal/fix`.

## Context

Plan 247 added `chmodFile` to `internal/fix` and
`internal/schema`. The `fix` package also got
`chmodFileMu` and a mutex-guarded test. The
`schema` package did not.

A test that injects `schema.chmodFile` while the
production path reads it races. No such test
exists yet. The pattern invites one without the
mutex.

## Design

Mirror the `internal/fix` pattern exactly:

```go
var chmodFileMu sync.Mutex
```

Production callers lock, copy, and unlock before
calling:

```go
chmodFileMu.Lock()
fn := chmodFile
chmodFileMu.Unlock()
if err := fn(path, mode); err != nil {
    return err
}
```

Test injections hold the mutex around the
assignment and the cleanup restore.

## Tasks

1. Add `var chmodFileMu sync.Mutex` to
   `internal/schema` (in the non-build-tagged
   file that already imports the `chmodFile`
   var).
2. Wrap every production call to `chmodFile` in
   `internal/schema` with the lock/copy/unlock
   pattern.
3. If a coverage test for the schema chmod error
   path does not yet exist, add one — holding
   the mutex around the injection and restore.
4. Verify: run `go test -race ./internal/schema/...`
   with concurrent invocations to confirm no
   data race.

## Acceptance Criteria

- [ ] `internal/schema` has `chmodFileMu
      sync.Mutex` alongside `chmodFile`
- [ ] Every production read of `chmodFile` in
      `internal/schema` uses the mutex
- [ ] A test covering the chmod error path
      in `internal/schema` holds the mutex
      around injection and restore
- [ ] `go test -race ./internal/schema/...`
      reports no data race
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
