---
id: 2606022125
title: "MDS019 catalog: CUE-expression row templates"
status: "✅"
summary: >-
  Let `<?catalog?>` row templates be CUE expressions
  evaluated against frontmatter, in addition to the
  existing `{path}` placeholder syntax. Unlocks list
  comprehension, ternary, and string interpolation so a
  structured field can render into a generated table
  without an external Go helper. `header:`/`footer:`
  stay placeholder-only (they emit literal text today).
model: sonnet
depends-on: []
---
# MDS019 catalog: CUE-expression row templates

## Goal

Today the `<?catalog?>` `row:` template can interpolate
scalar fields like `{title}` or `{filename}`. It cannot
project a list-typed field. The placeholder resolver in
[`internal/fieldinterp`][fi] walks `map[string]any`
only. It rejects composite leaves on purpose. There is
no syntax for list indexing, list iteration, or
conditional rendering.

The [peer-linter coverage matrix][matrix] is the live
example. Each rule README carries structured peer
mappings:

```yaml
markdownlint:
  - id: MD018
    name: no-missing-space-atx
    default: true
  - id: MD019
    name: no-multiple-space-atx
    default: true
```

The desired cell is
`MD018 ✅ no-missing-space-atx, MD019 ✅ no-multiple-space-atx`.

No combination of today's `{path}` placeholders
produces it. So `internal/release/coverage.go`
shipped ~360 lines of Go. The release tool walked the
list, formats each entry, and joins the result. The
same shape recurs anywhere a Markdown file needs to
project a list-typed field into a generated table.

After this plan, a row template can be a CUE expression
that runs against the matched file's frontmatter:

```markdown
<?catalog
glob: "internal/rules/MDS*/README.md"
where: 'category: "heading"'
sort: id
row-expr: '
  "| \(id) | " +
  strings.Join(
    [for m in markdownlint {
      "\(m.id) \([if m.default {"✅"}, if !m.default {"⚪"}][0]) \(m.name)"
    }],
    ", "
  ) + " |"
'
?>
```

CUE handles list comprehension, conditionals, string
interpolation, and `strings.Join`. The `where:`
parameter already evaluates CUE against the same scope.
The evaluator is in place. Only the row, header, and
footer rendering paths need the new branch. Existing
`row:` templates keep working unchanged.

The coverage matrix collapses to a hand-authored
skeleton with one `<?catalog?>` block per category.
The Go generator and its CI drift job delete. Future
call sites — multi-author plan lists, multi-tag
indexes, schema-row tables — pick up the same
capability for free.

## Tasks

1. **CUE evaluator for templates.** Add a function in
   [`internal/fieldinterp`][fi] (or a sibling package).
   It takes a CUE source string. It binds the file's
   frontmatter map as scope. It returns the rendered
   string. Reuse the loader the [catalog parser][parse]
   already uses for `where:`.

2. **New catalog parameter.** Add `row-expr:`, mutually
   exclusive with `row:`. Setting both is an MDS019
   diagnostic. `header:`/`footer:` stay placeholder-only:
   they are emitted as literal text today (no
   per-file scope), so a CUE-expression form would be
   ahead of demand. Document `row-expr:` in the
   [catalog README][catdoc].

3. **Diagnostics.** Surface CUE evaluation errors on
   the directive's opening line. Match the pattern from
   [`bad/where-invalid.md`][badwhere]. A row-expr that
   resolves to a non-string is an error, not a silent
   empty cell.

4. **Documentation.** Update the
   [generating-content guide][guide] with a worked
   example that projects a list-typed field. Add a note
   that contrasts `row:` (scalar fields, simple
   substitution) with `row-expr:` (lists, conditionals,
   derived values).

5. **Migrate the coverage matrix.** Rewrite the
   [matrix page][matrix] as a static skeleton with one
   `<?catalog?>` per category, using `row-expr:`
   against the structured peer-mapping frontmatter.
   Delete `coverage.go`, `coverage_test.go`,
   `sync_coverage.go`, `sync_coverage_test.go`, and the
   `coverage-matrix-drift` job in [`ci.yml`][ci].
   `mdsmith check` (via MDS019) is the new drift gate.

   The proto schema now requires `partial: bool`
   (previously `partial?: bool | *false`). All 42 rule
   READMEs were backfilled with `partial: false` on
   entries that lacked it. The row-expr reads `m.partial`
   directly without a guard. All 145 data rows produced
   by `<?catalog?>` match the former Go generator
   output byte-for-byte.

## Acceptance Criteria

- [x] The `row-expr:` parameter parses and evaluates
  as CUE against the matched file's frontmatter, with
  every identifier-safe frontmatter key bound at the
  expression's top-level scope.
- [x] Setting both `row:` and `row-expr:` on the same
  directive emits an MDS019 diagnostic on the opening
  line.
- [x] A row-expr that fails to evaluate, returns a
  non-string, or references an unknown field emits an
  MDS019 diagnostic on the opening line. The message
  names the failing field or CUE error.
- [x] The coverage matrix renders identically to its
  current byte content from `<?catalog?>` blocks
  alone. No `sync-coverage-matrix` is in the loop.
- [x] `coverage.go` and its companions are deleted.
  The `coverage-matrix-drift` job is removed from
  [`ci.yml`][ci]. `mdsmith check .` is the only gate.
- [x] A new fixture under the [catalog rule's good
  fixtures][catgood] exercises a `row-expr` that
  projects a list field via `strings.Join` and a
  ternary.
- [x] The [generating-content guide][guide] carries a
  worked example of `row-expr:` against a list-typed
  field.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
- [x] `go run ./cmd/mdsmith check .` passes

[fi]: ../internal/fieldinterp
[matrix]: ../docs/research/markdownlint-coverage/README.md
[ci]: ../.github/workflows/ci.yml
[parse]: ../internal/rules/catalog/parse.go
[catdoc]: ../internal/rules/MDS019-catalog/README.md
[badwhere]: ../internal/rules/MDS019-catalog/bad/where-invalid.md
[guide]: ../docs/guides/directives/generating-content.md
[catgood]: ../internal/rules/MDS019-catalog/good/
