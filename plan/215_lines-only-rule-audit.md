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

74 rule files reference `ast.Walk` or `f.AST`. Three
shapes don't need the tree:

- **Pure substring scans** — fixed byte patterns
  (trailing whitespace, hard tabs, BOM). Scan
  `f.Lines` with `bytes.IndexByte`/`bytes.Contains`;
  the walk only existed to call `seg.Value(source)`
  and run the same scan per leaf.
- **Line-level structural patterns** — heading
  style, list-marker style, fence-delimiter style.
  A regex on `f.Lines[i]` reaches them; code-block
  content is skipped by tracking fence state.
- **Front-matter-only rules** — read `f.FrontMatter`
  and ignore the body.

Rules that **must** keep the AST cover four
patterns. Link rules read `*ast.Link` target and
label. Heading-scoped rules (TOC, duplicate-
heading, single-H1) need heading levels across
the document. List-nesting and blockquote-depth
rules need structural relationships. Inline
emphasis rules need the span boundaries the lexer
recovered.

### The code-block-skipping side effect

The AST gives rules an implicit filter. A rule
that walks `*ast.Paragraph` children never sees
content inside `*ast.FencedCodeBlock`,
`*ast.CodeBlock`, `*ast.HTMLBlock`, or
`*ast.CodeSpan`. Those are separate node types the
walker skips by selector. A Lines-only rewrite
loses that filter. It must reproduce skipping in
bytes — and that costs real complexity:

- Track the open fence delimiter and length. Three
  or more `` ` `` or `~`, closed by the same
  character at the same or greater length.
- Distinguish indented code: four-space prefix
  after a blank line, but not inside a list item.
- Step over HTML blocks. CommonMark defines seven
  flavors with different end conditions.
- Skip inline code spans. Backtick runs match by
  count.
- Skip autolinks and ignore raw inline HTML.

This skipping is the actual reason a tree exists.
The fix is **not** to write a second forward
scanner. We already parse the file. We project
from the AST instead. Candidates split by what
skipping they need.

**Category A (no skipping)** — the rule applies
to every line including code. Examples: MDS001
line length, BOM detection, hard-tab presence.
Direct `f.Lines` scan, no skip logic. Biggest
win per line of code.

**Category B (prose-only)** — the rule must skip
code and HTML. Examples: bare URLs, proper-name
capitalization, forbidden text in prose, most
readability rules. These rules use the projection
described below.

### Phase zero — project prose ranges from the AST

The parse already classifies every byte. Expose
that classification as a flat slice on
`lint.File`:

```go
// internal/lint/file.go
type Range struct{ Start, End int } // f.Source byte offsets

