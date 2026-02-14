---
title: Choosing Readability, Conciseness, and Token Budget Metrics
description: Trade-offs, examples, and threshold guidance for readability, structure, length, conciseness, and token budgets.
---
# Choosing Readability, Conciseness, and Token Budget Metrics

## Scope and disclaimer

This guide compares existing mdsmith rules that touch readability and length with token budget awareness and the proposed conciseness scoring (plan 53). Conciseness scoring is not implemented yet; any conciseness scores below are illustrative, not normative.

## What the current rules measure

| Rule                         | Measures                                                                     | Default                         | What it misses                                                  |
|------------------------------|------------------------------------------------------------------------------|---------------------------------|-----------------------------------------------------------------|
| [MDS023](../rules/MDS023-paragraph-readability/README.md) `paragraph-readability` | Complexity using ARI (characters per word, words per sentence)               | `max-grade: 14.0`, `min-words: 20`  | Wordiness and filler; short but dense paragraphs can be skipped |
| [MDS024](../rules/MDS024-paragraph-structure/README.md) `paragraph-structure`   | Shape and length of paragraphs (sentences per paragraph, words per sentence) | `max-sentences: 6`, `max-words: 40` | Verbosity that fits within limits; dense but short prose        |
| [MDS022](../rules/MDS022-max-file-length/README.md) `max-file-length`       | Lines per file                                                               | `max: 300`                        | Token load and dense paragraphs                                 |
| [MDS028](../rules/MDS028-token-budget/README.md) `token-budget`          | Estimated token count per file (`heuristic` or `tokenizer` mode)                 | `max: 8000`, `mode: heuristic`      | Exact model token parity; tokenizer mode is still approximate   |
| [MDS001](../rules/MDS001-line-length/README.md) `line-length`           | Characters per line                                                          | `max: 80`                         | Verbosity and paragraph complexity                              |

## Planned metrics (not implemented)

| Metric              | Goal                                       | Status  |
|---------------------|--------------------------------------------|---------|
| [Conciseness Scoring](../plan/53_conciseness-scoring.md) | Flag low information density in paragraphs | Planned |

## What conciseness scoring is trying to measure

Conciseness scoring (plan 53) focuses on information density rather than complexity or structure. It aims to flag paragraphs that are easy to read but say too little with too many words, which can waste tokens and create drift in agentic contexts. A plausible starting point is a heuristic that penalizes filler words, hedging language, and verbose phrases while rewarding content-bearing terms.

## What token budget awareness is trying to measure

Token budget awareness ([MDS028](../rules/MDS028-token-budget/README.md)) focuses on file-level size in terms of tokens rather than lines or characters. It protects LLM context windows by warning when a file exceeds a configurable budget. `heuristic` mode uses word count multiplied by a ratio, which is fast but approximate. `tokenizer` mode uses tokenizer-aware splitting with a selected encoding for a closer estimate.

Tokenization happens before inference, so any LLM will read inputs as tokens. That means token budgets are only accurate when they use the same tokenizer as the target model. The trade-off is performance: exact tokenization is slower and needs vocab assets, while ratio-based estimates are fast and model-agnostic.

## Example paragraphs (paragraph-level metrics)

### Example A

In order to make sure that we are all on the same page, it is important to note that the system is, in most cases, able to handle requests pretty well, and this is something we should keep in mind.

### Example B

The synchronization algorithm enforces linearizability via per-shard lease epochs and monotonic commit indices.

### Example C

We should update the onboarding guide so that new contributors can quickly find the build steps, understand the release checklist, and avoid common pitfalls without needing to ask in chat, which will reduce interruptions for everyone.

### Example D

The plan is straightforward. We will add a new rule. It will report issues. It will include guidance. It will ship this week. It will help teams. It will reduce noise. It will keep docs short.

### Example E

Basically, we just want to make sure the plan is pretty clear to everyone. It is really just a simple update, and we might adjust it later.

## How the rules score these examples

Notes: ARI values use mdsmith's current formula. [MDS023](../rules/MDS023-paragraph-readability/README.md) skips paragraphs under `min-words`. [MDS024](../rules/MDS024-paragraph-structure/README.md) flags when sentences or words exceed limits. Conciseness scores below are illustrative heuristics, not an implemented rule. Token budget awareness is file-level; see the token budget examples right after this table.

