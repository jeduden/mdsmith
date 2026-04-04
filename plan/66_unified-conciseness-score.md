---
id: 66
title: "Unified Conciseness Score"
status: 🔲
---
# Plan 66: Unified Conciseness Score

## Goal

Produce a single `float64` conciseness score per
paragraph. Use a pure-Go linear classifier with 14
features. Require zero external dependencies.

## Motivation

Six plans and seven PRs addressed conciseness with
diverging approaches. This plan consolidates them into
one roadmap that produces a single number per paragraph:
a `float64` in `[0, 1]` where `1.0` means maximally
concise.

### Why this approach

We select the enhanced pure-Go linear classifier because:

1. **Infrastructure exists.** PR #33 has the classifier,
   embedded weights, checksum verification, and benchmark
   harness. 3.3 μs avg latency, +480 bytes binary,
   deterministic across runs.
2. **Zero dependencies.** Pure Go, `CGO_ENABLED=0`,
   single binary. No ONNX Runtime, no shared libraries,
   no Ollama, no Python at runtime.
3. **100 % deterministic.** Same input always produces
   the same score. Validated in PR #33 spike
   (`unique_hashes=1`).
4. **Extensible.** Adding features to a linear model is
   trivial. Each new feature is a pure-Go function that
   returns a `float64`. Retraining weights is a single
   Python script run offline.

## Consolidated Plan Relationships

<!-- supersedes: 53, 54, 56, 58 -->

### Superseded plans

| Plan | Title                    | Disposition            |
|------|--------------------------|------------------------|
| 53   | MDS029 conciseness score | Absorbed; close PR #21 |
| 54   | MDS029 conciseness rule  | Absorbed; close PR #24 |
| 56   | Ollama spike             | Won't continue; #34    |
| 58   | Classifier fallback      | Partial absorb; #31    |

### Completed foundation plans

| Plan | Title                    | Disposition           |
|------|--------------------------|-----------------------|
| 62   | Corpus acquisition       | Absorbed; merge #35   |
| 64   | Pure-Go classifier spike | Foundation; merge #33 |

### Merge order

1. PR #33 (plan 64) — base classifier
2. PR #35 (plan 62) — corpus
3. This plan's PR — extended features, retrained weights,
   MDS029 rule

PRs #21, #24, #31, #34 are closed with a comment linking
to this plan.

## Definition: The Conciseness Score

```text
conciseness ∈ [0.0, 1.0]

0.0 = maximally verbose (all filler, no content)
1.0 = maximally concise (every word carries meaning)
```

The score is the **sigmoid output** of a linear model
over paragraph-level features. The sigmoid maps to
`[0, 1]` and the model weights determine how each
feature contributes.

The MDS029 rule fires when `conciseness < threshold`
(default `0.20`, configurable in `.mdsmith.yml`).

```yaml
rules:
  conciseness-scoring:
    min-score: 0.20    # paragraphs below this are flagged
```

Diagnostic format:

```text
README.md:14:1 MDS029 paragraph conciseness 0.38 …
```

## Features

The unified scorer extracts these features from each
paragraph. All are pure Go, zero external dependencies.

### Existing features (from PR #33)

Implemented in the classifier package:

- **filler_density** — filler words / total words
- **modal_density** — modal verbs / total words
- **vague_density** — vague words / total words
- **action_density** — action verbs / total words
- **hedge_density** — hedge phrases / total words
- **verbose_density** — verbose phrases / total words
- **stop_ratio** — stop words / total words

### New features (this plan)

| Feature           | Signal                   |
|-------------------|--------------------------|
| compression_ratio | Redundancy via flate     |
| type_token_ratio  | Vocabulary repetition    |
| nominal_density   | Hidden verbs as nouns    |
| sent_len_variance | Sentence length spread   |
| func_word_ratio   | Function word dilution   |
| avg_word_length   | Word length distribution |
| ly_adverb_density | Adverb overuse           |

Total: 14 features (7 existing + 7 new).

## Implementation

### Files to create or modify

