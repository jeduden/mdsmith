---
id: 223
title: Split internal/lint along question lines
status: "✅"
summary: >-
  internal/lint mixed File/Diagnostic value
  types with gitignore matching, byte-limit
  guards, and PI parsing. The 2026-05-13 audit
  flagged the SRP violation; yamlsafe migrated
  since then and the three remaining concerns
  are now split into named sibling packages.
model: ""
depends-on: []
---
# Split internal/lint along question lines

## Goal

[internal/lint](../internal/lint/) answers
"does this file violate a rule?" but it
also owned gitignore matching, a generic
file-size guard, and
processing-instruction parsing. The
[2026-05-13 audit](../docs/development/architecture-audit.md)
scheduled this as tax; yamlsafe migrated
to `internal/yamlutil` since then.
The three remaining concerns now live in
sibling packages named for the question
they answer.

## Background

- `gitignore.go` — gitignore pattern
  matching. Moved to `internal/gitignore`.
  The matcher is `gitignore.Matcher`
  (constructor `gitignore.NewMatcher`);
  the type was renamed from
  `GitignoreMatcher` to avoid a
  package-name stutter under revive.
- `limits.go` — `ReadFileLimited`,
  `ReadFSFileLimited`, and
  `DefaultMaxInputBytes`. Answers "read
  a file up to a byte cap" — a general
  I/O concern. Moved to
  `internal/readlimit` (names unchanged).
- `pi.go` + `pi_parser.go` — goldmark
  processing-instruction block node and
  parser, re-exported from `pkg/markdown`.
  Moved to `internal/pi`. The forwarder
  was renamed `PIBlockParserPrioritized`
  → `BlockParserPrioritized` to avoid the
  same stutter; callers that used a local
  `pi` variable were renamed to `piNode`
  so the new package name does not shadow.

## Tasks

1. Create `internal/gitignore`. Move
   `gitignore.go` contents there as
   `gitignore.Matcher` / `NewMatcher`.
   Add `TestNewMatcher` and
   `TestMatcher_IsIgnored` unit tests
   (plus the migrated pattern-matching
   coverage). Update all callers.
2. Create `internal/readlimit`. Move
   `limits.go` contents there. Add
   `TestReadFileLimited` and
   `TestDefaultMaxInputBytes` unit tests.
   Update all callers.
3. Create `internal/pi`. Move `pi.go`
   and `pi_parser.go` contents there
   (`BlockParserPrioritized` forwarder).
   Add `TestBlockParserPrioritized` and
   `TestPIBlockParser_*` unit tests.
   Update all callers.
4. Add SRP bullet entries to
   `docs/development/architecture/go.md`
   for `internal/gitignore`,
   `internal/readlimit`, and
   `internal/pi`.
5. Run `go test ./...` and
   `go tool golangci-lint run`.

## Acceptance Criteria

- [x] `internal/gitignore` package exists
  with `TestNewMatcher` and
  `TestMatcher_IsIgnored`.
- [x] `internal/readlimit` package exists
  with `TestReadFileLimited` and
  `TestDefaultMaxInputBytes`.
  `ReadFileLimited`, `ReadFSFileLimited`,
  and `DefaultMaxInputBytes` removed
  from `internal/lint`.
- [x] `internal/pi` package exists with
  `TestBlockParserPrioritized` and
  `TestPIBlockParser_*` tests.
  `pi.go` and `pi_parser.go` removed
  from `internal/lint`.
- [x] `internal/lint` contains only
  `File`, `Diagnostic`, lint-local
  helpers, and the run cache. The four
  moved files no longer exist under
  `internal/lint/`.
- [x] `docs/development/architecture/go.md`
  lists all three new packages in the
  SRP section.
- [x] `go test ./...` passes.
- [x] `go tool golangci-lint run` clean.
