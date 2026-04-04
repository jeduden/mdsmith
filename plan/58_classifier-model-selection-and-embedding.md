---
id: 58
title: Select and Package Fast Weasel Classifier (CPU Fallback)
status: ⛔
---
# Select and Package Fast Weasel Classifier (CPU Fallback)

## Goal

Pick a fast classifier for conciseness weasel-language detection.
Package its weights for offline use. Define a CPU fallback path.

For this plan, "weasel-language" is operationalized as
`verbose-actionable` phrasing in conciseness evaluation.

## Detection Contract

Use one binary label with calibrated confidence:

- `verbose-actionable`: wording is unnecessarily long
- `acceptable`: wording is concise enough for intent

Classifier output contract:

```json
{
  "label": "verbose-actionable",
  "risk_score": 0.73,
  "threshold": 0.60,
  "model_id": "cue-linear-v1",
  "mode": "auto",
  "backend": "classifier",
  "version": "2026-02-15"
}
```

Contract semantics:

- `label` enum: `verbose-actionable` or `acceptable`
- `risk_score` range: `[0, 1]` where higher means more likely
  `verbose-actionable`
- `mode` enum: `classifier`, `heuristic`, or `auto`
- `backend` enum: runtime backend actually used
  (`classifier` or `heuristic`)
- decision rule: if `risk_score >= threshold` then label is
  `verbose-actionable`; otherwise `acceptable`

Threshold policy:

- Optimize threshold on `dev` for `F0.5` (precision-weighted)
- Freeze threshold and report on `test`
- Keep one default threshold in config for deterministic behavior

## Candidate Shortlist

Model candidates are scoped to CPU-first, local/offline use:

1. `cue-linear-lite-v1`: sparse linear model over cue counts
2. `cue-linear-v1`: sparse linear model over cues + density features
3. `hybrid-v1`: heuristic prefilter + linear classifier

Selection criteria:

- quality: precision, recall, `F1`, and `F0.5` on frozen `test`
- CPU latency: p50/p95 inference latency on local hardware
- artifact footprint: model size and binary size impact
- licensing: model weights and dependency compatibility with MIT repo

## Evaluation Harness Design

Harness inputs and assets:

- rubric: `eval/conciseness/rubric.md`
- schema: `eval/conciseness/dataset.schema.cue`
- scorecard: `eval/conciseness/scorecard-template.md`
- dataset splits: `train/dev/test/holdout-outofdomain` JSONL

Required benchmark outputs per candidate:

- confusion matrix and precision/recall/`F1`/`F0.5`
- calibration summary (at least Brier score)
- diagnostics per KLOC at selected threshold
- p50/p95 CPU inference latency

## Packaging Decision

Preferred packaging strategy:

1. Keep the selected weight artifact in-repo as versioned JSON
2. Embed the artifact with `go:embed` for offline default behavior
3. Store a SHA256 manifest for startup integrity checks
4. Allow an explicit external override path for local experiments
5. Reject mismatched checksum and fall back to heuristic mode

Reproducibility requirements:

- deterministic build pins model artifact path and checksum
- release notes record model version, checksum, and threshold
- checksum verification is covered by unit tests

## CPU Fallback Behavior

Runtime selection order:

1. if classifier artifact loads and checksum matches, use classifier
2. else use current `MDS029` heuristic scoring path
3. if classifier inference exceeds timeout, degrade to heuristic

Fallback must preserve rule behavior guarantees:

- deterministic output for same input and config
- no runtime network dependency
- consistent diagnostic schema regardless of backend

## Distribution Constraints

- Keep embedded artifact footprint small enough to avoid major binary bloat
- Record per-platform binary size delta before enabling by default
- Keep third-party runtime dependencies minimal for portability
- Include model/weights license notes in rule docs and release notes

## Integration Plan

1. Add classifier interface and output schema in
   `internal/rules/concisenessscoring/`
2. Add model loader with checksum verification and timeout controls
3. Add backend switch (`classifier`, `heuristic`, `auto`) in config
4. Add integration tests for classifier path and forced fallback path
5. Update MDS029 docs in
   `internal/rules/MDS029-conciseness-scoring/` with thresholds,
   packaging, and fallback semantics

## Tasks

1. Define detection contract:
   labels, score threshold policy, and expected output schema.
2. Shortlist lightweight classifier models suitable for local use
   (size, license, CPU speed, quality).
3. Build an evaluation harness over a labeled corpus and compare
   precision, recall, F1, and latency on CPU.
4. Choose a model artifact packaging strategy:
   embedded assets vs bundled files, checksum validation,
   and update workflow.
5. Implement runtime selection and fallback behavior so detection
   works in CPU-only environments without accelerator assumptions.
6. Document distribution constraints:
   binary size impact, model weight footprint, and licensing notes.

## Acceptance Criteria

- [ ] One classifier model is selected with documented quality
      and CPU performance metrics.
- [x] Model artifact packaging strategy is documented and reproducible.
- [ ] CPU fallback behavior is specified and validated.
- [x] Integration plan is ready for implementation in mdsmith.
