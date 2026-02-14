---
id: 57
title: Spike yzma for Embedded Weasel Detection
status: 🔲
template:
  allow-extra-sections: true
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

## Acceptance Criteria

- [ ] Deterministic behavior is verified for repeated runs.
- [ ] CPU performance and memory metrics are captured.
- [ ] Build and packaging constraints are documented.
- [ ] Recommendation is made on embedded inference feasibility.
