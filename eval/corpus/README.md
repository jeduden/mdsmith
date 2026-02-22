# Corpus Pipeline

This directory stores the Plan 62 corpus pipeline files.
It includes inputs, policy docs, and dataset outputs.

## Inputs

- `config.yml`: canonical source list with pinned commits,
  licenses, and per-source annotations.
- `config.local.yml`: optional local root overrides; ignored by Git.
  Start from `config.local.example.yml`.
- `taxonomy.md`: category definitions and classifier heuristic.
- `policy.md`: license and threshold policy.

## Build Dataset

Build from pinned remote repositories:

```bash
go run ./cmd/corpusctl build \
  -config eval/corpus/config.yml \
  -cache /tmp/corpusctl-cache \
  -out eval/corpus/datasets/v2026-02-16
```

Build with local overrides (no remote fetch if local roots exist):

```bash
cp eval/corpus/config.local.example.yml eval/corpus/config.local.yml
# edit local paths

go run ./cmd/corpusctl build \
  -config eval/corpus/config.yml \
  -out eval/corpus/datasets/v2026-02-16
```

## Measure Existing Corpus

`measure` reads an existing `manifest.jsonl` and writes aggregate
metrics by category.

```bash
go run ./cmd/corpusctl measure \
  -corpus eval/corpus/datasets/v2026-02-16 \
  -out eval/corpus/datasets/v2026-02-16/measure-report.json
```

## QA Evaluation

```bash
go run ./cmd/corpusctl qa \
  -sample eval/corpus/datasets/v2026-02-16/qa-sample.jsonl \
  -annotations eval/corpus/qa/annotations.csv \
  -out eval/corpus/datasets/v2026-02-16/qa-report.json
```

## Drift Detection

```bash
go run ./cmd/corpusctl drift \
  -baseline eval/corpus/datasets/v2025-12-15/report.json \
  -candidate eval/corpus/datasets/v2026-02-16/report.json \
  -out eval/corpus/datasets/v2026-02-16/drift-report.json
```

Only metadata and computed measurements are versioned under
`datasets/`. Source markdown content is not vendored.
