---
id: 2609201914
title: >-
  Add dedicated unit tests for the 2026-09-20 touched-set
  tax findings
status: "🔲"
model: haiku
summary: >-
  AdvancePastLine in internal/rules/astutil, two path-safety
  guards in cmd/mdsmith/backlinks.go, and four WASM bridge
  helpers landed with only behavior-level coverage, not a
  dedicated unit test by name. Flagged by the 2026-09-20
  audit as tax.
---
# Add dedicated unit tests for the 2026-09-20 touched-set tax findings

## Goal

Give each function below its own unit test by name, matching
the `TestFoo` convention [tests.md][tests] requires.

## Background

The 2026-09-20 audit (see [the audit log][audit-log]) swept
every file touched since the prior checkpoint. It found
four groups of functions with no test carrying their own
name. Each is exercised only indirectly through a caller's
scenario test:

- [internal/rules/astutil/astutil.go][astutil]:409 —
  `AdvancePastLine`, the cursor-threading helper
  `SectionBodies` and both `occurrence` and `overrepetition`
  call directly. Its contract (safe only for non-decreasing
  `start` over ascending, non-overlapping windows) is
  documented but only exercised as a side effect of
  `TestSectionBodies_*`.
- [cmd/mdsmith/backlinks.go][backlinks]:224,241 —
  `isAbsOrDriveOrUNC` and `isWorkspaceRelativeTarget`, the
  path-traversal guards `rename`, `deps`, and `move` all
  depend on. Covered only incidentally via
  `TestWorkspaceRelativePath_*` and e2e paths.
- [cmd/mdsmith-wasm/main.go][wasm]:44,90,226,235 —
  `resolveVersion`, `workspaceFromJS`, `uriAndSource`, and
  `allStrings`. These carry the `js`/`wasm` build constraint,
  so they cannot run under a native `go test`; they are
  exercised only through the `smoke_test.go`/`size_test.go`
  e2e harness that builds and runs the real WASM artifact.

None sit directly on a `rule.Rule` interface, LSP capability
handler, or CLI subcommand entry. Their callers already
carry that surface, so all four are `tax`, not `blocker`.

## Tasks

1. Add `TestAdvancePastLine` to
   `internal/rules/astutil/astutil_test.go`, driving the
   cursor-threading contract directly: an already-passed
   prefix, a boundary exactly at `start`, and an empty slice.
2. Add `TestIsAbsOrDriveOrUNC` and
   `TestIsWorkspaceRelativeTarget` to
   `cmd/mdsmith/backlinks_unit_test.go`, table-driven over
   POSIX-absolute, drive-letter, UNC, and relative-target
   inputs.
3. For the WASM bridge helpers, add native `go test`-able
   unit tests if the logic can be extracted behind a
   build-tag-free seam; if the `js`/`wasm` constraint makes
   that impractical, add a one-line "no test by design"
   exemption comment on each function per
   [tests.md § Exemptions][tests-exemptions], explaining that
   `smoke_test.go`/`size_test.go` are this function's only
   possible test boundary.
4. Do not delete or duplicate the existing behavior-level
   tests named above. They cover the public contract and
   stay as-is.
5. `go build ./...` passes.
6. `go test ./internal/rules/astutil/... ./cmd/mdsmith/...`
   passes.

## Acceptance Criteria

- [ ] `AdvancePastLine` has a dedicated `TestAdvancePastLine`.
- [ ] `isAbsOrDriveOrUNC` and `isWorkspaceRelativeTarget` each
      have a dedicated test by name.
- [ ] Each WASM bridge helper either has a dedicated test or
      a "no test by design" exemption comment explaining why.
- [ ] `go test ./...` is green.
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [ ] `mdsmith check .` is green.

[audit-log]: ../docs/development/architecture-audit.md
[tests]: ../docs/development/architecture/tests.md
[tests-exemptions]: ../docs/development/architecture/tests.md#exemptions
[astutil]: ../internal/rules/astutil/astutil.go
[backlinks]: ../cmd/mdsmith/backlinks.go
[wasm]: ../cmd/mdsmith-wasm/main.go
