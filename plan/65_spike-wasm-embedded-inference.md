---
id: 65
title: Spike WASM-Embedded Weasel Inference
status: ✅
---
# Spike WASM-Embedded Weasel Inference

## Goal

Evaluate a WASM-based inference path that can be embedded in mdsmith
with no runtime dynamic library dependency.

## Tasks

1. [x] Choose one Go-hosted WASM runtime candidate
   (`wazero` v1.11.0 with the default wazevo optimizing compiler)
   and define module loading strategy.
2. [x] Build a minimal proof of concept that embeds a `.wasm`
   artifact with `go:embed` and runs inference in-process. The
   guest reuses the plan-64 classifier package verbatim, compiled
   with `GOOS=wasip1 GOARCH=wasm -buildmode=c-shared`.
3. [x] Verify deterministic output behavior on a fixed corpus and
   fixed model parameters. Five in-process and five cross-process
   runs produced the same digest.
4. [x] Measure CPU latency and memory overhead versus current
   MDS029 heuristic, pure-Go spike, and yzma spike baselines.
5. [x] Measure binary-size impact with embedded `.wasm` artifact.
6. [x] Define artifact update workflow and integrity checks
   (SHA256 pin on embedded wasm bytes, wazero validates on load).
7. [x] Document fallback boundaries when WASM init or inference
   fails.

Findings and recommendation live in the [spike README][spike].

[spike]: ../docs/research/conciseness/spikes/wasm-embedded-inference/README.md

## Acceptance Criteria

- [x] Prototype runs with no `YZMA_LIB` and no external dynamic libs.
- [x] Embedded WASM artifact loads via `go:embed`.
- [x] Deterministic behavior is confirmed across repeat runs.
- [x] CPU latency and memory metrics are captured.
- [x] Binary-size impact is measured and documented.
- [x] Recommendation is made: reject this path at the current
      classifier size. Keep the pure-Go classifier (plan 64) as the
      CPU fallback for MDS029.
