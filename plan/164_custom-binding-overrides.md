---
id: 164
title: Custom binding overrides for mdsmith extract
status: "🔲"
model: opus
depends-on: [163]
summary: >-
  Add an opt-in `bind:` key that overrides the default
  schema-derived key in `mdsmith extract`, layered on
  the `keyFor` seam from plan 163.
---
# Custom binding overrides for mdsmith extract

## Goal

[Plan 163](163_schema-driven-data-extraction.md) derives
the extracted data tree from the schema hierarchy with no
annotations. This plan adds an opt-in `bind:` key that
renames or restructures a node when the default key is
wrong, without changing the default behavior.

## Why this is a small change

Plan 163 routes every key through one `keyFor(node)`
function. This plan only changes that function and adds
parsing. The walk, encoders, and CLI are untouched.

- **`keyFor(node)`** returns `node.Bind` when set, else
  the default slug.
- A node with `bind: ""` (explicit empty) is hoisted:
  its children merge into the parent instead of nesting.
- Composition rule: two kinds binding one composed node
  to different names is a compose-time error, reusing the
  collision diagnostic from plan 163.

## Tasks

1. **Parse `bind:`.** Add `Bind string` to `Scope` and
   `ContentEntry`; parse in `parse_inline.go` and
   `parse_file.go`. Unit-test round-trip.
2. **Override `keyFor`.** Return the bind name when set;
   implement hoist for `bind: ""`.
3. **Validate binds.** Reject duplicate sibling binds and
   unreachable binds via the schema diagnostic path.
4. **Compose binds.** Extend `schema.Compose()` so merged
   headings union bound children; conflicting names are a
   compose-time error.
5. **Fixtures and docs.** Add bind-override golden cases;
   document `bind:` under
   [schemas.md](../docs/guides/schemas.md).

## Acceptance Criteria

- [ ] `bind:` overrides the default key; output is
      otherwise identical to plan 163.
- [ ] `bind: ""` hoists a node's children into its
      parent.
- [ ] Duplicate or unreachable binds are rejected with
      actionable diagnostics.
- [ ] Conflicting binds across composed kinds are a
      compose-time error.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes
