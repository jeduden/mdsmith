---
id: 2610020305
title: "Homepage Tier 3 declutter: pillar numbers, tints, badges, eyebrows"
status: "⛔"
summary: >-
  Plan 2608301343 deferred its optional Tier 3 homepage cleanup
  pending a design confirmation. This plan tracks that work:
  drop the `.pillar-num` counters, collapse the six icon-tile
  tints to one hue, trim the hero badge row, and drop the two
  homepage section eyebrows, once the design call is confirmed.
  Superseded by the homepage redesign, which replaced the
  templates these items lived in.
model: sonnet
depends-on: [2608301343]
---
# Homepage Tier 3 declutter: pillar numbers, tints, badges, eyebrows

## Goal

Finish the optional Tier 3 cleanup from plan 2608301343. That
plan is done, so this one keeps the deferred items in view.

## Superseded

The homepage redesign replaced the templates below. This plan
is closed without its own change. Where each item ended up:

- The feature grid partial is gone; the new feature-group rows
  keep a two-digit group number as their only counter.
- The homepage no longer renders icon tiles, so the six
  `--tint-*` hues no longer cycle there.
- The hero badge row stays, styled for the dark homepage.
- The hero eyebrow is no longer printed; the "Why mdsmith" and
  "Install" section labels stay as section eyebrows.

## Background

Plan 2608301343 removed coined copy, the rule-ID chips, and dead
CSS. Its Tier 3 step was marked "optional, confirm first" and
deferred because it changes the visual design, not just copy.
The four items it named were in these templates:

- `.pillar-num` counters in `feature-grid.html`.
- The six `--tint-*` icon-tile hues cycled per card in the same
  partial.
- The `.hero-badges` row in
  [hero.html](../website/layouts/partials/hero.html).
- The two `.section-eyebrow` labels in
  [index.html](../website/layouts/index.html).

## Tasks

1. Get a design confirmation from the maintainer for each of the
   four items; record the decision here and drop any rejected
   item from scope.
2. Remove the confirmed markup and its now-unused CSS, and update
   [design-system.md](../docs/development/design-system.md) so it
   matches the shipped markup.
3. Update or remove the
   [home.spec.ts](../website/e2e/tests/home.spec.ts) icon-tile
   hue test if the tints collapse to one.
4. Rebuild the site, compare the homepage before and after, and
   run `mdsmith check .`.

## Acceptance Criteria

- [ ] Each of the four items is either removed or recorded here
      as rejected by the design review.
- [ ] No CSS rule or token is left without a template that uses
      it.
- [ ] The design-system doc matches the shipped homepage markup.
- [ ] Website e2e tests pass.
- [ ] `mdsmith check .` passes.
