# Conciseness Experiment Scorecard

## Run Metadata

- Date:
- Commit:
- Dataset version:
- Evaluator:
- CPU/OS:
- Go version:
- Arch:

## Threshold Policy Reference

Apply threshold policy from `approach-matrix.md` before filling
frozen `test` and holdout sections.

## Approaches

| ID  | Approach                   | Description                             |
|-----|----------------------------|-----------------------------------------|
| A0  | MDS029 heuristic           | Current shipped heuristic               |
| A1  | Heuristic + lexicon tuning | Expanded or tuned cue lists             |
| B0  | Simple classifier baseline | Logistic regression or similar baseline |
| B1  | Selected candidate         | Selected candidate from plan 58         |
| C0  | Hybrid                     | Heuristic prefilter + classifier        |

## Dev Set (threshold tuning only)

| ID  | Threshold | Precision | Recall | F0.5 | AUPRC | Brier | Latency ms (p50/p95) |
|-----|-----------|-----------|--------|------|-------|-------|----------------------|
| A0  |           |           |        |      |       |       |                      |
| A1  |           |           |        |      |       |       |                      |
| B0  |           |           |        |      |       |       |                      |
| B1  |           |           |        |      |       |       |                      |
| C0  |           |           |        |      |       |       |                      |

## Frozen Test Set

| ID  | Precision | Recall | F0.5 | AUPRC | Brier | Diags/KLOC | Latency ms (p50/p95) |
|-----|-----------|--------|------|-------|-------|------------|----------------------|
| A0  |           |        |      |       |       |            |                      |
| A1  |           |        |      |       |       |            |                      |
| B0  |           |        |      |       |       |            |                      |
| B1  |           |        |      |       |       |            |                      |
| C0  |           |        |      |       |       |            |                      |

## Holdout Set

| ID  | Precision | Recall | F0.5 | AUPRC | Brier | Diags/KLOC | Latency ms (p50/p95) |
|-----|-----------|--------|------|-------|-------|------------|----------------------|
| A0  |           |        |      |       |       |            |                      |
| A1  |           |        |      |       |       |            |                      |
| B0  |           |        |      |       |       |            |                      |
| B1  |           |        |      |       |       |            |                      |
| C0  |           |        |      |       |       |            |                      |

## Disagreement Review

- Sample size:
- Blind review pass rate:
- Top false-positive theme:
- Top false-negative theme:

## Shadow-Trial Outcomes

| Measure                   | Value |
|---------------------------|-------|
| Files in trial            |       |
| Words in trial            |       |
| Reviewer agreement rate   |       |
| False-positive escalation |       |
| False-negative escalation |       |
| Rollout recommendation    |       |

## Decision Gate

| Gate                              | Target     | Result | Pass |
|-----------------------------------|------------|--------|------|
| Precision delta vs A0             | `>= +0.05` |        |      |
| Recall delta vs A0                | `>= -0.02` |        |      |
| F0.5 delta vs A0                  | `>= +0.05` |        |      |
| p95 latency ratio vs A0           | `<= 2.0x`  |        |      |
| Diagnostics per KLOC change vs A0 | `<= +10%`  |        |      |
| Holdout precision drop vs test    | `<= 0.05`  |        |      |

- Final recommendation:
