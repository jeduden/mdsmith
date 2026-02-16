# Ollama Weasel Detection Spike

## Goal

Evaluate Ollama as a deterministic local inference backend for
weasel-language detection in mdsmith.

Plan: `plan/56_spike-ollama-weasel-detection.md`.

## Environment

- Date: 2026-02-16
- Host: macOS (`darwin/arm64`)
- Runtime: Docker
- Image repo: `ollama/ollama`
- Image digest:
  `sha256:dca1224ecd799f764b21b8d74a17afdc00505ecce93e7b55530d124115b42260`
- Ollama version: `0.16.1`
- Endpoint: `http://127.0.0.1:11434/api/generate`
- Models:
  - `qwen2.5:0.5b` (397,821,319 bytes, Q4_K_M)
  - `llama3.2:1b` (1,321,098,329 bytes, Q8_0)
  - `smollm2:360m` (725,566,512 bytes, F16)
- Corpus:
  `eval/conciseness/spikes/ollama-weasel-detection/corpus/*.md`

## Reproducible Setup

```bash
docker pull \
  ollama/ollama@sha256:dca1224ecd799f764b21b8d74a17afdc00505ecce93e7b55530d124115b42260
docker run -d --rm --name ollama-plan56 -p 11434:11434 \
  ollama/ollama@sha256:dca1224ecd799f764b21b8d74a17afdc00505ecce93e7b55530d124115b42260

docker exec ollama-plan56 ollama pull qwen2.5:0.5b
docker exec ollama-plan56 ollama pull llama3.2:1b
docker exec ollama-plan56 ollama pull smollm2:360m

RUNS=3 eval/conciseness/spikes/ollama-weasel-detection/run.sh
```

The benchmark script writes artifacts to:
`.tmp/eval/conciseness/spikes/ollama-weasel-detection/`.

To run against a non-Docker Ollama endpoint, set `CONTAINER=""`
and optionally override `API_URL` / `TAGS_URL`.

## Eval Fixes Applied

The first spike run exposed eval-path bugs and mode collapse symptoms.
The harness was fixed before final metrics were captured:

1. Added a clear non-weasel control:
   `corpus/direct-03.md`.
2. Switched contract to boolean `is_weasel` to avoid contradictory
   natural-language labels.
3. Replaced `format: "json"` with strict JSON schema requiring
   `is_weasel`, `confidence`, and `rationale`.
4. Fixed false-value parsing bug (`false` had been treated as missing).
5. Added summary sections:
   `label_distribution` and `label_collapse`.

## Deterministic Controls

Each request used:

- `temperature: 0`
- `top_p: 1`
- fixed `seed: 42`
- fixed prompt template with calibration examples
- strict JSON schema output
- `stream: false`

## Results

### Determinism (same process)

All model+file pairs produced one unique response hash across repeated runs:
`unique_hashes=1` for all 15 combinations.

### Determinism (restart)

After container restart, `restart-a` and `restart-b` hashes matched
for all 15 model+file combinations.

### CPU Performance and Memory

Steady metrics (`RUNS=3`, 15 steady requests per model):

| Model        | Avg latency (s) | Max latency (s) | Avg tokens/s | Warm avg latency (s) | Restart memory range |
|--------------|----------------:|----------------:|-------------:|---------------------:|----------------------|
| `qwen2.5:0.5b` | 2.2780          | 3.4618          | 16.50        | 2.2503               | 768 MiB - 908 MiB    |
| `llama3.2:1b`  | 3.9316          | 8.4270          | 9.13         | 3.8803               | 2.51 GiB - 2.71 GiB  |
| `smollm2:360m` | 2.6284          | 3.6367          | 12.31        | 2.5370               | 3.64 GiB - 3.69 GiB  |

Restart load durations (`restart-b`, `direct-01`):

- `qwen2.5:0.5b`: 2.879 s
- `llama3.2:1b`: 9.421 s
- `smollm2:360m`: 5.548 s

### Candidate Model Quality Trade-Offs

Quality remains weak on this spike corpus:

- `qwen2.5:0.5b`: accuracy `0.600`, parse rate `1.000`
- `llama3.2:1b`: accuracy `0.400`, parse rate `1.000`
- `smollm2:360m`: accuracy `0.400`, parse rate `1.000`

Observed bias patterns:

- `qwen2.5:0.5b`: 12 `weasel`, 3 `direct`
- `llama3.2:1b`: 12 `direct`, 3 `weasel`
- `smollm2:360m`: 15 `weasel`, 0 `direct` (`collapse=1`)

Control text behavior (`direct-03`: "build completed ... exit code 0"):

- `qwen2.5:0.5b`: predicted `weasel`
- `llama3.2:1b`: predicted `direct`
- `smollm2:360m`: predicted `weasel`

Trade-off summary:

- `qwen2.5:0.5b` is fastest and has best spike accuracy, but
  still over-flags direct text.
- `llama3.2:1b` is slower and under-flags weasel text.
- `smollm2:360m` still collapses to one class in this setup.

## Proposed mdsmith Integration Contract

Use Ollama only as an optional external backend with strict fallback.

```yaml
conciseness-scoring:
  provider: ollama
  endpoint: http://127.0.0.1:11434/api/generate
  model: qwen2.5:0.5b
  timeout: 8s
  retries: 0
  deterministic:
    temperature: 0
    top_p: 1
    seed: 42
```

Request/response contract:

- input: one paragraph string
- output: JSON object with:
  `is_weasel` (bool), `confidence` (number), `rationale` (string)
- strict schema validation before use
- no streaming

Failure policy:

- connection error, timeout, or schema-parse error must not fail lint run
- fallback to heuristic path (`MDS029` scoring logic)
- emit debug details only in verbose mode

## Operational Constraints

- First pull requires network for image and model artifacts.
- Repeated runs can work offline after models are cached.
- Startup and first-request latency are material for CLI workflows.
- Disk footprint grows with each pulled model.
- CI should pre-pull one chosen model and pin digest/version.

## Recommendation

Defer Ollama adoption as a default mdsmith backend.

Reasoning:

1. Determinism is good with fixed controls and schema enforcement.
2. CPU latency is acceptable for optional mode but still non-trivial.
3. Quality is not stable enough yet across candidates.
   One model still collapses to one class.

Ollama can remain experimental only if:

- strict schema validation and fallback behavior are kept, and
- later classifier/model selection work improves quality consistency.
