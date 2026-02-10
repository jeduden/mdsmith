# Test Case Folders with Settings Metadata

## Goal

Restructure rule test fixtures from single files (`bad.md`,
`good.md`) into folders (`bad/`, `good/`, `fixed/`) containing
multiple test cases, each specifying via frontmatter the rule
settings under which it should be checked.

## Tasks

### A. Fixture format

1. Define the frontmatter schema for test fixtures:

   ```yaml
   ---
   settings:
     max: 120
     exclude: []
   diagnostics:
     - line: 3
       column: 81
       message: "line too long (121 > 120)"
   ---
   ```

  - `settings` is optional; omit for the rule's
     default settings
  - `diagnostics` is required for `bad/` fixtures,
     absent for `good/` fixtures
  - `fixed/` fixtures have no `diagnostics` and their
     body is the expected output after fix

2. Define the folder layout:

   ```text
   rules/TM001-line-length/
     README.md
     bad/
       default.md
       no-exclusions.md
       max-120.md
     good/
       default.md
       long-url.md
     fixed/
       default.md
   ```

  - File names are descriptive, lowercase with hyphens
  - `default.md` uses the rule's default settings
  - `fixed/` files correspond by name to `bad/` files
     (e.g. `fixed/default.md` is the fix of
     `bad/default.md`)

### B. Update integration test framework

3. Update the fixture discovery in
   `internal/integration/rules_test.go` to scan for
   `bad/`, `good/`, and `fixed/` subdirectories in
   each rule folder. Each `.md` file in those folders
   becomes a test case.

4. Parse `settings` from fixture frontmatter and call
   `rule.ApplySettings` before running `Check`. This
   lets the same rule be tested under different
   configurations.

5. For `fixed/` fixtures: load the matching `bad/` file,
   apply settings, run `Fix`, and compare the output
   (with frontmatter stripped from both) against the
   `fixed/` file body.

6. Support both the old single-file format and the new
   folder format during migration. When `bad.md` exists
   and `bad/` does not, use the old format. When `bad/`
   exists, use the folder format. When both exist, prefer
   the folder.

### C. Migrate fixtures

7. Migrate TM001 (line-length) as the first rule. Create:

  - `bad/default.md` -- current bad.md
  - `bad/no-exclusions.md` -- `settings: {exclude: []}`
  - `bad/max-120.md` -- `settings: {max: 120}`
  - `good/default.md` -- current good.md
  - `good/long-url.md` -- line with URL excluded

8. Migrate all remaining rules. At minimum move:

  - `bad.md` to `bad/default.md`
  - `good.md` to `good/default.md`
  - `fixed.md` to `fixed/default.md`

9. Add additional settings-specific cases for rules with
   multiple configuration options: TM002 (heading-style),
   TM010 (fenced-code-style), TM016 (list-indent).

### D. Update ignore list

10. Update `.tidymark.yml` ignore patterns to match the
    new paths:

    ```yaml
    ignore:
      - "rules/*/bad/**"
      - "rules/*/bad.md"
    ```

### E. Tests

11. Verify the integration test framework discovers and
    runs fixtures from both folder and single-file
    formats correctly.

12. Ensure all existing tests pass after migration with
    no changes to rule implementations.

13. Add at least three settings-specific bad cases for
    TM001 to demonstrate the per-settings testing
    capability.

## Acceptance Criteria

- [ ] Each rule folder supports `bad/`, `good/`, `fixed/`
      subdirectories with multiple fixture files
- [ ] Fixture frontmatter specifies `settings` and
      `diagnostics` per test case
- [ ] Integration tests auto-discover folder-based
      fixtures and apply settings before checking
- [ ] Fixed-file comparison works with settings-aware
      fixtures
- [ ] All existing single-file fixtures migrated to
      folder format
- [ ] TM001, TM002, TM010, TM016 have multiple
      settings-specific test cases
- [ ] Backward-compatible with single-file format
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
