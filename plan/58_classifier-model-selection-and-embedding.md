---
id: 58
title: Select and Package Fast Weasel Classifier (CPU Fallback)
status: 🔲
template:
  allow-extra-sections: true
---
# Select and Package Fast Weasel Classifier (CPU Fallback)

## Goal

Pick a fast classifier for weasel-language detection.
Package its weights for offline use. Define a CPU fallback path.

## Tasks

1. Define detection contract:
   labels, score threshold policy, and expected output schema.
2. Shortlist lightweight classifier models suitable for local use
   (size, license, CPU speed, quality).
3. Build an evaluation harness over a labeled corpus and compare
   precision, recall, F1, and latency on CPU.
4. Choose a model artifact packaging strategy:
   embedded assets vs bundled files, checksum validation,
   and update workflow.
5. Implement runtime selection and fallback behavior so detection
   works in CPU-only environments without accelerator assumptions.
6. Document distribution constraints:
   binary size impact, model weight footprint, and licensing notes.

## Acceptance Criteria

- [ ] One classifier model is selected with documented quality
      and CPU performance metrics.
- [ ] Model artifact packaging strategy is documented and reproducible.
- [ ] CPU fallback behavior is specified and validated.
- [ ] Integration plan is ready for implementation in mdsmith.
