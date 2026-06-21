---
id: 2606210840
title: "Same-file anchor-resolution rule for true gomarklint parity"
status: "🔲"
summary: >-
  The per-linter parity conventions disable
  cross-file-reference-integrity (MDS027) because the peers'
  link-fragments / MD051 rules only resolve same-file `#anchor` links,
  while MDS027 also walks the workspace for cross-file links — a partial
  match that forces the goldmark parse. Add a lightweight, parse-skip-safe
  rule (or an MDS027 anchors-only mode) that checks only same-file anchor
  resolution, so gomarklint-parity, rumdl-parity, and markdownlint-parity
  can run a like-for-like anchor check without the cross-file walk.
model: sonnet
depends-on: []
---
# Same-file anchor-resolution rule for true gomarklint parity

## Background

The [parity conventions][conv] run the rule set a peer linter runs by
default. They disable [MDS027][mds027] for every peer. The reason is a
partial cover:

- gomarklint `link-fragments`, markdownlint `MD051`, and rumdl `MD051`
  resolve only same-file `#anchor` links. They read one file.
- mdsmith's MDS027 resolves same-file anchors and cross-file links. That
  needs the workspace graph and the parsed tree.

So matching the peers with MDS027 makes mdsmith do more work. It also
forces the goldmark parse. That parse is the cost the parity benchmark
isolates. Dropping MDS027 keeps the parity sets parse-skip-safe. But then
mdsmith runs no anchor check, while the peers do.

A dedicated same-file-anchor rule closes the gap. It checks only
`#anchor` links within a file. It matches the peers one to one.

## Goal

Add a parse-skip-safe rule that checks same-file `#anchor` link
resolution. It lets the parity conventions match the peers'
link-fragments check without the cross-file walk.

## Design notes

Two shapes are viable. Pick one during implementation:

1. A new rule (e.g. `same-file-anchor`). It scans the headings and
   `#fragment` links in one file. It reports unresolved anchors. It maps
   to the peers' link-fragments rules as a full cover, so the parity
   derivation enables it for gomarklint, rumdl, and markdownlint.
2. An anchors-only mode on MDS027. A setting skips the cross-file pass.
   The parity conventions select it. It reuses MDS027's slug logic. It
   must be parse-skip-safe in that mode.

Prefer option 1. It leaves MDS027's cross-file semantics untouched. It
also gives the coverage matrix an honest full-cover mapping. Fall back to
option 2 only if slug reuse forces it.

## Tasks

1. Decide between a new rule and an MDS027 anchors-only mode.
2. Implement the same-file anchor scan on the line/heading projection. It
   must resolve on the parse-skip (nil-AST) path.
3. Add unit tests (good/bad inline fixtures) and a rule fixture directory
   under `internal/rules/MDS###-<name>/`.
4. Point the peers' link-fragments / MD051 mappings at the new rule as a
   full cover. Re-run `mdsmith fix` to regenerate the coverage matrix.
5. Regenerate the parity conventions and the parity-rules fragment
   (`mdsmith-release sync-parity-rules`). Confirm the new rule is enabled
   in the relevant parity sets and they stay parse-skip-safe.
6. Re-run the benchmark to record the per-linter parity figures.

## Acceptance Criteria

- [ ] A same-file anchor check resolves `#fragment` links against
      headings in the same file and reports unresolved ones.
- [ ] The check runs on the parse-skip path (no goldmark AST required).
- [ ] gomarklint-parity, rumdl-parity, and markdownlint-parity enable
      the anchor check and stay parse-skip-safe where MDS020 allows.
- [ ] The coverage matrix records a full (non-partial) cover for the
      peers' link-fragments / MD051 rules.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes

[conv]: ../internal/convention/convention.go
[mds027]: ../internal/rules/MDS027-cross-file-reference-integrity/README.md
