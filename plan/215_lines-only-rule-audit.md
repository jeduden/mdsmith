---
id: 215
title: Audit AST-walking rules and rewrite the ones that only need f.Lines
status: "✅"
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

The AST gives rules an implicit filter: a rule that
walks `*ast.Paragraph` never sees `*ast.FencedCodeBlock`
or `*ast.CodeSpan` content. A Lines-only rewrite loses
that filter and would have to re-track fences, indented
code, HTML blocks, and code spans by hand — the very
complexity the tree exists to absorb. So Category B
rules project from the AST instead, via
`(f *File).ProseRanges() []Range` (landed in Task 2):
one memoized walk emits the prose byte ranges, excluding
code/HTML spans, and each rule scans those ranges with
`bytes` helpers. No parallel parser. **Category A** rules
(line length, BOM, hard tabs) need no skipping and scan
`f.Lines` directly.

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

The planned per-candidate steps were a parity unit
test (the pattern inside a fenced block, indented block,
HTML block, and code span), a Check rewrite, then a
fixture and alloc-budget recheck. Category A scans
`f.Lines`; Category B drives `f.ProseRanges()`. **No
candidate qualified** (Tasks 3–4, Risk), so this phase
shipped no rewrites.

### Phase three — gate

`TestRuleWalkAuditManifest` fails any rule that gains
an `f.AST` access without a manifest update. That keeps
the classification accurate. `TestPerRuleBenchBudget`
(Task 5) is the complementary per-rule cost gate.

## Tasks

1. [x] Audit harness (nil-AST probe, code-block-
   perturbation probe, `go/packages` static scan) plus
   the initial manifest. In `rule_walk_audit_test.go`;
   manifest `testdata/rule_walk_audit.json`.
2. [x] Add `(f *File).ProseRanges()` with the AST
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
   HTMLBlock, CodeSpan, AutoLink, inline-HTML
   tags — but not the visible text those tags wrap).
3. [x] Convert the highest-impact candidates, one commit
   per rule. **No conversions ship.** The audit manifest
   (`testdata/rule_walk_audit.json`) shows Category A
   holds no AST-walking rules left (plans 175/195/196
   already made the substring rules Lines-only). Finding
   recorded, not a code change.
4. [x] Convert Category B candidates against
   `f.ProseRanges()`, one commit per rule. **No
   conversions ship.** The cleanly-convertible
   Category-B set is exhaustively empty: every
   nil-AST-safe-but-code-sensitive rule that remains
   either reads link/label structure, drives an AST
   `Fix`, or is already Lines-only. `ProseRanges` landed
   (Task 2) as the projection a future conversion would
   target, but no current standalone-`Check` rule
   converts cleanly. See the exhaustive standalone-AST-
   rule finding in commits `19f94900` / `b224facb`.
5. [x] **Substitute deliverable (the conversion premise
   is dead, so the perf evidence moves here):** a
   per-opt-in-rule isolated benchmark + alloc+time gate
   in
   [perrule_bench_test.go](../internal/integration/perrule_bench_test.go),
   sitting alongside the existing gates (none removed or
   loosened). `BenchmarkOptInRule` reports each opt-in
   rule's isolated `Check` ns/op + allocs/op;
   `TestPerRuleBenchBudget` pins both a parse-subtracted
   allocs/op ceiling and a total parse+Check ns/op
   ceiling (~5x headroom) per rule, each its own subtest.
   Opt-in rules are enumerated programmatically from
   `rule.All()` (implements `rule.Defaultable` &&
   `!EnabledByDefault`), never hardcoded. See the
   baseline table below.
6. [x] Land the regression gate from phase three. The
   audit-manifest gate (`TestRuleWalkAuditManifest`,
   commit `1e655fc1`) fails a converted rule that
   regresses to `f.AST`; `TestPerRuleBenchBudget` is the
   complementary per-opt-in-rule cost regression gate.
7. [x] Update the perf guide at
   [high-performance-go.md](../docs/development/high-performance-go.md)
   with the Category A vs B guidance, a pointer to the
   manifest, and the per-opt-in-rule benchmark
   convention (how to pin a new rule's alloc+time
   budget).

## Risk

