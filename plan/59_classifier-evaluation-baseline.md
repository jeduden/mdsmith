---
id: 59
title: Classifier Evaluation Baseline
status: 🔲
---
# Classifier Evaluation Baseline

## Goal

Set up a clear baseline to test conciseness methods.
Use shared data and metrics to compare heuristic,
classifier, and hybrid results.

## Tasks

1. Define annotation rubric and label policy for
   `verbose-actionable` vs `acceptable`.
2. Create benchmark dataset format, split policy, and baseline
   sampling requirements across document types.
3. Implement an experiment matrix that compares at least:
   A0 (MDS029 heuristic), A1 (heuristic + lexicon tuning),
   B0 (simple classifier baseline), B1 (PR #15 classifier),
   and C0 (hybrid).
4. Define evaluation metrics and decision thresholds:
   precision, recall, F0.5, AUPRC, calibration,
   diagnostics per KLOC, and latency overhead.
5. Add reporting templates for dev/test metrics,
   disagreement review, and shadow-trial outcomes.

## Acceptance Criteria

- [ ] A written rubric exists with inclusion/exclusion guidance
      and at least 10 canonical examples.
- [ ] Dataset schema and split policy are documented and checked in.
- [ ] The approach matrix and threshold policy are documented.
- [ ] A scorecard template exists for per-approach comparison.
- [ ] Decision gate criteria are defined for promoting a model.
- [ ] `mdsmith check .` reports no issues.
