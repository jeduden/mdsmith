# QA Annotation Guide

1. Open `qa-sample.jsonl` from the dataset version you are
   reviewing. Use `qa-init` to generate a matching template:

```bash
go run ./cmd/corpusctl qa-init \
  -sample docs/research/corpus/datasets/<version>/qa-sample.jsonl \
  -out docs/research/corpus/qa/annotations.csv
```

2. For each row, decide the `actual_category` (`reference` or
   `other`).
3. Write annotations to `annotations.csv` with this header:

```csv
record_id,actual_category
```

4. Keep annotations scoped to the current sample.
`annotations.csv` may include fewer IDs than `qa-sample.jsonl`,
but it must not include IDs that are not in the sample.

5. Run `corpusctl qa` to compute coverage, agreement, category
precision/recall, and Cohen's kappa.
