# Corpus Workflow

This folder implements Plan 62 corpus acquisition and taxonomy
workflow for Markdown evaluation datasets.

## Deliverables

- `taxonomy.md`: category definitions, boundary rules, and examples.
- `policy.md`: source inclusion and exclusion policy.
- `config.yml`: deterministic build config.
- `measure.sh`: pinned external-source download + build workflow.
- `datasets/v2026-02-16/`: frozen manifest, QA sample, and reports.
- `refresh.md`: periodic refresh and drift process.

## Measure Process (Pinned Remote Sources)

Run:

```bash
./eval/corpus/measure.sh
```

This process:

- downloads selected MIT and CC-BY-4.0 repositories to `/tmp`
  at pinned commit hashes,
- verifies each checkout hash,
- generates `config.generated.yml` for traceability,
- runs `corpusctl build` against downloaded sources,
- writes outputs under `eval/corpus/datasets/<dataset_version>/`.

Pinned source set in `measure.sh`:

| Source                      | License   | Commit hash                              |
|-----------------------------|-----------|------------------------------------------|
| `openai/openai-cookbook`      | MIT       | `365dfaa2ef36e0a6b7639ba8d211a451e0e90455` |
| `openai/openai-agents-python` | MIT       | `84fa471e5fc538d744a3ae294749fedb3855131b` |
| `langchain-ai/langchain`      | MIT       | `fb0233c9b9cdb95386e8fbb96c5421245fc192d3` |
| `langchain-ai/langgraph`      | MIT       | `7216504ce2ecb56f62ebb08ac787d11b7491de5b` |
| `microsoft/semantic-kernel`   | MIT       | `91f795605e42f0dd03ed9cdfaf4ffd8bdb1ae553` |
| `anthropics/claude-cookbooks` | MIT       | `7cb72a9c879e3b95f58d30a3d7483906e9ad548e` |
| `kubernetes/website`          | CC-BY-4.0 | `695611df58280618252e50edf3962a8bd324731a` |
| `microsoft/autogen`           | CC-BY-4.0 | `13e144e5476a76ca0d76bf4f07a6401d133a03ed` |

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
