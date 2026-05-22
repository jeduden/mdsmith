---
id: 194
title: Frontpage persona audit — reduce AI-first framing, surface non-AI path
status: "✅"
model: opus
depends-on: []
summary: >-
  Five blind persona agents (Go infra, tech writer, staff frontend, OSS
  maintainer, regulated platform engineer) each ran 5 cold-start
  navigations of mdsmith.dev. All five flagged the homepage's AI-first
  framing as the top conversion blocker, the "Guardrails for
  AI-generated docs" feature name as misleading, and the missing
  migration / coexistence / default-state pages as second-tier friction.
  Rewrite the hero copy, rename and demote the AI-guardrails feature,
  add a non-AI quickstart, ship migration guides for markdownlint and
  Vale, expose a Neovim install tab, disclose the default state of
  every rule, and surface license, telemetry, and SBOM one click from
  the homepage.
---
# Frontpage persona audit — reduce AI-first framing, surface non-AI path

## Goal

[mdsmith.dev](../website/content/_index.md) over-indexes on AI and
Claude Code framing. A skeptical Markdown-linter visitor — wary of AI
bloat — bounces. Five blind persona agents each ran five cold-start
navigations and quoted the same offending sentences back.

This plan rewrites the hero. It demotes the Claude Code install tab.
It renames the AI-guardrails feature. It discloses default rule state.
It adds migration on-ramps for markdownlint, Vale, and prettier. And
it puts license, telemetry, and SBOM one hop from the homepage.

## Background

Each persona's blind agent could read only the public website source:
[website/content/](../website/content/) and [docs/](../docs/). No
access to [PLAN.md](../PLAN.md), [internal/](../internal/),
[CLAUDE.md](../CLAUDE.md), [AGENTS.md](../AGENTS.md), or
[LICENSE](../LICENSE) — only what a first-time visitor sees.

| Persona | Stack                                     | AI-bloat radar |
|---------|-------------------------------------------|----------------|
| Pavel   | Go, Neovim, markdownlint-cli, no AI tools | High           |
| Mira    | Vale + remark, Astro docs monorepo        | High           |
| Sven    | prettier + markdownlint-cli2 + husky      | High           |
| Aaliyah | prettier; ~6-file OSS repo; refuses AI    | Very high      |
| Diego   | Air-gapped CI, regulated bank, npm proxy  | High (memo)    |

Convergent findings (number of personas that independently flagged the
item):

| #   | Friction                                                       | Hits | Anchor                                                                                    |
|-----|----------------------------------------------------------------|------|-------------------------------------------------------------------------------------------|
| 1   | Hero summary reads as "AI tool"                                | 5/5  | [_index.md:3](../website/content/_index.md) "and AI-generated content"                    |
| 2   | "Guardrails for AI-generated docs" name implies AI-only        | 5/5  | [features/index.md:43-45](../docs/features/index.md)                                      |
| 3   | Claude Code install tab gets equal billing with go / npm / pip | 4/5  | [_index.md:34-38](../website/content/_index.md)                                           |
| 4   | Default state of guardrail rules is undisclosed                | 3/5  | [ai-guardrails.md, now size-and-readability.md](../docs/features/size-and-readability.md) |
| 5   | No markdownlint migration guide                                | 2/5  | [guides/](../docs/guides/) ships Hugo migration but no markdownlint on-ramp               |
| 6   | No Vale / remark coexistence story                             | 1/5  | hero names markdownlint, never Vale                                                       |
| 7   | No prettier coexistence story                                  | 1/5  | [features/auto-fix.md](../docs/features/auto-fix.md) leaves the boundary fuzzy            |
| 8   | Neovim absent from install widget                              | 1/5  | tabs are go / npm / pip / vs-code / claude — no neovim                                    |
| 9   | Telemetry never affirmatively denied                           | 1/5  | no public page says "no telemetry, no analytics"                                          |
| 10  | SBOM not advertised in releases                                | 1/5  | [guides/install.md](../docs/guides/install.md) lists signature + provenance               |
| 11  | Speed claim "faster than markdownlint" is not quantified       | 1/5  | [_index.md:11](../website/content/_index.md) — number lives only on perf page             |
| 12  | License not visible from homepage                              | 1/5  | footer-only on most page templates                                                        |
| 13  | No "minimal mode" / "small repo" path                          | 1/5  | 16 feature cards, no Core vs. Scale split                                                 |

A Pavel-shaped visitor also could not find a rules index at `/rules/`
(the topnav advertises it conditionally and several feature cards list
rule IDs without a destination). That is tracked elsewhere; this plan
treats it as out of scope.

## Tasks

Group A — homepage framing:

1. [x] Rewrite the hero summary in
   [website/content/_index.md](../website/content/_index.md) to drop
   "and AI-generated content" and describe the tool by what it checks
   (style, readability, structure, cross-file integrity).
2. [x] Rewrite the body sentence in
   [website/content/_index.md](../website/content/_index.md) so
   "Claude Code plugin" is not the climax of the "one engine, every
   surface" list. Move the Claude Code reference into a follow-on
   sentence rather than the list's final slot.
3. [x] Reorder the install widget tabs in
   [website/content/_index.md](../website/content/_index.md): keep
   `go`, `npm`, `pip`, `vs code` in their existing positions and move
   `claude code` to the end (or behind a "more editors" disclosure).
4. [x] Add a `neovim` tab to the install widget showing
   `mdsmith lsp` as a standalone command, linked to the new Neovim
   setup guide (task 11).
