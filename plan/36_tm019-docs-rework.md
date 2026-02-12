---
id: 36
title: TM019 Documentation Rework
status: ✅
---
# TM019 Documentation Rework

## Goal

Reorganize rule documentation with archetype-based
structure: archetypes define shared mechanics, rules
document concrete specifics. Apply to TM019 (catalog)
and establish the pattern for all rules.

## Tasks

1. Create `archetypes/` directory with
   `archetypes/generated-section/README.md` documenting
   the shared marker-based content generation mechanism:

  - Marker syntax (`<!-- tidymark:gen:start NAME -->`,
     `<!-- tidymark:gen:end -->`)
  - YAML body parsing rules
  - Template rendering pipeline (header, row, footer,
     empty)
  - Fix behavior (replace between markers, idempotent)
  - Common diagnostics (unclosed, nested, invalid YAML,
     non-string values, orphaned end)
  - Edge cases shared across all generated-section rules

2. Rewrite `rules/TM019-catalog/README.md` (renamed from
   TM019-generated-section) referencing the archetype:

  - Rule metadata (ID, name, default, fixable, category)
  - Link to `archetypes/generated-section/` for marker
     mechanics
  - Catalog-specific parameters (glob, sort, columns)
  - Template fields (filename, front matter)
  - Minimal mode vs template mode
  - Progressive examples (quick start through tables
     with column constraints)
  - Catalog-specific diagnostics only

3. Create `archetypes/README.md` index listing all
   archetypes with one-line descriptions.

4. Update `rules/TM019-catalog/README.md` to note
   simplified marker names (see Plan 37).

5. Run `tidymark check archetypes/ rules/TM019-catalog/`
   to verify all files pass linting.

## Acceptance Criteria

- [ ] `archetypes/generated-section/README.md` is a
      self-contained spec for the marker mechanism
- [ ] `rules/TM019-catalog/README.md` references the
      archetype and documents only catalog specifics
- [ ] No content lost from the original README
- [ ] All docs pass `tidymark check`
- [ ] Cross-references between files are correct

## Dependencies

- Depends on Plan 37 (rename) being complete first for
  the correct directory name
- Depends on Plan 38 (marker simplification) for updated
  marker syntax examples
