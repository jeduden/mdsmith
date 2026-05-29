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

- `gitignore.go` — gitignore pattern
  matching. Move to `internal/gitignore`.
- `limits.go` — `ReadFileLimited`,
  `ReadFSFileLimited`, and
  `DefaultMaxInputBytes`. Answers "read
  a file up to a byte cap" — a general
  I/O concern. Move to
  `internal/readlimit`.
- `pi.go` + `pi_parser.go` — goldmark
  processing-instruction block parser.
  Move to `internal/pi`.

## Tasks

1. Create `internal/gitignore`. Move
   `gitignore.go` contents there. Write
   `TestNewGitignoreMatcher` and
   `TestGitignoreMatcher_IsIgnored` unit
   tests. Update all callers.
2. Create `internal/readlimit`. Move
   `limits.go` contents there. Write
   `TestReadFileLimited` and
   `TestDefaultMaxInputBytes` unit tests.
   Update all callers.
3. Create `internal/pi`. Move `pi.go`
   and `pi_parser.go` contents there.
   Write `TestPI_Parse` and
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

- [ ] `internal/gitignore` package exists
  with `TestNewGitignoreMatcher` and
  `TestGitignoreMatcher_IsIgnored`.
- [ ] `internal/readlimit` package exists
  with `TestReadFileLimited` and
  `TestDefaultMaxInputBytes`.
  `ReadFileLimited`, `ReadFSFileLimited`,
  and `DefaultMaxInputBytes` removed
  from `internal/lint`.
- [ ] `internal/pi` package exists with
  `TestPI_Parse` and
  `TestPIBlockParser_*` tests.
  `pi.go` and `pi_parser.go` removed
  from `internal/lint`.
- [ ] `internal/lint` contains only
  `File`, `Diagnostic`, lint-local
  helpers, and the run cache.
- [ ] Zero stale callers of moved symbols
  outside `internal/lint/`. Verify with:

  ```sh
  grep -r \
    -e 'lint\.GitignoreMatcher' \
    -e 'lint\.NewGitignoreMatcher' \
    -e 'lint\.ReadFileLimited' \
    -e 'lint\.ReadFSFileLimited' \
    -e 'lint\.DefaultMaxInputBytes' \
    -e 'lint\.ProcessingInstruction' \
    -e 'lint\.PIBlockParser' \
    cmd/ internal/ | grep -v internal/lint/
  ```

- [ ] `docs/development/architecture/go.md`
  lists all three new packages in the
  SRP section.
- [ ] `go test ./...` passes.
- [ ] `go tool golangci-lint run` clean.
