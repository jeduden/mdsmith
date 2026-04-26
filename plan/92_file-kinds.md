---
id: 92
title: File kinds — config schema, assignment, merge
status: "✅"
summary: >-
  Add a kind: a named bundle of rule settings, assigned
  to files via front-matter `kinds:` or
  `kind-assignment:` globs, that reuses the override
  merge semantics.
---
# File kinds — config schema, assignment, merge

## Goal

Introduce a **kind**: a named bundle of rule settings
that can be applied to a set of files. Kinds reuse the
existing override-merge plumbing. A kind body has the
same shape as an override entry (minus `files:`).
Files are bound to kinds via front-matter `kinds:` list
or `kind-assignment:` globs.

## Config schema additions

Two top-level config keys:

- `kinds:` — a map from kind name to a body shaped like
  an override entry minus `files:` (a `rules:` map and
  optional `categories:` map).
- `kind-assignment:` — a list of `{files: [...], kinds:
  [...]}` entries. Each matching entry adds its kinds to
  a file's effective kind list in config order.

Per-file binding via front-matter `kinds:` list, e.g.
`kinds: [plan]`.

## Effective kind list for a file

Built from two sources, concatenated:

1. Front-matter `kinds:` field (list)
2. Matching entries in `kind-assignment:` (config
   order; each entry's kinds in the order listed)

Duplicate names are dropped after their first
occurrence. Referencing an undeclared kind is a config
error.

## Merge semantics

Kind-resolved rule config is merged with the same rules
as existing `overrides:`. If multiple kinds configure
the same rule, the kind appearing later in the
effective list **replaces that rule's entire config
block** (block-replace, same as overrides today —
deep-merge comes in plan 97).

## Tasks

1. [x] Add `kinds:` and `kind-assignment:` config keys
   to the config schema; reuse existing override merge
   to apply a kind's body.
2. [x] Add front-matter `kinds:` list field; resolve
   each file's effective kind list per the algorithm
   above.
3. [x] Wire kind-resolved rule config into the engine
   so each file is linted with its merged settings.
4. [x] Emit a clear config error when kind assignment
   or front matter references an undeclared kind name.
5. [x] Add a grep test asserting the linter's core
   contains no `if kind == "..."` branches and no
   hardcoded kind names.
6. [x] `mdsmith help kinds` prints a short concept page
   covering declaration, assignment, and merge order.

## Acceptance Criteria

- [x] A kind declaration parses with the same syntax
      as an override entry (minus `files:`); override
      merge is reused (verified by test).
- [x] Two project-declared kinds compose correctly
      with each other and with file-glob overrides
      (covered by test).
- [x] A file declaring multiple kinds via `kinds:
      [a, b]` in front matter merges them in list
      order (covered by test).
- [x] Conflicting settings between kinds resolve by
      block replacement — the later kind replaces the
      earlier kind's entire rule config (covered by
      test).
- [x] Files of a kind that sets
      `rules.required-structure.schema:` are validated
      against that schema (covered by test).
- [x] Referencing an undeclared kind name produces a
      clear config error.
- [x] No kind name is referenced by mdsmith's core
      (enforced by grep test).
- [x] `mdsmith help kinds` prints a concept page.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
