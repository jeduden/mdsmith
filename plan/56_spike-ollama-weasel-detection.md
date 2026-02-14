---
id: 56
title: Spike Ollama for Weasel Detection
status: 🔲
template:
  allow-extra-sections: true
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

## Acceptance Criteria

- [ ] Deterministic output is confirmed under fixed controls.
- [ ] CPU performance metrics are documented for benchmark files.
- [ ] Candidate model quality trade-offs are documented.
- [ ] Clear recommendation is produced for mdsmith adoption.
