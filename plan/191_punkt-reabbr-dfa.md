---
id: 191
title: Hand-rolled DFA for Punkt's `reAbbr` to skip regex backtracking
status: "🔲"
model: opus
depends-on: []
summary: >-
  Punkt's English-pass abbreviation classifier runs the regex
  `((?:[\w]\.)+[\w]*\.)` on every period-ending token. A focused
  profile of `SplitSentences` over a representative corpus shows
  `regexp.(*Regexp).tryBacktrack` at 13% flat / 24% cumulative of
  CPU — the largest single attributable cost inside the segmenter.
  Replace `reAbbr.FindAllString` with a hand-rolled DFA scanner
  that returns a byte-identical answer with no backtracking
  engine. The existing equivalence harness in `internal/mdtext`
  gates the change against `SplitSentences` drift.
---
# Hand-rolled DFA for Punkt's `reAbbr` to skip regex backtracking

## Goal

Cut MDS024's runtime without changing its diagnostic.
Target a 10–15% drop in `mdtext.SplitSentences` wall time
on prose-heavy input. Remove the `regexp` engine from the
hottest abbreviation classifier inside the trained Punkt
pipeline.

## Background

The cost is concrete and measured.
[`BenchmarkSplitSentences`][bench] runs at ~176 µs/op,
593 allocs/op. The CPU profile attributes the time as:

- `regexp.(*Regexp).tryBacktrack` — 13% flat, 24% cum.
  The NFA backtracking loop.
- `regexp.(*Regexp).doExecute` — 37% cum.
- `english.MultiPunctWordAnnotation.tokenAnnotation` —
  39% cum. It runs `reAbbr.FindAllString(tok, 1)` on
  every period-ending token.

[bench]: ../internal/mdtext/sentence_equivalence_test.go

`reAbbr` is defined at [`english/main.go:15`][upstream]
as `((?:[\w]\.)+[\w]*\.)`. It matches tokens with
multiple internal periods. Patterns like `U.S.`, `p.m.`,
`e.g.`, `i.e.`, and initials like `J.R.R.` match. These
are exactly the patterns where Punkt must demote a
tentative sentence boundary to "this is an abbreviation,
not a terminator".

[upstream]: https://github.com/neurosnap/sentences/blob/v1.1.2/english/main.go

The `(?:...)+` repetition drives `tryBacktrack`. Go's
`regexp` engine falls back to backtracking for patterns
it cannot run on the one-pass DFA path. This one does.
The cost scales with period-ending tokens per file.
That is the dominant token class on prose-heavy input.

`reAbbr` has a small, regular structure. A hand-rolled
DFA scanner over runes can decide membership in one
linear pass. No allocations. No backtracking. The
[equivalence harness][bench] already pins
`SplitSentences` output to be byte-identical to upstream
across a representative corpus. Drift is caught at the
next test run.

The change stays local to mdsmith. No fork.

The upstream library exposes the needed types:

- `AnnotateTokens` — the interface.
- `Storage` and `PunctStrings` — carry trained data.
- `MultiPunctWordAnnotation`'s component fields — all
  exported.

mdsmith builds its own tokenizer with three annotators:

- `TypeBasedAnnotation` — upstream, unchanged.
- `TokenBasedAnnotation` — upstream, unchanged.
- `FastMultiPunctWordAnnotation` — new. Only the
  abbreviation matcher differs from upstream.

## Tasks

1. [ ] Create this plan.
2. [ ] Add a `BenchmarkSplitSentences_Subset` that isolates the
   hot pattern (a corpus of abbreviation-heavy short paragraphs)
   so the optimization's effect is visible without being diluted
   by the rest of the equivalence corpus.
3. [ ] Implement `matchAbbrPattern(tok string) bool` in a new
   `internal/mdtext/abbr.go`. The function must return
   `true` if and only if `reAbbr.FindAllString(tok, 1)` would
   return a non-empty slice on the same input. Write the failing
   unit test first: a table of inputs the regex matches (`U.S.`,
   `p.m.`, `J.R.R.`, `a.b.c.`, `e.g.`) and inputs it does not
   (`Mr.`, `hello.`, `3.14`, `a..b`, `.`, empty string), with the
   reference being `reAbbr.FindAllString(tok, 1)` itself.
