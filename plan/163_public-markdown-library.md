---
id: 163
title: 'Extract mdsmith Markdown parse/produce as a public Go library'
status: "✅"
summary: >-
  Expose mdsmith's Markdown parsing and a
  canonical producer as a stable public Go
  package so the linter core and release
  tooling share one implementation instead
  of separate goldmark configs.
model: ""
depends-on: []
---
# Extract mdsmith Markdown parse/produce as a public Go library

## Goal

Give mdsmith one importable Markdown
surface. It covers parse and produce.
Callers stop reinventing it. They stop
coupling to the linter core.

## Context

This is option C deferred from PR #291. The
sync-docs work there needed to parse a doc
and emit Markdown. The three options were:

1. Raw goldmark inside
   [internal/release](../internal/release)
   (chosen, interim).
2. Reuse the linter parser — rejected: it
   couples release tooling to the linter
   core and the
   [architecture hub](../docs/development/architecture/index.md)
   keeps that boundary.
3. A public library — best end state, but a
   separate initiative, captured here.

Two facts make C real work, not a move:

- There is no producer to expose. `mdsmith
  fix` is edit-based: it applies per-rule
  segment edits, not an AST → Markdown
  render. A reusable producer is net-new
  design.
- Everything sits under
  [internal/](../internal). A public
  package is a semver and documentation
  commitment, and the parser config in
  [internal/rules/markdownflavor](../internal/rules/markdownflavor)
  imports
  [internal/lint](../internal/lint), so
  the parse path must be untangled from
  linter concerns first.

When this lands, the public package
replaces the interim goldmark config in
[internal/release/syncdocs.go](../internal/release/syncdocs.go).
Behavior does not change.

## Tasks

1. [x] Design the public API: a parse
   entry that returns the goldmark AST
   plus front matter, and a producer entry
   that surgically edits source spans (the
   model sync-docs already relies on).
   Decide the package path
   (`pkg/markdown` vs a top-level module).
   Decided: `pkg/markdown` inside the
   existing module
   (`github.com/jeduden/mdsmith/pkg/markdown`).
   Surface: `Parse(source) *Document`
   (front matter + body + AST),
   `Splice(body, []Edit) []byte` (the
   edit-based producer), `ParseContext`
   /`NewParser`/`StripFrontMatter`
   /`CountLines`, and the
   `ProcessingInstruction` node primitive.
   `internal/lint` re-exports these via
   type aliases and forwards so the single
   goldmark config lives only in
   `pkg/markdown`.
2. [x] Untangle the parser config from
   [internal/lint](../internal/lint) so
   parsing has no rule/config/diagnostic
   dependency.
3. [x] Build the producer: a documented,
   tested surgical-splice + render path
   that does not fight `mdsmith fix`
   output.
4. [x] Migrate
   [internal/release](../internal/release)
   off its local goldmark config onto the
   public package; assert identical
   sync-docs output.
5. [x] Migrate the linter core to the same
   package so there is a single parser
   config in the tree.
6. [x] Add API docs and a semver policy
   entry under
   [docs/development](../docs/development).

## Acceptance Criteria

- [x] A public package exposes parse +
  produce with a documented, stable API.
- [x] No public parse path imports the
  linter core
  ([internal/lint](../internal/lint)).
- [x] sync-docs and the linter both
  consume the public package; only one
  goldmark config remains in the tree.
- [x] sync-docs output is byte-identical
  before and after the migration.
- [x] All tests pass: `go test ./...`.
- [x] `go tool golangci-lint run` reports
  no issues.
- [x] `mdsmith check .` passes.
