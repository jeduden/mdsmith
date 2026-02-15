# Conciseness Evaluation Baseline

This folder defines baseline inputs and outputs for comparing
conciseness approaches.

## Files

- `rubric.md`: annotation policy for paragraph labels.
- `dataset.schema.json`: record schema for labeled examples.
- `scorecard-template.md`: report template for experiment results.

## Dataset Format

Store datasets as JSONL files where each line matches
`dataset.schema.json`.

For cue-based experiments, populate `cues` with matched
token or phrase evidence. Keep cue tags consistent with
schema kinds (`filler`, `hedge`, `verbose-phrase`,
`redundancy`, `other`).

Recommended files:

- `train.jsonl`
- `dev.jsonl`
- `test.jsonl`
- `holdout-outofdomain.jsonl`

## Split Rules

1. Split by file (never by paragraph within one file).
2. Keep document-type distribution similar across splits.
3. Freeze test and holdout before threshold tuning.
4. Use `dev` for threshold and calibration choices only.
