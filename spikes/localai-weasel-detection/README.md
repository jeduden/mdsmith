---
title: LocalAI Weasel Detection Spike
description: Determinism, CPU latency, memory, and integration
  trade-offs for LocalAI as a weasel-language backend.
---
# LocalAI Weasel Detection Spike

## Scope

This spike evaluates LocalAI as a local inference backend for
weasel-language detection in mdsmith (plan 55).
The target was deterministic behavior under fixed settings,
plus CPU latency and memory measurements.

## Environment

- Host: macOS (`darwin/arm64`)
- Runtime: Docker
- LocalAI image: `localai/localai:latest-aio-cpu`
- LocalAI binary version: `v3.11.0`
- Text model: `llama-3.2-1b-instruct:q4_k_m`
- Corpus:
  `spikes/localai-weasel-detection/corpus/*.md`

## Reproducible Setup

```bash
docker run -d --rm --name localai-plan55 -p 18080:8080 \
  localai/localai:latest-aio-cpu

docker exec localai-plan55 /local-ai models install \
  localai@llama-3.2-1b-instruct:q4_k_m

docker restart localai-plan55

LOCALAI_CONTAINER=localai-plan55 RUNS=6 \
  spikes/localai-weasel-detection/localai_weasel_benchmark.sh
```

The script stores raw responses and metrics under
`.tmp/spikes/localai-weasel-detection/`.

## Deterministic Controls

The request used fixed sampling controls:

- `temperature: 0`
- `top_p: 1`
- `seed: 42`
- `presence_penalty: 0`
- `frequency_penalty: 0`

## Results

### Determinism (same process)

Across 6 repeated calls per file, each file produced one unique response hash:

- `direct-01`: `unique_hashes=1`
- `direct-02`: `unique_hashes=1`
- `weasel-01`: `unique_hashes=1`
- `weasel-02`: `unique_hashes=1`

### Determinism (restart)

A restart check (`restart-a` vs `restart-b`) produced identical hashes
for all four files.

### CPU Latency

| File      | Avg latency (s) | Max latency (s) |
|-----------|----------------:|----------------:|
| `direct-01` | 5.8662          | 6.8727          |
| `direct-02` | 5.6662          | 6.4932          |
| `weasel-01` | 4.0441          | 4.2428          |
| `weasel-02` | 3.1619          | 3.4312          |

Overall: average `4.6846s`, min `2.8216s`, max `6.8727s` (`n=24`).

### Memory

`docker stats` samples during requests:

- Average: `495.5 MiB`
- Range: `473.3 MiB` to `510.8 MiB`

### Startup Cost

Restart-to-ready measurements:

- Run 1: `18s`
- Run 2: `16s`
- Run 3: `14s`

### Operational Footprint

- LocalAI image size: `292,172,060` bytes (~279 MiB)
- `/models` footprint in this setup: `2.6 GiB`
- First-time setup requires network pulls for image and models.
- After model install, restart runs used local artifacts only.
- Key artifacts:
  - `llama-3.2-1b-instruct-q4_k_m.gguf`: `771 MiB`
  - `stable-diffusion-v1-5-pruned-emaonly-Q4_0.gguf`: `1.5 GiB`
  - `granite-embedding-107m-multilingual-f16.gguf`: `211 MiB`
- Model gallery metadata declares license: `llama3.2`
- Startup logs repeatedly showed a bundled image backend error:
  missing `/backends/cpu-stablediffusion-ggml/run.sh`.

## Output and Quality Notes

- Determinism was strong with fixed sampling.
- Output schema compliance was weak:
  strict JSON parse succeeded `0/24` times on raw message content.
- One direct example produced an unrelated shell command response.
- In this tiny corpus sample, direct examples were not reliably
  classified as direct.

These quality issues are model/prompt-level, but they affect
backend viability for strict lint diagnostics.

## Proposed mdsmith Integration Shape

Use LocalAI as an optional external service backend:

```yaml
weasel-detection:
  provider: localai
  endpoint: http://127.0.0.1:18080/v1/chat/completions
  model: llama-3.2-1b-instruct:q4_k_m
  timeout: 8s
  retries: 0
  deterministic:
    temperature: 0
    top-p: 1
    seed: 42
```

Request contract:

- single paragraph in, structured label out (`weasel`/`direct` + score)
- no streaming
- strict timeout and no retry by default to protect lint latency

Failure handling:

- On connection errors, timeout, or parse failures:
  - do not fail the lint run
  - skip this check (or run a local heuristic fallback if enabled)
  - emit debug-only telemetry in verbose mode

## Recommendation

Defer LocalAI adoption for mdsmith weasel detection as a default path.

Reasoning:

1. Determinism is acceptable, but output-shape reliability is not.
2. End-to-end CPU latency is high for per-paragraph lint usage.
3. Operational footprint is heavy for default local tooling.

LocalAI remains viable as an experimental optional backend after:

- stronger prompt/grammar constraints with high parse success, and
- model selection work focused on small, stable classifiers.
