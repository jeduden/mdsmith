---
id: 64
title: Spike Pure-Go Embedded Weasel Classifier
status: 🔲
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

## Acceptance Criteria

- [ ] Prototype runs with no `YZMA_LIB` or external dynamic libraries.
- [ ] Embedded weights load from binary-only assets.
- [ ] Deterministic outputs are confirmed across repeat runs.
- [ ] CPU latency and memory metrics are captured.
- [ ] Binary-size delta is measured and documented.
- [ ] Recommendation is made: adopt, defer, or reject this path.
