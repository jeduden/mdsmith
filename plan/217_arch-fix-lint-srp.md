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
  pattern matching. Consumed by `files.go`
  inside `internal/lint`, plus five
  external callers: `cmd/mdsmith/export.go`,
  `internal/discovery/discovery.go`,
  `internal/engine`, `internal/fix`, and
  `internal/rules/catalog`.
- `limits.go` — `ReadFileLimited`,
  `ReadFSFileLimited`, and
  `DefaultMaxInputBytes`. Imported by
  17+ files across `cmd/mdsmith/` and
  `internal/` (engine, fix, githooks,
  lsp, metrics, rules, schema, …). It
  answers "read a file up to a byte cap"
  — a general I/O concern, not a lint
  concern.
- `pi.go` + `pi_parser.go` — goldmark
  processing-instruction block parser.
  Consumed by `codeblocks.go` inside
  `internal/lint`, and directly by
  `internal/schema` (two files),
  `internal/archetype`, `internal/lsp`,
  `internal/export`, `internal/linkgraph`,
  `internal/index`, and several rule
  packages.

## Tasks

1. Create `internal/gitignore` package.
   Move `gitignore.go` contents there.
   Write `TestNewPatternSet` and
   `TestPatternSet_Match` unit tests.
   Update `internal/lint/files.go` plus
   five external callers:
   `cmd/mdsmith/export.go`,
   `internal/discovery/discovery.go`,
   `internal/engine/runner.go`,
   `internal/fix/fix.go`, and
   `internal/rules/catalog/rule.go`.
2. Create `internal/readlimit` package.
   Move `limits.go` contents there
   (`ReadFileLimited`, `ReadFSFileLimited`,
   `DefaultMaxInputBytes`). Write
   `TestReadLimited` and
   `TestDefaultMaxInputBytes` unit
   tests. Update all callers across
   `cmd/` and `internal/`.
3. Create `internal/pi` package.
   Move `pi.go` and `pi_parser.go`
   contents there. Write
   `TestPI_Parse` and
   `TestPIBlockParser_*` unit tests.
   Update `internal/lint/codeblocks.go`
   and all direct callers:
   `internal/schema`
   (two files), `internal/archetype`,
   `internal/lsp`, `internal/export`,
   `internal/linkgraph`, `internal/index`,
   and all rule packages that import PI
   types (run
   `grep -r 'lint\.ProcessingInstruction\|lint\.PIBlockParser' cmd/ internal/`
   to enumerate them).
4. Add SRP bullet entries to
   `docs/development/architecture/go.md`
   for each new package, following the
   pattern established by `internal/punkt`
   in [plan/218](218_arch-fix-punkt-layering.md).
5. Run `go test ./...` and
   `go tool golangci-lint run`.

## Acceptance Criteria

- [ ] `internal/gitignore` package with
  `TestNewPatternSet` and
  `TestPatternSet_Match` unit tests.
- [ ] `internal/readlimit` package with
  `TestReadLimited` and
  `TestDefaultMaxInputBytes` unit tests.
  `ReadFileLimited`, `ReadFSFileLimited`,
  and `DefaultMaxInputBytes` removed
  from `internal/lint`.
- [ ] `internal/pi` package with
  `TestPI_Parse` and
  `TestPIBlockParser_*` unit tests.
  `pi.go` and `pi_parser.go` removed
  from `internal/lint`.
- [ ] `internal/lint` contains only
  `File`, `Diagnostic`, lint-local
  helpers, and the run cache (RunCache
  may import `internal/pi`; that is
  expected).
- [ ] No stale callers of moved symbols remain
  outside `internal/lint/`: grep `cmd/` and
  `internal/` for `lint.GitignoreMatcher`,
  `lint.NewGitignoreMatcher`,
  `lint.ReadFileLimited`,
  `lint.ReadFSFileLimited`,
  `lint.DefaultMaxInputBytes`,
  `lint.ProcessingInstruction`,
  `lint.PIBlockParser` — zero results.
- [ ] `docs/development/architecture/go.md`
  lists `internal/gitignore`,
  `internal/readlimit`, and
  `internal/pi` in the SRP section.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run`
  reports no issues.
