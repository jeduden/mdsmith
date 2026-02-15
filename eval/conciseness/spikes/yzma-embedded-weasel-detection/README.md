# yzma Embedded Weasel Detection Spike

## Goal

Evaluate yzma as an embedded inference path for deterministic,
low-overhead weasel-language detection.

## Scope

This spike used a small GGUF model and compared two paths:

- Embedded: in-process yzma calls from Go.
- Service: local `llama-server` over HTTP.

Both paths used the same model, prompt shape, and benchmark corpus.

## Environment

- Date: 2026-02-15
- OS: macOS 15.3.2 (arm64)
- Go: 1.24.7 (toolchain auto-switched to 1.25.7 for yzma commands)
- yzma: v1.9.0
- llama.cpp libs: CPU package installed by `yzma install`
- Model: `SmolLM-135M.Q2_K.gguf` (84 MB)

## Prototype

The prototype used:

- Direct `github.com/hybridgroup/yzma/pkg/llama` calls for embedded mode.
- `llama-server` for service mode.
- Fixed decoding controls: greedy sampling and `temperature: 0`.
- Corpus: 6 markdown-style prompts, 3 repetitions, `n_predict: 16`.

Reproduction script:
`eval/conciseness/spikes/yzma-embedded-weasel-detection/run.sh`.

## Determinism

### Embedded mode

- Same-process check: 5 repeated runs returned identical output.
- Restart check: 3 independent process runs returned identical output.

### Service mode

- Same-process check: 5 repeated requests returned identical output.
- Restart check: 3 independent server runs returned identical output.

Result: deterministic behavior was confirmed for this setup.

## Performance and Memory

### Embedded mode metrics

Representative warm-run metrics from the spike harness:

| Metric               | Value           |
|----------------------|-----------------|
| Startup model load   | 199.61 ms       |
| Requests             | 18              |
| Avg latency          | 51.67 ms        |
| P95 latency          | 53.00 ms        |
| Tokens generated     | 288             |
| Throughput           | 309.65 tokens/s |
| RSS after model load | 87,344 KB       |
| RSS after benchmark  | 84,480 KB       |

Notes:

- One cold-start outlier was observed at 8.86 s model load.
- Warm starts were stable near 200 ms on this machine.

### Service mode metrics

Two runs were captured to reduce outlier risk.

| Metric               | Run A           | Run B           |
|----------------------|-----------------|-----------------|
| Startup to healthy   | 981.92 ms       | 703.89 ms       |
| Requests             | 18              | 18              |
| Avg latency          | 58.63 ms        | 70.38 ms        |
| P95 latency          | 73.12 ms        | 73.13 ms        |
| Throughput           | 272.91 tokens/s | 227.33 tokens/s |
| RSS after model load | 134,304 KB      | 239,216 KB      |
| RSS after benchmark  | 147,504 KB      | 173,504 KB      |

### Comparison summary

- Embedded mode was faster on average and at P95 in this spike.
- Embedded mode used less RSS in observed runs.
- Service mode had higher startup and more runtime variance.

## Packaging Constraints

Measured artifact sizes:

- llama.cpp library bundle: 86 MB (`/tmp/yzma/lib`)
- model file: 84 MB (`SmolLM-135M.Q2_K.gguf`)
- binary size impact for adding yzma import: 2.2 MB -> 2.7 MB (+0.5 MB)

Operational constraints:

- Runtime needs local llama.cpp dynamic libraries.
- `YZMA_LIB` must point to those libraries.
- Model files must be bundled or provisioned separately.
- CPU fallback is available and works on this environment.

## Integration Boundaries for mdsmith

Recommended integration shape:

1. Compile-time guard with `yzma` build tag for embedded inference adapter.
2. Runtime feature flag (for example, `conciseness-scoring.classifier.mode`).
3. Required settings when enabled:
  `yzma-lib-path`, `model-path`, timeout, and max tokens.
4. Graceful fallback to current heuristic mode when:
  yzma is not built, libs are missing, model is missing, or inference fails.
5. Emit verbose diagnostics only in `--verbose` mode.

## Maintenance and Licensing Risk

Maintenance risks:

- yzma tracks `llama.cpp` closely; frequent updates can break compatibility.
- Model quality is tied to selected GGUF model and prompt template.
- Artifact management adds operational complexity to releases.

Licensing notes:

- yzma is Apache-2.0 and includes portions of gollama.cpp (MIT).
- The tested model card reports Apache-2.0.
- Final model choice still needs legal review for distribution policy.

## Recommendation

Proceed with embedded yzma as an optional, non-default path.

Rationale:

- Determinism was reproducible across repeated runs and restarts.
- Embedded mode met latency and memory goals relative to service mode.
- Packaging cost is manageable for optional distribution.

Gate for implementation:

- Keep heuristic path as default until plans 58 and 59 select and validate
  the final classifier model.
