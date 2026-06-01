---
id: 188
title: Regex-over-source rules — inventory and AST-resident replacements
status: "🔳"
model: opus
depends-on: []
summary: >-
  Plan 187 named "per-file regexp scans (nounusedlinkdefinitions ~70 ms)
  and per-file full-source regexp scans" as intrinsic without enumerating
  them. This plan inventories every rule that compiles or runs a regex
  over `f.Source`, identifies which have an AST-resident equivalent
  already produced during goldmark's canonical parse, and schedules the
  conversions. The lever is compounding: many small wins (5–10% each on
  prose-heavy or link-heavy input) the existing land-and-skip framing
  rejected one-by-one.
---
# Regex-over-source rules — inventory and AST-resident replacements

## Goal

Replace every per-file regex pass over `f.Source` with an
AST-resident lookup. The condition: the same information must
already be produced by goldmark's canonical parse. The shipped
`File.Memo` and `f.LinkReferences()` cache prove the pattern.
MDS053 and MDS054 already use it. Extend the pattern across the
regex-heavy tail plan 187's CPU profile names.

## Background

Plan 187's neutral CPU profile attributes ~70 ms of the 280 ms
neutral-corpus check time to `nounusedlinkdefinitions`'s
full-source regex scan. It also notes "per-file full-source
regexp scans" more generally without enumerating them. The plan
calls the cost "intrinsic" and stops there. The premise is wrong
on two counts:

1. **AST already carries most of what these regexes look for**.
   goldmark's parser produces typed nodes for headings, code
   fences, link references, list markers, blockquotes, and tables.
   A regex scanning `f.Source` for `^#{1,6} ` is reproducing the
   work the parser already did and discarded.
2. **The work isn't always pure**. `regexp.MustCompile` is cheap
   on the first call, free thereafter. But matching against
   `f.Source` per rule per file is N rules × M files × |Source|
   bytes. One parse is already shared across all rules.

[Plan 175](175_check-performance-gate.md) added
`f.LinkReferences()` to read goldmark's parse context once.
MDS053 and MDS054 stopped each re-parsing the document. The
template here is the same. Identify regex-only rules. Find the
AST node type each regex is approximating. Route through a
memoized per-File accessor — existing or new.

## Tasks

1. [x] Create this plan.
2. [x] Inventory every rule whose `Check` (or helpers called only
   from `Check`) calls `regexp.MatchString`, `regexp.FindAll*`, or
   compiles a package-level regex against `f.Source`. List the
   rule ID, the regex pattern, the AST node type that carries the
   same information (or "no AST equivalent — true regex"), and
   estimated cumulative cost from a fresh `pprof` over the neutral
   corpus. Record under `## Inventory` below.
3. [ ] For each rule with an AST equivalent, write the failing
   conversion test: feed a fixture that the regex catches, switch
   the rule to walk the AST node type (via a memoized
   `astutil`-level accessor if shared with other rules), and pin
   byte-identical diagnostics against the regex implementation.
4. [ ] Land conversions in batches keyed by AST node type
   (heading-based rules together, code-fence-based rules together,
   etc.) so each batch shares one new `astutil` accessor when
   useful.
5. [ ] After each batch, re-profile and record the new
   `BenchmarkCheckCorpus{Small,Large}` numbers. Reject any batch
   that regresses either benchmark beyond noise.
6. [ ] For rules with no AST equivalent (true regex work — e.g.
   placeholder grammar checks, content-pattern rules like
   `proper-names`, `forbidden-text`), record them as "kept regex"
   with the reason. They are not the lever.

## Inventory

Authoritative as of this branch's base (origin/main). Every rule
whose non-test code imports `regexp` was read. The test that
decides conversion is **what the regex matches against**, not
whether `regexp` is imported:

- A regex run against a **per-line slice** (`f.Lines[i]`) or against
  **AST-derived text** (a heading's plain text, a paragraph's
  extracted text, a table cell, a code-fence info string, a user
  recipe command) does not re-derive block structure. The parse
  result is already its input. These are not the lever.
- A regex run against the **whole `f.Source`** with a multiline
  (`(?m)`) anchor to rediscover line or block structure *is* the
  lever. goldmark's canonical parse already produced that
  structure and discarded the re-scan's answer.

### The lever (full-source structural re-scan → AST-resident)

