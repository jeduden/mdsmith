---
title: Development
weight: 60
summary: Build commands, project layout, code style, test fixtures, coverage gate, and merge conflicts.
---

Build and test reference for mdsmith contributors.
See also:

<?catalog
glob:
  - "*.md"
  - "*/index.md"
  - "!index.md"
sort: title
row: "- [{title}]({filename})"
?>
- [Adding a peer linter](add-peer-linter.md)
- [Architecture audit log](architecture-audit.md)
- [Architecture principles](architecture/index.md)
- [Coverage Gate](coverage.md)
- [File Placement](file-placement.md)
- [High-Performance Go](high-performance-go.md)
- [Merge Queue](merge-queue.md)
- [PR Fixup Workflow](pr-fixup-workflow.md)
- [Public Markdown Library](markdown-library.md)
- [Release Pipeline](release.md)
- [Release Tooling Architecture](release-tooling.md)
- [Secret Rotations](secret-rotations.md)
- [Ship and adopt new directive syntax](adopt-new-directive-syntax.md)
- [Website configuration](website-config.md)
<?/catalog?>

## Build & Test Commands

Requires Go 1.24+.

- `go build ./...` — build all packages
- `go test ./...` — run all tests
- `go test -run TestName ./...` — run a specific test
- `go run ./cmd/mdsmith check .` — lint markdown
- `go run ./cmd/mdsmith fix .` — auto-fix markdown
- `go tool golangci-lint run` — run linter
- `go vet ./...` — run go vet

## Project Layout

Follows the [standard Go project layout][stdlayout]:

- `cmd/mdsmith/` — main entry point.
- `internal/` — private packages.
- `internal/rules/<rule-name>/` — rule code (e.g.
  `paragraphstructure/`).
- `internal/rules/MDS###-<rule-name>/` — rule README
  and good/bad fixtures (e.g.
  `MDS024-paragraph-structure/`).
- `testdata/` — shared markdown fixtures.
- `pkg/goldmark/` — vendored goldmark fork.

[stdlayout]: https://go.dev/doc/modules/layout

## Code Style

- Follow standard Go conventions (gofmt, goimports).
- Use golangci-lint for linting.
- Keep functions small and focused.
- Error messages: lowercase, no trailing punctuation.
- Prefer returning errors over panicking.

## Defensive Code

Add a defensive branch only when you can drive it
red/green. Write the failing test first. Then add
the code that takes the branch.

## Allocation Budget

**A rule's `Check` allocates ≤ 10 times per call on
representative input.** Enforced by
`internal/integration/alloc_budget_test.go`; most
rules allocate 0–6.

- Walk `f.Lines` / `f.AST` directly.
- Prefer `bytes.IndexByte` / `bytes.Contains` over
  `regexp` for fixed searches.
- Compile every `regexp.Regexp` at package scope.
- Pre-size slices with `make([]X, 0, n)`.
- Reuse loop-local buffers via `buf = buf[:0]`.
- Return `nil`, not an empty slice, on no diagnostics.

## Test Fixtures

Rule test fixtures live in
`internal/rules/MDS###-<rule-name>/` (e.g.
`MDS024-paragraph-structure/`). Each rule has `good/`
and `bad/` examples (or `good.md` / `bad.md`).

Good fixtures must pass **all default-enabled rules**
plus the rule under test. Opt-in rules are skipped:
a good MDS001 fixture need not also satisfy MDS043.
When a good fixture uses non-default settings,
override them in `.mdsmith.yml` so `mdsmith check .`
also passes. Bad fixtures are excluded via the
`ignore:` section.

When adding or changing a rule, add both:

1. **Unit tests** in `rule_test.go` (inline markdown,
   fast red/green). Use `require` for preconditions
   and `assert` for checks; `Same`/`NotSame` for
   pointer identity.
2. **Fixture tests** under
   `internal/rules/MDS###-<rule-name>/` with YAML
   frontmatter specifying expected diagnostics.
   Discovered automatically by
   `internal/integration/rules_test.go`.

## Config Merge Semantics

Layered config (defaults → kinds → overrides) is
**deep-merged** rule by rule:

- Maps merge key by key; siblings set in earlier
  layers survive partial overrides.
- Scalar leaves are replaced wholesale.
- List settings replace by default. Opt into
  `append` by implementing
  `rule.ListMerger.SettingMergeMode(key)`. The
  placeholder vocabulary is the canonical example.
- A bool-only layer (`rule-name: false`) toggles
  `enabled` without erasing inherited settings.

New list-typed settings must document the choice
next to their `ApplySettings` handler.

## Generated Sections

Content between `<?directive ... ?>` and
`<?/directive?>` markers is auto-generated. Edit
directive parameters or the source file, then run
`mdsmith fix <file>` — never the body by hand. Run
`mdsmith merge-driver install [files...]` once per
clone so generated-section conflicts resolve
automatically.
