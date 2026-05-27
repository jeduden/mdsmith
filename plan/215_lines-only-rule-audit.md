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
Reproducing skipping in every rule duplicates
parser work and risks divergence from goldmark.
So candidates split by what skipping they need.

**Category A (no skipping)** — the rule applies
to every line including code. Examples: MDS001
line length, BOM detection, hard-tab presence.
Direct `f.Lines` scan, no skip logic. Biggest win
per line of code.

**Category B (prose-only)** — the rule must skip
code and HTML. Examples: bare URLs, proper-name
capitalization, forbidden text in prose, most
readability rules. A correctness-equivalent
rewrite needs a shared scaffold.

Category B is only worth converting if the
scaffold is cheaper than the AST walk. If it has
to re-implement half of CommonMark block
parsing, the rule paid for parsing twice and the
saving evaporates.

### Phase zero — the prose-scanner scaffold

Before any Category B conversion, build
`internal/lint/prosescan/` — one forward pass
over `f.Lines` that yields prose byte ranges
stripped of fenced code, indented code, HTML
blocks, code spans, autolinks, and inline HTML.
Zero allocations per call (state on the stack,
output via callback). It is tested against a
CommonMark fixture corpus whose ground truth
comes from an AST walk over the same input.

The phase-zero benchmark is the gate. If
`prosescan` lands under 50 % of the AST walk's
CPU per file, it becomes the substrate for
Category B. Otherwise **Category B is dropped**:
only Category A ships and the perf target
shrinks. No Category B rule converts before the
gate passes.

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
   needs the skipping. Requires the phase-zero
   `prosescan` scaffold; convert only if
   phase-zero's perf gate passes.
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
2. Rewrite Check. A Category A rewrite uses a
   direct `f.Lines` scan with `bytes` helpers, no
   skip logic, and any regex compiled at package
   scope. A Category B rewrite drives the
   `prosescan` package and scans only the prose
   ranges it yields. Per-rule code that
   re-implements fence or HTML detection is **not**
   allowed; the scaffold is the single owner of
   that work. If a rule needs skipping the
   scaffold does not yet provide, extend the
   scaffold.
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
   with each rule classified A, B, AST-required,
   or hybrid.
2. Convert the three highest-allocating Category
   A candidates, one commit per rule.
3. Build the phase-zero `prosescan` package with
   the CommonMark-equivalence fixture corpus and
   the zero-allocation guarantee. Benchmark
   against the equivalent AST walk.
4. If `prosescan` benchmarks under 50 % of the
   AST walk's CPU per file, convert Category B
   candidates one commit per rule. If it lands
   at parity or worse, close out after Category
   A — drop the scaffold if it cannot beat the
   walk. Record the decision and numbers here.
5. Run `BenchmarkCheckCorpusLarge` after each
   conversion; record the cumulative wall-time
   and allocs delta.
6. Land the regression gate from phase three.
7. Update the perf guide at
   [high-performance-go.md](../docs/development/high-performance-go.md)
   with the Category A vs B guidance and a
   pointer to the manifest.

## Risk

The code-block-skipping side effect is the
biggest hazard. A rule may *appear* Lines-only on
audit fixtures, then break when a real document
puts the pattern inside a fence or code span. The
phase-one perturbation probe is the defense. A
rule whose diagnostics change when only the
code-block content changes is routed to Category
B. The scaffold owns the skipping there.

The phase-zero scaffold itself is the second
hazard. If `prosescan` disagrees with goldmark on
any CommonMark corner case (lazy continuation
inside a list item, a fence opened by a tab, an
HTML block whose end condition matches inside the
block), every Category B conversion inherits the
bug. The equivalence fixture corpus is the
defense — every fixture's prose ranges must
byte-match what an AST walk over the same input
produces.

The scaffold may also fail to beat the AST walk.
Phase-zero's gate exists precisely to surface
that early. If it fails, Category B stays on the
AST and we ship a smaller win.

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
