---
id: 167
title: Custom binding overrides for mdsmith extract
status: "✅"
model: opus
depends-on: [166]
summary: >-
  Add an opt-in `bind:` key that overrides the default
  schema-derived key in `mdsmith extract`, layered on
  the `keyFor` seam from plan 166.
---
# Custom binding overrides for mdsmith extract

## Goal

[Plan 166](166_schema-driven-data-extraction.md) derives
the extracted data tree from the schema hierarchy with no
annotations. This plan adds an opt-in `bind:` key that
renames or restructures a node when the default key is
wrong, without changing the default behavior.

## Why this is a small change

Plan 166 routes every key through one `keyFor(node)`
function. This plan only changes that function and adds
parsing. The walk, encoders, and CLI are untouched.

- **`keyFor(node)`** returns the bind value when present,
  else the default slug. `Bind` is a `*string` so an
  unset key and an explicit empty one are distinct.
- A node with `bind: ""` (present, empty) is hoisted: its
  children merge into the parent instead of nesting.
- Composition rule: two kinds binding one composed node
  to different names is a compose-time error, reusing the
  collision diagnostic from plan 166.

## Tasks

1. **Parse `bind:`.** Add `Bind *string` to `Scope` and
   `ContentEntry` (nil = unset, non-nil = present, so
   `bind: ""` is distinguishable); parse in
   `parse_inline.go` and `parse_file.go`. Unit-test
   round-trip including unset vs. explicit-empty.
2. **Override `keyFor`.** Return the bind value when
   present; implement hoist for `bind: ""`.
3. **Validate binds.** Reject duplicate sibling binds and
   unreachable binds via the schema diagnostic path.
4. **Compose binds.** Extend `schema.Compose()` so merged
   headings union bound children; conflicting names are a
   compose-time error.
5. **Fixtures and docs.** Add bind-override golden cases;
   document `bind:` under
   [schemas.md](../docs/guides/schemas.md).

## Acceptance Criteria

- [x] `bind:` overrides the default key; output is
      otherwise identical to plan 166.
- [x] `bind: ""` hoists a node's children into its
      parent.
- [x] Duplicate or unreachable binds are rejected with
      actionable diagnostics.
- [x] Conflicting binds across composed kinds are a
      compose-time error.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
- [x] `mdsmith check .` passes