**Measured 2026-05-29: the ≥5 % wall-time target is
not reachable by the available conversions.** The
manifest found no AST-walking Category A rules. The
Category B prose rules are opt-in, so they never run
in `BenchmarkCheckCorpusLarge`. The one default rule,
MDS054, contributes ~11 ms and ~0.3 % of allocs.
`ProseRanges` landed regardless; the engine-wide lever
this plan assumed was already harvested by plans
175/195/196. See return notes.

Code-block skipping is the standing hazard for a
future conversion. A rule may *appear* Lines-only on
fixtures yet break on a pattern inside a fence. The
perturbation probe routes such rules to Category B;
`ProseRanges` owns the skipping. Node positions are
gated byte-equal by parity fixtures.

## Per-opt-in-rule baselines

Measured 2026-05-29 on a 4-core dev box. The fixture is
`perRuleBenchDoc`, a ~240-line compliant doc. Columns are
total parse+Check ns/op (the ~170 µs parse is constant
across rules) and parse-subtracted allocs/op. Ceilings are
pinned at `Time` ≈ 5x baseline and `Allocs` ≈ baseline +
max(20 %, 4). All 26 opt-in rules:

| Rule   | Name                       | ns/op   | allocs/op |
| ------ | -------------------------- | ------- | --------- |
| MDS024 | paragraph-structure        | ~192 µs | 36        |
| MDS029 | conciseness-scoring        | ~178 µs | 24        |
| MDS033 | directory-structure        | ~166 µs | 0         |
| MDS034 | markdown-flavor            | ~197 µs | 0         |
| MDS035 | toc-directive              | ~228 µs | 84        |
| MDS036 | max-section-length         | ~193 µs | 0         |
| MDS037 | duplicated-content         | ~241 µs | 108       |
| MDS041 | no-inline-html             | ~185 µs | 0         |
| MDS042 | emphasis-style             | ~176 µs | 0         |
| MDS043 | no-reference-style         | ~477 µs | 320       |
| MDS044 | horizontal-rule-style      | ~174 µs | 0         |
| MDS045 | list-marker-style          | ~184 µs | 1         |
| MDS046 | ordered-list-numbering     | ~175 µs | 0         |
| MDS047 | ambiguous-emphasis         | ~165 µs | 0         |
| MDS048 | git-hook-sync              | ~172 µs | 0         |
| MDS049 | no-space-in-link-text      | ~183 µs | 1         |
| MDS050 | proper-names               | ~165 µs | 0         |
| MDS051 | single-h1                  | ~176 µs | 1         |
| MDS052 | no-space-in-code-spans     | ~177 µs | 0         |
| MDS055 | forbidden-paragraph-starts | ~179 µs | 0         |
| MDS056 | forbidden-text             | ~174 µs | 0         |
| MDS057 | required-text-patterns     | ~171 µs | 0         |
| MDS058 | required-mentions          | ~172 µs | 0         |
| MDS063 | descriptive-link-text      | ~179 µs | 36        |
| MDS067 | callout-type               | ~182 µs | 8         |
| MDS068 | link-style                 | ~172 µs | 0         |

MDS043 is the outlier: it parses the source a second time
via `LinkReferences`, so its ceilings are 2.5 ms / 384.

## Acceptance Criteria

- [x] `internal/integration/rule_walk_audit_test.go`
      lands with the initial classification
      manifest checked in.
- [x] ~~At least the three highest-impact Category A
      candidates are converted~~ — **superseded.** The
      manifest proves the cleanly-convertible Category-A
      *and* Category-B sets are exhaustively empty
      (plans 175/195/196 already moved the substring
      rules), so no conversions ship. Substitute
      deliverable: the per-opt-in-rule benchmark + gate
      suite (Task 5).
- [x] ~~`BenchmarkCheckCorpusLarge` p95 improves ≥ 5 %~~
      — **not reachable**, documented under Risk (no
      AST-walking Category A rules; Category B is opt-in
      and absent from the corpus bench). The wall-time
      target is withdrawn; the existing `Time` budget is
      left intact, not loosened.
- [x] ~~`BenchmarkCheckCorpusLarge` median allocs/op
      does not regress~~ — preserved: no rule `Check`
      changed, so the corpus bench `Allocs` budget is
      untouched and still green.
- [x] A rule that regresses to reading `f.AST` fails the
      audit test (`TestRuleWalkAuditManifest`); the new
      `TestPerRuleBenchBudget` adds a per-opt-in-rule
      cost regression gate alongside it.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no
      issues.
- [x] `mdsmith check .` passes.
