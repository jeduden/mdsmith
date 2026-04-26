---
id: 92
title: File kinds — config schema, assignment, merge
status: "🔲"
summary: >-
  Add user-declared file `kinds` that share the existing
  rule-config syntax. A file's effective kind list is
  built from front-matter `kinds:` plus `kind-assignment:`
  globs; merging follows override semantics with
  list-driven order.
---
# File kinds — config schema, assignment, merge

## Goal

Introduce a **kind**: a named bundle of rule settings
that can be applied to a set of files. Kinds reuse the
existing override-merge plumbing — no new bespoke
schema. The term aligns with Hugo's `type` (front-matter
content category), not Hugo's `archetype`.

This plan delivers only the kind machinery. Placeholder
grammar (plan 93), lint-once for embeds (plan 94),
troubleshooting CLI (plan 95), and adoption in this repo
(plan 96) build on it.

## Background

Today files are linted under a single global rule set
plus glob `overrides:`. Files that share a purpose
(schema, template, fragment, prompt, …) tend to need
the same per-rule tuning. Without a named grouping,
that tuning has to be repeated across `overrides:` or
managed by ad-hoc `ignore:` entries.

A *kind* names the grouping. Files of the same kind
share the kind's rule body. Kind names belong to the
project — mdsmith ships **no built-in kinds**.

## Design

### Kind declaration

A kind is a named block under `kinds:`. Its body has
the **same shape as an override entry** (`rules:`,
`front-matter:`, etc.) — minus `files:`, since files
are bound to kinds separately. The kind name (`plan`
below) is whatever the project picks. A kind that sets
`rules.required-structure.schema:` in its body attaches
that CUE schema to every file of that kind:

```yaml
kinds:
  plan:                          # project-chosen name
    rules:
      required-structure:
        schema: plan/proto.md    # CUE schema for this
                                 # kind
      paragraph-readability: false
  proto:
    rules:
      paragraph-readability: false
      front-matter: false
```

Kinds merge with the same rules as overrides. If
multiple kinds configure the same rule, the kind
appearing later in the file's resolved effective
kind list **replaces that rule's entire config block**
— nested settings do not deep-merge across kinds, the
same way `overrides:` already work today.

### Kind assignment

A file's effective kind list is built from two sources,
concatenated in this order:

1. front-matter `kinds:` field (a YAML list; a
   single-kind file still uses a one-item list);
2. matching entries in `kind-assignment:` (config
   order; each entry's kinds in the order listed).

Duplicate names are dropped after their first
occurrence. Referencing an undeclared kind is a config
error.

### Composability

1. **Kind merge follows override merge.** Kinds apply
   in *effective-list* order: front-matter first, then
   `kind-assignment:` matches in config order;
   duplicates dropped after first occurrence. The
   file's own glob overrides apply last. Order is
   list-driven, so it is stable across runs.
2. **No kind names in rule code.** Rules read their
   own settings; they never branch on a kind name.
   New kinds cannot regress existing behavior.

### Conflict resolution

Two kinds can disagree about a setting. The later kind
in the effective list replaces the earlier kind's
config for that rule wholesale — no deep-merge, same
as `overrides:` today. A follow-up may add deep-merge
across both kinds and overrides; see plan 97.

## Examples

> Kind names below (`plan`, `proto`, `tip`,
> `worksheet`) are fictional — projects pick their own.

### Explicit kinds in front matter

```markdown
---
kinds: [plan]
id: 92
status: 🔲
---
# File kinds
```

A file with multiple kinds uses a multi-element list:
`kinds: [tip, worksheet]`. Merge order matches list
order.

### Glob-based assignment

`kind-assignment:` is a **list** of entries with the
same YAML shape as `overrides:` entries. The `files:`
list uses the same glob matcher as `overrides:` and
`ignore:` — no `!`-negation syntax. To exclude a
narrower path, write a glob that doesn't match it.
For example, `plan/[0-9]*_*.md` excludes
`plan/proto.md` naturally. Alternatively, assign the
more specific kind in a later entry; the file carries
both kinds and the later one wins on conflict.

```yaml
kind-assignment:
  - files: ["plan/[0-9]*_*.md"]    # excludes proto.md
    kinds: [plan]
  - files: ["**/proto.md"]
    kinds: [proto]
  - files: [".github/PULL_REQUEST_TEMPLATE.md"]
    kinds: [worksheet]
  - files: ["docs/_partials/**"]
    kinds: [tip]
```

## Tasks

1. Add the `kinds:` and `kind-assignment:` config keys
   to the config schema; reuse the existing override
   merge to apply a kind's body.
2. Add the front-matter `kinds:` list field; resolve
   each file's effective kind list per Kind assignment
   above (front-matter first, then `kind-assignment:`
   matches in config order; dedup by first occurrence).
3. Wire kind-resolved rule config into the engine so
   each file is linted with its merged settings.
4. Emit a clear config error when kind assignment or
   front matter references an undeclared kind name.
5. Grep test asserts the linter's core contains no
   `if kind == "..."` branches and no hardcoded kind
   names.

## Acceptance Criteria

- [ ] A kind declaration parses with the same syntax
      as an override entry (minus `files:`); override
      merge is reused for kind merge (verified by
      test).
- [ ] Two project-declared kinds compose correctly
      with each other and with file-glob overrides
      (covered by test).
- [ ] A file declaring multiple kinds via
      `kinds: [a, b]` in front matter merges them in
      list order (covered by test).
- [ ] Conflicting settings between kinds resolve by
      block replacement — the later kind in the
      effective list replaces the earlier kind's
      entire rule config (matching today's
      `overrides:` behavior, covered by test).
- [ ] Files of a kind that sets
      `rules.required-structure.schema:` are validated
      against that schema (covered by test).
- [ ] Referencing an undeclared kind name produces a
      clear config error.
- [ ] No kind name is referenced by mdsmith's core
      (enforced by grep test).
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
