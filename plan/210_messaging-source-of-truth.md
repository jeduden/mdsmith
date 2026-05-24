---
id: 209
title: Single source of truth for product messaging via `mdsmith extract`
status: "✅"
summary: >-
  Hold the mdsmith product slogan, lead, and
  per-surface descriptions in one
  schema-conformant Markdown file. Generate
  Markdown intros via `<?include?>` fragments;
  sync non-Markdown surfaces from the same
  source via a new `mdsmith-release
  sync-messaging` subcommand. A CI drift check
  fails the build when any surface diverges.
model: opus
depends-on: []
---
# Single source of truth for product messaging

## Goal

Hold every product-level slogan, lead, and
description in one canonical Markdown file.
Every public surface derives its copy from that
source. Drift fails CI.

## Background

Today the same idea is restated in eleven
places. Each phrasing is slightly different.
Updating the slogan touches every file by hand.
Surfaces drift between releases.

The `extract` command projects a schema-bound
Markdown file into JSON. That gives us a
one-to-many pipeline. It fits the existing
generated-section idiom.

## Source file shape

A new file at `docs/brand/messaging.md` under a
new `messaging` kind in `.mdsmith.yml`. The
kind is wired via `kind-assignment` (not the
kind's `path-pattern`) so the synced website
parallel at `website/content/docs/brand/messaging.md`
can also be assigned the kind without
tripping MDS020. All fields live in
frontmatter, which `extract` projects as keys
under the root `frontmatter` object. The body
is a short prose explanation of how the file
is consumed.

Frontmatter fields (final names settled during
implementation):

- `title`, `summary` — kind contract.
- `eyebrow` — short label above the hero
  headline.
- `headline-pre`, `headline-em`,
  `headline-post` — hero headline parts. Split
  so the website template can style the `<em>`
  segment.
- `lead` — multi-line hero lead and README
  opening paragraph.
- `tagline` — one-sentence short form. Used
  for footers and package manifest
  `description` fields.
- `vscode-description` — role-scoped variant
  for the VS Code extension `package.json`.
- `claude-code-lsp-description` — role-scoped
  variant for the Claude Code LSP plugin
  manifest.
- `claude-code-skills-description`,
  `claude-code-audit-description` —
  role-scoped variants for the two Claude Code
  plugins that carry product framing.

## Pipeline

**Markdown surfaces** (READMEs, website body)
consume two generated fragment files via
`<?include?>`:

- `docs/brand/fragments/lead.fragment.md` — the
  multi-line lead.
- `docs/brand/fragments/tagline.fragment.md` —
  the one-sentence tagline.

The fragments are produced by the sync
command, not authored by hand. They carry the
standard "do not edit by hand" header
comment.

**Non-Markdown surfaces** are patched directly
by the sync command. Eleven JSON, TOML, and
YAML-frontmatter fields across nine files. The
implementation extends `internal/release`. A
new `MessagingTargets()` registry sits
alongside the existing `TrackedManifests()`.
Each entry names a file plus a typed patcher
(JSON-key, TOML-key, or YAML-frontmatter).

## Sync command

A new `mdsmith-release sync-messaging`
subcommand registered in
`cmd/mdsmith-release/main.go`:

- `sync-messaging` — load source via
  `internal/extract` (no subprocess).
  Regenerate fragment files. Patch every
  registered surface. Print a summary of bytes
  changed.
- `sync-messaging --check` — same load and
  render. Compare against on-disk contents.
  Exit non-zero on drift. CI uses this gate.

## Tasks

1. **Plan and brand source.** Land this plan.
   Draft `docs/brand/messaging.md` with the
   slogan copy locked in conversation. Add the
   `messaging` kind to `.mdsmith.yml` with an
   inline schema (kind declaration only — no
   rule changes).
2. **Extract round-trip test.** Add an e2e
   test that runs `mdsmith extract messaging
   docs/brand/messaging.md` and asserts the
   JSON tree contains every required field.
3. **Sync command — read path.** Add a
   `sync-messaging` subcommand that loads the
   source via `internal/extract` and prints
   the parsed messaging struct. Unit test for
   the loader.
4. **Patchers per target type.** JSON-key
   patcher for `package.json` and
   `plugin.json`. TOML-key patcher for
   `hugo.toml`. YAML-frontmatter patcher for
   `_index.md`. Fragment writer for
   `<?include?>`-consumable files. Each lands
   in `internal/release` with unit tests. The
   existing TOML version-stamp helper at
   `internal/release/version.go` is the
   precedent.
5. **Target registry.** Add
   `MessagingTargets()` listing every tracked
   surface and the patcher each one uses.
   Wire the `sync-messaging` apply path to
   walk it.
6. **Drift check.** Implement `sync-messaging
   --check`. Add an integration test that
   mutates a surface and asserts the command
   exits non-zero with a clear message.
7. **Wire the surfaces.** Run `sync-messaging`
   once. Commit the generated fragments and
   the patched surfaces. Replace the
   hand-written intro paragraphs in
   `README.md`, `npm/mdsmith/README.md`, and
   `python/README.md` with `<?include?>`
   directives that pull from the generated
   fragments.
8. **CI gate.** Add `mdsmith-release
   sync-messaging --check` to the existing CI
   workflow. Document the workflow in
   `docs/development/release-tooling.md`.

## Acceptance Criteria

- [x] `docs/brand/messaging.md` exists and
  `mdsmith check .` passes against the new
  `messaging` kind.
- [x] `mdsmith extract messaging
  docs/brand/messaging.md -f json` emits a
  tree whose `frontmatter` object contains
  every documented field as a non-empty
  string.
- [x] `mdsmith-release sync-messaging`
  regenerates fragments and patches every
  tracked surface. Running it twice in a row
  is a no-op (byte-stable).
- [x] `mdsmith-release sync-messaging --check`
  exits 0 when the tree is clean. It exits
  non-zero with a diff-style message when any
  surface drifts.
- [x] Every tracked surface renders the locked
  slogan copy after the sync.
- [x] `<?include?>` directives in the three
  README intros pull from the generated
  fragments. `mdsmith fix` is a no-op against
  them.
- [x] CI runs `sync-messaging --check` and
  blocks merge on drift.
- [x] All tests pass: `go test ./...`.
- [x] `go tool golangci-lint run` reports no
  issues.

## Out of scope

- Updating role-specific descriptions for
  `claude-code-dev`, `claude-code-reviewer`,
  and `claude-code-autofix`. They describe
  narrow tools, not the product. The registry
  is easy to extend later.
- Translating slogans. Single-language source
  for now.
- The website topnav copy. It is currently the
  bare wordmark with no tagline.
