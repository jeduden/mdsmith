---
id: 2610031436
title: Share one CommonMark fence scanner across packages
status: "🔲"
model: sonnet
summary: >-
  Eight packages carry their own fenced-code opener and
  closer scanners, and they drifted: six of them missed
  the backtick-in-info-string rule that internal/lint
  already had, and PR #895 patched each copy by hand. Move
  one scanner into a shared leaf package and point every
  caller at it.
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
- [internal/lint/lineclass_scan.go][lineclass] —
  `detectFenceOpen` / `isFenceClose`, a second copy in the
  same package
- [internal/directivefiles/directivefiles.go][directivefiles]
  — `openingFence` / `isClosingFence`
- [internal/rules/listscan/listscan.go][listscan] —
  `openingFenceRel` / `closingFence`
- [internal/rules/requiredstructure/rule.go][reqstruct] —
  `fenceOpenRun` / `fenceClose`
- [internal/rules/include/headings.go][headings] —
  `fenceOpenMarker` (a wrapper over `countFenceRun`) /
  `isClosingFence`
- [internal/rules/include/links.go][links] —
  `countFenceRun`, the `rewriteSkippingCode` opener
- [internal/rules/slidevstructure/rule.go][slidev] —
  `isCodeFence`, a prefix-only open/close toggle
- [internal/rules/fencepos/fencepos.go][fencepos] —
  `isFenceOpenLine`, the fallback scan in `OpenLineRange`
  for a hand-built node with no parser position
- [internal/release/website.go][website] — `fenceMarker` /
  `opensFence` / `fenceLineEmptyAfter`, the
  `applyOutsideFences` scanner
- [internal/release/sitereleases.go][sitereleases] —
  `openingFence` (string-based, over `fenceMarker`)

Review round 2 found the same bug in five more copies and
patched each one by hand: `fenceOpenRun`, `codeFenceRe`,
`countFenceRun`, `isCodeFence`, and the fencepos fallback
scan all took "```a`b" as an opener. Round 3 found it in
`applyOutsideFences` and added `opensFence`. Each fix
duplicated the backtick check again, which is the drift
this plan removes. Other differences remain. `isCodeFence`
toggles on any run without matching the closer's character
or length. The include scanners strip any amount of leading
whitespace, while the others allow at most three spaces.

The empty-fence position also has two answers.
`fencepos.OpenLineRange` reads the parser's node position,
so it finds the opener of an empty, info-less fence.
`lint.FindFencedOpenLine` and the Layer 0 mirror in
[layer0_fence.go][layer0] still return no line for that
shape. The shared package should settle one answer.

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
      closer logic: every function and regex named in
      Background is deleted, and a search for each name
      finds no definition outside the shared package
- [ ] The contract test passes, and its cases include
      "```a`b" (not an opener) and "~~~a`b" (an opener)
- [ ] `internal/lint` benchmarks show no regression
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues

[layer0]: ../internal/lint/layer0_fence.go
[lineclass]: ../internal/lint/lineclass_scan.go
[directivefiles]: ../internal/directivefiles/directivefiles.go
[listscan]: ../internal/rules/listscan/listscan.go
[reqstruct]: ../internal/rules/requiredstructure/rule.go
[headings]: ../internal/rules/include/headings.go
[slidev]: ../internal/rules/slidevstructure/rule.go
[links]: ../internal/rules/include/links.go
[fencepos]: ../internal/rules/fencepos/fencepos.go
[sitereleases]: ../internal/release/sitereleases.go
[website]: ../internal/release/website.go
