# Lint Plan Documents

## Goal

Remove `plan/**` from the `.tidymark.yml` ignore list so plan documents are
linted, fix any violations in existing plan files, and document in `CLAUDE.md`
that plan files must pass linting.

## Prerequisites

None — can start immediately.

## Tasks

1. Remove `"plan/**"` from the `ignore` list in `.tidymark.yml`.

2. Run `tidymark check plan/` to identify violations in existing plan files.

3. Fix all violations in `plan/*.md` files. Common expected issues:

  - Missing first-line heading (TM004) — `plan/proto.md` starts
    with `#` but other plans should be checked
  - Fenced code blocks without language tags (TM011)
  - Line length (TM001)
  - Trailing spaces (TM006)
  - Blank lines around headings/lists/code blocks
    (TM013/TM014/TM015)

4. If any plan files contain content that structurally cannot pass a rule
   (e.g. intentional long URLs), add a targeted override in `.tidymark.yml`
   for `plan/*.md` rather than re-ignoring the whole directory.

5. Update `CLAUDE.md` in the "Plans" section to note that plan files
   must pass tidymark linting:

   ```text
   Plan files must pass `tidymark check plan/` with zero diagnostics.
   ```

6. Verify `tidymark check .` still exits 0.

## Acceptance Criteria

- [ ] `plan/**` is no longer in the ignore list
- [ ] `tidymark check plan/` exits 0 (all plan files pass)
- [ ] `CLAUDE.md` documents that plan files must pass linting
- [ ] `tidymark check .` exits 0
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
