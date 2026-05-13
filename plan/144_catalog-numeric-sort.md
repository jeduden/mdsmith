---
id: 144
title: Numeric sort for `<?catalog?>` directive
status: "✅"
model: sonnet
depends-on: []
summary: >-
  Add a `numeric:` sort prefix to the catalog
  directive so PLAN.md and similar catalogs
  ordered by an integer field render in numeric
  order. Today `sort: id` is lexicographic; mixed
  2-digit and 3-digit plan IDs collate the
  2-digit ones after the 100s.
---
# Numeric sort for `<?catalog?>` directive

## Goal

Let a `<?catalog?>` directive sort entries
numerically when the chosen field holds an
integer. PLAN.md becomes scan-friendly: 52, 61,
65, …, 100, 101, …, 132, 133, …, 152.

## Background

[`internal/rules/catalog/rule.go`](../internal/rules/catalog/rule.go)
implements `sortEntries` with `strings.Compare`
on the lower-cased field value (or `path`
fallback). When PLAN.md sorts on `id`, the
result is lexicographic: `100` < `132` < `52`,
because `1` < `5` in string order.

The repo's existing plan IDs are a mix of
2-digit (52, 61, 65, 78, 83, 84, 85, 86, 89, 90,
91, 92, 93, 94, 95, 96, 97, 98) and 3-digit
(100+). The mix predates this plan; renumbering
would invalidate every git commit, PR, and
review thread that names a plan by ID, and is
therefore not on the table.

The fix is in the directive itself: detect when
every entry's value for the sort field parses
as an integer and compare numerically when so.

## Non-Goals

- Renumbering existing plans. Out of scope.
- A general-purpose typed sort (date, semver,
  …). The catalog field types are already
  stringy; this plan adds one numeric mode for
  ID-shaped fields.
- Auto-detecting the type. The user opts in
  explicitly so behavior stays predictable when
  one entry's field is missing or malformed.

## Design

### Surface

A new `sort:` prefix, `numeric:`, that parses
the named field as an int before comparing:

```markdown
<?catalog
glob:
  - "plan/*.md"
  - "!plan/proto.md"
sort: numeric:id
header: ...
row: "..."
?>
```

The `numeric:` prefix says "parse the named
field as an int, fall back to string compare on
parse failure". Descending works the same way:
`-numeric:id`.

The existing `sort: id` continues to work
(lexicographic) for backwards compatibility.
The new mode is opt-in.

### Semantics

For each entry, attempt `strconv.Atoi` on the
trimmed field value. If every entry parses,
sort by the integer values. If any entry fails
to parse, fall back to the existing string
compare (no surprise behavior on a
non-numeric field).

### Tiebreaker

Same as today: secondary by `path` ascending,
case-insensitive. Two plans with the same id
(historical: 121 had two entries) keep their
relative ordering by file path, deterministic.

### PLAN.md migration

After the directive supports the new mode,
update PLAN.md's catalog block to use
`sort: numeric:id` and run `mdsmith fix
PLAN.md`. The diff is whitespace-equivalent
except for the row order.

## Tasks

1. Extend `parseSort` in
   [`internal/rules/catalog/rule.go`](../internal/rules/catalog/rule.go)
   to recognize the `numeric:` prefix and
   return a numeric-mode flag alongside key
   and descending.
2. In `sortEntries`, when numeric mode is set,
   parse each entry's field via `strconv.Atoi`.
   If all parse, sort by the integers; on any
   parse failure fall back to string compare.
3. Update the directive doc at the
   [generating-content guide](../docs/guides/directives/generating-content.md)
   with the new mode and a worked example.
4. Update PLAN.md's catalog block to use
   `sort: numeric:id` and regenerate via
   `mdsmith fix PLAN.md`.
5. Tests:

  - mixed 2-digit and 3-digit IDs sort
     numerically with `sort: numeric:id`,
  - descending form (`-numeric:id`) reverses,
  - a non-numeric field with `numeric:` falls
     back to string compare without an error,
  - the existing `sort: id` regression case
     still produces lexicographic order.

## Acceptance Criteria

- [x] `sort: numeric:id` orders entries by the
      integer value of the `id` field; mixed
      2-digit and 3-digit IDs interleave
      correctly (52 before 100, 100 before
      132).
- [x] `-numeric:id` reverses the order.
- [x] A non-parseable value with `numeric:`
      falls back to string compare; no error.
- [x] PLAN.md's catalog uses
      `sort: numeric:id` and renders 52 before
      100.
- [x] The
      [generating content](../docs/guides/directives/generating-content.md)
      guide documents the mode with one
      example.
- [x] Existing `sort: id` behavior is
      unchanged (regression test).
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no
      issues.
