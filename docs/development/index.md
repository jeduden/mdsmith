---
title: Development
summary: Build commands, project layout, code style, test fixtures, coverage gate, and merge conflicts.
---

Build and test reference for mdsmith contributors.
See also:

- [Coverage gate](coverage.md)
- [File placement](file-placement.md)
- [Merge queue](merge-queue.md)
- [PR fixup workflow](pr-fixup-workflow.md)

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

Follow the [standard Go project
layout](https://go.dev/doc/modules/layout):

- `cmd/mdsmith/` — main application entry point
- `internal/` — private packages not importable by
  other modules
- `internal/rules/` — rule documentation
  (`internal/rules/<id>-<name>/README.md`)
- `testdata/` — test fixtures (markdown files for
  testing rules)

## Code Style

- Follow standard Go conventions (gofmt, goimports)
- Use golangci-lint for linting
- Keep functions small and focused
- Error messages should be lowercase, no trailing
  punctuation
- Prefer returning errors over panicking

## Test Fixtures

Rule test fixtures live in
`internal/rules/<id>-<name>/`. Each rule has `good/` and
`bad/` examples (or `good.md` / `bad.md`).

Good fixtures must pass **all** rules, not just their
own. When a good fixture uses non-default settings
(e.g. setext headings, tilde fences), add a matching
override in `.mdsmith.yml` so that `mdsmith check .`
also passes.

Bad fixtures are excluded via the `ignore:` section in
`.mdsmith.yml`.

When adding or changing a rule feature, add both:

1. **Unit tests** in `rule_test.go` (inline markdown,
   fast red/green cycle). Use `require` from testify
   for preconditions that abort the test and `assert`
   for checks that continue. Use `Same`/`NotSame` for
   pointer identity.
2. **Fixture tests** under `internal/rules/<id>-<name>/`
   (`good/` and `bad/` markdown files with YAML
   frontmatter specifying expected diagnostics). These
   are discovered automatically by the integration test
   runner in `internal/integration/rules_test.go`.

## Merge Semantics

Rule settings **deep-merge** across config layers.
Block replacement is gone.

Layer order is: top-level `rules:`, then matching
`kinds:` in effective list order, then matching
`overrides:` in config order. Front-matter `kinds:`
come first; `kind-assignment:` matches follow.

For each layer that touches a rule:

- **Maps** merge key by key. Nested maps recurse.
- **Scalars** take the later value.
- **Lists** replace wholesale by default. A rule may
  opt a setting key into **append** by implementing
  `rule.ListMerger`.

Today the `placeholders:` key uses `append` on these
rules: `first-line-heading`, `heading-increment`,
`no-emphasis-as-heading`, `paragraph-readability`,
`paragraph-structure`, `required-structure`,
`cross-file-reference-integrity`.

A layer that restates the full rule body still wins on
every leaf. Setting `<rule>: false` clears the
accumulator and disables the rule outright.

## Generated Sections

Content between `<?directive ... ?>` and
`<?/directive?>` markers is auto-generated. Do not
edit the body by hand — edit the directive parameters
or the source file it references, then run
`mdsmith fix <file>` to regenerate.

After merge conflicts in generated sections, run
`mdsmith fix <file>` to regenerate. The
`merge-driver` command automates this:

```bash
mdsmith merge-driver install [files...]
```

Run `mdsmith merge-driver install` once per clone.
