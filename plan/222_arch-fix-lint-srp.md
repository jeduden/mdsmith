---
id: 222
title: Split internal/lint along question boundaries
status: "🔲"
summary: >-
  internal/lint answers too many questions.
  Move gitignore, limits, PI, yamlsafe,
  parsecache, and runcache into sibling
  packages each named for their question.
model: ""
depends-on: []
---
# Split internal/lint along question boundaries

## Goal

[internal/lint](../internal/lint/) violates
SRP. The package comment answers twelve
distinct questions:

- `File` / `Diagnostic` / `Range` value
  types — the core.
- Code-block AST helpers (`codeblocks.go`).
- Gitignore-pattern matching
  (`gitignore.go`).
- Byte-limit guards (`limits.go`).
- Processing-instruction parsing
  (`pi.go`, `pi_parser.go`).
- Front-matter extraction
  (`frontmatter.go`).
- Parse cache (`parsecache.go`).
- Run cache (`runcache.go`).
- Prose-range projection
  (`proserange.go`).

The audit noted this in May 2026. Three
more files have been added since. No
plan existed.

The
[Go architecture doc](../docs/development/architecture/go.md)
requires each package to answer one
question. A package doc comment that
needs "and" to describe it wants to be
two.

## Tasks

1. Move `gitignore.go` to a new package
   `internal/gitignore`. Update all
   callers. The package doc: "gitignore
   matches a path against .gitignore
   patterns."
2. Move `limits.go` to a new package
   `internal/bytelimit`. Update callers.
   The package doc: "bytelimit guards
   against oversized inputs."
3. Move `pi.go` and `pi_parser.go` to a
   new package `internal/piparser`.
   Update callers. The package doc:
   "piparser extracts processing
   instructions from Markdown."
4. Assess `parsecache.go` and
   `runcache.go`: both depend on
   `lint.File`. If they can be cleanly
   separated without a circular import,
   move to `internal/parsecache` and
   `internal/runcache`. Otherwise keep
   them in `lint` and document the
   decision here.
5. Verify `internal/lint` now answers
   one question: "what is a parsed
   Markdown file and its diagnostics?"
6. Run `go build ./...` and
   `go test ./...`.

## Acceptance Criteria

- [ ] `internal/gitignore` exists with a
  focused package doc.
- [ ] `internal/bytelimit` exists with a
  focused package doc.
- [ ] `internal/piparser` exists with a
  focused package doc.
- [ ] `internal/lint` package doc no
  longer needs "and" to describe it.
- [ ] `go build ./...` clean.
- [ ] `go test ./...` passes.
- [ ] `go tool golangci-lint run` clean.
