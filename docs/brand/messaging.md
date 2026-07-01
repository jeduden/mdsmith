---
title: mdsmith product messaging
summary: >-
  Canonical slogan, lead, tagline, and per-surface descriptions
  for the mdsmith product. READMEs read these sections directly
  via `<?include extract:?>`, and the non-Markdown surfaces
  enumerated in plan 210 derive their copy from this file.
---
# mdsmith product messaging

Every field below is read two ways. Markdown surfaces (the
READMEs and feature docs) pull a section in place with
`<?include file: docs/brand/messaging.md extract: <path> ?>`.
The JSON, TOML, and YAML surfaces enumerated in
[plan 210](../../plan/210_messaging-source-of-truth.md) derive
their copy through `mdsmith-release sync-messaging`.

Edit a section here, then run `mdsmith fix` to refresh the
Markdown includes and `mdsmith-release sync-messaging` to
propagate the change to every non-Markdown surface. CI runs
`sync-messaging --check` and fails the build on drift.

The Headline keeps exactly one `*text*` emphasis span (single
asterisks, not `**`). The hero template splits the line on that
span, and `sync-messaging --check` fails if the span is missing
or doubled.

The Lead stays category + promise, with no rule-area or feature
enumeration: the homepage renders the concrete scope (style,
readability, structure, cross-file integrity, auto-fix) in the
positioning statement directly below the hero, so any
enumeration in the Lead reads twice on one screen. The fuller
enumeration lives in the Tagline, whose surfaces (package
registries, meta description, footer) never sit next to the
statement.

## Headline

Mark*down*, smithed.

## Eyebrow

Markdown as a single source of truth

## Lead

mdsmith is a Markdown linter and formatter that keeps your
writing neat and consistent — fast enough to stay out of your
way.

## Tagline

Write content; mdsmith keeps your Markdown neat and consistent
— fast enough to stay out of your way. Auto-fix on save, instant
navigation, cross-file integrity, and generated sections that
keep a single source of truth in sync across files and
pipelines.

## VS Code

Keep your Markdown neat and consistent: inline diagnostics
with lightbulb quick fixes, fix-on-save, cross-file link
and anchor integrity, generated TOCs / catalogs /
includes, frontmatter schemas, and a bundled CLI that
extracts Markdown sections as JSON or YAML. The `.vsix`
bundles the mdsmith binary, so no separate install is
needed.

## VS Code overview

The extension is a thin LSP client over the bundled
mdsmith binary, which it runs with the lsp subcommand.
Diagnostics appear inline as squiggles, and every fixable
rule contributes a lightbulb quick fix. A whole-buffer fix
action runs on demand or on save, with an optional
Refactor Preview before edits land. Cross-file navigation
extends to Go to Definition, Find All References,
workspace symbol search, and a call hierarchy across
includes, catalogs, builds, and Markdown links. The
mdsmith Command Palette runs Initialize Config, Fix All
Markdown, Install Git Merge Driver, Explain Rule on This
File, and Show Resolved Config. The .vsix bundles the
mdsmith binary for every supported OS and architecture, so
no separate install is needed.

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
