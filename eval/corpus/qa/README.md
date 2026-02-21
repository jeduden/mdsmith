# QA Annotation Guide

1. Open `qa-sample.jsonl` from the dataset version you are
   reviewing.
2. For each row, decide the `actual_category` (`reference` or
   `other`).
3. Write annotations to `annotations.csv` with this header:

```csv
record_id,actual_category
```

4. Run `corpusctl qa` to compute agreement, category precision,
   recall, and Cohen's kappa.
