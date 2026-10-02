---
id: 2610020233
title: "Drop the unused rules? key from the feature kind schema"
status: "🔲"
summary: >-
  The homepage feature grid no longer renders rule-ID chips, and
  no feature page sets `rules:` any more, so the optional
  `"rules?"` front-matter key on the `feature` kind in
  `.mdsmith.yml` has no consumer. Remove it, with explicit user
  consent because it edits linter config.
model: haiku
depends-on: [2608301343]
---
# Drop the unused rules? key from the feature kind schema

## Goal

Remove the dead `"rules?": '[...string]'` front-matter key from
the `feature` kind in [`.mdsmith.yml`](../.mdsmith.yml), so the
schema no longer advertises a field that nothing reads.

## Background

Plan 2608301343 removed the `.card-rules` chip row from the
[feature grid](../website/layouts/partials/feature-grid.html),
which was the only reader of `Params.rules`. It then dropped the
`rules:` lines from the five feature pages under `docs/features/`.
The schema key in [`.mdsmith.yml`](../.mdsmith.yml) is still there,
because that file needs explicit user consent to edit.

## Tasks

1. Get explicit user consent to edit
   [`.mdsmith.yml`](../.mdsmith.yml).
2. Delete the `"rules?": '[...string]'` line from the
   `feature` kind's `schema.frontmatter` block.
3. Confirm no file under `docs/features/` sets `rules:` in its
   front matter, then run `mdsmith check .`. A plain grep for
   `^rules:` is not enough: the YAML examples in
   `size-and-readability.md` start lines with `rules:` too.

## Acceptance Criteria

- [ ] The `feature` kind schema has no `rules?` key.
- [ ] `mdsmith list query 'rules: _' docs/features/` prints
      nothing (front matter only, so code blocks do not match).
- [ ] `mdsmith check .` passes.
