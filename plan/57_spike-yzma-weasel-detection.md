---
id: 57
title: Spike yzma for Embedded Weasel Detection
status: ✅
---
# Spike yzma for Embedded Weasel Detection

## Goal

Evaluate yzma as an embedded Go inference path for deterministic,
low-overhead weasel-language detection.

## Tasks

1. Build a minimal yzma prototype inside mdsmith or a side package
   to test direct in-process inference.
2. Verify deterministic decoding behavior using greedy settings
   (equivalent to `temperature: 0`) and controlled prompt input.
3. Measure CPU performance and memory footprint vs service-based
   approaches on the same benchmark corpus.
4. Evaluate packaging complexity for binaries across supported
   platforms (build flags, runtime dependencies, portability).
5. Define integration boundaries:
   optional build tags, feature flags, and graceful fallback mode.
6. Document maintenance risk:
   dependency churn, model compatibility, and upgrade strategy.

## Results

See `eval/conciseness/spikes/yzma-embedded-weasel-detection/README.md`.

Highlights from the spike:

- Deterministic outputs were identical across repeated runs and process
  restarts under greedy decoding (`temperature: 0`).
- Embedded mode outperformed local service mode in observed warm runs
  on this machine, with lower average and P95 latency.
- Embedded mode used lower RSS than service mode in observed runs.
- Packaging impact is manageable for optional use:
  +0.5 MB binary delta, plus model and runtime library artifacts.
- Recommendation: proceed with optional embedded integration, keep
  heuristic/default path as fallback until classifier selection and
  implementation are complete (plan 58).

## Acceptance Criteria

- [x] Deterministic behavior is verified for repeated runs.
- [x] CPU performance and memory metrics are captured.
- [x] Build and packaging constraints are documented.
- [x] Recommendation is made on embedded inference feasibility.
