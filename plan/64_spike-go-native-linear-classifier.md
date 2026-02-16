---
id: 64
title: Spike Pure-Go Embedded Weasel Classifier
status: ✅
---
# Spike Pure-Go Embedded Weasel Classifier

## Goal

Evaluate a fully embedded, pure-Go classifier path for weasel-language
(or `verbose-actionable`) detection with no runtime dynamic libraries.

## Tasks

1. Define a pure-Go model family to evaluate first
   (for example sparse linear classifier over cue and n-gram features).
2. Build a minimal prototype inference package that runs with stdlib-only
   runtime dependencies and deterministic scoring.
3. Define a weight packaging path that is fully embedded in the mdsmith
   binary (for example `go:embed` plus checksum verification).
4. Measure CPU latency and memory on the same benchmark corpus used in
   previous weasel spikes.
5. Measure binary-size impact versus current mdsmith and compare with the
   yzma spike artifact footprint.
6. Define integration boundaries and fallback behavior for MDS029:
   backend mode switch, timeout policy, and diagnostic stability.
7. Document maintenance workflow: training export format, versioning,
   and safe model update procedure.

## Results

See `eval/conciseness/spikes/go-native-linear-classifier/README.md`.

Highlights from the spike:

- Prototype is fully pure-Go with stdlib-only runtime dependencies.
- Weights are embedded with `go:embed` and verified by pinned SHA256.
- Determinism was confirmed across in-process and process-restart runs
  (`unique_hashes=1`).
- Latency and memory metrics were captured on the same six-sample corpus
  used in the yzma spike.
- mdsmith binary delta for embedded artifact was measured at +1,136 bytes,
  versus yzma's +0.5 MB binary delta plus external model/library artifacts.
- Recommendation: adopt this path as the CPU fallback candidate for plan 58,
  pending full dataset quality validation.

## Acceptance Criteria

- [x] Prototype runs with no `YZMA_LIB` or external dynamic libraries.
- [x] Embedded weights load from binary-only assets.
- [x] Deterministic outputs are confirmed across repeat runs.
- [x] CPU latency and memory metrics are captured.
- [x] Binary-size delta is measured and documented.
- [x] Recommendation is made: adopt, defer, or reject this path.
