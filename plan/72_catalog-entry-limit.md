---
id: 72
title: Catalog entry limit
status: "🔲"
summary: >-
  Add a limit parameter to the catalog directive that
  caps how many entries are rendered.
---
# Catalog entry limit

## Context

The catalog directive renders every file matching its
glob. For large directories this can produce hundreds of
entries, making the generated section unwieldy. Users need
a way to show only the first N entries after sorting.

## Goal

Add an optional `limit` parameter to the catalog directive
so users can cap the number of rendered entries.

## Design

The YAML body accepts `limit: "5"` (quoted string, like
all other non-columns parameters). The value must parse as
a positive integer (`>= 1`). When present,
[`buildCatalogEntries`](../internal/rules/catalog/rule.go)
truncates the sorted entry slice to at most `limit`
entries. If the glob matches fewer files than the limit,
all entries are rendered (no diagnostic).

Interaction with existing parameters:

- Sorting happens before the limit is applied.
- `empty` fallback is only used when zero files match the
  glob, not when `limit` would reduce to zero (limit must
  be `>= 1`).
- Minimal mode (no `row`) respects the limit.

## Tasks

1. Add failing tests in
   [internal/rules/catalog/rule_test.go](../internal/rules/catalog/rule_test.go)
   covering: limit with template mode, limit with minimal
   mode, limit larger than match count (all shown), limit
   combined with sort, invalid limit values (zero,
   negative, non-numeric, empty string), and limit with
   empty result set.
2. Add `validateLimit` in
   [internal/rules/catalog/rule.go](../internal/rules/catalog/rule.go)
   that checks `limit` is a non-empty string parseable as
   an integer `>= 1`, and call it from
   `validateCatalogDirective`.
3. In `buildCatalogEntries`, after sorting, parse `limit`
   and truncate the entries slice.
4. Make tests green, run `go test ./...`.
5. Update the
   [MDS019 catalog README](../internal/rules/MDS019-catalog/README.md):
   add `limit` to the parameters table, add a usage
   example, and add an edge-case row.
6. `go test ./...` passes.
7. `go run ./cmd/mdsmith check .` passes.

## Acceptance Criteria

- [ ] `limit: "3"` renders at most 3 entries
- [ ] Limit applies after sorting
- [ ] Limit works in both template and minimal mode
- [ ] Limit larger than match count renders all entries
- [ ] `limit: "0"`, `limit: "-1"`, and non-numeric limit
      produce a diagnostic
- [ ] Omitting `limit` renders all entries (backward
      compatible)
- [ ] MDS019 README documents the parameter
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
