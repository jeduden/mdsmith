---
id: 2606110639
title: Audience split for website-published docs
status: ✅
summary: >-
  Remove maintainer- and agent-facing content from every
  website-published doc page and rehome it in repo-only
  trees.
---
# Audience split for website-published docs

## Goal

Every page that reaches mdsmith.dev serves users only.
Maintainer- and agent-facing material lives in repo-only
trees instead.

## Scope

The [website sync](../docs/development/website-config.md)
publishes these pages:

- the [background](../docs/background/index.md),
  [features](../docs/features/index.md),
  [guides](../docs/guides/index.md), and
  [reference](../docs/reference/index.md) trees
- the [homepage](../website/content/_index.md)
- the [rule index](../internal/rules/index.md) and every
  rule README under `internal/rules/`

The repo-only trees (`docs/development`, `docs/research`,
`docs/security`, `docs/brand`, `plan/`) hold contributor
and agent content. A deep-dive link from a published page
into a repo-only tree is fine. The sync rewrites such
links to GitHub URLs.

Out of scope: full prose rewrites. The review lens is the
audience split. Wording changes only where content moves
or a plan reference is replaced.

## Tasks

1. Triage every published page for maintainer- or
   agent-facing content; record per-file verdicts.
2. Split the
   [linter comparison](../docs/background/markdown-linters.md):
   move the README-presentation notes into
   [add-peer-linter](../docs/development/add-peer-linter.md),
   drop the Future Plans tracking section, and strip
   inline plan references. Keep the pin-a-version advice.
3. Clean the concepts pages ([engine-api][ea] and
   [flavor-rule-convention-kind][frck]): state shipped
   behavior without plan-tracking framing.
4. Replace plan-number references in
   [guides](../docs/guides/index.md) and
   [reference](../docs/reference/index.md) pages with
   plain feature descriptions.
5. Clean the flagged rule READMEs the same way (MDS020,
   MDS024, MDS047, MDS053 through MDS058).
6. Apply remaining triage findings, regenerate catalogs
   with `mdsmith fix`, and leave `mdsmith check .` green.

## Acceptance Criteria

- [x] No published page references a plan file, a plan
  number, or [PLAN.md](../PLAN.md) outside fenced example
  content.
- [x] Maintainer guidance found on published pages now
  lives under
  [docs/development](../docs/development/index.md).
- [x] `mdsmith check .` passes.

[ea]: ../docs/background/concepts/engine-api.md
[frck]: ../docs/background/concepts/flavor-rule-convention-kind.md
