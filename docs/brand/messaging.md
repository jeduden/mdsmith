---
title: mdsmith product messaging
summary: >-
  Canonical slogan, lead, tagline, and per-surface descriptions
  for the mdsmith product. Generated fragments and the
  non-Markdown surfaces enumerated in plan 210 derive their copy
  from this file.
---
# mdsmith product messaging

Every field below feeds the generated fragments under
`docs/brand/fragments/` and the JSON, TOML, and YAML surfaces
enumerated in
[plan 210](../../plan/210_messaging-source-of-truth.md).

Edit a section here, then run `mdsmith-release sync-messaging`
to propagate the change to every tracked surface. CI runs
`sync-messaging --check` and fails the build on drift.

## Headline

The website hero template renders the headline as
`<h1>{pre}<em>{em}</em>{post}</h1>`. The raw Markdown source
lives in the code block below; `mdsmith-release sync-messaging`
parses the single emphasis span (`*…*`) to derive the pre / em
/ post split.

```markdown
Mark*down*, smithed.
```

## Eyebrow

Markdown as a single source of truth

## Lead

Write content; mdsmith keeps your Markdown neat and consistent
— fast enough to stay out of your way. Auto-fix on save, instant
navigation, cross-file integrity, and generated sections that
keep derived data in sync, so the same Markdown drives docs,
READMEs, and downstream pipelines without drift.

## Tagline

Write content; mdsmith keeps your Markdown neat and consistent
— fast enough to stay out of your way. Auto-fix on save, instant
navigation, cross-file integrity, and generated sections that
keep a single source of truth in sync across files and
pipelines.

## VS Code

mdsmith for VS Code — neat, consistent Markdown via inline
diagnostics, auto-fix on save, and instant cross-file
navigation. Your Markdown stays a single source of truth.

## Claude Code LSP

mdsmith in Claude Code — neat, consistent Markdown with inline
diagnostics and cross-file navigation, so your Markdown stays a
single source of truth.

## Claude Code skills

Slash-command skills for mdsmith fix, kinds, and check — keep
your Markdown neat and consistent from inside Claude Code, with
your Markdown as a single source of truth.

## Claude Code audit

Audit Markdown file organization in an mdsmith repository —
catalogs, kinds, schemas, and generated sections that keep your
Markdown neat, consistent, and a single source of truth.
