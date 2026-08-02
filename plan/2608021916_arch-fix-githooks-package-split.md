---
id: 2608021916
title: >-
  Split internal/githooks by responsibility
status: "🔲"
model: sonnet
summary: >-
  internal/githooks's package doc names three joined
  responsibilities — hook-script generation,
  .gitattributes glob/managed-block I/O, and
  directive-file discovery — a package-by-question SRP
  smell per go.md. Flagged by the 2026-08-02 audit.
---
# Split internal/githooks by responsibility

## Goal

Split `internal/githooks` so each resulting package
answers one question, per go.md's "Split a package by
question" refactor move.

## Background

The 2026-08-02 audit (see
[the audit log](../docs/development/architecture-audit.md))
found this SRP smell:

- [githooks.go](../internal/githooks/githooks.go)'s
  package doc comment names three joined
  responsibilities: "managing the pre-merge-commit
  hook, merge-driver assignments in `.gitattributes`,
  and discovery of files that contain generated-section
  directives."
- [go.md](../docs/development/architecture/go.md)'s
  refactor-moves section: "Split a package by question.
  If the package doc comment requires 'and' to describe
  ... the package wants to be two."
- The file is 1,346 lines and cleanly separates into:
  - hook-script generation/validation
    (`BuildHookScript`, `HookMatchesCanonical`,
    the staging shell-function builder);
  - `.gitattributes` glob/managed-block read-write
    (`GlobsFromConfig`, `WriteGitattributes`,
    `ExtractGlobs`, `StageGitattributes`);
  - directive-file discovery (`DiscoverFiles`,
    the directive-marker scanner).
- The project has precedent for this exact move: `internal/gitignore`,
  `internal/bytelimit`, and `internal/piparser` were all
  split out of `internal/lint` once their question
  diverged from "model a parsed Markdown file" (see
  go.md's package list).

## Tasks

1. Read [githooks.go](../internal/githooks/githooks.go)
   and its test file in full; group every exported and
   unexported symbol by the three questions above.
2. Create `internal/gitattributes` for the
   `.gitattributes` glob/managed-block read-write group;
   move `GlobsFromConfig`, `WriteGitattributes`,
   `ExtractGlobs`, `StageGitattributes`, and their
   dedicated tests into it.
3. Decide which package keeps `DiscoverFiles` — the
   merge-driver install path is the deciding consumer;
   read its call sites in `cmd/mdsmith/mergedriver.go`
   before choosing.
4. Update every import of the moved symbols across
   `cmd/mdsmith` and `internal/...`.
5. Keep `internal/githooks` scoped to hook-script
   generation/validation only.
6. `go build ./...` passes.
7. `go test ./...` passes.
8. `go tool -modfile=tools/go.mod golangci-lint run`
   reports no issues.

## Acceptance Criteria

- [ ] `internal/githooks`'s package doc no longer needs
      "and" to describe its responsibility.
- [ ] `.gitattributes` glob/managed-block logic lives in
      its own package with its own tests.
- [ ] No behavior change: `mdsmith merge-driver install`
      and `mdsmith pre-merge-commit install` produce
      identical output before and after the split.
- [ ] `go test ./...` is green.
- [ ] `mdsmith check .` is green.
