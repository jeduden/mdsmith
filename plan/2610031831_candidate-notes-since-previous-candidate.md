---
id: 2610031831
title: List only a candidate's own changes in its release notes
status: "✅"
summary: >-
  A release candidate's notes repeat every change since the last
  stable release, so each new candidate re-lists the earlier ones.
  Start a candidate's GitHub-generated notes at the previous
  candidate of the same version, and on the mdsmith.dev release
  notes page show each candidate minus the entries its predecessor
  already listed. Stable release notes keep spanning the whole
  release.
model: sonnet
depends-on: [2610030015]
---
# List only a candidate's own changes in its release notes

## Goal

A release candidate's notes list only what changed since the
previous candidate. A stable release still lists every change
since the previous stable release.

## Background

`mdsmith-release release-notes` pins GitHub's
`previous_tag_name` to the last stable tag for every release,
candidates included. So `v0.56.0-rc.5` lists every merge since
`v0.55.1`, the same entries `rc.4` listed plus one.

Published releases are immutable, so the candidates already out
keep their cumulative notes on GitHub. The release notes page
renders those bodies, so it needs its own subtraction to show
the existing candidates as deltas.

## Tasks

1. Add `release.PreviousNotesTag`. For a `vX.Y.Z-rc.N` tag it
   returns the highest `vX.Y.Z-rc.M` tag with `M < N`. With no
   such candidate, and for any other tag, it returns
   `PreviousStableTag`.
2. Use it in `GenerateReleaseNotes`, so a candidate's notes and
   its "Full Changelog" link start at the previous candidate.
3. In `BuildSiteReleases`, take from each shown candidate's body
   the list items its next-older shown candidate already has.
   Drop a heading whose items are all gone, and point the
   "Full Changelog" compare link at the previous candidate.
   Subtract before the heading rewrite, which scopes ids per tag.
4. Update the page copy, `release-candidates.md`, and the
   website README to say candidates list their own changes.

## Acceptance Criteria

- [x] `release-notes` for `v0.56.0-rc.3` pins
      `previous_tag_name` to `v0.56.0-rc.2`
- [x] `release-notes` for `v0.56.0-rc.1` and for `v0.56.0` pins
      the last stable tag
- [x] The page shows each candidate without the entries its
      predecessor listed, with its compare link starting there
- [x] The oldest shown candidate keeps its full notes
- [x] All tests pass: `go test ./...`
- [x] `go tool -modfile=tools/go.mod golangci-lint run` reports
      no issues
- [x] `mdsmith check .` passes
