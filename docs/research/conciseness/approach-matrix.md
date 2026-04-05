# Conciseness Approach Matrix and Threshold Policy

This document defines the baseline approaches, threshold policy,
and model-promotion gate for conciseness experiments.

For this baseline, "weasel-language" means
`verbose-actionable` phrasing in the rubric, not all forms
of stylistic hedging.

## Why This Policy Exists

The baseline is designed for safe rollout decisions:

- protect precision so diagnostics stay actionable
- allow measured recall gains, but not at any cost
- keep evaluation deterministic and reproducible
- prevent test-set tuning and hidden overfitting

This policy optimizes decision quality, not headline score.

## Approach Matrix

| ID  | Family     | Core signal                            | Raw output  |
|-----|------------|----------------------------------------|-------------|
| A0  | heuristic  | current `MDS029` cues and score        | score       |
| A1  | heuristic  | tuned cue lists and weights            | score       |
| B0  | classifier | simple linear baseline                 | probability |
| B1  | classifier | selected candidate from plan 58        | probability |
| C0  | hybrid     | heuristic prefilter + classifier score | probability |

## Normalized Evaluation Score

For evaluation, all approaches must produce:

`risk_score` in `[0, 1]`, where higher means more likely
`verbose-actionable`.

Normalization rules:

- A0/A1:
  `risk_score = 1 - conciseness_score`
- B0/B1/C0:
  `risk_score = P(verbose-actionable)`

Thresholding, confusion matrices, and scorecards must use
`risk_score`, not mixed raw outputs.

## Metrics

Track these metrics on `dev`, `test`, and `holdout`.

### Dataset Split Roles

- `dev`: tune thresholds and compare approach variants.
- `test`: frozen in-domain benchmark for final comparison.
- `holdout`: frozen out-of-domain check for generalization.

### Metric Definitions

1. `precision`
   True positives divided by all predicted positives.
   Measures how many flagged paragraphs are truly
   `verbose-actionable`. Higher means fewer false positives.
2. `recall`
   True positives divided by all actual positives.
   Measures how many `verbose-actionable` paragraphs are caught.
   Higher means fewer misses.
3. `F1`
   Harmonic mean of precision and recall.
   Balanced summary when both are weighted equally.
4. `F0.5`
   Precision-weighted F-score.
   Used here because false positives are costly in lint workflows.
5. `AUPRC`
   Area under the precision-recall curve across thresholds.
   Threshold-independent ranking quality on imbalanced labels.
6. `Brier score`
   Mean squared error of predicted probability vs true label.
   Lower is better. Measures probability calibration quality.
7. `diagnostics per KLOC`
   Number of emitted diagnostics per 1,000 lines.
   Operational noise metric for reviewer workload.
8. `p50 latency (ms)`
   Median inference latency. Represents typical runtime.
9. `p95 latency (ms)`
   95th percentile latency. Captures slow-tail behavior.

## Threshold Policy

1. Sweep thresholds from `0.05` to `0.95` in `0.01` steps on `dev`.
2. Drop any threshold with precision below `0.75` on `dev`.
3. Pick threshold with highest `F0.5` among remaining candidates.
4. Break ties by higher recall, then lower diagnostics per KLOC.
5. Freeze one threshold per approach before running `test` and
   `holdout-outofdomain`.
6. Do not retune threshold after seeing `test` or holdout metrics.

## Impact of This Threshold Policy

- Precision floor (`>= 0.75`) reduces noisy diagnostics and reviewer load.
- `F0.5` favors precision over recall, matching lint workflows where
  false positives are expensive.
- Frozen thresholds make test and holdout comparisons credible.
- Tie-breaking by diagnostics per KLOC prefers lower operational churn.

## Alternatives Considered

1. Optimize `F1` directly:
   rejected because it allows precision to drop too easily.
2. Tune thresholds on `test`:
   rejected due to leakage and optimistic estimates.
3. Use per-doc-type thresholds in baseline:
   deferred because it raises overfitting risk before enough data exists.
4. Promote on AUPRC only:
   rejected because rollout needs one operating threshold, not just curve
   area.

## Promotion Gate

Promote `B1` over `A0` only when all criteria pass on frozen `test`:

1. Precision improves by at least `+0.05` absolute vs `A0`.
2. Recall does not drop by more than `0.02` absolute vs `A0`.
3. `F0.5` improves by at least `+0.05` absolute vs `A0`.
4. p95 latency is less than or equal to `2x` `A0` latency.
5. Diagnostics per KLOC does not increase by more than `10%`.
6. Holdout precision is within `0.05` of test precision.

If any gate fails, keep classifier mode experimental.

## Why Promotion Gates Are Strict

- protects users from quality regressions hidden by average metrics
- enforces latency budgets for CPU-only environments
- guards against distribution drift between test and holdout

## Run Artifacts

Each evaluation run must include:

- completed `scorecard-template.md`
- threshold sweep table or CSV for each approach
- confusion matrix for `test` and holdout
- benchmark environment details (CPU, Go version, dataset version)
- notes for disagreement themes and follow-up fixes
