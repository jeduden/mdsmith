---
title: Design system
summary: >-
  Color roles, type rules, spacing and shadow tiers, component
  policy, and iconography for mdsmith.dev and the other brand
  surfaces. The live tokens are `website/static/css/`.
---
# Design system

The mdsmith brand is "forge meets terminal". One warm
forge-orange accent sits over cool steel grays on a warm
paper canvas. Monospace type appears wherever diagnostics
live. The tokens live in
`website/static/css/colors_and_type.css`. The component
styles live in `website/static/css/app.css`. Fonts and
logos sit under `website/static/`. To restyle a surface,
edit the CSS; the markup stays put.

The system comes from the "mdsmith Design System" Claude
Design project. The files in this repository are the
production truth. Product copy is out of scope on this
page. The slogan, hero lead, and tagline live in
[`docs/brand/messaging.md`](../brand/messaging.md).
`mdsmith-release sync-messaging` propagates them, and CI
fails on drift.

## Voice and casing

- The brand name is always lowercase: **mdsmith**. Never
  "MDSmith" or "Markdown Smith".
- Headings use sentence case. Commands, config keys, and
  rule IDs (`MDS027`) render in mono, even in prose.
- Marketing surfaces avoid softeners ("powerful",
  "blazing") and earn claims with rule IDs, numbers, or
  command output.
- Status emoji (✅ 🔲 🔳) are data and stay as Unicode.
  Decorative section emoji become Lucide icons on the
  website.

## Color roles

| Token family                          | Role                                                                                     |
| ------------------------------------- | ---------------------------------------------------------------------------------------- |
| `--forge-*`                           | The only accent. CTAs, brand mark, hover and focus rings. Use sparingly.                 |
| `--steel-*`                           | Structural base. 900 for terminal backgrounds, 500-700 for UI text, 100-300 for borders. |
| `--canvas` (#FAF7F2)                  | The paper. Warm off-white page surface; no pure-white page backgrounds.                  |
| `--ok-500`, `--warn-500`, `--err-500` | Diagnostic semantics, matching `mdsmith check` terminal output.                          |
| `--term-*`                            | Terminal mock palette only. Never mixed with UI tokens.                                  |
| `--tint-*`                            | bg/fg/line triads for icon tiles. Cycle them; never two adjacent tiles in one hue.       |
| `--grad-*`                            | Warm-only gradients: hero glow, bento cells, dark panels, gradient icon tiles.           |

Gradients carry two extra rules. Never place one behind body
text on a light surface. Use at most one gradient surface per
viewport.

## Type

- **IBM Plex Sans** carries all UI text and headings.
- **IBM Plex Serif italic** is reserved for one display
  moment per page, in the hero only.
- **0xProto Nerd Font** is the official brand mono: code,
  diagnostics, terminal mocks, eyebrow labels, rule IDs.
- All three are self-hosted woff2 files under
  `website/static/fonts/`; no runtime font CDN.
- Modular scale, ratio 1.20:
  12 / 13 / 15 / 17 / 20 / 24 / 30 / 38 / 48 / 60 / 76.
  Body is 15px.
- Tracking: `-0.02em` on headings at 30px and larger,
  `0.08em` on all-caps mono eyebrows.

## Spacing, radii, borders, shadows

- Section rhythm: 80px of vertical section padding, 52px
  between pillars, 24px inside components.
- Max widths: prose 720px, tables and diagnostics 960px;
  full-bleed only in the hero.
- 1px is the only border weight. `--border-subtle` divides
  rows, `--border` wraps cards and inputs,
  `--border-strong` asserts separation (table headers).
- Radii ladder: 2px inline code, 4px buttons and inputs,
  6px cards and terminal mocks, 10px heroes, 16px modals,
  pill radius for status pills only. Nothing larger.
- Shadows are rare: `--shadow-xs` scrolled nav,
  `--shadow-sm` resting cards, `--shadow-md` hover and
  dropdowns, `--shadow-lg` modals. No glow and no colored
  shadows.

## Components

- Capsules (pills and chips) mark **status only**: rule
  state, version chips, filter toggles. Navigation is
  plain text links with a solid underline on hover.
- Bento grids (`.bento` / `.bento-card`) lay out feature
  cells on six columns with a 14px gap and `--radius-lg`
  corners; cells span 2, 3, 4, or 6 columns. At most one
  `is-glow` and one `is-dark` cell per bento.
- Cards: `--bg-raised` background, 1px `--border`, 6px
  radius, `--shadow-sm` at rest. No left-border accent
  stripe; use a rule-ID chip instead.
- Hover: links underline; buttons darken one step on the
  forge ramp. Never opacity-only hover.
- Press: 1px translate down. No scale effects.
- Focus: `--shadow-focus` ring, always visible. Never
  remove an outline without a replacement.
- Disabled: 50% opacity with the hue kept.
- Animation is restrained: hover transitions at 150ms or
  less, state changes at 220ms or less, no bounces and no
  parallax.

## Iconography and brand marks

- Icons are [Lucide](https://lucide.dev/), stroke-only,
  sized to context (12-24px); glyphs inside icon tiles are
  18px, or 24px in the 36px lead tiles. Icons punctuate;
  they never carry a layout.
- Bare glyphs in prose and nav inherit `currentColor`.
  Colored icons appear only inside `.icon-tile` tiles,
  which take a `--tint-*` hue (or the forge gradient for
  a lead tile).
- The brand mark is the hash-anvil with hammer:
  `website/static/img/logo-mark.svg` (square),
  `logo-lockup.svg` (mark plus wordmark, light
  backgrounds), and `logo-lockup-inverse.svg` (dark
  backgrounds).
- Terminal mocks are the brand's image library. Where a
  hero or example wants an illustration, render a mock
  diagnostic block instead.

## Banned motifs

- Gradients outside the warm `--grad-*` set; bluish-purple
  stays a hard veto.
- Photography and hand-drawn illustration.
- Frosted-glass blur outside the top nav and the command
  palette scrim.
- Glow shadows and colored shadows.
- Cards with a colored left border as the only accent.
- Blobby corner radii of 24px or larger.
- Serif italic anywhere outside the hero's one display
  moment.
