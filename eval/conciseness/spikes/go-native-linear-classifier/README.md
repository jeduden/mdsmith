# Go-Native Linear Classifier Spike

## Goal

Evaluate a fully embedded pure-Go classifier path for
`verbose-actionable` detection with no runtime dynamic libraries.

## Environment

- Date: 2026-02-16
- OS: macOS 15.3.2 (arm64)
- Go: 1.24.7
- mdsmith commit: `7440e61`
- Harness: `eval/conciseness/spikes/go-native-linear-classifier/run.sh`

## Prototype

Implementation files:

- classifier package:
  `internal/rules/concisenessscoring/classifier/`
- embedded weights:
  `internal/rules/concisenessscoring/classifier/data/cue-linear-v1.json`
- benchmark command:
  `eval/conciseness/spikes/go-native-linear-classifier/main.go`

Model shape:

- sparse linear logistic model over cue rates and density metrics
- output contract:
  `label`, `risk_score`, `threshold`, `model_id`, `backend`, `version`
- runtime dependency footprint: Go stdlib only

## Embedded Weight Packaging

Artifact details:

- path: `data/cue-linear-v1.json`
- embedded bytes: 335
- SHA256:
  `a17544b94507ad05e5d9db33078ca7a63d3fccd94b1d091ee1c85a88bbc81e44`

Load path:

1. embed artifact bytes via `go:embed`
2. verify SHA256 against pinned constant
3. decode JSON into model weights and threshold
4. fail closed on checksum/decode/validation errors

## Corpus and Determinism

The benchmark corpus matches the six-sample set used in the yzma spike
(`weasel-01..03` and `direct-01..03`).

Determinism checks:

- in-process repeats (`determinism-runs=5`): `unique_hashes=1`
- process-restart repeats (`run.sh`): `unique_hashes=1`
- digest:
  `41f7fe0f2d6d755b647f4f923f79c5d682cfcda75add60a7e2df3fcba29fce08`

Result: deterministic outputs were confirmed.

## Latency and Memory

Measured with `ROUNDS=4000` (`24,000` requests total):

| Metric                 | Value      |
|------------------------|------------|
| Model load             | 0.0300 ms  |
| Avg latency            | 2.8613 us  |
| P50 latency            | 2.8330 us  |
| P95 latency            | 3.6670 us  |
| Max latency            | 95.0420 us |
| RSS after load         | 3,872 KB   |
| RSS after benchmark    | 7,776 KB   |
| Heap alloc after bench | 417 KB     |
| Heap sys after bench   | 7,776 KB   |

Per-sample risk scores:

- `weasel-01`: 0.6323
- `weasel-02`: 0.3833
- `weasel-03`: 0.2101
- `direct-01`: 0.0472
- `direct-02`: 0.0619
- `direct-03`: 0.0627

The embedded threshold for this spike artifact is `0.20`.

## Binary-Size Impact

Measured by building `cmd/mdsmith` with and without the
`spike_gonative_classifier` tag:

- base mdsmith: 24,511,378 bytes
- mdsmith with embedded classifier artifact: 24,512,514 bytes
- delta: 1,136 bytes

Comparison with yzma spike footprint:

- go-native spike: +1.1 KB binary delta, no external model/libs
- yzma spike: +0.5 MB binary delta plus 84 MB model and 86 MB libs

## MDS029 Integration Boundaries

Recommended runtime modes:

- `classifier`: force embedded classifier path
- `heuristic`: force current MDS029 heuristic path
- `auto`: try classifier first, degrade to heuristic on failure

Recommended timeout policy:

- classify with bounded context timeout (for example 2 ms default)
- on timeout, treat as classifier failure and fall back to heuristic
- count fallback events for `--verbose` reporting only

Diagnostic stability requirements:

- preserve one diagnostic schema across backends
- keep thresholded binary decision semantics stable
- include `backend` and `model_id` in verbose/debug output only

## Maintenance Workflow

Safe model update path:

1. train/export next artifact in deterministic JSON format
2. replace `cue-linear-v1.json` with versioned artifact content
3. recompute SHA256 and update pinned checksum constant
4. run `go test ./internal/rules/concisenessscoring/classifier`
5. run `eval/conciseness/spikes/go-native-linear-classifier/run.sh`
6. record version, checksum, and size/latency deltas in release notes

Versioning guidance:

- keep explicit artifact version in JSON (`version`)
- keep model identifier stable (`model_id`) until feature contract changes
- require checksum updates in the same commit as weight updates

## Recommendation

Adopt this path as the default CPU fallback candidate for plan 58.

Rationale:

- deterministic behavior is straightforward to guarantee
- binary and artifact footprint are effectively negligible
- runtime has no dynamic-library or external-service dependency

Remaining gap:

- quality must still be validated on the larger plan-59 dataset before
  promoting classifier mode to default for MDS029.
