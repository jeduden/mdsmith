# Conciseness Evaluation Baseline

This folder defines baseline inputs and outputs for comparing
conciseness approaches.

Current implementation status: `MDS029` is experimental and disabled by
default until classifier-based evaluation is complete.

## Files

- `rubric.md`: annotation policy for paragraph labels.
- `dataset.schema.cue`: record schema for labeled examples.
- `approach-matrix.md`: approach definitions and threshold policy.
- `scorecard-template.md`: report template for experiment results.
- `spikes/yzma-embedded-weasel-detection/README.md`:
  embedded yzma spike findings and recommendation.
- `spikes/go-native-linear-classifier/README.md`:
  fully embedded pure-Go classifier spike findings.
- `spikes/wasm-embedded-inference/README.md`:
  embedded wasm inference spike design.

## Dataset Format

Store datasets as JSONL files where each line matches
`dataset.schema.cue` (`#ConcisenessEvalRecord`).

For cue-based experiments, populate `cues` with matched
token or phrase evidence. Keep cue tags consistent with
schema kinds (`filler`, `hedge`, `verbose-phrase`,
`redundancy`, `other`).

Recommended files:

- `train.jsonl`
- `dev.jsonl`
- `test.jsonl`
- `holdout-outofdomain.jsonl`

## Sampling Requirements

Use at least 5 document types in each baseline release:

- guide
- tutorial
- reference
- runbook
- policy

Per split, target this minimum balance:

- `train`: at least 20 records per document type
- `dev`: at least 8 records per document type
- `test`: at least 8 records per document type
- `holdout-outofdomain`: at least 5 records per document type

If a type has too few records, label the gap in the scorecard and
exclude promotion decisions for that type.

## Split Rules

1. Split by file (never by paragraph within one file).
2. Keep document-type distribution similar across splits.
3. Freeze test and holdout before threshold tuning.
4. Use `dev` for threshold and calibration choices only.
5. Do not move records from `test` or `holdout` after tuning starts.
6. Keep an immutable dataset version tag in each scorecard run.

See `approach-matrix.md` for threshold policy and decision gates.