4. [ ] Build a `FastMultiPunctWordAnnotation` (in
   `internal/mdtext/fastpunct.go`) that mirrors upstream
   `english.MultiPunctWordAnnotation` line-for-line except the
   `reAbbr.FindAllString(tokOne.Tok, 1)` call is replaced by
   `matchAbbrPattern(tokOne.Tok)`. The annotator embeds the same
   `*Storage`, `PunctStrings`, `TokenExistential`, `TokenParser`,
   `TokenGrouper`, and `Ortho` fields the upstream type uses, so
   no further behavioural drift is possible.
5. [ ] Switch `initTokenizer` in
   [`internal/mdtext/mdtext.go`](../internal/mdtext/mdtext.go) to
   construct the tokenizer manually with annotators
   `[TypeBasedAnnotation, TokenBasedAnnotation,
   FastMultiPunctWordAnnotation]`. Keep the previous
   `english.NewSentenceTokenizer(nil)` path behind a build tag
   `mdtext_punkt_upstream` for A/B verification.
6. [ ] Run the existing [`TestSplitSentences_IsItsOwnReference`][bench]
   and the `english/main_test.go` golden-rules corpus through
   the fast path. Both must be byte-for-byte identical to the
   upstream output. If either drifts, the hand-rolled matcher
   is wrong — fix it.
7. [ ] Profile and record the new
   `BenchmarkSplitSentences` and `BenchmarkSplitSentences_Subset`
   numbers in this plan's `## Results` section. Confirm
   `BenchmarkCheckCorpus{Small,Large}` stay within budget
   (Small p95 27 ms / 2 s, Large p95 189 ms / 12 s) and that the
   abbreviation-heavy `BenchmarkSplitSentences_Subset` drops by
   at least 10% — if it does not, the lever was smaller than the
   profile suggested and the change is rejected.
8. [ ] If the change ships, document the fast-path annotator in
   the MDS024 README under "How it works" so the optimization is
   visible to anyone reading the rule docs.

## Results

To be filled in by task 7. Initial baseline (recorded today):

```text
BenchmarkSplitSentences-4    13256    176457 ns/op    29747 B/op    593 allocs/op
```

CPU profile attribution (regex-heavy frames):

- `regexp.(*Regexp).tryBacktrack`: 13.02% flat / 23.64% cum
- `regexp.(*Regexp).doExecute`: 0.65% flat / 37.09% cum
- `english.MultiPunctWordAnnotation.tokenAnnotation`: 1.52% flat / 39.05% cum

## Risk

The hand-rolled matcher must be byte-equivalent to the regex on
every conceivable token shape Punkt feeds it. The equivalence
harness covers a representative corpus but is not exhaustive over
the rune space. Task 3's unit test must use
`reAbbr.FindAllString(tok, 1)` directly as the reference, so a
property-style table of inputs hits every branch of the DFA
against the regex's own answer.

If the upstream library updates `reAbbr` in a future
neurosnap/sentences release, the local fast path silently
diverges. The equivalence harness catches that on the next test
run. To make the drift visible without running tests, the build
tag `mdtext_punkt_upstream` (task 5) lets a developer flip back
to upstream and verify.

## Acceptance Criteria

- [ ] `matchAbbrPattern` returns the same boolean as
      `reAbbr.FindAllString(tok, 1) != nil` for every input in
      the abbreviation-equivalence table (task 3); the test
      explicitly cross-checks against the regex.
- [ ] `TestSplitSentences_IsItsOwnReference` passes — the fast
      path is byte-identical to upstream over the equivalence
      corpus.
- [ ] `BenchmarkSplitSentences_Subset` (abbreviation-heavy)
      improves by ≥10% on the 4-vCPU sandbox; absolute number
      recorded in `## Results`.
- [ ] `BenchmarkCheckCorpus{Small,Large}` remain within budget.
- [ ] `mdsmith check .` passes.
- [ ] `go test ./...` and `go test -race ./...` pass.
- [ ] `go tool golangci-lint run` reports no issues.
