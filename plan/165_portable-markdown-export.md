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

This is not schema extraction. `extract` (plan 166)
projects a kind's schema into a data tree. `export` is a
source-to-source transform of the document itself. It
needs no kind, schema, or conformance gate — only that
the file parses and its directive bodies are fresh.

Mixing it into `extract --format markdown` would couple a
plain-document transform onto the schema-projection
command. A dedicated `export` keeps the two concerns
apart and leaves room to grow (output path, later batch).

## Staleness: check by default, never auto-fix

`export` does **not** silently regenerate directive
bodies. Auto-fixing on export is surprising and would
mask drift between a directive and its rendered body.
The default is to *check*, not to *fix*:

- **Default (check).** Before stripping, verify each
  directive body equals what the engine would generate.
  If any body is stale, export writes nothing and exits
  non-zero with a diagnostic naming the stale directive
  and advising `mdsmith fix` or `--fix`. The export is
  faithful — it never papers over drift.
- **`--no-check`.** Skip the staleness check and export
  bodies exactly as they appear in the file. For callers
  who know the file is fresh or deliberately want the
  on-disk bytes.
- **`--fix`.** Regenerate stale bodies in memory (same
  engine as `mdsmith fix`) before stripping. Opt-in
  convenience for a one-shot fresh export.

`--fix` and `--no-check` are mutually exclusive (one
regenerates, the other trusts as-is); passing both is a
usage error. In every mode the source file is never
modified.

## Behavior

- Drop the opening and closing marker lines of every
  directive region; keep the body text between them
  verbatim (regenerated first only under `--fix`).
- `<?include?>` bodies are already expanded by
  regeneration, so keeping the body inlines the included
  content (recursively).
- Markerless directives with no body (for example
  `<?allow-empty-section?>`, `<?require?>`) are removed
  outright.
- Only lines the engine's marker-pair detection
  recognizes as real directive start/end markers are
  removed. Marker-like text the engine treats as literal
  content (for example inner same-type markers nested in
  an outer directive) is left untouched.
- After stripping, normalize blank lines so the output is
  stable and lint-clean. Front matter is kept as-is.
- Exporting an already directive-free file is a no-op;
  `export` is idempotent.

## Tasks

1. **Export core (red/green).** Add `internal/export`
   with `Export(f *lint.File, mode Mode) ([]byte,
   []lint.Diagnostic)` — mirroring plan 166's `Extract`
   signature. It operates purely on the already-parsed
   in-memory `*lint.File`, so it performs no I/O and
   returns no `error`; file reads and `-o` writes are the
   CLI layer's job (task 5) and surface as a real `error`
   there. Contract: exactly one of the two return values
   is populated. **Success** → the exported bytes (which
   are never `nil`, since a directive-free file still
   yields its own content) and a `nil` diagnostic slice.
   **Refusal** (stale body in `Check` mode, or any
   document-level problem) → `nil` bytes and a non-empty
   diagnostic slice; the caller exits non-zero. `Mode` is
   the staleness mode from task 4. Unit-test marker
   removal, body retention, include-body inlining, and
   the no-directive no-op.
2. **Nested / literal-content markers.** Drive removal
   off the engine's own marker-pair detection —
   `gensection.FindMarkerPairs` in
   [internal/archetype/gensection](../internal/archetype/gensection/parse.go),
   whose `MarkerPair.StartLine`/`EndLine` give the exact
   start- and end-marker line for every directive (not
   just the include/catalog *body* ranges that
   `lint.File.GeneratedRanges` records for diagnostic
   suppression). Only lines the engine recognizes as real
   markers are removed, so inner same-type markers that
   the engine treats as literal content survive. Add a
   test.
3. **Whitespace normalization.** Collapse the blank
   lines left by removed markers so output is stable and
   passes `mdsmith check`. Test idempotence: export of
   export equals export.
4. **Staleness check and modes.** Add a checker that
   compares each directive's on-disk body to what the
   engine would generate, reusing the `mdsmith fix`
   directive engine. `Mode` is `Check` (default), `Fix`,
   or `NoCheck`. In `Check`, each stale body appends one
   `lint.Diagnostic` (naming the directive, positioned at
   its start marker) and `Export` returns `nil` bytes.
   `Fix` regenerates stale bodies in memory before
   stripping. `NoCheck` skips the comparison. The CLI
   maps `--fix`/`--no-check` to the mode and rejects the
   combination. Unit-test all three modes on a stale
   fixture.
5. **`export` subcommand.** Register `export` in
   [main.go](../cmd/mdsmith/main.go); `mdsmith export
   <file>` writes to stdout, `-o/--output <path>` writes
   a file, `--fix` and `--no-check` select the staleness
   mode (rejecting the combination). Never mutate the
   source. Reuse the config and file-load helpers that
   back `fix` in [main.go](../cmd/mdsmith/main.go). Exit
   non-zero with a clear message on parse errors and on a
   stale body in the default mode.
6. **Fixtures and integration test.** Add `testdata`
   inputs covering include, catalog, toc, and build
   directives with golden directive-free outputs. Add a
   stale-body fixture: assert default mode exits non-zero
   with no output, `--fix` produces the fresh golden, and
   `--no-check` exports the stale bytes as-is. Assert
   idempotence and that fresh output passes `mdsmith
   check`.
7. **Docs.** Add `docs/reference/cli/export.md` (covering
   the default check, `--fix`, and `--no-check`) and link
   it from the CLI reference catalog. Run `mdsmith fix`
   so catalogs and PLAN.md regenerate.

## Acceptance Criteria

- [ ] `mdsmith export <file>` removes every line the
      engine recognizes as a real directive start/end
      marker, keeps generated bodies, and inlines
      `<?include?>` content. Marker-like text treated as
      literal content is left in place.
- [ ] The source file is never modified in any mode.
- [ ] Default mode: a stale directive body makes
      `export` exit non-zero with a diagnostic naming the
      directive and writes no output.
- [ ] `--fix` regenerates stale bodies in memory before
      stripping; `--no-check` exports on-disk bytes as-is;
      passing both is a usage error.
- [ ] Nested same-type literal-content markers are
      preserved.
- [ ] Output is idempotent and (when fresh) passes
      `mdsmith check`.
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
- **Check by default, never auto-fix.** A stale body
  fails the export rather than being silently
  regenerated, so the output faithfully reflects the
  file. `--fix` opts into regeneration; `--no-check`
  opts out of the check.
- **Front matter retained.** It is not a directive;
  stripping it is out of scope.
- **Single file first.** Directory or glob batch export
  is a possible follow-up, not in this plan.
