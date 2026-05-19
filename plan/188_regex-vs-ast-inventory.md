---
id: 188
title: Regex-over-source rules — inventory and AST-resident replacements
status: "🔲"
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

1. [ ] Create this plan.
2. [ ] Inventory every rule whose `Check` (or helpers called only
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

To be filled in by task 2. Initial suspects from plan 187 and from
file grep:

- `nounusedlinkdefinitions` — plan 187 names it explicitly at ~70 ms.
  Definitions are in `f.LinkReferences()` already; usages are in
  the AST as `*ast.Link` / `*ast.Image` with `ReferenceLink: true`.
  **Likely conversion: route through `f.LinkReferences()` +
  AST walk for usages.**
- `noundefinedreferencelabels` — symmetric to the above.
- `nobareurls` — currently scans `f.Source` for URL patterns.
  AST equivalent: `*ast.AutoLink` and `*ast.Text` segments.
  Conversion is less clean (bare URLs are exactly the case the
  parser does NOT pick up), so this likely stays regex.
- `noinlinehtml` — could use `*ast.RawHTML` and `*ast.HTMLBlock`
  rather than scan source.
- `notrailingpunctuation` — operates on heading text; AST already
  exposes heading children.
- `linelength` — pure line-based, no AST equivalent, stays as-is.

The above is unverified; task 2 produces the authoritative list
with patterns and cost numbers.

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
