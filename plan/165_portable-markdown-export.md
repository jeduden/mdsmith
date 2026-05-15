---
id: 165
title: Portable Markdown export (mdsmith export)
status: "🔲"
model: opus
depends-on: []
summary: >-
  Add an `export` subcommand that writes a portable,
  directive-free copy of a Markdown file: markers
  removed, generated bodies kept, includes inlined.
---
# Portable Markdown export (mdsmith export)

## Goal

`mdsmith export <file>` writes a portable copy of a
Markdown file with every `<?…?>` directive marker
removed. Generated bodies stay as plain Markdown and
`<?include?>` content is inlined. The result renders
identically on any Markdown tool with no mdsmith
knowledge.

## Why a separate command

This is not schema extraction. `extract` (plan 163)
projects a kind's schema into a data tree. `export` is a
source-to-source transform of the document itself. It
needs no kind, schema, or conformance gate — only that
the file parses and its directive bodies are fresh.

Mixing it into `extract --format markdown` would couple a
plain-document transform onto the schema-projection
command. A dedicated `export` keeps the two concerns
apart and leaves room to grow (output path, later batch).

## Behavior

- Regenerate directive bodies in memory first, reusing
  the same engine as `mdsmith fix`, so output is never
  stale. The source file is never modified.
- Drop the opening and closing marker lines of every
  directive region; keep the body text between them
  verbatim.
- `<?include?>` bodies are already expanded by
  regeneration, so keeping the body inlines the included
  content (recursively).
- Markerless directives with no body (for example
  `<?allow-empty-section?>`, `<?require?>`) are removed
  outright.
- Nested same-type markers that the engine treats as
  literal content of an outer directive are preserved,
  by reusing the engine's directive-range detection
  rather than a separate scan.
- After stripping, normalize blank lines so the output is
  stable and lint-clean. Front matter is kept as-is.
- Exporting an already directive-free file is a no-op;
  `export` is idempotent.

## Tasks

1. **Export core (red/green).** Add `internal/export`
   with `Export(f *lint.File) ([]byte, error)`:
   regenerate directive bodies in memory via the `fix`
   directive engine, then remove marker lines while
   keeping bodies. Unit-test marker removal, body
   retention, include inlining, and the no-directive
   no-op.
2. **Nested / literal-content markers.** Drive removal
   off the engine's directive ranges (e.g.
   `lint.File` generated ranges) so inner same-type
   markers that are literal content survive. Add a test.
3. **Whitespace normalization.** Collapse the blank
   lines left by removed markers so output is stable and
   passes `mdsmith check`. Test idempotence: export of
   export equals export.
4. **`export` subcommand.** Register `export` in
   [main.go](../cmd/mdsmith/main.go); `mdsmith export
   <file>` writes to stdout, `-o/--output <path>` writes
   a file. Never mutate the source. Reuse the config and
   file-load helpers that back `fix` in
   [main.go](../cmd/mdsmith/main.go). Exit non-zero with
   a clear message on parse errors.
5. **Fixtures and integration test.** Add `testdata`
   inputs covering include, catalog, toc, and build
   directives with golden directive-free outputs. Assert
   idempotence and that the output passes `mdsmith
   check`.
6. **Docs.** Add `docs/reference/cli/export.md` and link
   it from the CLI reference catalog. Run `mdsmith fix`
   so catalogs and PLAN.md regenerate.

## Acceptance Criteria

- [ ] `mdsmith export <file>` emits the file with all
      directive markers removed and generated bodies
      kept; `<?include?>` content is inlined.
- [ ] The source file is never modified.
- [ ] Stale directive bodies are regenerated before
      stripping, so the output is never stale.
- [ ] Nested same-type literal-content markers are
      preserved.
- [ ] Output is idempotent and passes `mdsmith check`.
- [ ] `-o <path>` writes to a file; stdout is the
      default.
- [ ] A parse error or missing file exits non-zero with
      a clear message.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes

## Decisions

- **Keep generated bodies.** Markers are stripped but
  TOC, catalog, and included content stay as plain
  Markdown; includes are inlined for a portable copy.
- **New `export` subcommand.** Not a fourth `extract`
  format and not a `fix` flag; a dedicated command keeps
  the source-to-source transform separate from schema
  extraction.
- **Front matter retained.** It is not a directive;
  stripping it is out of scope.
- **Single file first.** Directory or glob batch export
  is a possible follow-up, not in this plan.