```text
internal/rules/concisenessscoring/
├── classifier/
│   ├── model.go           # extend extractors
│   ├── model_test.go      # extend tests
│   ├── features.go        # NEW: 7 features
│   ├── features_test.go   # NEW
│   └── data/
│       └── cue-linear-v2.json  # NEW: weights
├── scorer.go              # NEW: interface
├── scorer_test.go         # NEW
├── rule.go                # NEW: MDS029 rule
└── rule_test.go           # NEW
internal/rules/MDS029-conciseness-scoring/
└── README.md              # update rule spec
```

### Weight retraining

After adding the new features, retrain the model:

1. Use the corpus from PR #35 (plan 62).
2. Extract all 14 features from each labeled paragraph.
3. Fit logistic regression (`sklearn.linear_model`).
4. Export weights and bias to `cue-linear-v2.json`.
5. Generate SHA-256 checksum for `go:embed` verification.
6. Validate determinism: assert `unique_hashes=1`.

The retraining script lives in `eval/conciseness/train/`
and runs offline when features or corpus change.

## Tasks

1. Merge PR #33 (plan 64 base classifier)
2. Merge PR #35 (plan 62 corpus)
3. Add 7 new feature extractors in `features.go`
4. Add feature tests in `features_test.go`
5. Retrain weights with 14 features, export v2 JSON
6. Implement `Scorer` interface in `scorer.go`
7. Implement MDS029 rule in
   `internal/rules/concisenessscoring/rule.go`
8. Update MDS029 rule spec in
   `internal/rules/MDS029-conciseness-scoring/`
9. Add config support for `min-score` threshold
10. Run determinism and benchmark validation
11. ~~Close superseded PRs #21, #24, #31, #34~~ (done)

## Acceptance Criteria

- [ ] `mdsmith check` reports MDS029 diagnostics with
  a conciseness score
- [ ] Score is a `float64` in `[0, 1]`, printed to
  2 decimal places
- [ ] Threshold configurable via `.mdsmith.yml`
  `rules.conciseness-scoring.min-score`
- [ ] All 14 features extracted in pure Go,
  `CGO_ENABLED=0`
- [ ] Deterministic: same paragraph produces same score
  across runs and platforms
- [ ] Binary size delta < 2 KB over PR #33 baseline
- [ ] Latency < 100 μs per paragraph (p95)
- [ ] `go test ./...` passes
- [ ] `golangci-lint run` passes
- [ ] `mdsmith check PLAN.md` passes
- [ ] Superseded PRs (#21, #24, #31, #34) closed

## Future: Transformer-Based Scoring

The pure-Go ML ecosystem is maturing. Two projects
deserve re-evaluation in Q3 2026:

- **GoMLX** (`gomlx/gomlx`) — pure-Go ML framework
  with transformer support and SIMD acceleration.
- **Hugot** (`knights-analytics/hugot`) — runs
  HuggingFace pipelines in pure Go.
- **gonnx** (`AdvancedClimateSystems/gonnx`) — pure-Go
  ONNX runtime, ~8x slower but zero C deps.

A fine-tuned small transformer could replace the linear
model for higher accuracy. Gate behind a build tag
(`-tags conciseness_ml`). The linear classifier remains
the default.

See issue #111 for tracking.

## Testing

```bash
GOCACHE=/tmp/mdsmith-gocache go test ./...
GOCACHE=/tmp/mdsmith-gocache \
  GOLANGCI_LINT_CACHE=/tmp/mdsmith-golangci-cache \
  go tool golangci-lint run --allow-parallel-runners
GOCACHE=/tmp/mdsmith-gocache go run ./cmd/mdsmith check \
  PLAN.md plan/66_unified-conciseness-score.md
```

## References

- PR #33: pure-Go classifier spike (plan 64)
- PR #35: corpus acquisition (plan 62)
- PR #24: MDS029 rule definition (plan 54)
- PR #31: classifier fallback interface (plan 58)
- ConCISE (2025, arxiv:2511.16846): reference-free
  conciseness metric via compression ratios
- EMNLP 2022 TSAR: "Conciseness: An Overlooked Language
  Task" (Stahlberg et al.)
- ACL 2023: compression-based text classification
