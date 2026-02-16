# Refresh Workflow

Run corpus refresh monthly or before major model evaluation runs.

## Steps

1. Copy `config.yml` to a new version folder and update:

  - `dataset_version`,
  - `collected_at`,
  - per-source `commit_sha` and quality metadata.

2. Build the dataset with `corpusctl build`.
3. Label QA sample and run `corpusctl qa`.
4. Compare drift against prior release with `corpusctl drift`.
5. Publish all artifacts under `datasets/<version>/`.

## Required Outputs Per Refresh

- `manifest.jsonl`
- `report.json`
- `qa-sample.jsonl`
- `qa-report.json`
- `drift-report.json`

## Drift Checks

Review these metrics before publishing:

- total record delta,
- category count and share deltas,
- README share delta,
- balance range violations,
- QA agreement and confusion deltas.

If drift is outside policy, adjust source selection or
balance thresholds and rerun.
