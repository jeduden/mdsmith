---
id: 37
title: Rename TM019 generated-section to catalog
status: ✅
---
# Rename TM019 generated-section to catalog

## Goal

Rename TM019 from `generated-section` to `catalog` across
code, config, docs, and tests to reflect that the rule is
the catalog rule, not a generic framework.

## Tasks

### A. Code rename

1. Rename package directory
   `internal/rules/generatedsection/` to
   `internal/rules/catalog/`.

2. Update rule metadata in `rule.go`:

  - `Name()` returns `"catalog"` (was `"generated-section"`)
  - `ID()` stays `"TM019"`
  - `Category()` stays `"meta"`

3. Update blank import in `cmd/tidymark/main.go`:
   `_ ".../internal/rules/catalog"`.

4. Update blank import in
   `internal/integration/rules_test.go`.

### B. Docs rename

5. Rename `rules/TM019-generated-section/` to
   `rules/TM019-catalog/`.

6. Update `.tidymark.yml` config key from
   `generated-section` to `catalog`.

7. Update any cross-references in other rule READMEs
   or plan files.

### C. Tests

8. Update all test references to the old rule name.

9. Run `go test ./...` and
    `go tool golangci-lint run`.

## Acceptance Criteria

- [ ] Package is `internal/rules/catalog/`
- [ ] `Name()` returns `"catalog"`
- [ ] Config key is `catalog`
- [ ] All rule README paths updated
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
