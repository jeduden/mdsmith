---
id: 2610020233
title: "Drop the unused rules? and link? keys from the feature kind schema"
status: "🔲"
summary: >-
  The homepage feature grid no longer renders rule-ID chips, and
  no feature page sets `rules:` any more, so the optional
  `"rules?"` front-matter key on the `feature` kind in
  `.mdsmith.yml` has no consumer. The `"link?"` key is just as
  dead: no template, catalog row, or Go code reads it, and the
  card links to the page's own permalink. Remove both keys, with
  explicit user consent because it edits linter config.
model: haiku
depends-on: [2608301343]
---
# Drop the unused rules? and link? keys from the feature kind schema

## Goal

Remove two dead front-matter keys from the `feature` kind in
[`.mdsmith.yml`](../.mdsmith.yml). The keys are
`"rules?": '[...string]'` and `"link?": string`. Nothing reads
them.

## Background

Plan 2608301343 removed the `.card-rules` chip row from the
[feature grid](../website/layouts/partials/feature-grid.html),
which was the only reader of `Params.rules`. It then dropped the
`rules:` lines from the five feature pages under `docs/features/`.
The same plan's round-3 review found that `link:` was dead too:
the card href is `$page.RelPermalink`, and nothing else reads
`Params.link`. It dropped the `link:` lines from all 18 feature
pages. Both schema keys in [`.mdsmith.yml`](../.mdsmith.yml) are
still there, because that file needs explicit user consent to
edit.

## Tasks

1. Get explicit user consent to edit
   [`.mdsmith.yml`](../.mdsmith.yml).
2. Delete the `"rules?": '[...string]'` and `"link?": string`
   lines from the `feature` kind's `schema.frontmatter` block.
3. Confirm no file under `docs/features/` sets `rules:` or
   `link:` in its front matter, then run `mdsmith check .`. A plain grep for
   `^rules:` is not enough: the YAML examples in
   `size-and-readability.md` start lines with `rules:` too.

## Acceptance Criteria

- [ ] The `feature` kind schema has no `rules?` or `link?`
      key.
- [ ] `mdsmith list query 'rules: _' docs/features/` and
      `mdsmith list query 'link: _' docs/features/` print
      nothing (front matter only, so code blocks do not match).
- [ ] `mdsmith check .` passes.
