---
id: 70
title: Multi-glob lists, brace expansion docs, and folded scalar restriction
status: "🔳"
summary: >-
  Let the catalog glob key accept a YAML list of patterns,
  document brace expansion, and document the folded-scalar
  restriction in all generated-section rule docs.
---
# Multi-glob lists, brace expansion docs, and folded scalar restriction

## Context

The catalog directive's `glob:` key accepts only a single
string. Users who need files from multiple unrelated
directories must resort to brace expansion or multiple
catalog blocks. Supporting a YAML list of patterns is more
natural. Additionally, YAML folded scalars (`>`, `>-`)
break markdown parsing inside processing directives and
this is undocumented. Brace expansion `{a,b}` already
works (via doublestar) but is only mentioned in an edge
case table.

## Goal

Let the catalog `glob:` key accept a YAML list of
patterns. Document `{}` brace expansion. Document
the folded-scalar restriction across all
generated-section rule docs.

## Tasks

1. Add failing tests in
   [internal/rules/catalog/rule_test.go](../internal/rules/catalog/rule_test.go)
   for YAML list glob, inline and block syntax,
   deduplication, sorting, empty element diagnostic,
   absolute path diagnostic, `..` diagnostic, invalid
   pattern diagnostic, non-string element diagnostic,
   and single-string backward compatibility.
2. Modify `ValidateStringParams` in
   [parse.go](../internal/archetype/gensection/parse.go)
   to handle `[]any` values: when every element is a
   string, join with `\n`; otherwise emit a per-key
   diagnostic.
3. Add `splitGlobs` helper in
   [internal/rules/catalog/rule.go](../internal/rules/catalog/rule.go).
   Update `validateGlob` to validate each pattern
   independently. Update `buildCatalogEntries` to glob
   each pattern, deduplicate by path, then sort.
4. Make tests green, run `go test ./...`.
5. In the [generated-section archetype
   README](../docs/design/archetypes/generated-section/README.md),
   add folded scalar restriction, list-valued parameters
   note, and brace expansion documentation.
6. In the [MDS019 catalog
   README](../internal/rules/MDS019-catalog/README.md),
   update parameters for multi-glob list, document brace
   expansion, add folded scalar note.
7. In the [MDS021 include
   README](../internal/rules/MDS021-include/README.md),
   add folded scalar restriction note.
8. `go test ./...` passes.
9. `go run ./cmd/mdsmith check .` passes.

## Acceptance Criteria

- [ ] `glob:` accepts a YAML list of string patterns in
      catalog directives
- [ ] `glob:` with a single string still works
- [ ] Files matched by multiple patterns appear once
- [ ] Each pattern in a list is validated independently
      (absolute path, `..`, invalid pattern)
- [ ] Non-string list elements produce a diagnostic
- [ ] Folded scalar restriction is documented in
      archetype, MDS019, and MDS021 READMEs
- [ ] Brace expansion is documented in archetype and
      MDS019 READMEs
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
