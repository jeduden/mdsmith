---
id: 2610020305
title: "Homepage Tier 3 declutter: pillar numbers, tints, badges, eyebrows"
status: "🔲"
summary: >-
  Plan 2608301343 deferred its optional Tier 3 homepage cleanup
  pending a design confirmation. This plan tracks that work:
  drop the `.pillar-num` counters, collapse the six icon-tile
  tints to one hue, trim the hero badge row, and drop the two
  homepage section eyebrows, once the design call is confirmed.
model: sonnet
depends-on: [2608301343]
---
# Homepage Tier 3 declutter: pillar numbers, tints, badges, eyebrows

## Goal

Finish the optional Tier 3 cleanup from plan 2608301343. That
plan is done, so this one keeps the deferred items in view.

## Background

Plan 2608301343 removed coined copy, the rule-ID chips, and dead
CSS. Its Tier 3 step was marked "optional, confirm first" and
deferred because it changes the visual design, not just copy.
The four items it named are still in the shipped templates:

- `.pillar-num` counters in
  [feature-grid.html](../website/layouts/partials/feature-grid.html).
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
