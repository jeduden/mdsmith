---
id: 215
title: Audit AST-walking rules and rewrite the ones that only need f.Lines
status: "🔲"
model: opus
depends-on: [198]
summary: >-
  ~74 rule files walk the AST via ast.Walk or read
  f.AST. A subset of those only inspect inline text
  patterns (trailing space, bare URLs, fixed
  substrings) and never use structural metadata. Move
  that subset onto f.Lines + bytes scans so their
  Check call skips the AST walk entirely. Targets the
  70 % check slice that arena-mode (plan 198) left
  untouched.
---
# Audit AST-walking rules and rewrite the ones that only need f.Lines

## Goal

Identify every rule whose Check walks the AST but
never reads structural information (node kind,
parent, sibling order, position relative to other
nodes, link target vs label, code-span vs text).
Rewrite those rules to scan
[f.Lines](../internal/lint/file.go) with `bytes`
helpers instead. The AST walk skipped per rule per
file is pure CPU saved.

## Background

Plan 198's arena cut allocations 54 % on the large
corpus benchmark. Wall-time p95 moved from 247 ms to
237 ms — a smaller fraction of the allocation win,
because parse is only ~30 % of CPU on the same
corpus (measured 2026-05-24:
parse 29.3 % / check 70.7 %). The remaining lever
inside the check slice is **walks that did not need
to happen**.

74 rule files reference `ast.Walk` or `f.AST`. A
spot-check of the candidates suggests at least three
shapes that don't need the tree:

- **Pure substring scans.** Rules that look for a
  fixed byte pattern anywhere in the document
  (trailing whitespace, hard tabs, BOM, specific
  characters) can scan `f.Lines` with
  `bytes.IndexByte` or `bytes.Contains`. The walk
  visits every node only to call `seg.Value(source)`
  and run the same substring search per leaf.
- **Line-level structural patterns.** Rules that
  check the *first byte(s)* of a line (heading-style
  by `#` count, list-marker style, fence delimiter
  style) read information the AST extracts from the
  same line. A regex on `f.Lines[i]` reaches it
  without the walk — and skips code-block content,
  which the AST already skipped, by tracking fence
  state inline.
- **Front-matter-only rules.** Rules that read only
  `f.FrontMatter` and ignore the body never need
  AST.

Rules that **must** keep the AST fall in four
buckets. Link rules read `*ast.Link` for target vs
label. TOC, duplicate-heading, and single-H1 read
heading levels across the document. List-nesting
and blockquote-depth rules read structural
relationships. Inline emphasis rules read span
boundaries the lexer recovered.

## Non-Goals

- Removing the AST from `lint.File`. Most rules
  still need it.
- A general "lines vs ast" abstraction. Each rewrite
  is local; no shared helper layer is introduced
  unless two converted rules share more than five
  lines of scan logic.
- Rewriting NodeChecker rules. Their walk is shared
  via the multiplex pass in
  [internal/engine/check.go](../internal/engine/check.go)
  and per-rule walk cost is already amortized.
  Standalone `Check`-implementing rules are the
  target.

## Approach

Three phases.

### Phase one — survey

A single PR adds
`internal/integration/rule_walk_audit_test.go`. It
walks every registered rule and classifies it. The
detector cannot wrap `f.AST` (it is an exported
field on `lint.File`, not a method, so reads cannot
be intercepted). Instead the harness runs each rule
against a fixture set twice: once normally, once
with `f.AST` set to nil. The classification:

1. **Lines-only candidate** — the nil-AST run
   produces identical diagnostics to the normal
   run (and does not panic).
2. **AST-required** — the nil-AST run panics on
   the nil dereference or produces different
   diagnostics.
3. **Hybrid** — the nil-AST run succeeds but emits
   a different diagnostic set. Convert later, if
   ever; not in scope.

A static check complements the runtime probe. It
uses `go/packages` over `internal/rules/`. The
check records which rule packages reference
`ast.Walk` or `f.AST`. The manifest carries both
signals.

The audit emits a JSON manifest. The path is
`internal/integration/testdata/rule_walk_audit.json`.
Each rule appears with its classification. Phase
two uses the manifest as a work queue. Phase three
uses it to gate regressions.

### Phase two — rewrite

For each Lines-only candidate, in its own commit:

1. Add a unit test asserting identical diagnostics
   on a small fixture.
2. Rewrite Check to scan `f.Lines` with `bytes`
   helpers. Track fenced-code state inline if the
   rule should skip code blocks (most do). Compile
   any regex at package scope.
3. Confirm the rule's `bad/` and `good/` fixtures
   under `internal/rules/MDS###-*/` still pass.
4. Re-run the allocation-budget test at
   [alloc_budget_test.go](../internal/integration/alloc_budget_test.go).
   A converted rule should land at 0–2 allocs/call.

### Phase three — gate

Extend the audit test from phase one to assert that
no rule on the Lines-only list regresses to touching
`f.AST` without a corresponding manifest update.
The same test fails a new AST access in a converted
rule, so the manifest stays accurate.

## Tasks

1. Implement the audit harness — the nil-AST
   runtime probe plus the `go/packages` static
   scan over `internal/rules/`. Land the initial
   manifest.
2. Pick the three highest-allocating Lines-only
   candidates from the manifest and convert them
   one commit per rule, each with the unit test
   precondition.
3. Run `BenchmarkCheckCorpusLarge` after each
   conversion; record the cumulative wall-time and
   allocs delta in this plan.
4. Convert the long tail of Lines-only candidates,
   batched by rule package. Same commit shape.
5. Land the regression gate from phase three so
   the manifest is enforced.
6. Update the perf guide at
   [high-performance-go.md](../docs/development/high-performance-go.md)
   with the "prefer f.Lines for pure pattern scans"
   guideline and a pointer to the manifest.

## Risk

A rule may *appear* to be Lines-only but rely on
the AST's code-block-skipping side effect. The unit
test on a fixture containing a fenced code block
with the pattern inside catches this — required
before conversion.

Node-derived positions may differ from the Lines
path. The fixture-parity assertion catches drift.
It checks byte-equal column numbers. Any drift
forces revert or a deliberate plan amendment.

## Acceptance Criteria

- [ ] `internal/integration/rule_walk_audit_test.go`
      lands with the initial classification
      manifest checked in.
- [ ] At least the three highest-impact Lines-only
      candidates are converted, with their fixtures
      green and the audit manifest updated.
- [ ] `BenchmarkCheckCorpusLarge` p95 wall time
      improves by ≥ 5 % vs the post-198 baseline
      (237 ms → ≤ 225 ms), or the plan documents
      why the measured gain is smaller and adjusts
      the target.
- [ ] `BenchmarkCheckCorpusLarge` median allocs/op
      does not regress vs 255 k post-198 baseline.
- [ ] A converted rule that regresses to reading
      `f.AST` fails the audit test.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues.
- [ ] `mdsmith check .` passes.
