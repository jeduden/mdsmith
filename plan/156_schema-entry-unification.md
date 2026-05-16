---
id: 156
title: >-
  Section schema — unify entry shape under
  `heading:` discriminator
status: "✅"
model: opus
depends-on: [146]
summary: >-
  Collapse the section-entry vocabulary to one
  discriminator (`heading:` — null, string, or
  mapping). Match with `regex:`, a CUE
  expression evaluated as a raw-interpolation
  string. Two helpers in scope: `digits` (named
  numeric capture) and `fmvar(name)`
  (frontmatter variable, regex-escaped).
  Quantify with `repeat: {min, max}`.
  `sequential:` survives as a sibling. Drop
  `aliases:`, `required:`, the
  `{unlisted: true}` mapping, scope-level
  `repeats:`/`min:`/`max:`, and
  `require.filename:` (which flattens to
  top-level `filename:`). Hard cutover; rewrite
  in-repo callers in the same PR.
---
# Section schema — unify entry shape under `heading:` discriminator

## Goal

Make a section schema read as a quantified
regex over the document's heading sequence.
One discriminator. One matcher. One
cardinality field. The old shape had six
sibling fields plus three forms of `heading:`.

## Background

Plan 146 shipped the engine. The vocabulary
accreted around it: `required:`, `aliases:`,
`{unlisted: true}`, `repeats:`, `sequential:`,
scope-level `min:`/`max:`. Each addressed a
real need but read as ad-hoc knobs.

In-repo usage is small. Five inline kinds in
`.mdsmith.yml` lines 303–371 use `heading:
null` plus `heading: {unlisted: true}` plus
`required:`. One fixture uses `aliases:`. The
repeat-pattern keys are unused outside
research files. Narrowing now beats narrowing
later.

## Non-Goals

- Migration tool. Hand-rewrite in this PR.
- Backwards-compat shim. Hard cutover.
- `content:` redesign (plan 149).
- Per-scope rule-override stacking.
- Renaming the `<?require filename: ?>`
  directive in proto.md bodies.

## Design

Full specification:
[Section schema reference](../docs/reference/section-schema.md).
The page is committed alongside this plan with
an **upcoming** notice. The notice comes off
when implementation lands.

Three axes describe each entry:

- **Discriminator** — `heading:` value
  (`null`, string, or mapping).
- **Matcher** — `regex:` inside the mapping
  form. The YAML value is the body of a CUE
  raw-interpolation string (`#"..."#`).
  Backslashes pass through to RE2.
  Interpolation uses `\#(expr)`. Two helpers
  in scope: `digits` (named numeric capture
  `(?P<n>[0-9]+)`) and `fmvar(name)`
  (frontmatter lookup + regex-escape).
- **Cardinality** — `repeat: { min, max }`
  inside the mapping form.

Plus three migration-only changes:

- `require.filename:` flattens to top-level
  `filename:`.
- Frontmatter-only kinds reject schema-level
  `closed:`.
- Every removed key
  (`aliases:` / `required:` / `unlisted:` /
  scope-level `repeats:` / `sequential:` /
  `min:` / `max:` / `require:`) parse-errors
  with a "removed; see plan 156" diagnostic
  naming the replacement.

Diagnostics follow the existing
`parse_inline.go` style: path prefix,
lowercase message, no trailing punctuation.
Final wording refined in code review.

### Migration surface

Files rewritten as part of implementation:

- `.mdsmith.yml` — 5 kinds, lines 303–371.
- The runbook fixture under
  `internal/rules/MDS020-required-structure/good/`
  (uses `aliases:`).
- `docs/guides/schemas.md` — rewrite to
  describe only the new shape.
- The MDS020 README — same.
- `docs/reference/section-schema.md` —
  remove the "upcoming" notice.
- A "superseded" note on plan 146's
  entry-shape section.

## Tasks

1. [x] Update `Scope` in `internal/schema/schema.go`.
   Drop `Aliases`, `Required`, `Repeats`,
   `Sequential` (top-level), `Min`/`Max`,
   `Wildcard`. Add a `Matcher` sub-struct
   (`Regex`, `Repeat`, `Sequential`). Keep
   `Preamble` as a derived flag.
