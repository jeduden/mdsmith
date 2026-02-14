---
id: 40
title: Normalize rule READMEs to proto template
status: ✅
---
# Normalize rule READMEs to proto template

## Goal

Ensure all 18 rule READMEs (MDS001--MDS018) follow the
structure and content conventions defined in
`rules/proto.md`.

## Current state

All rules already have front matter, metadata bullets,
Config, and Examples sections. Deviations are minor:

- MDS017 has a "Details" section (non-standard name)
- Some rules may be missing Diagnostics or Edge Cases
  tables where they would be useful

## Tasks

1. For each rule README (MDS001--MDS018), verify it matches
   `rules/proto.md` structure:

  - Front matter: `id`, `name`, `description`
  - Title: `# MDSXXX: rule-name`
  - Description paragraph (verbatim repeat)
  - Metadata bullets in order: ID, Name, Default, Fixable,
     Implementation, Category
  - Settings section (only if Configurable)
  - Config section (enable + disable + custom)
  - Examples section (Good/Bad subsections)
  - Diagnostics table (if multiple messages)
  - Edge Cases table (if complex)

2. Fix any deviations found:

  - MDS017: rename "Details" to appropriate section name
     or fold content into description
  - Add missing Diagnostics tables where rules emit
     multiple messages
  - Ensure Config sections show both enable and disable

3. Run `mdsmith check rules/` to verify all pass.

## Acceptance Criteria

- [ ] All 18 rule READMEs follow `rules/proto.md` structure
- [ ] No non-standard section names
- [ ] Config sections show enable and disable examples
- [ ] All docs pass `mdsmith check rules/`
