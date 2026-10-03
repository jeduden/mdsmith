---
id: 2610030015
title: Publish release notes on mdsmith.dev
status: "✅"
summary: >-
  mdsmith.dev has no release notes; its only "Releases" link
  leaves for GitHub. Fetch every published GitHub release into
  a gitignored `website/data/releases.json` at site-build time
  and render a `/releases/` page: stable releases first, then
  release candidates below, each with its notes, and a link
  through to the GitHub release for its downloads.
model: sonnet
depends-on: []
---
# Publish release notes on mdsmith.dev

## Goal

Show the notes of every published release on
mdsmith.dev. Stable releases come first. Release
candidates follow below. A reader then needs GitHub
only for the downloads.

## Background

Each stable release and each `vX.Y.Z-rc.N` candidate carries
GitHub-generated notes (`mdsmith-release release-notes`). The
site footer links straight to the GitHub releases list, so a
reader has to leave the site to see what changed.

The site is static Hugo output. The notes live only on GitHub,
so the build must fetch them. The fetch is a `mdsmith-release`
subcommand, per
[release-tooling.md](../docs/development/release-tooling.md).

A candidate is cut on every merge to `main`, but `pages.yml`
deploys only on doc, website, or generator changes. Without an
extra deploy, a new candidate would not reach the site until
the next doc edit.

## Tasks

1. Add `release.BuildSiteReleases` to split published releases
   into `stable` and `candidates` (by GitHub's `prerelease`
   flag), drop drafts, sort each list newest first, and demote
   body headings (ATX and setext) by two levels so
   `## What's Changed` sits under the release's own heading.
   Each body heading also gets an id scoped by the tag
   (`{#v0-55-1-whats-changed}`), since every release renders
   its own "What's Changed". A heading's own `{#id}` is scoped
   too, and headings inside block quotes and list items are
   rewritten. A zero-width space splits each Hugo shortcode
   opener in a body: the page renders bodies with
   `RenderString`, which expands shortcodes and fails on an
   unknown one. Candidates published before the latest stable
   release are dropped: their cumulative notes are already in
   that release, and keeping them would grow the page by one
   changelog per merge.
2. Add `release.SyncReleases` to list every release via the
   paginated `GET /repos/{repo}/releases` endpoint and write the
   result as JSON.
3. Add the `mdsmith-release sync-releases [--out <path>]`
   subcommand (default `website/data/releases.json`); gitignore
   the output.
4. Add `website/content/releases.md` and the `releases` layout:
   stable releases as `<details>` cards (newest open), then a
   "Release candidates" section of collapsed cards, each with
   date, notes, and a GitHub link. Hugo picks the template by
   content path (`layouts/releases/page.html`), because the
   `website-page` kind schema declares no `layout:` key. A small
   script opens the card a `#tag` link points into. Without the
   data file the page says the notes are not bundled and links
   to GitHub.
5. Point the footer's "Releases" link at `/releases/`.
6. Run `sync-releases` in both `pages.yml` jobs, and dispatch
   `pages.yml` on `main` from `release-candidate.yml` after a
   candidate publishes, so each candidate reaches the site.
   Dispatching on `main` builds the newest commit, so the
   candidate's older commit never rolls back a docs merge. The PR-time
   fetch may fail without failing the job (the page then renders
   its no-data note); the deploy's fetch must succeed.
7. Add a Playwright spec for the footer link and the no-data
   fallback (the e2e build has no token, so no data file).
8. Document the subcommand, the page, and the deploy trigger.

## Acceptance Criteria

- [x] `mdsmith-release sync-releases` writes stable releases and
      candidates, newest first, with drafts left out
- [x] `/releases/` lists stable releases first and the release
      candidates since the latest stable release below, each
      with its rendered notes and a link to its GitHub release
- [x] The page renders a fallback with a GitHub link when the
      data file is absent (local builds, e2e)
- [x] The footer "Releases" link goes to `/releases/`
- [x] `release-candidate.yml` dispatches `pages.yml` on `main`
      after the candidate's publish step
- [x] All tests pass: `go test ./...`
- [x] `go tool -modfile=tools/go.mod golangci-lint run` reports
      no issues
- [x] `mdsmith check .` passes
