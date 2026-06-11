---
id: 2606111048
title: Consolidate tinygo compat stubs into internal/oscompat
status: "🔲"
summary: >-
  Sixteen build-tagged compat files wrapping
  os.Chmod, os.SameFile, and
  filepath.EvalSymlinks are scattered across six
  packages. Consolidate them into a single
  internal/oscompat package.
model: ""
depends-on: [247]
---
# Consolidate tinygo compat stubs into internal/oscompat

## Goal

Move every `_tinygo.go` / `_notinygo.go`
build-tagged pair into `internal/oscompat`. Each
OS-compat decision lives in one place, and future
tinygo target gaps are patched there.

## Context

Plan 247 added build-tagged seams for `os.Chmod`,
`os.SameFile`, and `filepath.EvalSymlinks`. Seams
were added package-by-package to keep that PR
small. The result is sixteen files across six
packages:

- `internal/fix/chmod_tinygo.go` +
  `chmod_notinygo.go`
- `internal/schema/chmod_tinygo.go` +
  `chmod_notinygo.go`
- `internal/schema/evalsymlinks_tinygo.go` +
  `evalsymlinks_notinygo.go`
- `internal/githooks/compat_tinygo.go` +
  `compat_notinygo.go`
- `internal/rules/requiredstructure/compat_tinygo.go` +
  `compat_notinygo.go`
- `internal/rules/crossfilereferenceintegrity/evalsymlinks_tinygo.go` +
  `evalsymlinks_notinygo.go`
- `internal/lsp/evalsymlinks_tinygo.go` +
  `evalsymlinks_notinygo.go`
- `internal/rules/build/evalsymlinks_tinygo.go` +
  `evalsymlinks_notinygo.go`

There are also inconsistencies. The `githooks`
package calls its chmod var `chmodFn`. The `fix`
and `schema` packages call theirs `chmodFile`.
The two `sameFile` stubs return opposite values.
That difference is correct for each caller, but
there is no comment cross-referencing the two.

## Design

Create `internal/oscompat` with one file pair per
wrapped call:

- `chmod.go` / `chmod_tinygo.go` — exports
  `Chmod(path string, mode os.FileMode) error`
- `samefile.go` / `samefile_tinygo.go` — exports
  `SameFile(fi1, fi2 os.FileInfo) bool`
- `evalsymlinks.go` / `evalsymlinks_tinygo.go` —
  exports
  `EvalSymlinks(path string) (string, error)`

Tinygo stubs follow the same contracts as today.
Calling packages import `internal/oscompat` and
delete their own pairs.

The `requiredstructure` caller logic stays
package-local. It returns `false` from
`oscompat.SameFile` on tinygo, then falls back to
path equality — that is correct behavior.
The TOCTOU shortcut in `githooks` is also
expressed by the caller, not the stub.

## Tasks

1. Create `internal/oscompat/` with the three
   exported wrappers and their tinygo stubs.
2. Update `internal/fix` to import
   `oscompat.Chmod`. Delete `fix/chmod_tinygo.go`
   and `fix/chmod_notinygo.go`.
3. Update `internal/schema` the same way. Delete
   both chmod and evalsymlinks file pairs. Add
   `chmodFileMu sync.Mutex` if plan 2606111050
   has not yet landed.
4. Update `internal/githooks` to use
   `oscompat.Chmod`. Rename `chmodFn` to
   `chmodFile` to match the other packages. For
   `sameFile`, keep the caller-level tinygo seam
   rather than delegating to `oscompat.SameFile`.
   Delete the existing pair.
5. Update `internal/rules/requiredstructure` to
   use `oscompat.SameFile`. Delete its pair.
6. Update `internal/rules/crossfilereferenceintegrity`
   to use `oscompat.EvalSymlinks`. Delete its
   pair.
7. Update `internal/lsp` and
   `internal/rules/build` the same way.
8. Verify the wasm build compiles and all tests
   pass.

## Acceptance Criteria

- [ ] `internal/oscompat` contains the three
      exported wrappers; no other package has
      its own `_tinygo.go` / `_notinygo.go`
      pair for these three calls
- [ ] `go build ./...` succeeds with the
      standard toolchain
- [ ] `tinygo build -target wasm -o /dev/null
      ./cmd/mdsmith-wasm/` compiles without
      errors
- [ ] No `chmodFn` / `chmodFile` naming split;
      all packages use `chmodFile`
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
