---
id: 2606211907
title: 'arch-fix: split internal/engine/runner.go'
status: '✅'
summary: >-
  Split runner.go (1 290 lines) into sibling
  files along its logical groups to restore the
  1 000-line guideline and satisfy SRP for the
  engine package.
model: ''
depends-on: []
---
# arch-fix: split internal/engine/runner.go

## Context

Closes audit entry 2026-06-21.
[`internal/engine/runner.go`](../internal/engine/runner.go)
reached 1 290 lines.

Go arch doc §"Common violations to flag"
names engine as a place that grows unbounded
when it becomes a dumping ground.
The audit checklist calls out the 1 000-line
threshold explicitly.

The file mixes seven concerns:

- File dispatch (`Run`, `runFiles`,
  `lintFile`, `filterIgnored`, …)
- Layer-0 parse-skip gate
  (`layer0SkipEnabled`,
  `layer0SkipEligible`,
  `allEnabledRulesSkipSafe`,
  `computeFlatLayer0Active`, …)
- Config-resolution cache
  (`effectiveCache`, `runResolve`,
  `configuredRules`, `effectiveCached`,
  `runCacheForCall`, …)
- Source-mode lint path (`RunSource`,
  `runSource`, `parseForSource`,
  `populateFileFields`, …)
- Front-matter parsing
  (`parseFrontMatterKinds`,
  `parseFrontMatter`, …)
- Config-target rules
  (`runConfigTargetRules`,
  `runConfigRule`, `anyRepoScopedEnabled`)
- Logging and sorting (`log`, `logRules`,
  `ruleCategoryLookup`, `sortDiagnostics`)

## Goal

Split `runner.go` into sibling files within
the same package. Each sibling answers one
question. The primary file stays under
1 000 lines.

## Tasks

1. Create `internal/engine/runner_layer0.go`:
   move Layer-0 gate functions and
   `computeFlatLayer0Active`.
2. Create `internal/engine/runner_cache.go`:
   move cache types and their methods.
3. Create `internal/engine/runner_log.go`:
   move logging helpers and
   `sortDiagnostics`.
4. Remove the moved blocks from `runner.go`
   and trim now-unused imports.
5. Run `go build ./...` — confirm no errors.
6. Run `go test ./...` — confirm no
   regressions.
7. Confirm `wc -l runner.go` is under
   1 000 lines.

## Acceptance Criteria

- [x] `internal/engine/runner.go` is under
  1 000 lines.
- [x] `go build ./...` passes.
- [x] `go test ./...` passes.
- [x] `go tool golangci-lint run` reports
  no new issues.
- [x] No logic changed — pure file
  reorganisation within the `engine`
  package.
