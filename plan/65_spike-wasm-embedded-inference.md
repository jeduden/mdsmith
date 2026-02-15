---
id: 65
title: Spike WASM-Embedded Weasel Inference
status: 🔲
---
# Spike WASM-Embedded Weasel Inference

## Goal

Evaluate a WASM-based inference path that can be embedded in mdsmith
with no runtime dynamic library dependency.

## Tasks

1. Choose one Go-hosted WASM runtime candidate
   (for example `wazero`) and define module loading strategy.
2. Build a minimal proof of concept that embeds a `.wasm` artifact with
   `go:embed` and runs inference in-process.
3. Verify deterministic output behavior on a fixed prompt and fixed
   model parameters.
4. Measure CPU latency and memory overhead versus:
   current MDS029 heuristic, pure-Go spike, and yzma spike baselines.
5. Measure binary-size impact with embedded `.wasm` artifact.
6. Define artifact update workflow and integrity checks
   (checksum/version pinning).
7. Document fallback boundaries when WASM init or inference fails.

## Acceptance Criteria

- [ ] Prototype runs with no `YZMA_LIB` and no external dynamic libs.
- [ ] Embedded WASM artifact loads via `go:embed`.
- [ ] Deterministic behavior is confirmed across repeat runs.
- [ ] CPU latency and memory metrics are captured.
- [ ] Binary-size impact is measured and documented.
- [ ] Recommendation is made: adopt, defer, or reject this path.
