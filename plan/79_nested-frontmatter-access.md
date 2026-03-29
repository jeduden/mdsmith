---
id: 79
title: Nested front-matter access
status: "✅"
summary: >-
  Use CUE path syntax in {field} placeholders
  for nested and quoted front-matter access.
---
# Nested front-matter access

Follow-on to the user-model work in
[plan 73](73_unify-template-directives.md).
Extends
[plan 75](75_single-brace-placeholders.md)
(unified `{field}` syntax).

Depends on: plan 75 (unified `{field}` must
land first so CUE paths extend the shared
interpolation engine).

## Goal

`{a.b}` resolves nested front matter. Works
in both catalog rows and schema headings (plan
75 unifies them). Quoted CUE labels handle
non-identifier keys: `{"my-key".sub}`.

A document with:

```yaml
---
params:
  subtitle: Overview
my-key: value
---
```

matches `# {params.subtitle}` in a schema and
renders `{params.subtitle}` as `Overview` in
a catalog row. `{"my-key"}` resolves to
`value` in both contexts.

## Context

Plan 75 replaces Go `text/template` with a
shared `{field}` interpolation engine for both
catalog and required-structure. This plan
extends that engine with CUE path resolution
for nested maps and quoted keys.

## Design

Extend the `fieldinterp` engine (plan 75) to
parse CUE paths inside `{...}` placeholders.

- `{a.b.c}` resolves nested maps:
  `fm["a"].(map)["b"].(map)["c"]`
- `{"my-key".sub}` quotes non-identifier keys
  (hyphens, dots, spaces), same as CUE
- `{"a.b"}` is one key with a literal dot,
  distinct from `{a.b}` (two nested keys)
- If any step is not a map, emit a diagnostic:
  `front-matter key "a.b" is not a map`

### Why CUE paths

The tool already uses CUE for front-matter
schema validation. One path grammar everywhere:

| Context             | Syntax       | Example            |
|---------------------|--------------|--------------------|
| Schema front matter | CUE expr     | `'string & != ""'` |
| Schema heading      | `{CUE path}` | `{params.title}`   |
| Catalog row         | `{CUE path}` | `{params.title}`   |
| Quoted key          | `{"..."}`    | `{"my-key".sub}`   |

Same `{...}` syntax, same CUE path resolution,
in every context. No Go template syntax
anywhere in the user-facing surface.

## Tasks

1. Update front-matter handling in
   `catalog/rule.go` (`readFrontMatter`) and
   `requiredstructure/rule.go`
   (`readDocFrontMatterRaw`/`stringifyFrontMatter`)
   to preserve nested `map[string]any` values.
2. Extend the `fieldinterp` engine (plan 75)
   with a `resolveCUEPath` function:

  - Parse CUE path segments (identifiers and
    quoted labels)
  - Walk nested `map[string]any`
  - Return resolved string value or error

3. Update the placeholder regex in `fieldinterp`
   to capture CUE paths: identifiers, dots,
   and quoted strings inside `{...}`.
4. Verify CUE schema derivation in
   `requiredstructure/rule.go` handles nested
   front matter; only adjust if gaps remain.
5. Add unit tests for nested and quoted access
   in both catalog and required-structure.
6. Add fixtures with nested front matter.
7. Run `mdsmith check .` to verify.

## Acceptance Criteria

- [ ] `{a.b}` resolves nested front-matter
      in both catalog and schema headings
- [ ] `{"my-key".sub}` resolves quoted
      non-identifier key in both contexts
- [ ] `{"a.b"}` resolves a single key with
      a literal dot (CUE quoting)
- [ ] Missing nested key emits a diagnostic
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
- [ ] `mdsmith check .` passes
