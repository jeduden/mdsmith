# WASM Embedded Inference Spike

## Goal

Test whether mdsmith can run weasel detection via an embedded `.wasm`
module without runtime dynamic library dependencies.

## Why This Spike Exists

This is a separate option from:

- `spikes/yzma-embedded-weasel-detection` (native dynamic libs)
- `spikes/go-native-linear-classifier` (all-Go model code)

WASM may offer a middle path:

- portable artifact format
- deterministic runtime with pinned module bytes
- stronger isolation around inference execution

## Candidate Runtime

Initial candidate: `wazero` (pure-Go WebAssembly runtime).

## Evaluation Focus

1. cold start and warm latency
2. memory overhead under repeated calls
3. binary-size cost of embedded `.wasm`
4. determinism and fallback behavior

## Next Step

Execute `plan/65_spike-wasm-embedded-inference.md`.
