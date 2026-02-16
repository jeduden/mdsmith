# Corpus Workflow

This folder implements Plan 62 corpus acquisition and taxonomy
workflow for Markdown evaluation datasets.

## Deliverables

- `taxonomy.md`: category definitions, boundary rules, and examples.
- `policy.md`: source inclusion and exclusion policy.
- `config.yml`: deterministic build config.
- `datasets/v2026-02-16/`: frozen manifest, QA sample, and reports.
- `refresh.md`: periodic refresh and drift process.

## Build Corpus

```bash
go run ./cmd/corpusctl build \
  -config eval/corpus/config.yml \
  -out eval/corpus/datasets/v2026-02-16
```

The build writes:

- `manifest.jsonl`: one record per file with provenance metadata.
- `report.json`: filter, dedup, split, and balance statistics.
- `qa-sample.jsonl`: stratified sample for manual label QA.

## Run Manual QA Metrics

1. Label `qa-sample.jsonl` into a CSV with columns:
   `record_id,actual_category`.
2. Run:

```bash
go run ./cmd/corpusctl qa \
  -sample eval/corpus/datasets/v2026-02-16/qa-sample.jsonl \
  -annotations eval/corpus/qa/annotations.csv \
  -out eval/corpus/datasets/v2026-02-16/qa-report.json
```

## Run Drift Report

```bash
go run ./cmd/corpusctl drift \
  -baseline eval/corpus/datasets/v2026-02-16/report.json \
  -candidate eval/corpus/datasets/vNEXT/report.json \
  -out eval/corpus/datasets/vNEXT/drift-report.json
```

Use `refresh.md` for cadence and release checklist.
