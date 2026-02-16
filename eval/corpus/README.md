# Corpus Workflow

This folder implements Plan 62 corpus acquisition and taxonomy
workflow for Markdown evaluation datasets.

## Deliverables

- `taxonomy.md`: category definitions, boundary rules, and examples.
- `policy.md`: source inclusion and exclusion policy.
- `config.yml`: pinned remote source set (hashes + annotations).
- `config.local.yml`: local-only source set for fast iteration.
- `measure.sh`: thin wrapper around `corpusctl measure`.
- `datasets/v2026-02-16/`: frozen manifest, QA sample, and reports.
- `refresh.md`: periodic refresh and drift process.

## Source of Truth

Pinned source selection now lives in `config.yml`, not in shell code.
Each source entry includes:

- `commit_sha`: immutable pin,
- `license`: allowlist gate,
- `annotations`: rationale and content-scope metadata,
- `quality`: policy metadata snapshot.

## Measure Process (Pinned Remote Sources)

Canonical command:

```bash
go run ./cmd/corpusctl measure \
  -config eval/corpus/config.yml \
  -out eval/corpus/datasets/v2026-02-16
```

Wrapper command (equivalent defaults):

```bash
./eval/corpus/measure.sh
```

The measure flow:

- fetches each source at pinned `commit_sha`,
- verifies fetched hash matches pin,
- builds corpus artifacts,
- writes `config.generated.yml` into dataset output for provenance.

## Build Corpus (No Fetch)

If sources are already present on disk, use:

```bash
go run ./cmd/corpusctl build \
  -config eval/corpus/config.yml \
  -out eval/corpus/datasets/v2026-02-16
```

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
