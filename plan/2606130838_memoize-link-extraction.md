---
id: 2606130838
title: Memoize linkgraph link and image extraction on the File
status: "🔲"
model: sonnet
summary: >-
  linkgraph.ExtractLinks runs a fresh ast.Walk on every call and is
  not memoized. crossfilereferenceintegrity and mdsmith deps both
  walk it per file. Memoize the link and image collections on the
  per-Check File, the way plan 187 memoized CollectSectionParagraphs,
  so the link walk runs once per file rather than per caller.
depends-on: []
---
# Memoize linkgraph link and image extraction on the File

## Goal

Run the link-and-image AST walk once per file. Today
[`linkgraph.ExtractLinks`](../internal/linkgraph/linkgraph.go) walks
`f.AST` fresh on every call. Memoize the result on the per-Check
`*lint.File` so repeated callers share one walk.

## Background

A CPU profile of `mdsmith check` over the repo corpus, taken after
PR #600, shows `linkgraph.ExtractLinks` at roughly 7%. It is its
own `ast.Walk`, separate from the engine's multiplexed walk.

`ExtractLinks` is not memoized. It rebuilds the link slice on each
call. The MDS027 cross-file-reference-integrity rule calls it
during Check. The `mdsmith deps` command and the LSP
call-hierarchy walk the same graph. Each entry point pays its own
full walk.

Plan 187 set the pattern to copy. It memoized
`astutil.CollectSectionParagraphs` on the File via the `File.Memo`
primitive. The prose rules then shared one paragraph walk. The same
primitive fits link extraction. The collection is a pure function
of `f.AST`, so one cached slice serves every caller.

This is a single-rule lever on the check path, so the gain is
modest. The value is the removed redundant walk and a shared seam
that `deps` and the LSP can reuse. It does not touch the
multiplexed walk, which plans 189 and 2606022128 already settled.

## Design

Memoize the link and image slices on the File:

- Add memoized accessors built on `File.Memo`, keyed per
  collection. `ExtractLinks` and `ExtractImages` compute once and
  cache the result on the File.
- The cached slice is read-only for callers. They already treat the
  returned slice as a value to range over, so no caller mutates it.
- The result must stay byte-identical to today's output. Order is
  document order. Lines are body-relative. Reference-style links
  are still skipped the same way.
- Keep the lifetime inside one Check. The memo lives on the File,
  which the engine builds per file and discards after Check. No
  cross-file or cross-run caching is added here.

## Tasks

1. [ ] Add a test that pins reference identity. Two calls to the
   memoized link accessor on one File return the same backing slice.
   A second test pins identical contents to the current
   `ExtractLinks` output across a fixture set.
2. [ ] Memoize `ExtractLinks` and `ExtractImages` on the File via
   `File.Memo`, keyed per collection.
3. [ ] Point the MDS027 cross-file-reference-integrity rule at the
   memoized accessor.
4. [ ] Point the `mdsmith deps` command and any LSP caller that
   walks links at the same accessor, where they operate on a live
   per-Check File.
5. [ ] Verify behaviour. Run the integration fixtures,
   `go test ./...`, and `mdsmith check .`. Diagnostics and `deps`
   output are unchanged.
6. [ ] Re-profile the repo corpus. Record the measured delta, or a
   negative, in this plan.

## Acceptance Criteria

- [ ] The memoized accessor runs the link walk once per File,
      pinned by a reference-identity test.
- [ ] Link and image collections are byte-identical to the current
      `ExtractLinks` and `ExtractImages` output across the fixture
      set.
- [ ] `crossfilereferenceintegrity` diagnostics and `mdsmith deps`
      output are unchanged.
- [ ] `BenchmarkCheckCorpus{Small,Large}` stay within budget.
- [ ] `mdsmith check .` passes (generated sections in sync).
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run` reports no
      issues.
