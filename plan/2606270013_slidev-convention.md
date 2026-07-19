---
id: 2606270013
title: Add built-in Slidev convention
status: "✅"
summary: >-
  Add a slidev built-in convention that disables the
  eight rules that produce false positives on Slidev
  presentation files, giving users a one-key preset
  instead of a manual eight-rule override block.
model: sonnet
---
# Add built-in Slidev convention

## Goal

Add a `slidev` built-in convention that disables
the eight default-on rules that fire false positives
on Slidev presentation Markdown, so users can apply
a single `convention: slidev` instead of a manual
eight-rule override block. Closes issue #41.

## Context

Slidev separates slides with `---`. This confuses
the parser. Headings jump per slide, slides can share
H1 titles, emphasis stands in for headings, and layout
front-matter blocks look like setext headings. The
current workaround is a manual eight-rule override
block.

The parser-level `---`-as-page-separator issue is
out of scope for this plan. The convention addresses
the rule-configuration false positives only.

Eight default-on rules must be disabled:

| Rule ID | Name                                 | Why it fires                            |
| ------- | ------------------------------------ | --------------------------------------- |
| MDS002  | `heading-style`                      | Layout blocks parsed as setext headings |
| MDS003  | `heading-increment`                  | Each slide restarts at H1               |
| MDS004  | `first-line-heading`                 | Front matter before first heading       |
| MDS005  | `no-duplicate-headings`              | Same heading on multiple slides         |
| MDS013  | `blank-line-around-headings`         | Layout blocks interfere                 |
| MDS017  | `no-trailing-punctuation-in-heading` | Stylistic slide titles                  |
| MDS018  | `no-emphasis-as-heading`             | Bold used for slide-level emphasis      |
| MDS030  | `empty-section-body`                 | Layout-only slides have no body         |

## Tasks

1. [x] Add a `slidev` entry to the `conventions`
   map in
   [`internal/convention/convention.go`](../internal/convention/convention.go).
   Use `FlavorAny` (no flavor enforcement — users
   may use GFM, CommonMark, or Obsidian flavors).
   Disable the eight rules listed above.
2. [x] Add `TestLookup_Slidev` in
   `internal/convention/convention_test.go`.
   Assert that `Lookup("slidev", nil)` resolves
   without error and that all eight rules are
   present with `Enabled: false`.
3. [x] Update
   [`docs/reference/conventions.md`](../docs/reference/conventions.md)
   to document the new convention: purpose,
   the eight-rule list, example config, and the
   known limitation (parser-level `---` handling
   is not addressed).
4. [x] Run `go run ./cmd/mdsmith fix .` and
   `go run ./cmd/mdsmith check .` — zero failures.
5. [x] Run `go test ./internal/convention/...` and
   `go test ./...` — all green.

## Acceptance Criteria

- [x] `Lookup("slidev", nil)` returns a convention
  with eight rules, all `Enabled: false`.
- [x] `internal/convention/convention_test.go`
  has a `TestLookup_Slidev` that covers all
  eight disabled rules.
- [x] `docs/reference/conventions.md` documents
  `slidev` with purpose, rule list, and limitation
  note.
- [x] `go test ./...` green.
- [x] `go tool golangci-lint run` reports no issues.
- [x] `go run ./cmd/mdsmith check .` — 0 failures.
