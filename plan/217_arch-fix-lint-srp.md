---
id: 217
title: Split internal/lint along question lines
status: "🔲"
summary: >-
  internal/lint mixes File/Diagnostic value
  types with gitignore matching, byte-limit
  guards, and PI parsing. The 2026-05-13 audit
  flagged the SRP violation; yamlsafe migrated
  since then but three concerns remain. Split
  them into named sibling packages.
model: ""
depends-on: []
---
# Split internal/lint along question lines

## Goal

[internal/lint](../internal/lint/) answers
"does this file violate a rule?" but it
also owns gitignore matching, a generic
file-size guard, and
processing-instruction parsing. The
[2026-05-13 audit](../docs/development/architecture-audit.md)
scheduled this as tax; yamlsafe migrated
to `internal/yamlutil` since then.
Three concerns remain: move them into
sibling packages named for the question
they answer.

## Background

- `gitignore.go` (278 lines) — gitignore
  pattern matching. Consumed today only
  by `files.go` inside `internal/lint`.
- `limits.go` — `ReadFileLimited` and
  `DefaultMaxInputBytes`. Already
  imported by 6 files outside
  `internal/lint` (`cmd/mdsmith/main.go`,
  `mergedriver.go`, `extract.go`,
  `kinds.go`, `rename.go`,
  `internal/engine`). It answers "read a
  file up to a byte cap" — a general I/O
  concern, not a lint concern.
- `pi.go` + `pi_parser.go` — goldmark
  processing-instruction block parser.
  Consumed by `file.go` and
  `runcache.go` inside `internal/lint`,
  plus by `internal/schema` indirectly
  via `lint.RunCache`.

## Tasks

1. Create `internal/gitignore` package.
   Move `gitignore.go` contents there.
   Write `TestNewPatternSet` and
   `TestPatternSet_Match` unit tests.
   Update the single import site in
   `internal/lint/files.go`.
2. Create `internal/readlimit` package.
   Move `limits.go` contents there.
   Write `TestReadLimited` and
   `TestDefaultMaxInputBytes` unit
   tests. Update all 6 external import
   sites.
3. Create `internal/pi` package.
   Move `pi.go` and `pi_parser.go`
   contents there. Write
   `TestPI_Parse` and
   `TestPIBlockParser_*` unit tests.
   Update import sites in
   `internal/lint/file.go` and
   `internal/lint/runcache.go`.
4. Run `go test ./...` and
   `go tool golangci-lint run`.

## Acceptance Criteria

- [ ] `internal/gitignore` package with
  `TestNewPatternSet` and
  `TestPatternSet_Match` unit tests.
- [ ] `internal/readlimit` package with
  `TestReadLimited` unit test.
  `ReadFileLimited` and
  `DefaultMaxInputBytes` removed from
  `internal/lint`.
- [ ] `internal/pi` package with
  `TestPI_Parse` unit test.
  `pi.go` and `pi_parser.go` removed
  from `internal/lint`.
- [ ] `internal/lint` contains only
  `File`, `Diagnostic`, lint-local
  helpers, and the run cache.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run`
  reports no issues.
