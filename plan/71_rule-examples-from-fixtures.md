---
id: 71
title: Rule README examples must include from fixture files
status: "🔲"
summary: >-
  Replace inline code-block examples in every rule README
  with include directives that reference actual good/ and
  bad/ fixture files, so examples never drift from tests.
---
# Rule README examples must include from fixture files

## Context

Every rule README (MDS001–MDS031) has an `## Examples`
section with inline code blocks showing good and bad
Markdown. These snippets are disconnected from the test
fixtures in each rule's `good/` and `bad/` directories.
When a fixture changes the README can become wrong. When
someone edits only the README the example may not match
what the tests assert.

The `<?include?>` directive (MDS021) already exists and
can pull file content into a document. Using it for rule
examples guarantees every example shown in documentation
is a real, tested fixture file.

## Goal

Replace inline examples in rule README `## Examples`
sections with `<?include?>` directives that point at
files in each rule's `good/` and `bad/` directories.
After this change, no rule README contains an example
that is not a fixture file.

## Tasks

1. Audit every rule README (MDS001 through MDS031):
   list each inline example and identify which existing
   `good/` or `bad/` fixture it corresponds to.
2. Where an inline example has no matching fixture, create
   a new fixture file in the appropriate `good/` or `bad/`
   directory that reproduces the example. Add `diagnostics`
   front matter as required so the fixture passes its own
   rule's tests.
3. For each rule README, replace every inline example code
   block in the `## Examples` section with an
   `<?include?>` directive referencing the corresponding
   fixture file. Use `wrap: markdown` when the fixture
   should render inside a fenced code block.
4. Verify every rule README still passes linting:
   `go run ./cmd/mdsmith check internal/rules/`.
5. Verify all existing tests still pass: `go test ./...`.
6. Run `go run ./cmd/mdsmith check .` to confirm the full
   repo passes.

## Acceptance Criteria

- [ ] No rule README contains an inline example code block
      in its `## Examples` section that is not backed by a
      file in `good/` or `bad/`
- [ ] Every example in a rule README uses an `<?include?>`
      directive pointing at a `good/` or `bad/` fixture
- [ ] No existing test is broken by the change
- [ ] All fixtures have correct `diagnostics` front matter
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
- [ ] `go run ./cmd/mdsmith check .` passes
