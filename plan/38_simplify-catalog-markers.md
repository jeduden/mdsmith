---
id: 38
title: Simplify catalog marker syntax
status: ✅
---
# Simplify catalog marker syntax

## Goal

Replace the verbose
`<!-- tidymark:gen:start catalog ... -->` /
`<!-- tidymark:gen:end -->` markers with shorter,
catalog-specific markers. No backward compatibility.

## Tasks

### A. Design new markers

1. New marker format:

   ```text
   <!-- catalog
   glob: "*.md"
   -->
   ...generated content...
   <!-- /catalog -->
   ```

   The opening marker is `<!-- catalog` (rule name as
   the HTML comment keyword). The closing marker is
   `<!-- /catalog -->` (slash prefix, mirroring HTML
   closing tags).

### B. Update parser

2. Replace marker constants in `parse.go`:

  - `startPrefix` → `"<!-- catalog"`
  - `endMarker` → `"<!-- /catalog -->"`
  - Remove old `<!-- tidymark:gen:start` format
  - Remove directive name extraction (the marker *is*
     the directive)

3. Extract the marker format so future generated-section
   archetype rules can define their own marker names
   (prepare for Plan 39).

### C. Update fixer and tests

4. Update all test fixtures and test strings to use
   new marker format.

5. Update `rules/TM019-catalog/` fixture files
   (bad/, good/) to use new markers.

6. Update any markdown files in the repo that use
   the old markers (e.g., README.md examples).

7. Run `go test ./...` and
    `go tool golangci-lint run`.

## Acceptance Criteria

- [ ] `<!-- catalog ... -->` / `<!-- /catalog -->`
      are the only recognized markers
- [ ] Old format is removed entirely
- [ ] All test fixtures updated
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
