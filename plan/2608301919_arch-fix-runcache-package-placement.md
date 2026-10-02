---
id: 2608301919
title: >-
  Relocate RunCache out of internal/lint
status: "✅"
model: sonnet
summary: >-
  RunCache (then internal/lint/runcache.go) memoizes state
  across every host file in one engine.Run pass — a
  cross-file, whole-run scope that answers a different
  question than internal/lint's stated charter of modeling
  one parsed Markdown file. Flagged by the 2026-08-30 audit
  as tax; moved to the leaf package internal/runcache.
---
# Relocate RunCache out of internal/lint

## Goal

Move `RunCache` to the package whose charter matches its
actual scope. [go.md][go] calls this move "Split a package
by question." Its behavior, and its callers' observable
results, stay unchanged.

## Background

The 2026-08-30 audit (see [the audit log][audit-log]) found
this scope mismatch:

- [internal/lint][lint]'s stated charter is to model a
  parsed Markdown file: source, AST, front matter,
  diagnostics, caches, prose ranges — one file at a time.
- [runcache.go][runcache]'s own doc comment says `RunCache`
  "memoizes per-target-file reads ... across every host file
  processed in one engine.Run pass" and is installed on
  `engine.Runner.RunCache` — a cross-file, whole-run scope,
  not a per-file one.
- The file has grown to 676 lines and ten distinct cache
  slots (`frontMatter`, `rawSchemaFile`, `includes`,
  `anchors`, `wikilinks`, `globMatches`, `parsedSchema`,
  `compiledCUE`, `duplicateParagraphs`, `corpusIndex`, plus
  the `uniqueFieldIndex`/`uniqueFieldScopes` pair) — distinct
  from the per-file `Memo` type internal/lint already owns.
- [go.md][go]'s refactor-moves section: package-by-question
  SRP smells split along the seam where the joined
  responsibilities diverge. `internal/engine` is the package
  whose stated job is "orchestrate rules over files; owns the
  run loop" — the natural home for a whole-run cache.

## Tasks

1. Read [runcache.go][runcache] and its test file in full;
   confirm every exported symbol (`RunCache` and its
   methods) and every private helper type it depends on.
2. Read every non-test caller before moving anything:
   [internal/schema/compile_cache.go][schema-cc],
   [internal/schema/validate.go][schema-validate],
   [internal/engine/runner_cache.go][runner-cache],
   [internal/engine/runner.go][runner],
   [internal/rules/duplicatedcontent/rule.go][dup],
   [internal/rules/requiredstructure/rule.go][reqstruct],
   [internal/rules/requiredstructure/runcache_wiring.go][reqstruct-wiring],
   [internal/linkgraph/wikilinks.go][wikilinks], and
   [pkg/mdsmith/session.go][session].
3. Move `RunCache` and its dependent types from
   `internal/lint` to a new leaf package, `internal/runcache`.
   `internal/engine` would create an import cycle:
   `engine` imports `lint` directly, and `lint.File` holds
   the `*RunCache` field, so `lint` would have to import
   `engine`. `schema` and `linkgraph` also call `RunCache`
   directly, and `engine` reaches `schema` via `config`.
4. Update every import across the files listed in task 2.
5. Keep `internal/lint`'s per-file `Memo` type in place;
   only the cross-file `RunCache` moves.
6. `go build ./...` passes.
7. `go test ./...` passes.
8. `go tool -modfile=tools/go.mod golangci-lint run` reports
   no issues.

## Acceptance Criteria

- [x] `internal/lint`'s package doc no longer lists a
      cross-file, whole-run cache among its responsibilities.
- [x] `RunCache` lives in a package whose charter matches its
      whole-run scope: `internal/runcache`, a leaf package.
      (`internal/engine` was ruled out by an import cycle; see
      task 3.)
- [x] No behavior change: `mdsmith check .` and `mdsmith lsp`
      produce identical diagnostics before and after the
      move.
- [x] `go test ./...` is green.
- [x] `mdsmith check .` is green.

[audit-log]: ../docs/development/architecture-audit.md
[go]: ../docs/development/architecture/go.md
[lint]: ../internal/lint/
[runcache]: ../internal/runcache/runcache.go
[schema-cc]: ../internal/schema/compile_cache.go
[schema-validate]: ../internal/schema/validate.go
[runner-cache]: ../internal/engine/runner_cache.go
[runner]: ../internal/engine/runner.go
[dup]: ../internal/rules/duplicatedcontent/rule.go
[reqstruct]: ../internal/rules/requiredstructure/rule.go
[reqstruct-wiring]: ../internal/rules/requiredstructure/runcache_wiring.go
[wikilinks]: ../internal/linkgraph/wikilinks.go
[session]: ../pkg/mdsmith/session.go
