# Conciseness Experiment Scorecard

## Run Metadata

- Date:
- Commit:
- Dataset version:
- Evaluator:

## Approaches

| ID  | Approach                   | Description                             |
|-----|----------------------------|-----------------------------------------|
| A0  | MDS029 heuristic           | Current shipped heuristic               |
| A1  | Heuristic + lexicon tuning | Expanded or tuned cue lists             |
| B0  | Simple classifier baseline | Logistic regression or similar baseline |
| B1  | PR #15 classifier          | Candidate classifier model              |
| C0  | Hybrid                     | Heuristic prefilter + classifier        |

## Dev Set (threshold tuning only)

| ID  | Threshold | Precision | Recall | F0.5 | AUPRC | Brier |
|-----|-----------|-----------|--------|------|-------|-------|
| A0  |           |           |        |      |       |       |
| A1  |           |           |        |      |       |       |
| B0  |           |           |        |      |       |       |
| B1  |           |           |        |      |       |       |
| C0  |           |           |        |      |       |       |

## Frozen Test Set

| ID  | Precision | Recall | F0.5 | AUPRC | Brier | Diags/KLOC | p95 latency ms |
|-----|-----------|--------|------|-------|-------|------------|----------------|
| A0  |           |        |      |       |       |            |                |
| A1  |           |        |      |       |       |            |                |
| B0  |           |        |      |       |       |            |                |
| B1  |           |        |      |       |       |            |                |
| C0  |           |        |      |       |       |            |                |

## Disagreement Review

- Sample size:
- Blind review pass rate:
- Top false-positive theme:
- Top false-negative theme:

## Decision Gate

- Precision improvement vs A0:
- Recall delta vs A0:
- Runtime overhead vs A0:
- Recommendation:
