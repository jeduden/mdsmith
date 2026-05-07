---
id: 139
title: Field-presence kind assignment
status: "🔲"
model: sonnet
summary: >-
  Auto-assign a kind when a file's front matter
  carries a configured set of fields. Removes the
  per-file `kinds:` list boilerplate for projects
  that identify file types by FM shape.
---
# Field-presence kind assignment

## Goal

Let a project assign a kind from front-matter
shape instead of a glob or an explicit
`kinds:` list. A file that carries `status`,
`priority`, and `assignee` is a `task`,
regardless of where it lives.

## Background

Plan **M-1** in the
[mdbase research](../docs/research/mdbase-vs-mdsmith/learn-from-mdbase.md).
mdbase ships `fields_present:` as a first-class
selector; mdsmith's `kind-assignment:` accepts
globs and explicit tags.

The trigger is a project carrying enough files
that boilerplate (a `kinds: [task]` line on
every task) is paying less than the FM-shape
rule that would replace it.

The selector composes with the existing glob
form. A project keeps using globs for stable
layouts. Field-presence covers the cases where
the layout does not capture intent.

## Non-Goals

- Field-value matching. "Files where status is
  open" is a separate selector tracked as M-2.
  Presence is the simpler half; ship it first.
- Closed assignment. A file matching no rule
  stays untyped, the same as today.
- A precedence overhaul. Multiple matching
  rules combine via the existing OR semantics;
  conflicts surface through `mdsmith kinds`
  (plan 95).

## Design

### Config shape

```yaml
kind-assignment:
  - glob: ["docs/**"]
    kinds: [doc]
  - fields-present: [status, priority, assignee]
    kinds: [task]
  - glob: ["plan/*.md"]
    fields-present: [id]
    kinds: [plan]
```

Each entry is the existing `KindAssignment`
struct with one new optional field. Within an
entry, `glob:` and `fields-present:` combine
with **AND**: both must match. Across entries,
the OR semantics from today are unchanged.

### Presence semantics

A field is "present" when it appears in front
matter with a non-null value. This mirrors
mdbase's "non-null required" semantics and
matches what users mean by "this file has a
status".

A field present but null (`status: null`) does
**not** count. The user wrote it; they did not
fill it.

### Surfacing the match

`mdsmith kinds resolve <file>` names the
matching rule:

```text
file: docs/api.md
effective kinds:
  doc  (from kind-assignment[0]: glob docs/**)

file: plan/132_inline.md
effective kinds:
  plan  (from kind-assignment[2]:
        glob plan/*.md AND fields-present id)
```

A user can see at a glance why a file got the
kind it did. This is the field-presence
analogue of plan 95's per-file provenance.

### Performance

Presence is a constant-time map lookup per
field per file. The matcher walks every entry
in config order and unions the kinds from each
matching entry, the same as
[`resolveEffectiveKinds`](../internal/config/merge.go)
does today. At 50,000 files the overhead is in
the front-matter parse, which mdsmith already
does.

## Tasks

1. Extend the `KindAssignment` struct in
   `internal/config/` with
   `FieldsPresent []string`.
2. Update the kind matcher to AND `glob:` and
   `fields-present:` within an entry. The match
   logic lives in `resolveEffectiveKinds`
   ([`internal/config/merge.go`](../internal/config/merge.go));
   the provenance counterpart lives in
   [`internal/config/provenance.go`](../internal/config/provenance.go).
3. Define presence semantics: a non-null value
   in FM. Document and test the null case.
4. Extend `mdsmith kinds resolve <file>` output
   to show the matching entry index and the
   selector that matched.
5. Update
   [`docs/guides/file-kinds.md`](../docs/guides/file-kinds.md)
   with a worked example combining `glob:` and
   `fields-present:`.
6. Tests:

  - a file with all listed fields matches the
    kind,
  - a file missing one field does not match,
  - a file with the field set to null does
    not match,
  - `glob:` + `fields-present:` AND together,
  - rules without `fields-present:` keep
    working unchanged (regression).

## Acceptance Criteria

- [ ] A `kind-assignment:` entry with
      `fields-present: [a, b]` matches files
      whose FM contains both fields with
      non-null values.
- [ ] An entry combining `glob:` and
      `fields-present:` matches only files
      satisfying both.
- [ ] Existing entries without
      `fields-present:` retain current
      behavior (regression test).
- [ ] A field present but null does not count
      as present (regression test).
- [ ] `mdsmith kinds resolve <file>` output
      names the matching entry and selector.
- [ ] [`docs/guides/file-kinds.md`](../docs/guides/file-kinds.md)
      describes the new selector with one
      worked example.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues.
