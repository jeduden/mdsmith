---
id: 79
title: Nested front-matter access
status: "🔲"
summary: >-
  Support dot-separated keys in {field} and
  {{.field}} for nested YAML front matter.
---
# Nested front-matter access

Follow-on to the user-model work in
[plan 73](73_unify-template-directives.md).
Related to
[plan 75](75_single-brace-placeholders.md)
(`{field}` syntax).

Depends on: plan 75 (single-brace syntax must
land first so both syntaxes are updated
together).

## Goal

`{params.sub}` in a schema heading and
`{{.params.sub}}` in a catalog row both resolve
nested YAML front matter. A document with:

```yaml
---
params:
  subtitle: Overview
---
```

matches `# {params.subtitle}` in the schema
and renders `{{.params.subtitle}}` as
`Overview` in catalog output.

## Context

Required-structure parses front matter into
`map[string]any` but stringifies values for
sync checks. Catalog reads front matter via
`readFrontMatter` in `catalog/rule.go`, also
flattening to strings. Neither path resolves
dotted keys into nested maps. Hugo users
expect `.Params.subtitle`; mdsmith does not
support this yet.

## Design

Keep the existing `map[string]any` parse but
add dot-path resolution that walks nested maps
instead of flattening to strings.

- `{a.b.c}` splits on `.` and walks:
  `fm["a"].(map)["b"].(map)["c"]`
- If any step is not a map, emit a diagnostic:
  `front-matter key "a.b" is not a map`
- Top-level keys with literal dots (e.g.
  `"a.b": value`) take precedence over nested
  lookup to avoid breaking existing configs
- Catalog `{{.a.b}}` already works in Go
  `text/template` if the data is a nested map;
  only the data structure needs to change

### Grammar comparison: `{field}` vs CUE paths

CUE paths and `{field}` paths both use `.` for
nesting but differ in key quoting:

| Feature     | CUE path          | `{field}` path         |
|-------------|-------------------|------------------------|
| Separator   | `.`               | `.`                    |
| Simple key  | `a.b`             | `{a.b}`                |
| Hyphenated  | `"my-key".sub`    | `{my-key.sub}`         |
| Quoted dots | `"a.b"` (one key) | literal-dot precedence |
| Optional    | `a?.b`            | not supported          |

CUE requires quoting for non-identifier keys.
The `{field}` syntax does not quote; instead,
literal-dot keys take precedence. This keeps
the placeholder syntax simple (no quotes inside
braces) at the cost of not supporting keys
that are both dotted and nested. This is an
acceptable trade-off: YAML keys with literal
dots are rare in practice.

## Tasks

1. Update front-matter handling in
   `catalog/rule.go` (`readFrontMatter`) and
   `requiredstructure/rule.go`
   (`readDocFrontMatterRaw`/`stringifyFrontMatter`)
   to preserve nested `map[string]any` values.
2. Add a `resolvePath(fm map[string]any,
   path string) (string, error)` helper that
   splits on `.` and walks nested maps.
3. Update `requiredstructure/rule.go`:

  - `resolveFields` uses `resolvePath`
  - `docFM` type changes to `map[string]any`
  - Update `fieldPattern` regex to allow dots
    in placeholder names (`\{([\w.]+)\}`) so
    `{a.b}` is captured as a dotted path

4. Update `catalog/generate.go`:

  - Template data uses `map[string]any`
  - Go `text/template` handles nested access
    natively via `.a.b`

5. Verify CUE schema derivation in
   `requiredstructure/rule.go` already handles
   nested front matter; only adjust if gaps
   remain for nested placeholder resolution.
6. Add unit tests for nested access in both
   required-structure and catalog.
7. Add fixtures with nested front matter.
8. Run `mdsmith check .` to verify.

## Acceptance Criteria

- [ ] `{a.b}` in a schema heading resolves
      nested front-matter key `a.b`
- [ ] `{{.a.b}}` in catalog row resolves
      nested front-matter key `a.b`
- [ ] Literal dot-key `"a.b": val` takes
      precedence over nested lookup
- [ ] Missing nested key emits a diagnostic
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
- [ ] `mdsmith check .` passes
