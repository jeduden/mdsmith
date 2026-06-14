---
id: 2606130837
title: Fast-path front-matter field reads for cross-file rules
status: "🔳"
model: opus
summary: >-
  The catalog rule reads every globbed target's front matter
  through a full yaml.v3 decode. On the repo corpus those decodes
  are the dominant cross-file cost (yaml parser/node allocations
  ~6-9 MB plus CPU). Add a line scanner for the common flat-scalar
  front matter that falls back to the full safe decode on anything
  non-trivial, preserving alias rejection.
depends-on: []
---
# Fast-path front-matter field reads for cross-file rules

## Goal

Read flat `key: scalar` front matter from cross-file targets with a
direct line scan. Keep the full yaml.v3 decode only for front
matter that needs it. The catalog directive reads one target per
globbed file, so this is its hot path. Parsed values and the
YAML-safety guarantee must not change.

## Background

A CPU profile of `mdsmith check` over the repo corpus still shows
[`catalog.cachedFrontMatter`](../internal/rules/catalog/rule.go) as
a top cross-file cost. It calls `readFrontMatter`, which calls
`yamlutil.UnmarshalSafe`. The profile was taken after the PR #600
single-parse YAML change.

The reads add up because catalogs glob wide. The `CLAUDE.md`
catalog globs the whole `docs/**` tree and reads each file's
`summary`. The `PLAN.md` catalog reads every `plan/*.md`. The
`-alloc_space` profile charges several MB to the yaml.v3 parser and
node tree on this path.

Plan 192 already de-duplicates these reads across host files. A
target globbed by N catalogs is parsed once. The residual cost is
the first parse of each distinct target. Front matter here is
almost always a flat mapping of scalar values. Common keys are
`title`, `summary`, `status`, `model`, and `id`. A single line scan
reads that far cheaper than a yaml.v3 decoder and a node tree.

## Design

Add a fast-path reader. It returns the same scalar-map shape
`readFrontMatter` produces today. It runs only for inputs it can
read with certainty:

- Walk the stripped front-matter bytes line by line. Accept a line
  only when it is a top-level `key: scalar` pair. The key must have
  no leading indentation. The value must be a plain or simply
  quoted scalar. The line must carry no YAML metacharacter that
  changes meaning. Those include `&`, `*`, `<<`, `|`, `>`, `?`,
  `[`, `{`, a `#` comment mid-value, and an empty value that opens
  a nested mapping.
- Bail out the instant a line leaves that grammar. Triggers are
  nesting, sequences, block scalars, multi-document markers,
  anchors, and aliases. On bail-out, fall back to the existing
  `yamlutil.UnmarshalSafe`.
- The fallback preserves the security property. It already rejects
  anchors and aliases. A billion-laughs payload is not flat
  scalars, so it never reaches the fast path. The fallback rejects
  it as it does today.
- Scalar canonicalization must match yaml. Quoted versus bare, and
  int and bool spellings, must agree. Otherwise catalog rows and
  `unique-frontmatter` comparisons would shift. Reuse the
  canonicalization already in
  [`yamlutil`](../internal/yamlutil/yamlutil.go). It lives in
  `TopLevelScalarField` and `canonicalScalar`.

A differential test gates correctness. For a set of real and
adversarial samples, the fast path must either return exactly what
`UnmarshalSafe` returns or defer to it. Any other outcome fails the
test.

The change is opt-in at the call site. Only cross-file readers that
want a few scalar fields route through it. The catalog is the first
such reader. The generic `lint.ParseFrontMatter` typed-struct path
stays on yaml.

## Tasks

1. [x] Add a differential test harness. It runs the fast path and
   `yamlutil.UnmarshalSafe` over one shared sample set. The set
   covers flat scalars and quoted, int, and bool spellings. It also
   covers nesting, sequences, block scalars, multi-doc, anchors,
   and aliases. It asserts equality or fallback.
2. [x] Implement the flat-scalar line scanner with the bail-out
   grammar above. Reuse `yamlutil` canonicalization. Fall back to
   `UnmarshalSafe` on any non-flat or metacharacter-bearing input.
3. [x] Route
   [`catalog.readFrontMatter`](../internal/rules/catalog/rule.go)
   through the fast path. Keep the RunCache slot and shape so plan
   192 cross-host sharing still holds.
4. [x] Add a test that hostile front matter reaching the catalog
   still has its anchors and aliases rejected, via the fallback.
5. [x] Verify behaviour. Run the integration fixtures,
   `go test ./...`, and `mdsmith check .`. The `CLAUDE.md` and
   `PLAN.md` catalogs must regenerate byte-identically.
6. [ ] Re-profile both corpora. Record the measured CPU and alloc
   delta, or a negative, in this plan.

## Acceptance Criteria

- [x] The fast path returns values byte-identical to
      `yamlutil.UnmarshalSafe` for every flat-scalar sample. It
      defers for every non-flat or metacharacter-bearing one. The
      differential test pins this.
- [x] Anchors and aliases in catalog-read front matter are still
      rejected, via the fallback. A test pins this.
- [x] `CLAUDE.md` and `PLAN.md` catalog bodies regenerate
      unchanged under `mdsmith fix`.
- [ ] Cross-file front-matter CPU and yaml allocations fall
      measurably on the repo-corpus profile. The number is recorded
      here.
- [x] `BenchmarkCheckCorpus{Small,Large}` stay within budget.
- [x] `mdsmith check .` passes (generated sections in sync).
- [x] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run` reports no
      issues (golangci-lint requires Go 1.25.8+; environment has 1.25.0).
