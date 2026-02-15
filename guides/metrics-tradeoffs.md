---
title: Choosing Readability, Conciseness, and Token Budget Metrics
description: Trade-offs, examples, and threshold guidance for readability, structure, length, conciseness, and token budgets.
---
# Choosing Readability, Conciseness, and Token Budget Metrics

## Scope

This guide compares mdsmith rules that shape readability and length.
`conciseness` is now implemented as [MDS026](../rules/MDS026-conciseness/README.md).
Token budget awareness is still planned in
[plan 47](../plan/47_token-budget-awareness.md).

## What the current rules measure

| Rule                         | Measures                                                      | Default                         | What it misses                                     |
|------------------------------|---------------------------------------------------------------|---------------------------------|----------------------------------------------------|
| [MDS023](../rules/MDS023-paragraph-readability/README.md) `paragraph-readability` | Complexity via ARI (characters per word, words per sentence)  | `max-grade: 14.0`, `min-words: 20`  | Filler and low information density                 |
| [MDS024](../rules/MDS024-paragraph-structure/README.md) `paragraph-structure`   | Paragraph shape (sentences per paragraph, words per sentence) | `max-sentences: 6`, `max-words: 40` | Verbosity that still fits structure limits         |
| [MDS026](../rules/MDS026-conciseness/README.md) `conciseness`           | Heuristic information density and verbosity                   | `min-score: 55.0`, `min-words: 20`  | Domain-specific language where hedging is required |
| [MDS022](../rules/MDS022-max-file-length/README.md) `max-file-length`       | Lines per file                                                | `max: 300`                        | Token load and paragraph quality                   |
| [MDS001](../rules/MDS001-line-length/README.md) `line-length`           | Characters per line                                           | `max: 80`                         | Readability and information density                |

## Conciseness heuristic (MDS026)

MDS026 evaluates paragraphs with a weighted penalty model:

```text
score = 100
  - filler_ratio*100*filler_weight
  - hedge_ratio*100*hedge_weight
  - verbose_phrase_density_per_100_words*verbose_phrase_weight
  - max(0, min_content_ratio-content_ratio)*100*content_weight
```

A diagnostic is emitted when `score < min-score`.

Defaults are tuned to be conservative:

- `min-score: 55.0`
- `min-words: 20`
- `min-content-ratio: 0.45`
- `filler-weight: 1.0`
- `hedge-weight: 0.8`
- `verbose-phrase-weight: 4.0`
- `content-weight: 1.2`

Lexical signals are configurable with `filler-words`, `hedge-words`, and
`verbose-phrases`.

## Calibration snapshot

Representative project sample (default settings):

- `README.md`
- `guides/metrics-tradeoffs.md`
- `internal/rules/MDS023-paragraph-readability/README.md`
- `internal/rules/MDS024-paragraph-structure/README.md`
- `internal/rules/MDS026-conciseness/README.md`

Result: `0` MDS026 diagnostics across those files.

Synthetic stress sample:

- `internal/rules/MDS026-conciseness/bad/default.md`

Result: `1` MDS026 diagnostic (score below threshold).

False-positive risk at defaults is low for technical docs in the sample above,
but risk remains for document classes that intentionally use qualifiers
(legal, policy, safety notes). Use path overrides for those files.

## Trade-offs by metric

| Metric                  | Strengths                                         | Risks                                       |
|-------------------------|---------------------------------------------------|---------------------------------------------|
| Readability ([MDS023](../rules/MDS023-paragraph-readability/README.md))    | Encourages broadly accessible prose               | Penalizes dense technical terminology       |
| Structure ([MDS024](../rules/MDS024-paragraph-structure/README.md))      | Low-noise constraints on paragraph shape          | Does not target filler or drift             |
| Conciseness ([MDS026](../rules/MDS026-conciseness/README.md))    | Targets verbosity and low information density     | Heuristic can penalize necessary qualifiers |
| Length ([MDS022](../rules/MDS022-max-file-length/README.md), [MDS001](../rules/MDS001-line-length/README.md)) | Strong baseline controls for file and line growth | Weak proxy for token budget and density     |
| Token budget (planned)  | Direct guardrail for context window size          | Estimation can be noisy                     |

## Tuning guidance

1. Start with default `MDS026` settings and collect diagnostics for a week.
2. If too noisy, lower `min-score` or reduce `hedge-weight`.
3. If too permissive, raise `min-score` or `min-content-ratio`.
4. Use overrides by path for docs that need heavier qualification.
5. Recalibrate after major writing-style changes.

## Recommendation

Use [MDS023](../rules/MDS023-paragraph-readability/README.md),
[MDS024](../rules/MDS024-paragraph-structure/README.md), and
[MDS026](../rules/MDS026-conciseness/README.md) together for paragraph quality.
Keep [MDS022](../rules/MDS022-max-file-length/README.md) and
[MDS001](../rules/MDS001-line-length/README.md) as baseline size controls.
Add token budget awareness when plan 47 is implemented.
