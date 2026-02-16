# Refresh Workflow

Run corpus refresh monthly or before major model evaluation runs.

## Steps

1. Update `eval/corpus/config.yml`:

  - `dataset_version`,
  - `collected_at`,
  - each source `commit_sha`,
  - any changed `quality` and `annotations` metadata.

2. Run measure:

```bash
go run ./cmd/corpusctl measure \
  -config eval/corpus/config.yml \
  -out eval/corpus/datasets/<version>
```

3. Label QA sample and run `corpusctl qa`.
4. Compare drift against prior release with `corpusctl drift`.
5. Publish all artifacts under `datasets/<version>/`.

## Required Outputs Per Refresh

- `manifest.jsonl`
- `report.json`
- `qa-sample.jsonl`
- `qa-report.json`
- `drift-report.json`
- `config.generated.yml`

## Drift Checks

Review these metrics before publishing:

- total record delta,
- category count and share deltas,
- README share delta,
- balance range violations,
- QA agreement and confusion deltas.

If drift is outside policy, adjust source selection or
balance thresholds and rerun.