2. [x] Rewrite the heading-mapping parser in
   `internal/schema/parse_inline.go` to
   accept `{ regex, repeat?, sequential? }`.
   Reject every removed key by name with a
   "removed; see plan 156" diagnostic naming
   the replacement.
3. [x] In the same parser, flatten
   `require.filename:` to top-level
   `filename:`. Reject schema-level `closed:`
   on kinds without `sections:`.
4. [x] Update `internal/schema/parse_file.go` so
   `## ?` and `## ...` map to the new
   `Matcher` (`regex: '.+'`, plus
   `repeat: { min: 0 }` for `...`). Keep
   `{n}` and `{field}` token expansion in
   heading rows. (Scope note: MDS020's file-
   schema check still routes through its
   legacy `parseSchema`/`parsedSchema` pipeline
   so the `{field}` heading/body sync feature
   survives; this parser is exercised by the
   schema-package tests and prepares the
   ground for the cutover in a follow-up plan.)
5. [x] Rewrite the validator in
   `internal/schema/validate.go` to match
   the heading sequence as a positional
   quantified regex. Each `Matcher` consumes
   between `repeat.min` and `repeat.max`
   consecutive headings whose text matches
   `regex:`. Diagnostics point at the
   entry's source location.
6. [x] Update fixtures and tests under
   `internal/schema/` and
   `internal/rules/MDS020-required-structure/`.
   Add one fixture per new parse-time
   diagnostic.
7. [x] Rewrite the 5 inline kinds in
   `.mdsmith.yml` and the affected fixture
   to the new shape. Run `mdsmith check .`
   and `mdsmith fix .`.
8. [x] Rewrite `docs/guides/schemas.md` and the
   MDS020 README to describe only the new
   shape. Every reference to `aliases:`,
   `required:`, `unlisted:`, scope-level
   `repeats:`/`sequential:`/`min:`/`max:`, and
   `require:` is removed; one worked example
   of the new shape replaces the old one.
9. [x] Remove the "upcoming" notice from
   `docs/reference/section-schema.md`.
   Run `mdsmith fix CLAUDE.md` so the catalog
   line for the page survives any title /
   summary edits.
10. [x] Add a "superseded" note to plan 146's
    entry-shape section pointing at this plan
    and the reference page.

## Acceptance Criteria

- [x] A section entry uses exactly one of
      three shapes: `heading: null`,
      `heading: <string>`, or `heading: {
      regex, repeat?, sequential? }`.
- [x] `repeat:` omitted → exactly one;
      `repeat: { min: 0 }` → zero or more;
      `repeat: { min: 0, max: 1 }` →
      optional; bounded forms enforce the
      bounds.
- [x] Regex matching is whole-string
      anchored against rendered plain text
      (fixture: `## **Overview**` matches
      `regex: 'Overview'`).
- [x] `{n}` expands to a numeric capture;
      `sequential: true` flags out-of-order
      or gapped numbers.
- [x] `{field}` interpolates the document's
      frontmatter value at validate-time.
- [x] `heading: null` outside index 0
      parse-errors.
- [x] `heading:` mapping without `regex:`
      parse-errors.
- [x] `repeat: {}`, `repeat: { max: 0 }`,
      `min > max` each parse-error.
- [x] `sequential: true` without `{n}`
      parse-errors.
- [x] Invalid regex parse-errors with the
      RE2 message and field path.
- [x] Removed keys each parse-error with a
      "removed; see plan 156" message
      naming the replacement.
- [x] The 5 rewritten inline kinds and the
      rewritten fixture emit the same MDS020
      diagnostics they did before the
      cutover (where the constraint is
      preserved).
- [x] `docs/guides/schemas.md` and the
      MDS020 README describe only the new
      shape — `grep -E 'aliases:|required:|unlisted:|repeats:|^require:'`
      against either file returns no matches
      in prose.
- [x] `docs/reference/section-schema.md`
      reads as a current spec — its
      "upcoming" notice is removed and the
      page links from `docs/guides/schemas.md`.
- [x] `mdsmith check .` reports no
      diagnostics.
- [x] All tests pass: `go test ./...`.
- [x] `go tool golangci-lint run` reports
      no issues.
