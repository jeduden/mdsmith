# Line-Length Feature Parity

## Goal

Extend TM001 (line-length) with per-category maximum lengths
and stern mode, matching the feature set of markdownlint MD013
while keeping tidymark's own configuration syntax.

## Tasks

### A. Per-category maximum lengths

1. Add new settings to TM001:

   | Setting | Type | Default | Description |
   |---------|------|---------|-------------|
   | `heading-max` | int | -- | Max length for heading lines |
   | `code-block-max` | int | -- | Max for code block lines |

   When unset (nil/absent), the per-category max inherits
   from `max`. When set to an explicit value, that value
   overrides `max` for lines of that type.

2. Update the `Rule` struct in
   `internal/rules/linelength/rule.go` to hold the new
   fields. Use pointer-to-int (`*int`) so unset is
   distinguishable from zero.

3. Update `ApplySettings` to parse `heading-max` and
   `code-block-max`. Validate that values are positive
   integers when present.

4. Update `DefaultSettings` to include the new keys with
   nil defaults (inheriting from `max`).

5. Update `Check`: when evaluating a heading line, use
   `heading-max` if set, otherwise `max`. When evaluating
   a line inside a fenced code block, use
   `code-block-max` if set, otherwise `max`.

### B. Stern mode

6. Add a `stern` setting (bool, default `false`):

  - Normal mode (current, `stern: false`): the `exclude`
     list determines which line categories to skip
  - Stern mode (`stern: true`): lines exceeding the
     limit are flagged only if they contain a space
     character at or beyond the limit column. Lines with
     no space past the limit (e.g. a long URL at the
     end of a line) are allowed.

   Stern mode applies independently of `exclude`. A line
   inside a code block that is excluded via
   `exclude: [code-blocks]` is still skipped regardless
   of stern.

7. Update `Check` to implement stern logic. After
   determining a line exceeds the active max, check
   whether any byte at or beyond the limit column is a
   space. If `stern` is true and no space exists past
   the limit, skip the diagnostic.

8. Update `ApplySettings` and `DefaultSettings` for the
   `stern` key.

### C. Documentation

9. Update `rules/TM001-line-length/README.md`:

  - Add `heading-max`, `code-block-max`, and `stern`
     to the Settings table
  - Add config examples showing per-category limits
  - Add a section explaining stern mode behaviour
  - Add Bad/Good examples for each new setting

10. Add test fixtures for new settings (following Plan 27
    format if available, otherwise single files):

  - `bad/heading-over-limit.md` with
      `settings: {heading-max: 60}`
  - `good/heading-within-limit.md`
  - `bad/stern-spaces-past-limit.md` with
      `settings: {stern: true}`
  - `good/stern-no-spaces-past-limit.md`
  - `good/code-block-within-limit.md` with
      `settings: {code-block-max: 120}`

### D. Tests

11. Unit tests for `heading-max`:

  - Heading within `heading-max` but over `max`: pass
  - Heading over `heading-max`: fail
  - `heading-max` unset: inherits from `max`
  - `heading-max` with `exclude` interaction

12. Unit tests for `code-block-max`:

  - Code line within `code-block-max` but over `max`:
      pass
  - Code line over `code-block-max`: fail
  - `code-block-max` unset: inherits from `max`
  - Code blocks still skippable via
      `exclude: [code-blocks]`

13. Unit tests for `stern` mode:

  - Long line with spaces past limit: flagged
  - Long line with no spaces past limit: allowed
  - Stern + exclude interaction: excluded lines stay
      excluded
  - Stern with per-category max: stern applies to
      the active max for that line type

14. Integration tests with fixtures.

## Acceptance Criteria

- [ ] `heading-max` overrides `max` for heading lines
- [ ] `code-block-max` overrides `max` for code block
      lines
- [ ] Unset per-category max inherits from `max`
- [ ] `stern: true` flags lines with spaces past the
      limit
- [ ] `stern: true` allows lines without spaces past
      the limit
- [ ] Stern mode and `exclude` settings compose correctly
- [ ] Rule documentation updated with new settings and
      examples
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
