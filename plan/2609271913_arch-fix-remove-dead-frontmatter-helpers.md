---
id: 2609271913
title: >-
  Remove dead front-matter parse helpers in internal/index
status: "✅"
model: haiku
summary: >-
  internal/index/build.go's frontMatterSymbols,
  frontMatterScalar, and frontMatterStringList are no
  longer called by production code — frontMatterAll
  replaced them — and are kept alive only so
  coverage_test.go has something to test. Flagged by the
  2026-09-27 audit as tax.
---
# Remove dead front-matter parse helpers in internal/index

## Goal

Delete the three superseded front-matter parsing helpers
and their coverage-only tests. `internal/index` should
carry no production code whose only reason to exist is a
test.

## Background

The 2026-09-27 audit (see [the audit log][audit-log]) found:

- [internal/index/build.go][build]'s `frontMatterSymbols`,
  `frontMatterScalar`, and `frontMatterStringList`
  (lines 354–487) are single-purpose predecessors to
  `frontMatterAll`, which now does the same job in one YAML
  pass. `frontMatterAll`'s own doc comment says it is
  "equivalent to calling `frontMatterSymbols` +
  `frontMatterScalar(title)` + `frontMatterStringList(kinds)`."
- The only remaining callers are in
  [internal/index/coverage_test.go][coverage-test]; the
  target of the coverage claims these functions are "kept
  ... for the targeted coverage test."
- [tests.md][tests]'s per-function unit-test rule exists to
  make sure production code is exercised, not to justify
  keeping unused production code alive as a test subject —
  this inverts that rule. It also leaves ~130 lines of dead
  code a future reader could mistake for a live path, and a
  duplicate YAML-parse implementation that can silently
  diverge from `frontMatterAll`'s behavior with nothing in
  the real build path to catch it.

## Tasks

1. Read [internal/index/build.go][build]'s
   `frontMatterSymbols`, `frontMatterScalar`, and
   `frontMatterStringList` and confirm (e.g. via
   `grep -rn` across non-test files) that `frontMatterAll`
   is the only production caller path and none of the three
   helpers is called outside `internal/index`.
2. Delete the three functions from
   [internal/index/build.go][build].
3. Delete their dedicated tests in
   [internal/index/coverage_test.go][coverage-test]; if any
   of those test cases exercise a YAML edge case (invalid
   scalar, missing key, wrong-typed value) not already
   covered by `frontMatterAll`'s own tests, port the case
   into a `frontMatterAll` test first, then delete the old
   test.
4. `go build ./...` passes.
5. `go test ./...` passes, and `internal/index`'s coverage
   does not drop below the project's coverage gate (see
   [coverage.md][coverage]).
6. `go tool -modfile=tools/go.mod golangci-lint run` reports
   no issues.

## Deviations

The port found drift. `frontMatterAll` did not agree with
the old helpers or with the [internal/lint][lint-fm]
decoders. The port fixes the drift:

- A null `title:` (`null`, `~`, empty) yields no title.
  Before, `frontMatterAll` returned the text `null` or `~`.
- A title collapses runs of Unicode whitespace to one
  space and trims its ends, so a workspace symbol name has
  no newlines. A `title` key that appears twice yields no
  title; other duplicate keys leave it alone. A title set
  only through a merge (`<<`) key is read, as the engine's
  map decode reads it.
- Typed scalar titles (int, float, uint64, bool, date) keep
  their source text. The deleted `frontMatterScalar` wrote
  a date in RFC 3339 form; the index never used that path.
- Kinds now come from `lint.FrontMatterKindsFromNode`, the
  engine's own decode, run on the node the index already
  parsed when the walk sees a `kinds` or `<<` key. So
  `- 42` is `"42"`, a mapping entry or duplicate key drops
  the list, and only the bytes `kinds:` are read. Before,
  the index kept kinds the engine never applied. The YAML
  is parsed once per file.
- A `!!binary` title shows its decoded text, as the
  engine's map decode does. One that is not valid base64
  or not UTF-8 has no title.
- `yamlutil.UnmarshalSafe` and the new
  `yamlutil.DecodeNodeSafe` turn a yaml.v3 decode panic
  into an error. A complex key next to a merge key made
  yaml.v3 panic, so one crafted file crashed the process.
  `requiredstructure` decoded document front matter with a
  raw `yaml.Unmarshal`; it now uses `UnmarshalSafe` and
  words the alias case apart through `yamlutil.ErrAliases`.
  Kind, convention, and word-list files had the same crash
  in their strict decoders; they now share
  `yamlutil.UnmarshalStrictSafe`.
- The index's `stripDelimiters` had a fallback for a closing
  `---` with no newline, which `StripFrontMatter` never
  produces. It and the other exact copies of the fence
  trim now call one helper, `lint.FrontMatterYAML`. That
  fixed the catalog reader, which cut the YAML at a `---`
  line inside a block scalar. The trims in
  `requiredfrontmatter` and `requiredstructure` call it
  too. Their extra branches, for a bare `---` close and a
  block with no fence, ran only on test input.

## Acceptance Criteria

- [x] `frontMatterSymbols`, `frontMatterScalar`, and
      `frontMatterStringList` no longer exist in
      `internal/index`.
- [x] `frontMatterAll`'s test coverage includes every YAML
      edge case the deleted helpers' tests exercised.
- [x] `go test ./...` is green.
- [x] `mdsmith check .` is green.
- [x] Codecov coverage gate still passes (no net coverage
      regression from the deletion).

[audit-log]: ../docs/development/architecture-audit.md
[tests]: ../docs/development/architecture/tests.md
[coverage]: ../docs/development/coverage.md
[build]: ../internal/index/build.go
[coverage-test]: ../internal/index/coverage_test.go
[lint-fm]: ../internal/lint/frontmatter.go
