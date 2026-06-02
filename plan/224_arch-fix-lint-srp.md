---
id: 224
title: Split internal/lint along question boundaries
status: "🔲"
summary: >-
  internal/lint answers too many questions.
  Move gitignore, limits, and PI into sibling
  packages each named for their question.
  parsecache and runcache stay in lint due to
  a circular-import constraint.
model: ""
depends-on: []
---
# Split internal/lint along question boundaries

## Goal

[internal/lint](../internal/lint/) violates
SRP. The package has no doc comment, but
its twelve non-test source files mix one
coherent model with three standalone
utilities.

The **core parsed-file model** (stays in
`lint` — these are facets of one subject,
a parsed Markdown file):

- `File` / `Diagnostic` / `Range` value
  types (`file.go`, `diagnostic.go`).
- Code-block AST helpers (`codeblocks.go`).
- Front-matter extraction
  (`frontmatter.go`).
- Parse cache (`parsecache.go`).
- Run cache (`runcache.go`).
- Prose-range projection
  (`proserange.go`).
- Workspace file discovery — Markdown
  detection and glob expansion that pick
  the files to parse (`files.go`). Stays:
  it feeds the parsed-file model and does
  not depend on the three utilities below.

The **standalone utilities** (extracted by
this plan — each answers its own question,
unrelated to the parsed-file model):

- Gitignore-pattern matching
  (`gitignore.go`).
- Byte-limit guards (`limits.go`).
- Processing-instruction parsing
  (`pi.go`, `pi_parser.go`).

The audit noted this in May 2026. Three
more files have been added since. No
plan existed.

The
[Go architecture doc](../docs/development/architecture/go.md)
requires each package to answer one
question. A package doc comment that joins
two *unrelated* responsibilities with "and"
wants to be two packages; listing the
facets of a single subject is fine.

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
   `runcache.go`: `lint.File` embeds
   `*RunCache`
   ([internal/lint/file.go](../internal/lint/file.go):136),
   so moving `runcache.go` to
   `internal/runcache` creates a circular
   import and is not viable. Keep both in
   `lint` and add a comment in
   [internal/lint/file.go](../internal/lint/file.go)
   explaining the coupling. Document the
   decision here.
5. Add `internal/lint/doc.go` with a
   package doc whose subject is one noun
   — the parsed Markdown file — e.g.
   "lint models a parsed Markdown file:
   its source, AST, front matter,
   diagnostics, caches, and prose ranges."
   (The "and" here lists facets of one
   subject, not separate responsibilities.)
6. Verify `internal/lint` now answers
   one question: "what is a parsed
   Markdown file?"
7. Run `go build ./...` and
   `go test ./...`.

## Acceptance Criteria

- [ ] `internal/gitignore` exists with a
  focused package doc.
- [ ] `internal/bytelimit` exists with a
  focused package doc.
- [ ] `internal/piparser` exists with a
  focused package doc.
- [ ] `internal/lint/doc.go` exists with
  a package doc whose subject is a single
  noun — the parsed Markdown file — not a
  conjunction of unrelated responsibilities.
- [ ] `go build ./...` clean.
- [ ] `go test ./...` passes.
- [ ] `go tool golangci-lint run` clean.
