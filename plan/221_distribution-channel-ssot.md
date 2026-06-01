---
id: 221
title: Single source of truth for distribution channels
status: "🔲"
summary: >-
  Hold every distribution channel — push, pull, and
  toolchain — in one schema-bound directory, and
  generate every channel list from it: the install
  guide table, the release pipeline table, the README,
  the feature card, the website hero, and an
  interactive website picker. A `sync-channels --check`
  gate fails CI when any surface drifts.
model: opus
depends-on: []
---
# Single source of truth for distribution channels

## Goal

Adding or retiring a distribution channel should be a
one-file edit. Every list of channels — across the
docs, the README, and the website — derives from that
one source. Drift fails CI instead of shipping.

## Background

mdsmith ships through a growing set of channels, and
they are enumerated by hand in at least five places.
The copies have already drifted:

- The channels table in
  [install.md](../docs/guides/install.md) is currently
  complete (Go, npm, npx, PyPI, uvx, pipx, Homebrew,
  mise, asdf, GitHub release) but hand-maintained.
- The "Installs everywhere" card in
  [features/index.md](../docs/features/index.md) and
  the
  [install-everywhere.md](../docs/features/install-everywhere.md)
  page both omit Homebrew.
- The [README](../README.md) install block omits
  Homebrew and still flags asdf as "pending", though
  the explicit-URL asdf install works today.
- The website hero `install:` block in
  [_index.md](../website/content/_index.md) is a
  curated five-tab subset (go, npm, pip, vs code,
  claude) with neither asdf nor Homebrew.

Only one surface is generated. The publishing table in
[release.md](../docs/development/release.md) renders a
`<?catalog?>` over
[release-channels/](../docs/development/release-channels).
That directory holds only the five **push** channels.
Those are the ones a CI job publishes with a credential
(npm, PyPI, Open VSX, Visual Studio Marketplace, GitHub
Releases).

The other channels have no file at all. The **pull**
channels read from a release: Homebrew via the
`notify-homebrew-tap` job, asdf via
`jeduden/asdf-mdsmith`, and mise via the `ubi` backend.
The **toolchain** channels are `go install` and the
`npx` / `uvx` / `pipx` runners. Adding one updates only
the doc the author remembered. That push/pull split is
the root cause of the drift.

Two precedents already exist in this repo:

- [plan/210](210_messaging-source-of-truth.md) did this
  for product messaging: one schema-bound source,
  `<?include?>` / `<?catalog?>` for the Markdown
  surfaces, and a `mdsmith-release sync-messaging
  --check` drift gate for the non-Markdown surfaces.
- The `npmPlatformBuilds` array in
  [buildnpm.go](../internal/release/buildnpm.go), gated
  by `TestNpmChannelDocMatchesPlatformBuilds`, is the
  precedent for a Go-list ↔ doc drift test.

This plan generalizes the first precedent from
messaging to channels.

## Design

### Source

Make the
[release-channels/](../docs/development/release-channels)
directory the single source of truth for every channel.
Keep the directory name. A rename would ripple across
[.mdsmith.yml](../.mdsmith.yml), the release docs, and
the Go release tests. Each channel stays one file.

Extend
[proto.md](../docs/development/release-channels/proto.md).
The publish-only fields become optional. These new
discriminator fields are added:

- `mechanism: "push" | "pull" | "toolchain"`.
- `artifact: "cli" | "vscode-extension" | "claude-plugin"`.
- `command` — the install one-liner.
- `audience` — the "Best for" cell.
- `status: "live" | "pending"` — `pending` marks the
  short-form asdf and mise installs still waiting on a
  registry PR
  ([plan/145](145_asdf-mise-registry-submissions.md)).
- `platforms` — optional tags (`macos`, `linux`,
  `windows`, `node`, `python`, `go`, `editor`, `agent`)
  that drive the interactive picker's filters.
- `registry`, `credential`, `job` — now optional, set
  only on `mechanism: push` channels.

Add the seven missing files: `go.md`, `npx.md`,
`uvx.md`, `pipx.md`, `homebrew.md`, `asdf.md`,
`mise.md`. The five push files already exist.

Extending the schema edits `proto.md` (the referenced
schema file), not [.mdsmith.yml](../.mdsmith.yml)
itself. If the kind's `path-pattern` or
`kind-assignment` must change, that needs explicit
maintainer consent per CLAUDE.md.

### Extraction model

Two generators already exist, and each fits a
different shape. This plan uses both, and leans on
`extract` harder than the catalog-only first draft.

`<?catalog?>` aggregates many files into rows. It reads
frontmatter only. So it stays the tool for the
cross-file tables in install.md and release.md.

`mdsmith extract` projects one schema-bound file into a
typed tree of frontmatter plus body sections. Its
read-side, `<?include ... extract:?>`, splices one
typed leaf back into Markdown with no fragment file
([plan/211](211_include-extract-value.md)). Three
surfaces should use it:

- **Typed channel bodies.** Give each channel file a
  section schema. The body then carries structured
  detail: per-platform commands, verify steps, and the
  artifact list. This is the "deeper levels" payoff.
- **Per-channel install sections.** Each `##` section
  in install.md pulls its own command from the channel
  file with `<?include ... extract: command ?>`. The
  prose can no longer drift from the source.
