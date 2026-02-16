# Ollama Weasel Detection Spike

## Goal

Evaluate Ollama as a deterministic local inference backend for
weasel-language detection in mdsmith.

Plan: `plan/56_spike-ollama-weasel-detection.md`.

## Environment

- Date: 2026-02-16
- Host: macOS (`darwin/arm64`)
- Runtime: Docker
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
docker pull ollama/ollama:latest
docker run -d --rm --name ollama-plan56 -p 11434:11434 ollama/ollama:latest

docker exec ollama-plan56 ollama pull qwen2.5:0.5b
docker exec ollama-plan56 ollama pull llama3.2:1b
docker exec ollama-plan56 ollama pull smollm2:360m

RUNS=3 eval/conciseness/spikes/ollama-weasel-detection/run.sh
```

The benchmark script writes artifacts to:
`.tmp/eval/conciseness/spikes/ollama-weasel-detection/`.

## Deterministic Controls

Each request used:

- `temperature: 0`
- `top_p: 1`
- fixed `seed: 42`
- fixed prompt template
- `format: "json"` and `stream: false`

## Results

### Determinism (same process)

All model+file pairs produced one unique response hash across repeated runs:
`unique_hashes=1` for all 12 combinations.

### Determinism (restart)

After container restart, `restart-a` and `restart-b` hashes matched
for all 12 model+file combinations.

### CPU Performance and Memory

Steady metrics (`RUNS=3`, 12 steady requests per model):

| Model        | Avg latency (s) | Max latency (s) | Avg tokens/s | Warm avg latency (s) | Steady memory range |
|--------------|----------------:|----------------:|-------------:|---------------------:|---------------------|
| `qwen2.5:0.5b` | 6.0598          | 23.6725         | 16.23        | 4.6154               | 829 MiB - 952 MiB   |
| `llama3.2:1b`  | 7.6956          | 21.6986         | 7.13         | 6.5569               | 2.56 GiB - 2.73 GiB |
| `smollm2:360m` | 10.5636         | 21.3492         | 5.69         | 8.6820               | 3.66 GiB - 3.75 GiB |

Observed first-load durations from `load_duration`:

- `qwen2.5:0.5b`: 9.685 s
- `llama3.2:1b`: 14.912 s
- `smollm2:360m`: 11.213 s

### Candidate Model Quality Trade-Offs

Quality on this tiny spike corpus was poor for all candidates:

- parse rate: `1.000` for every model (strict JSON shape produced)
- accuracy: `0.500` for every model
- behavior: all models labeled every sample as `weasel`
- consequence: direct examples were always false positives

Trade-off summary:

- `qwen2.5:0.5b` was fastest and smallest but still misclassified
  all direct text.
- `llama3.2:1b` was slower and larger, with no quality gain on this corpus.
- `smollm2:360m` had the highest memory footprint here and no quality gain.

## Proposed mdsmith Integration Contract

Use Ollama only as an optional external backend with strict fallbacks.

```yaml
conciseness-scoring:
  provider: ollama
  endpoint: http://127.0.0.1:11434/api/generate
  model: qwen2.5:0.5b
  timeout: 8s
  retries: 0
  deterministic:
    temperature: 0
    top-p: 1
    seed: 42
```

Request/response contract:

- input: one paragraph string
- output: JSON object with `label`, `confidence`, `rationale`
- no streaming
- fixed deterministic options

Failure policy:

- connection error, timeout, or parse error must not fail lint run
- fallback to heuristic path (`MDS029` scoring logic)
- emit debug details only in verbose mode

## Operational Constraints

- First pull requires network for both image and model artifacts.
- Repeated runs can work offline if models are already cached.
- Startup and first-request latency are material for CLI workflows.
- Disk footprint grows with each pulled model.
- CI should pre-pull exactly one chosen model to avoid noisy latency.

## Recommendation

Defer Ollama adoption as a default mdsmith backend.

Reasoning:

1. Determinism is good under fixed controls.
2. CPU latency and startup cost are still high for per-paragraph linting.
3. Candidate quality in this spike is not acceptable
   (systematic direct-text false positives).

Ollama can remain an experimental optional backend only if:

- strict timeout + fallback behavior is implemented, and
- later classifier/model work (plans 58 and 59) improves quality.
