---
id: 55
title: Spike LocalAI for Weasel Detection
status: ✅
---
# Spike LocalAI for Weasel Detection

## Goal

Evaluate LocalAI as a deterministic local inference backend
for weasel-language detection in mdsmith.

## Tasks

1. Build a reproducible LocalAI spike setup for local execution
   (no runtime network dependency).
2. Verify deterministic behavior using fixed sampling controls
   (`temperature: 0`, fixed `seed`, constrained sampling params).
3. Run latency and memory measurements on CPU using a small
   representative markdown corpus.
4. Validate output consistency across repeated runs and process restarts.
5. Define integration shape for mdsmith:
   optional external service, request format, timeout behavior,
   and fallback when service is unavailable.
6. Record operational footprint: model artifacts, startup cost,
   portability, and licensing constraints.

## Acceptance Criteria

- [ ] Determinism is demonstrated with repeated identical outputs
      under fixed settings.
- [ ] CPU latency and memory metrics are documented for the spike corpus.
- [ ] Integration approach and failure handling are documented.
- [ ] Recommendation is made: proceed, defer, or reject for mdsmith.

## Notes

- Spike report:
  [`spikes/localai-weasel-detection/README.md`](
    ../spikes/localai-weasel-detection/README.md)
