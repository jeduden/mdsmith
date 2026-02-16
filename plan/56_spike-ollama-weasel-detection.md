---
id: 56
title: Spike Ollama for Weasel Detection
status: ✅
---
# Spike Ollama for Weasel Detection

## Goal

Evaluate Ollama as a deterministic local inference backend
for weasel-language detection in mdsmith.

## Tasks

1. Build a reproducible Ollama spike setup suitable for local
   development and CI-like environments.
2. Verify deterministic behavior using fixed sampling controls
   (`temperature: 0`, fixed `seed`, stable prompt template).
3. Measure CPU latency, throughput, and memory across a small
   markdown benchmark set.
4. Compare candidate lightweight models available in Ollama
   for consistency and detection quality.
5. Define mdsmith integration contract:
   invocation mode, timeout/retry policy, and strict fallback path.
6. Document operational constraints: model pull strategy,
   artifact caching, and offline execution behavior.

## Results

See `eval/conciseness/spikes/ollama-weasel-detection/README.md`.

Highlights from the spike:

- Deterministic output was stable under fixed controls
  (`temperature: 0`, fixed `seed`), including post-restart checks.
- CPU latency and memory were measured across 3 lightweight candidates:
  `qwen2.5:0.5b`, `llama3.2:1b`, and `smollm2:360m`.
- Candidate quality was weak on the spike corpus:
  all models over-predicted `weasel` and scored `0.500` accuracy.
- Integration contract is defined as optional external provider with
  strict timeout/no-retry behavior and mandatory fallback path.
- Recommendation: defer default adoption; keep Ollama experimental only.

## Acceptance Criteria

- [x] Deterministic output is confirmed under fixed controls.
- [x] CPU performance metrics are documented for benchmark files.
- [x] Candidate model quality trade-offs are documented.
- [x] Clear recommendation is produced for mdsmith adoption.