One rule qualifies: **MDS043 `no-reference-style`**, in
`collectReferenceDefinitions`.

- **Pattern.** It runs a **second full `goldmark` parse**
  (`lint.NewParser().Parse`) to read `ctx.References()`. It then
  runs `refDefRE` over all of `f.Source` to locate each definition.
  `refDefRE` is `` (?m)^[ ]{0,3}\[([^\]\n]+)\]:[ \t]*\S+.*$ ``.
- **AST equivalent.** `f.LinkReferences()` returns the
  canonical-parse reference set, already memoized on the `*File`.
  The locate step routes through the hand-rolled `scanRefDefLine`
  byte scanner. MDS053 already proved that scanner equal to this
  exact regex, byte for byte.
- **Cost.** About 468 µs and 537 allocs per `Check` on the per-rule
  bench doc (`BenchmarkOptInRule/MDS043`). The rule's own budget
  comment attributes the bulk to the second parse.

MDS043 is the **only** rule on this branch still doing a redundant
second parse. The check `grep -rn 'NewParser().Parse\|ParseContext'
internal/rules/*/rule.go` returns MDS043 alone. It is also the only
rule running a `(?m)` regex over `f.Source` to re-derive line
structure (`refDefRE`, plus `footnoteDefRE`, which is true regex —
see below).

### Already converted — the realized template (no work)

- MDS053 `no-unused-link-definitions` and MDS054
  `no-undefined-reference-labels` — plan 195 (#387) already routed
  both through `f.LinkReferences()` + a hand-rolled `scanRefDefLine`
  byte scanner. Neither imports `regexp`. They are the pattern this
  plan extends, exactly as the Goal states. Plan 187's "~70 ms
  `nounusedlinkdefinitions` full-source regex scan" no longer
  exists on this branch.

### Kept regex — no AST equivalent (true regex), per task 6

These run `regexp`, but not over `f.Source` to re-derive structure,
so the AST carries nothing to route them through:

- MDS043 footnote scans — `footnoteRefRE` = `` \[\^([^\]\n]+)\] ``
  and `footnoteDefRE` = `` (?m)^[ ]{0,3}\[\^([^\]\n]+)\]: `` over
  `f.Source`. **No AST equivalent**: the canonical lint parser does
  not enable goldmark's footnote extension, so the AST surfaces no
  footnote nodes. Kept (and left untouched by the MDS043 conversion).
- MDS012 `no-bare-urls` (`urlPattern`) — bare URLs are exactly the
  text goldmark does *not* turn into a node; the regex runs over
  AST text segments, not whole source. Kept (matches the plan's own
  prediction).
- MDS001 `line-length` (`setextUnderlineRe`), MDS059
  `blockquote-whitespace` (`reBlockquotePrefix`, `reMultiSpace`) —
  per-line slices. Kept.
- MDS067 `callout-type` (`calloutRE`) — first line segment of a
  blockquote (AST-derived). MDS025 `table-format`
  (`separatorRe`) — a table cell (AST-derived). MDS035
  `toc-directive` (`[TOC]` etc.) — a paragraph's source line
  (AST-derived). MDS030 `empty-section-body`
  (`htmlCommentPattern`), MDS062 `link-validity` (`reversedRe`) —
  run over masked/section-scoped text, not whole-source structure.
  Kept.
- MDS040 `recipe-safety` (`placeholderRe`, `fusedRe`) — a
  user-declared recipe command string. MDS020
  `required-structure`, MDS057 `required-text-patterns`, MDS036
  `max-section-length`, MDS028 `token-budget` — user-supplied or
  tokenizer patterns over AST-derived section/heading text. Kept.

## Acceptance Criteria

- [ ] The inventory section names every regex-over-source rule, its
      pattern, the AST equivalent (or "kept regex" with reason), and
      the measured cumulative CPU cost per rule on the neutral corpus.
- [ ] Each converted rule has a byte-identity test pinning its
      diagnostics against the previous regex implementation on a
      representative fixture corpus.
- [ ] `BenchmarkCheckCorpus{Small,Large}` improve in net (gain ≥
      cumulative cost of converted rules from the profile) and stay
      within the existing budget (Small p95 27 ms / 2 s, Large p95
      189 ms / 12 s).
- [ ] `mdsmith check .` passes; the full fixture suite is unchanged.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