| Example | Words | Sentences | ARI  | MDS023 result        | MDS024 result          | Conciseness score (illustrative) |
|---------|------:|----------:|-----:|----------------------|------------------------|----------------------------------|
| A       | 40    | 1         | 16.6 | Fail (16.6 > 14.0)   | Pass                   | 36.2                             |
| B       | 13    | 1         | 20.2 | Skipped (< 20 words) | Pass                   | 84.6                             |
| C       | 36    | 1         | 22.1 | Fail (22.1 > 14.0)   | Pass                   | 63.9                             |
| D       | 36    | 8         | 0.3  | Pass                 | Fail (8 > 6 sentences) | 50.0                             |
| E       | 26    | 2         | 4.6  | Pass                 | Pass                   | 50.0                             |

## Token budget examples (file-level)

These examples assume an illustrative ratio of `0.75 tokens per word` and a budget of `2,000 tokens`.

- File F: 2,800 words -> ~2,100 tokens, flagged by token budget even if line count is below `max-file-length`.
- File G: 1,200 words with heavy code blocks -> estimate ~900 tokens, but actual tokens could be higher; ratio tuning or code weighting may be needed.

## Trade-offs by metric

| Metric                  | Strengths                                                    | Risks                                                                        |
|-------------------------|--------------------------------------------------------------|------------------------------------------------------------------------------|
| Readability ([MDS023](../rules/MDS023-paragraph-readability/README.md))    | Encourages simple, broadly accessible prose                  | Penalizes technical terms; misses wordiness; can skip short dense paragraphs |
| Structure ([MDS024](../rules/MDS024-paragraph-structure/README.md))      | Enforces consistent paragraph shape with low false positives | Does not address filler or redundancy                                        |
| Length ([MDS022](../rules/MDS022-max-file-length/README.md), [MDS001](../rules/MDS001-line-length/README.md)) | Prevents runaway size and formatting drift                   | Poor proxy for token load or verbosity                                       |
| Token budget ([MDS028](../rules/MDS028-token-budget/README.md))   | Directly targets context window size                         | Estimation is noisy; code blocks and symbols can skew counts                 |
| Conciseness (proposed)  | Targets verbosity and token waste                            | Heuristic; can penalize necessary qualifiers or legal language               |

## How to choose limits

1. Start with defaults for [MDS023](../rules/MDS023-paragraph-readability/README.md) and [MDS024](../rules/MDS024-paragraph-structure/README.md) to establish baseline structure and readability.
2. Sample a representative set of documents and collect results before tightening thresholds.
3. For token budgets, pick a target based on your context window and allocate a safe share per document (for example, reserve 20 to 30 percent of a prompt budget for a single doc). Choose an initial word-to-token ratio and adjust for code-heavy files.
4. For conciseness scoring, set an initial threshold that flags only the worst 10 to 20 percent of paragraphs, then adjust.
5. Use path-based overrides to reflect different document types, such as onboarding guides vs architecture specs.
6. Re-evaluate thresholds after major content changes or when onboarding new teams.

## When to use one measure instead of many

If you need a single metric to minimize complexity, choose the one that best matches your risk:

- Choose [MDS024](../rules/MDS024-paragraph-structure/README.md) `paragraph-structure` when you want predictable, low-noise enforcement.
- Choose [MDS023](../rules/MDS023-paragraph-readability/README.md) `paragraph-readability` when broad comprehension is the highest priority.
- Choose [MDS028](../rules/MDS028-token-budget/README.md) `token-budget` when context window limits are the dominant constraint and you want a file-level guardrail.
- Choose conciseness scoring when token budget and drift are the main risks and you accept heuristic trade-offs.

## Recommendation for mdsmith users

Start with [MDS023](../rules/MDS023-paragraph-readability/README.md) and [MDS024](../rules/MDS024-paragraph-structure/README.md) enabled. Use [MDS022](../rules/MDS022-max-file-length/README.md) and [MDS001](../rules/MDS001-line-length/README.md) as baseline file and line controls. Add [MDS028](../rules/MDS028-token-budget/README.md) when context limits matter, then add conciseness scoring only after calibrating its thresholds and confirming it improves signal without harming necessary precision.