5. [x] Replace the bare "faster than Node markdownlint" in
   [website/content/_index.md](../website/content/_index.md) with a
   specific multiple ("roughly 4× faster on a 700-file corpus") and
   link the sentence to
   [docs/features/performance.md](../docs/features/performance.md).
6. [x] Surface the MIT license on the homepage — add an "MIT
   licensed" chip next to the CI / coverage / Go Report Card row in
   [layouts/partials/hero.html](../website/layouts/partials/hero.html)
   so the answer is reachable in zero clicks.

Group B — AI-guardrails feature reframing:

7. [x] Rename the feature file
   [docs/features/ai-guardrails.md](../docs/features/size-and-readability.md)
   to a neutral name (e.g. "Size and readability limits"). Update
   the feature-grid blurb in
   [docs/features/index.md](../docs/features/index.md) and the
   front-matter `title`. Hugo derives the URL from the content
   path, so the old `/features/ai-guardrails/` URL stops resolving;
   adding `aliases:` for redirect would need the feature kind
   schema in `.mdsmith.yml` to permit the field — tracked as a
   follow-up rather than landed here.
8. [x] On the renamed feature page, state the **default state** of
   each rule (`MDS022`, `MDS023`, `MDS024`, `MDS028`, `MDS037`):
   which are on by default, which are opt-in, and the exact config
   key that toggles each. Include a copy-pasteable `.mdsmith.yml`
   snippet that disables all five at once.
9. [x] Move the renamed feature card below
   [features/cross-file-integrity.md](../docs/features/cross-file-integrity.md),
   [features/rename.md](../docs/features/rename.md), and
   [features/live-diagnostics.md](../docs/features/live-diagnostics.md)
   in the grid (adjust `weight:` in front matter) so the cross-file
   value lands before the readability rules.

Group C — coexistence and migration on-ramps:

10. [x] Add `docs/guides/migrate-from-markdownlint.md` — a rule-by-rule
    mapping table (mdsmith ID, markdownlint ID, behavioural delta),
    the exact `.markdownlint.yaml` → `.mdsmith.yml` rewrite, and the
    list of markdownlint rules mdsmith does not implement (per the
    [markdown-linters](../docs/background/markdown-linters.md)
    comparison).
11. [x] Add `docs/guides/editors/neovim.md` with an `init.lua`
    snippet using Neovim's built-in LSP, no plugin required, mirroring
    the structure of [editors/vscode.md](../docs/guides/editors/vscode.md).
12. [x] Add `docs/guides/coexist-with-prettier.md` — a shared
    `husky` / `lint-staged` config that orders
    `mdsmith fix` before `prettier --write`, plus one paragraph stating
    who owns formatting (prettier) vs. who owns lint, generated
    sections, and cross-file checks (mdsmith).
13. [x] Add `docs/guides/coexist-with-vale-and-remark.md` — a
    200-word boundary statement: prose voice vs. formatting and
    structure, what each tool owns, and a sample shared CI pipeline.

Group D — small-repo and CLI-only path:

14. [x] Add a "Quick start (CLI only)" section near the top of
    [docs/guides/install.md](../docs/guides/install.md): install the
    binary, run `mdsmith fix README.md`, done. No `.mdsmith.yml`
    needed. Explicit "no kinds, no schemas, no LSP required."
15. [x] Split the feature grid in
    [docs/features/index.md](../docs/features/index.md) into "Core
    (every repo)" and "Scale (large docs sites)" sub-sections via a
    second-level heading and / or a `weight:` reshuffle, so a six-file
    visitor sees the relevant half first.

Group E — compliance answers one hop from homepage:

16. [x] Add
    [`docs/reference/telemetry.md`](../docs/reference/telemetry.md)
    that affirmatively states no telemetry, no analytics, no runtime
    network calls from the CLI or LSP. Link the file from the footer in
    [layouts/partials/footer.html](../website/layouts/partials/footer.html).
17. [x] Publish a CycloneDX (or SPDX) SBOM as a release asset from
    the release pipeline, and document its verification command
    alongside the existing `cosign verify-blob` and `gh attestation
    verify` lines in
    [docs/guides/install.md](../docs/guides/install.md).
18. [x] Add one sentence to the start of the Claude Code section in
    [docs/guides/install.md](../docs/guides/install.md): "The Claude
    Code plugin is an optional editor surface. mdsmith itself never
    calls an LLM or external service at runtime."

## Acceptance Criteria

- [x] `mdsmith check .` passes on every file the plan touches
- [x] Hero summary on
      [website/content/_index.md](../website/content/_index.md)
      no longer contains the phrase "AI-generated content"
- [x] AI-guardrails feature is renamed, default rule state is stated
      inline, and the disable-everything snippet is present
- [x] Install widget shows `neovim` as a tab and `claude code` is no
      longer the first or second tab
- [x] New guides land: markdownlint migration, Neovim editor,
      prettier coexistence, Vale / remark coexistence
- [x] Quick start (CLI only) section is the first numbered section
      on [docs/guides/install.md](../docs/guides/install.md)
- [x] Telemetry page is reachable from the footer in one click
- [x] SBOM is a published release asset and the install guide shows
      its verification command
- [x] MIT license badge is visible on the homepage without scrolling
- [x] Hero speed claim links to a quantified number on
      [docs/features/performance.md](../docs/features/performance.md)
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
