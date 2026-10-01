---
id: 2608301918
title: >-
  Add dedicated unit tests for the 2026-08-30 touched-set
  tax findings
status: "✅"
model: haiku
summary: >-
  WordFrequencyInto in internal/mdtext and three fence
  helpers in internal/directivefiles had no dedicated unit
  test. The first got one; the second package turned out
  to be dead and was deleted, and its fence logic was
  consolidated into internal/mdfence.
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
- `internal/directivefiles/directivefiles.go` —
  `openingFence`, `isClosingFence`, and `isIndentedCodeBlock`,
  the fence-tracking helpers `hasDirectiveMarker` used to skip
  directive-marker matches inside code blocks. Covered only
  indirectly via the package's fixture tests.

Both are `tax`: neither sits on a public surface by itself
(their callers already have tests).

## Tasks

1. [x] Add `TestWordFrequencyInto` to
   [wordfreq_test.go][wordfreq-test], calling
   `WordFrequencyInto` directly across at least two
   accumulate/`clear`/accumulate cycles on one reused map.
2. [x] Add table-driven tests for the three
   `directivefiles` fence helpers. Done first, then made
   moot by task 5.
3. [x] Writing those tests found a bug: a backtick fence
   whose info string holds a backtick (```` ```x``` ````)
   was read as a fence opener. goldmark reads it as an
   inline code span. The same bug sat in the fence
   scanners of `include` (MDS021), `required-structure`
   (MDS020), `slide-structure` (MDS073), and the release
   website link rewriter. Each was fixed with a red/green
   test, and each fix changes that rule's output.
4. [x] Code review of those scanners found more gaps, each
   fixed with a red/green test. `include` no longer reads
   `---` after an ATX heading, a fence line, or a setext
   underline as a setext underline, and a CRLF closing
   fence now closes. `slide-structure` tracks the
   opener's character and length. `required-structure`
   treats a tab-indented fence line as indented code.
5. [x] Deviation: delete `internal/directivefiles` and
   `cmd/mdsmith/discover.go`. History shows discovery lost
   its last production caller in PR #213 (2026-05-02),
   when `merge-driver install` moved to config-derived
   globs and the pre-merge-commit hook to a fixed script.
   Only tests called it since. No plan needs it.
6. [x] Deviation: the fence open/close logic was copied
   into about eight packages. It now lives in one
   zero-allocation leaf package,
   [internal/mdfence][mdfence], with `Open`, `Close`, and
   a `Tracker`, each with its own table-driven test.
   `internal/lint`, `listscan`, `include`,
   `required-structure`, `slide-structure`,
   `fenced-code-language`, and the release website
   rewriter call it. Their local copies are gone.
7. [x] Two follow-up gaps, each fixed red/green against
   the AST. `include` no longer reads an HTML or PI
   line as setext text, nor shifts headings inside an
   HTML block; a new leaf package, `internal/mdhtml`,
   classifies HTML block starts. `mdfence.OpenFinal`
   drops a one-byte info string on a final line with
   no newline, as goldmark does.
8. [x] `go build ./...` and `go test ./...` pass.

## Acceptance Criteria

- [x] `WordFrequencyInto` has a test carrying its own name;
      the `directivefiles` helpers are deleted with their
      package, and their replacement `mdfence` functions
      each have one.
- [x] `go test ./...` is green.
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [x] `mdsmith check .` is green.

[audit-log]: ../docs/development/architecture-audit.md
[tests]: ../docs/development/architecture/tests.md
[wordfreq]: ../internal/mdtext/wordfreq.go
[wordfreq-test]: ../internal/mdtext/wordfreq_test.go
[mdfence]: ../internal/mdfence/mdfence.go