// ProseRanges returns byte ranges inside prose
// nodes (Paragraph, Heading, ListItem text,
// Blockquote text), excluding the spans of
// FencedCodeBlock, CodeBlock, HTMLBlock,
// CodeSpan, AutoLink, and inline HTML. Computed
// once per file from f.AST; memoized.
func (f *File) ProseRanges() []Range
```

One AST walk emits the slice. Every Category B
rule scans the ranges with `bytes` helpers and
never walks the AST itself. Deriving the
projection from `f.AST` rather than
re-implementing from `f.Lines` eliminates the
parallel-parser class of divergence. Bugs in
the projection walk itself can still diverge —
missed node type, wrong byte boundary — and the
fixture parity tests below gate that. No parallel
parser, no equivalence corpus, no benchmark gate.

Cost: one AST walk plus a `[]Range` per file.
Amortizes across every Category B rule (each
walks the tree itself today); the slice lands
in plan 198's arena as a single slab growth.

## Non-Goals

- Removing the AST from `lint.File`. Most rules
  still need it.
- A general "lines vs ast" abstraction. Each rewrite
  is local; no shared helper layer is introduced
  unless two converted rules share more than five
  lines of scan logic.
- Rewriting NodeChecker rules. Their walk is
  shared via the multiplex pass in
  [check.go](../internal/engine/check.go); cost
  is already amortized. Target standalone
  `Check`-implementing rules.

## Approach

### Phase one — survey

`internal/integration/rule_walk_audit_test.go`
walks every registered rule and classifies it via
two probes. `f.AST` is an exported field, not a
method, so a wrapper cannot intercept reads.
Probe one runs the rule twice — once normally,
once with `f.AST = nil`. Probe two runs the rule
on the original fixture, then on a perturbation
where only code-block content is mutated.
Together the probes yield three signals: nil-AST
safety, code-block sensitivity, and diagnostic
equality. The classification:

1. **Category A (no skipping)** — the nil-AST run
   matches the normal run, AND the code-block
   perturbation produces the same diagnostics.
   The rule was applying to every line regardless
   of code-block context. Direct `f.Lines`
   conversion, no scaffold needed.
2. **Category B (prose-only)** — the nil-AST run
   matches the normal run, but the code-block
   perturbation changes diagnostics. The rule
   needs the skipping. Rewrite drives
   `f.ProseRanges()`.
3. **AST-required** — the nil-AST run panics or
   produces different diagnostics on unperturbed
   input. Keep the AST.
4. **Hybrid** — the nil-AST run survives but
   emits a different diagnostic set on
   unperturbed input. Out of scope.

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

For each candidate, in its own commit:

1. Add a unit test asserting identical diagnostics
   on a small fixture. The fixture **must**
   include a fenced code block, an indented code
   block, an HTML block, and an inline code span
   containing the pattern the rule looks for. The
   converted rule must agree with the AST version
   byte-for-byte on those.
2. Rewrite Check. Category A uses a direct
   `f.Lines` scan with `bytes` helpers — no skip
   logic, regex compiled at package scope.
   Category B calls `f.ProseRanges()` and scans
   only the byte ranges it returns. Per-rule
   re-implementation of fence or HTML detection
   is **not** allowed; extend the projection if
   a rule needs a span it does not yet emit.
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

1. Implement the audit harness (nil-AST probe,
   code-block-perturbation probe, `go/packages`
   static scan) and land the initial manifest
   classifying each rule A / B / AST-required /
   hybrid.
2. Add `(f *File).ProseRanges()` with the AST
   projection. Memoize via the `atomic.Bool +
   sync.Mutex` pattern `newlineOffsets` and
   `codeBlockLines` use in
   [file.go](../internal/lint/file.go); the
   field comments there explain why `sync.Once`
   was rejected for per-File caches in this
   codebase. The projection inherits that
   constraint. Unit tests cover Paragraph,
   Heading, ListItem, Blockquote text, and the
   exclusions (FencedCodeBlock, CodeBlock,
   HTMLBlock, CodeSpan, AutoLink, inline HTML).
3. Convert the three highest-allocating Category
   A candidates, one commit per rule.
4. Convert Category B candidates against
   `f.ProseRanges()`, one commit per rule.
5. Run `BenchmarkCheckCorpusLarge` after each
   conversion; record cumulative wall-time and
   allocs delta. After Category A lands, tighten
   `BenchmarkCheckCorpusLarge`'s `Time` and
   `Allocs` budgets in
   [bench_test.go](../internal/engine/bench_test.go)
   to the measured new ceiling (with the same
   ~15-20 % headroom the existing budgets use) so
   the wall-time and alloc gates actually enforce
   the improvement rather than passing under the
   loose post-196 budgets.
6. Land the regression gate from phase three.
7. Update the perf guide at
   [high-performance-go.md](../docs/development/high-performance-go.md)
   with the Category A vs B guidance and a
   pointer to the manifest.

## Risk

Code-block skipping is the biggest hazard. A rule
may *appear* Lines-only on audit fixtures and
break when a real document puts the pattern
inside a fence or code span. The phase-one
perturbation probe routes such rules to Category
B, where the AST projection owns the skipping.

Projection memory is the second hazard. One
`[]Range` per file lives until the file goes out
of scope. Route the allocation through plan 198's
arena; verify `BenchmarkCheckCorpusLarge` median
allocs/op stays at or below 255 k.

Node-derived positions may differ from the
projection. The fixture-parity check gates byte-
equal column numbers; any drift forces revert or
amendment.

## Acceptance Criteria

- [ ] `internal/integration/rule_walk_audit_test.go`
      lands with the initial classification
      manifest checked in.
- [ ] At least the three highest-impact Category A
      candidates are converted, with their fixtures
      green and the audit manifest updated.
- [ ] `BenchmarkCheckCorpusLarge` p95 wall time
      improves by ≥ 5 % vs the post-198 baseline
      (237 ms → ≤ 225 ms), or the plan documents
      why the measured gain is smaller and adjusts
      the target. The new ceiling is encoded in
      `bench_test.go`'s `Time` budget.
- [ ] `BenchmarkCheckCorpusLarge` median allocs/op
      does not regress vs 255 k post-198 baseline,
      and the bench file's `Allocs` budget is
      tightened to reflect the new floor.
- [ ] A converted rule that regresses to reading
      `f.AST` fails the audit test.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues.
- [ ] `mdsmith check .` passes.
