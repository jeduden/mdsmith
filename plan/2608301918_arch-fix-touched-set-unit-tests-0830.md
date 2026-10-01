---
id: 2608301918
title: >-
  Add dedicated unit tests for the 2026-08-30 touched-set
  tax findings
status: "✅"
model: haiku
summary: >-
  WordFrequencyInto in internal/mdtext and three helpers in
  internal/directivefiles landed with only behavior-level
  coverage, not a dedicated unit test by name. Flagged by
  the 2026-08-30 audit as tax.
---
# Add dedicated unit tests for the 2026-08-30 touched-set tax findings

## Goal

Give each function below its own unit test by name. Match
the `TestFoo` convention [tests.md][tests] requires — the
one every untouched sibling package already follows.

## Background

The 2026-08-30 audit (see [the audit log][audit-log]) swept
every file touched since the prior checkpoint. It found four
functions with no test carrying their own name. Each is
exercised only indirectly through a caller's scenario test:

- [internal/mdtext/wordfreq.go][wordfreq]:31 —
  `WordFrequencyInto`, the zero-allocation inner loop that
  accumulates word counts into a caller-owned, caller-cleared
  map across multiple scope units. Covered only via
  `WordFrequency`'s own tests in
  [wordfreq_test.go][wordfreq-test], none of which call
  `WordFrequencyInto` directly across repeated
  accumulate/clear cycles — the exact zero-alloc-reuse
  behavior it exists for.
- [internal/directivefiles/directivefiles.go][directivefiles]:190,226,171 —
  `openingFence`, `isClosingFence`, and `isIndentedCodeBlock`,
  the fence-tracking helpers `hasDirectiveMarker` uses to skip
  directive-marker matches inside code blocks. Covered only
  indirectly via the `TestDiscoverFiles_Ignores*` and
  `TestHasDirectiveMarker_*` fixture tests in
  [directivefiles_test.go][directivefiles-test].

Both are `tax`: neither sits on a public surface by itself
(their callers already have tests).

## Tasks

1. Add `TestWordFrequencyInto` to
   [wordfreq_test.go][wordfreq-test], calling
   `WordFrequencyInto` directly across at least two
   accumulate/`clear`/accumulate cycles on one reused map, so
   the test exercises the reuse contract the doc comment
   describes rather than a single one-shot call.
2. Add `TestOpeningFence`, `TestIsClosingFence`, and
   `TestIsIndentedCodeBlock` to
   [directivefiles_test.go][directivefiles-test] as
   table-driven tests covering the fence-character and
   run-length matching each helper performs.
3. Do not delete or duplicate the existing behavior-level
   tests named above. They cover the public contract and
   stay as-is.
4. `go build ./...` passes.
5. `go test ./internal/mdtext/... ./internal/directivefiles/...`
   passes.
6. Deviation found while writing `TestOpeningFence`:
   `openingFence` accepted a backtick fence whose info
   string holds a backtick (such as ```` ```x``` ````).
   goldmark and `internal/lint` read that line as an inline
   code span. Discovery then hid every directive marker
   below it. Fixed in place, with a red/green
   `TestHasDirectiveMarker_BacktickInInfoIsNotAFence`.
   No production code calls `DiscoverFiles` today
   (`deadcode` reports the package unreachable), so the
   fix has no user-visible effect.
7. The same backtick-info gap sat in three rules' own
   fence scanners. Each is fixed with a red/green test,
   and each fix changes rule output. In `include`
   (MDS021), heading shifts and link rewrites now
   continue past a ```` ```x``` ```` line. In
   `required-structure` (MDS020), body-sync collection
   skips directive blocks after such a line. In
   `slide-structure` (MDS073), such a line no longer
   toggles code-block state.
8. Code review of those scanners found more fence gaps,
   each fixed with a red/green test. `include` no longer
   reads `---` after an ATX heading, a fence line, or a
   setext underline as a setext underline, and a CRLF
   closing fence now closes. `slide-structure` tracks
   the opener's character and length, so a shorter inner
   fence is code content. `required-structure` treats a
   tab-indented fence line as indented code. The release
   website link rewriter's `fenceMarker` got the same
   backtick-info fix.

## Acceptance Criteria

- [x] Every function named in the Background section has a
      test carrying its own name.
- [x] `go test ./...` is green.
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [x] `mdsmith check .` is green.

[audit-log]: ../docs/development/architecture-audit.md
[tests]: ../docs/development/architecture/tests.md
[wordfreq]: ../internal/mdtext/wordfreq.go
[wordfreq-test]: ../internal/mdtext/wordfreq_test.go
[directivefiles]: ../internal/directivefiles/directivefiles.go
[directivefiles-test]: ../internal/directivefiles/directivefiles_test.go
