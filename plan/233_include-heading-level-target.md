---
id: 233
title: numeric heading-level target for include
status: "✅"
summary: >-
  Extend the `<?include?>` directive's `heading-level`
  parameter (MDS021) to accept an integer 1-6 that pins
  the shallowest embedded heading to that level, beside
  the existing `"absolute"` keyword. A pinned target is
  robust to source-file changes where `heading-offset`
  (a delta) is not.
model: opus
depends-on: [232]
---
# numeric heading-level target for include

## Goal

Let `heading-level` name an explicit target level, not
just the `"absolute"` keyword. `heading-level: "2"` pins
the shallowest included heading to level 2, whatever the
source starts at.

## Context

The directive now offers two heading strategies. Plan 232
added `heading-offset`. This plan adds a third form so the
trio is complete:

- `heading-level: "absolute"` — parent-relative. Nests the
  embed one level under the nearest preceding heading. At
  the document root it is a no-op (no parent to read).
- `heading-level: "N"` — target. Pins the shallowest
  embedded heading to level N, regardless of the source or
  any parent. This plan.
- `heading-offset: "N"` — source-relative. Adds N to every
  heading. The result tracks the source's own levels.

`"absolute"` already computes a target: it shifts so the
shallowest heading becomes `parentLevel + 1`. A numeric
form is the same machinery with the target stated outright
instead of derived from the parent. So the value reads as
"what level should this land at" — one inferred, one
explicit.

Why keep all three instead of one? It is an owner call. A
target is robust when the source changes: it always lands
at N. A delta keeps a hand-set relationship instead. And
`"absolute"` is the no-setup default for the common case —
nesting under the current section.

## Design

### Syntax

```text
<?include
file: features.md
heading-level: "2"
?>
## Pinned To Level 2
<?/include?>
```

`heading-level` accepts `"absolute"` or an integer from 1
to 6. The two heading parameters stay mutually exclusive:
`heading-level` (either form) cannot pair with
`heading-offset`, and neither pairs with `extract:`.

### Shift

For target N the shift is `N - minLevel`, where `minLevel`
is the shallowest heading in the source. A new
[`adjustHeadingsToLevel`][headings] reuses the existing
`findMinHeadingLevel` and `applyShift` helpers. It clamps
to 1-6, allows promotion (negative shift), and no-ops when
the shift is zero or the source has no heading. It omits
the `parentLevel <= 0` and `shift <= 0` guards that
`adjustHeadings` needs only for parent-relative nesting.

`"absolute"` behavior is unchanged. The numeric path is a
new branch. So tracked files that already use
`heading-level: "absolute"` produce the same output under
both the branch binary and the pinned one.

### Validation

- `heading-level` must be `"absolute"` or an integer 1-6.
  Anything else is a lint error. The message becomes
  `"heading-level" must be "absolute" or an integer
  between 1 and 6`.
- The existing mutual-exclusion checks already key on the
  presence of the `heading-level` parameter, so they cover
  the numeric form with no change.

### Pinned baseline

The new value is new directive syntax. Per
[the adopt process][adopt], no tracked Markdown may use
`heading-level: "<number>"` until a release ships it and
`setup-mdsmith-pinned-version` is bumped, or
`mdsmith-fixed-version` fails. This plan ships the feature
only: the numeric form appears solely in ignored fixtures
and in fenced doc examples, never as a live directive.

[headings]: ../internal/rules/include/headings.go
[adopt]: ../docs/development/adopt-new-directive-syntax.md

## Tasks

1. [x] Add `adjustHeadingsToLevel(content, target)` to
   [`headings.go`][headings], reusing `findMinHeadingLevel`
   and `applyShift`. Unit tests for pin, promote, clamp,
   zero-shift no-op, no-heading, and setext land first,
   red.
2. [x] Accept an integer 1-6 for `heading-level` in
   `validateIncludeDirective`; update the error message. A
   unit test pins the valid and invalid cases.
3. [x] Apply the numeric target in `processIncludedContent`
   via `adjustHeadingsToLevel`, leaving the `"absolute"`
   branch untouched.
4. [x] Add a good/bad/fixed fixture trio under
   [MDS021-include][dir] exercising `heading-level: "2"`.
5. [x] Update the rule README ([MDS021-include][readme]):
   the parameter table, the heading-level section, an
   example, and the new diagnostic message row.
6. [x] Update the directive guide ([generating][guide])
   and the built-in help ([help][help]).
7. [x] Run `go test ./...`, `go tool golangci-lint run`,
   the allocation-budget test, and `mdsmith check .`.
   Confirm `mdsmith check .` shows no churn in the tracked
   files that use `heading-level: "absolute"`.

[dir]: ../internal/rules/MDS021-include/
[readme]: ../internal/rules/MDS021-include/README.md
[guide]: ../docs/guides/directives/generating-content.md
[help]: ../internal/directives/generating-content.md

## Deferred — gated on the pin bump

8. [x] Adopt `heading-level: "<number>"` in tracked
   Markdown. The gate cleared: the feature shipped in
   v0.32.0 and the pinned baseline is now v0.33.0.
   `README.md` drops its top-level H1 — the logo lockup
   is the title — and pins the features `<?include?>` to
   `heading-level: "2"`. A `README.md`-only
   `first-line-heading: false` override covers the now
   intentionally H1-less file.

## Acceptance Criteria

- [x] `heading-level: "2"` pins the shallowest embedded
      heading to level 2 on `mdsmith fix`, even at the
      document root
- [x] `heading-level: "1"` promotes a source that starts
      deeper, clamped at level 1
- [x] `heading-level: "absolute"` output is unchanged for
      existing tracked includes (no churn)
- [x] A non-integer or out-of-range `heading-level` is a
      lint error naming the allowed values
- [x] `heading-level` numeric still cannot pair with
      `heading-offset` or `extract`
- [x] `README.md` adopts the numeric form; the pinned
      baseline (v0.33.0) parses it, so
      `mdsmith-fixed-version` stays green
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
