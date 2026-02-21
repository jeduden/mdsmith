# Refresh Workflow

Use this workflow to refresh the corpus dataset.

1. Update source pins in `eval/corpus/config.yml`.

- bump each source `commit_sha`
- update source `annotations` when selection rationale changes
- update `dataset_version` and `collected_at`

2. Build dataset.

```bash
go run ./cmd/corpusctl build \
  -config eval/corpus/config.yml \
  -cache /tmp/corpusctl-cache \
  -out eval/corpus/datasets/<dataset-version>
```

3. Annotate QA sample (`eval/corpus/qa/annotations.csv`).

4. Run QA report.

```bash
go run ./cmd/corpusctl qa \
  -sample eval/corpus/datasets/<dataset-version>/qa-sample.jsonl \
  -annotations eval/corpus/qa/annotations.csv \
  -out eval/corpus/datasets/<dataset-version>/qa-report.json
```

5. Run drift report against baseline.

```bash
go run ./cmd/corpusctl drift \
  -baseline eval/corpus/datasets/v2025-12-15/report.json \
  -candidate eval/corpus/datasets/<dataset-version>/report.json \
  -out eval/corpus/datasets/<dataset-version>/drift-report.json
```

6. Review outputs and commit the new dataset artifacts.
