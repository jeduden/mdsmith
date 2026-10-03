---
id: 2610031436
title: Share one CommonMark fence scanner across packages
status: "🔲"
model: sonnet
summary: >-
  Five packages carry their own fenced-code opener and
  closer scanners, and they have drifted: directivefiles
  missed the backtick-in-info-string rule that
  internal/lint already had. Move one scanner into a
  shared leaf package and point every caller at it.
---
# Share one CommonMark fence scanner across packages

## Goal

Every byte-level fence check in mdsmith uses one scanner,
so a CommonMark fix lands in all callers at once.

## Background

The PR #895 review found that `openingFence` in
[directivefiles.go][directivefiles] accepted "```a`b" as
an opener. Goldmark and the scanner in
[layer0_fence.go][layer0] reject that line. The bug was
fixed in place, but the copies remain:

- [internal/lint/layer0_fence.go][layer0] —
  `openingFence` / `closingFence`
- [internal/directivefiles/directivefiles.go][directivefiles]
  — `openingFence` / `isClosingFence`
- [internal/rules/listscan/listscan.go][listscan] —
  `openingFenceRel` / `closingFence`
- [internal/rules/include/headings.go][headings] —
  `isClosingFence`
- [internal/release/sitereleases.go][sitereleases] —
  `openingFence` (string-based)

## Tasks

1. Write a table-driven contract test that runs every
   existing scanner on the same CommonMark fence cases
   (indent, run length, tilde vs backtick, backtick in
   info string, closer trailing text, closer shorter than
   opener). Record where the copies disagree.
2. Add a small leaf package (for example
   `internal/fence`) that exports an opener and a closer
   function. Base it on the `internal/lint` version and
   keep its allocation-free, by-value return.
3. Move each caller onto the shared package, one commit
   per package, and delete its local copy. `listscan`
   needs a relative-column variant, so give the shared
   package a column parameter or offset helper.
4. Benchmark `internal/lint` before and after to confirm
   the hot per-line path does not regress.

## Acceptance Criteria

- [ ] Only the shared package defines fence opener or
      closer logic; `grep -rn "func openingFence"` finds
      only that package
- [ ] The contract test passes, and its cases include
      "```a`b" (not an opener) and "~~~a`b" (an opener)
- [ ] `internal/lint` benchmarks show no regression
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues

[layer0]: ../internal/lint/layer0_fence.go
[directivefiles]: ../internal/directivefiles/directivefiles.go
[listscan]: ../internal/rules/listscan/listscan.go
[headings]: ../internal/rules/include/headings.go
[sitereleases]: ../internal/release/sitereleases.go
