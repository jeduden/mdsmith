---
id: 2608301343
title: "Reduce mdsmith.dev homepage clutter and coined words"
status: "🔲"
summary: >-
  Replace coined forge-metaphor copy on mdsmith.dev with plain
  words the target audience knows on first read — the hero
  "smithed", the "Forged whole" section title, and its "forges
  the whole tree" lead — then thin the homepage's decorative
  chrome: opaque MDS### rule chips, pillar numbers, icon-tile
  hue variety, and the four-badge hero row.
model: sonnet
---
# Reduce mdsmith.dev homepage clutter and coined words

## Goal

Make [mdsmith.dev](https://mdsmith.dev/) speak in words the
target audience — people who write and maintain Markdown —
understands on first read, and quiet the homepage so the product
claims stand out instead of the decoration. The trigger is the
hero word "smithed": a coined verb no reader knows out of the
box.

## Scope

This plan changes homepage copy and the homepage feature grid
only. Interior docs pages already read cleanly and stay as they
are. No rule config changes: [`.mdsmith.yml`](../.mdsmith.yml) is
out of scope.

## Snapshot

A full-page capture of the live homepage (light theme; the site
ships one theme) is attached to the pull request, alongside the
features, guides, reference, and a docs-single page. The findings
below cite what is visible in that capture.

## Findings

Three tiers, most to least important. Tier 1 is the explicit ask;
Tiers 2 and 3 are the rest of the clutter that competes for a
reader's attention.

### Tier 1 — coined words the audience must decode

These are the [design-system](../docs/development/design-system.md)
"forge meets terminal" metaphor made visible. Each is a real
word swapped for a coined or archaic one; the swap keeps the
meaning and drops the puzzle.

- Hero headline "Mark*down*, smithed." "smithed" is not a word a
  reader knows. The canonical source is the `## Headline` section
  of [`docs/brand/messaging.md`](../docs/brand/messaging.md); it
  renders through [`website/content/_index.md`](../website/content/_index.md)
  (`hero.headline_pre` / `_em` / `_post`) and
  [`website/layouts/partials/hero.html`](../website/layouts/partials/hero.html).
  Recommended: **Neat, consistent *Markdown*.** — it mirrors the
  lead's own promise and keeps exactly one single-asterisk span,
  which `sync-messaging --check` requires. Alternates: "Markdown,
  kept *tidy*." or "Clean, consistent *Markdown*." (the second
  reuses pillar 1's name, so prefer the first).
- Section title "Forged *whole.*" in
  [`website/layouts/index.html`](../website/layouts/index.html).
  Recommended: **Checks the *whole* tree.** — it says the actual
  claim (whole tree, not one file) in plain words.
- Section lead "…mdsmith forges the whole tree:" in the same
  file. Change "forges" to "checks".

### Tier 2 — opaque codes on a marketing page

- Rule-ID chips (13 of them: MDS034, MDS022, MDS023 …) sit on the
  homepage feature cards, emitted by the
  [feature grid](../website/layouts/partials/feature-grid.html)
  (`.card-rules` / `.rule-chip`). A first-time visitor cannot
  decode "MDS034". Recommended: drop the chip row from the
  homepage cards. The codes stay on each feature's own page and
  the Rules index, where they are defined. This contradicts the
  current [design-system](../docs/development/design-system.md)
  card rule ("use a rule-ID chip instead" of an accent stripe),
  so update that line too.

### Tier 3 — decorative density (optional)

- Pillar numbers "01"–"05" (`.pillar-num` in the feature grid)
  are decoration; the pillar name and hairline already separate
  the groups. Drop the number span.
- Icon-tile hue variety: leads carry a forge-gradient tile and
  the rest cycle six tints (`ember`, `sky`, `moss`, `amber`,
  `clay`, `slate`) across ~19 cards. Collapse to one calm tint so
  the grid reads as one system.
- Hero badge row: four badges (CI, Go Report Card, Codecov, MIT)
  occupy the hero's prime slot in
  [`website/layouts/partials/hero.html`](../website/layouts/partials/hero.html).
  Keep at most the CI badge in the hero; move the rest to the
  footer's Community column.
- Section eyebrows: the mono all-caps labels "Why mdsmith" and
  "Install" restate the H2 directly under them. Optional: drop
  those two homepage eyebrows; keep the hero eyebrow, which
  carries a distinct positioning line.

## Notes on propagation

- The headline is source-of-truth in
  [`docs/brand/messaging.md`](../docs/brand/messaging.md). After
  editing `## Headline`, run `mdsmith fix` to refresh the README
  include, then `mdsmith-release sync-messaging` to update
  `_index.md`, `hugo.toml`, and the package-registry copy. CI
  runs `sync-messaging --check` and fails on drift or on a
  Headline that lacks exactly one single-asterisk span.
- The old headline string also appears as literal sample data in
  the [extract reference](../docs/reference/cli/extract.md) and
  the [extract guide](../docs/guides/extract-markdown-as-data.md).
  Those illustrate the `extract` command; leave them, or update
  them deliberately — do not let a find-and-replace rewrite them
  silently.
- Internal token and class names (`forge-grad`, the `--tint-*`
  hue names) are not visible to readers; they are out of scope
  here and can stay.

## Tasks

1. Tier 1: edit the `## Headline` section of
   [`docs/brand/messaging.md`](../docs/brand/messaging.md) to the
   chosen plain headline (default: `Neat, consistent *Markdown*.`).
   Run `mdsmith fix`, then `mdsmith-release sync-messaging`, and
   confirm `sync-messaging --check` passes.
2. Tier 1: in
   [`website/layouts/index.html`](../website/layouts/index.html)
   change the section title to "Checks the *whole* tree." and
   change "forges the whole tree" to "checks the whole tree" in
   the lead.
3. Tier 2: remove the `.card-rules` chip block from the
   [feature grid](../website/layouts/partials/feature-grid.html)
   and update the matching card rule in
   [`design-system.md`](../docs/development/design-system.md).
4. Tier 3 (optional, confirm first): drop `.pillar-num`, collapse
   the icon-tile tints to one, trim the hero badge row, and drop
   the two homepage section eyebrows.
5. Rebuild the site locally, re-capture the homepage, and check
   the before/after side by side. Run `mdsmith check .`.

## Acceptance Criteria

- [ ] No coined or metaphor word remains in visible homepage
      copy: "smithed", "Forged", and "forges" are gone.
- [ ] The hero headline reads in plain words and keeps exactly
      one single-asterisk emphasis span; `sync-messaging --check`
      passes.
- [ ] The homepage feature cards carry no MDS### chips; the codes
      still appear on the feature pages and the Rules index.
- [ ] The [design-system](../docs/development/design-system.md)
      doc matches the shipped markup (card chips, and any Tier 3
      changes taken).
- [ ] `mdsmith check .` passes and the website build is green.
