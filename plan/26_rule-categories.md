# Rule Categories

## Goal

Add category metadata to rules so users can enable or disable
entire groups of rules in configuration, and improve
documentation by grouping related rules together.

## Tasks

### A. Category metadata

1. Define the category vocabulary. Each rule belongs to
   exactly one category:

   | Category | Rules |
   |----------|-------|
   | `heading` | TM002, TM003, TM004, TM005, TM013, TM017, TM018 |
   | `whitespace` | TM006, TM007, TM008, TM009 |
   | `code` | TM010, TM011, TM015 |
   | `list` | TM014, TM016 |
   | `line` | TM001 |
   | `link` | TM012 |
   | `meta` | TM019, TM020 |
   | `table` | TM021 |

   New rules added later inherit their category from their
   domain. The vocabulary is extensible.

2. Add `category` field to rule README frontmatter:

   ```yaml
   ---
   id: TM001
   name: line-length
   description: Line exceeds maximum length.
   category: line
   ---
   ```

3. Add `Category() string` method to the `rule.Rule`
   interface in `internal/rule/rule.go`.

4. Update all rule implementations to return their
   category from the new method.

### B. Config support

5. Add `categories` key to the config schema:

   ```yaml
   categories:
     heading: true
     whitespace: true
     code: true
     list: true
     line: true
     link: true
     meta: true
     table: true
   ```

   Setting a category to `false` disables all rules in
   that category. Default: all categories enabled.

6. Update `config.Config` struct to include
   `Categories map[string]bool`.

7. Update `config.Load` and `config.Merge` to parse and
   merge the `categories` map.

8. Update `config.Effective` to apply category settings.
   Resolution order: category enable/disable is applied
   first, then per-rule settings override. A per-rule
   `true` re-enables a rule even if its category is
   disabled.

### C. Config override support

9. Add `categories` to the `Override` struct so file
   pattern overrides can also set categories:

   ```yaml
   overrides:
     - files: ["README.md"]
       categories:
         heading: false
   ```

### D. Documentation

10. Add the `category` bullet to rule README metadata:

    ```markdown
    - **Category**: heading
    ```

11. Update the project README rule catalog to show the
    category column.

### E. Tests

12. Unit tests for config: load categories, merge with
    defaults, effective rules with category disabled.

13. Unit test: category disabled, per-rule enabled
    overrides category.

14. Unit test: override applies category settings to
    specific file patterns.

15. Integration test: disable a category, verify all
    rules in that category produce no diagnostics.

16. Verify every rule's `Category()` return value matches
    the `category` field in its README frontmatter.

## Acceptance Criteria

- [ ] All rules implement `Category() string`
- [ ] Rule READMEs include `category` in frontmatter
- [ ] Config supports `categories:` section
- [ ] `categories: { heading: false }` disables all
      heading rules
- [ ] Per-rule `true` overrides a disabled category
- [ ] Overrides support per-file-pattern categories
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