- **The machine data file.** `website/data/channels.yaml`
  is the output of `mdsmith extract --format yaml`, not
  a hand-rolled serializer. The picker reads that tree.

`extract` is single-file, so the set stays one file per
channel. That also keeps the catalog tables and the
per-channel website pages working. The sync command
loads each file through `extract`, the way
`sync-messaging` already does, and assembles the array.

### Generated surfaces

1. The [install.md](../docs/guides/install.md) table
   becomes a `<?catalog?>` over the directory, sorted
   by `weight`, with `command` and `audience` columns.
   Each per-channel section below the table embeds its
   own command with `<?include ... extract: command ?>`.
2. The [release.md](../docs/development/release.md)
   table becomes a `<?catalog?>` with `where: mechanism
   == "push"`, so Homebrew, asdf, and mise are excluded
   by filter rather than by omission.
3. The feature card and
   [install-everywhere.md](../docs/features/install-everywhere.md)
   pull a generated channel-name fragment via
   `<?include?>`, so the prose cannot drift.
4. A new `website/data/channels.yaml` is the output of
   `mdsmith extract --format yaml`, assembled by the
   sync command below. The hero `install:` block reads
   that data and stays a curated subset via a
   `featured: true` flag on the chosen channels.

### Interactive picker

Add a Hugo partial `install-picker.html` to the install
page. It reads `website/data/channels.yaml`. It filters
channels by OS and ecosystem, using the `platforms`
tags. It then shows the matching `command` with a copy
button. Reuse the styling from
[install-list.html](../website/layouts/partials/install-list.html).
A picker keeps every channel visible as the list grows
past a flat table.

### Sync command and drift gate

Add a `sync-channels` command to the release tool,
modeled on `sync-messaging`. It loads each channel file
through `mdsmith extract`. It writes the Hugo data file
from that typed output. It also patches the hero block
on the home page.

The `--check` flag exits non-zero on drift. Wire it into
[ci.yml](../.github/workflows/ci.yml) next to the
`sync-messaging --check` step. The Markdown tables stay
current through `mdsmith fix` and the catalog-drift
check.

## Phasing

- **Phase 1 — docs SSOT.** Generalize `proto.md`, add
  the seven channel files, and convert install.md,
  release.md, and the feature card to generated output.
  No Go or website changes.
- **Phase 2 — sync and gate.** Add `sync-channels
  --check`, the hero patcher, `website/data/channels.yaml`,
  and the CI gate.
- **Phase 3 — interactive.** Add the Hugo install
  picker.

## Tasks

1. Extend
   [proto.md](../docs/development/release-channels/proto.md):
   make `registry` / `credential` / `job` optional and
   add `mechanism`, `artifact`, `command`, `audience`,
   `status`, `platforms`. Add a section schema so each
   channel body projects through `extract`. Backfill the
   five existing push files so `mdsmith check .` stays
   green.
2. Add `go.md`, `npx.md`, `uvx.md`, `pipx.md`,
   `homebrew.md`, `asdf.md`, and `mise.md` with full
   frontmatter.
3. Convert the
   [install.md](../docs/guides/install.md) channels
   table to a `<?catalog?>`; run `mdsmith fix` and
   confirm the rendered table matches today's content.
   Embed each per-channel command with
   `<?include ... extract: command ?>`.
4. Convert the
   [release.md](../docs/development/release.md) table to
   a `where: mechanism == "push"` catalog.
5. Replace the channel-name list in the feature card and
   [install-everywhere.md](../docs/features/install-everywhere.md)
   with a `<?catalog?>` of names, so the aggregate
   prose stays generated.
6. Add `mdsmith-release sync-channels` plus unit tests
   under [internal/release/](../internal/release),
   following the `sync-messaging` pattern. It loads each
   file via `mdsmith extract` and writes
   `website/data/channels.yaml` from the typed output.
7. Add `sync-channels --check`, wire it into
   [ci.yml](../.github/workflows/ci.yml), and document
   it in
   [release-tooling.md](../docs/development/release-tooling.md).
8. Add the `install-picker.html` partial and styling,
   and point the install page at it.

## Acceptance Criteria

- [ ] Every channel is one file under the
  [release-channels/](../docs/development/release-channels)
  directory; no other file enumerates channels by hand.
- [ ] `mdsmith fix` regenerates the install.md and
  release.md tables byte-for-byte from the source.
- [ ] `mdsmith extract` projects each channel file as
  frontmatter plus a typed body, and
  `website/data/channels.yaml` is that output.
- [ ] The README, the feature card, and
  [install-everywhere.md](../docs/features/install-everywhere.md)
  list Homebrew and asdf, and no surface calls asdf
  "pending" once the source marks it live.
- [ ] `mdsmith-release sync-channels` is byte-stable on
  a second run; `--check` exits non-zero on drift and is
  enforced in [ci.yml](../.github/workflows/ci.yml).
- [ ] The website renders an interactive picker that
  filters channels by OS and ecosystem.
- [ ] All tests pass: `go test ./...`.
- [ ] `go tool golangci-lint run` reports no issues.

## Out of scope

- The asdf and mise registry submissions themselves
  ([plan/145](145_asdf-mise-registry-submissions.md)).
- The six Claude Code plugins — they are an editor
  surface, not a binary channel, and stay as prose in
  [install.md](../docs/guides/install.md).
- Translating channel copy into other languages.
