# WASM Embedded Inference Spike

## Goal

Evaluate a WASM-based inference path that can be embedded in mdsmith
with no runtime dynamic library dependency. Compare it against the
current MDS029 heuristic, the pure-Go classifier spike (plan 64),
and the yzma embedded spike.

## Environment

- Date: 2026-04-22
- OS: Linux 4.4.0 (amd64)
- Go: 1.25.8
- wazero: v1.11.0 (pure-Go runtime, wazevo optimizing compiler)
- Harness:
  `docs/research/conciseness/spikes/wasm-embedded-inference/run.sh`

## Prototype

Implementation files:

- wasm guest source:
  `docs/research/conciseness/spikes/wasm-embedded-inference/wasm/main.go`
- host harness:
  `docs/research/conciseness/spikes/wasm-embedded-inference/main.go`
- size-measurement hook:
  `internal/rules/concisenessscoring/wasmclassifier/embed.go`
- build-tag stub:
  `cmd/mdsmith/spike_wasm_classifier.go`

The guest reuses the plan-64 classifier package verbatim
(`internal/rules/concisenessscoring/classifier`) so the wasm-vs-native
comparison isolates runtime cost. The guest is compiled with
`GOOS=wasip1 GOARCH=wasm -buildmode=c-shared`, which produces a WASI
reactor module exposing:

- `alloc(size int32) int32` — reserve guest memory for host input
- `free(ptr int32)` — release a prior alloc
- `classify(ptr, len int32) int64` — returns `(outPtr<<32)|outLen`
  pointing at a JSON result in a static guest buffer

The host calls `_initialize` once, then calls `alloc`, writes the
input text into guest linear memory, calls `classify`, and reads the
JSON result. `wazero.NewRuntime` uses its default optimizing compiler
(wazevo) on linux/amd64.

## Module Loading Strategy

- wazero runtime is created with default config
  (`wazero.NewRuntimeConfig().WithCloseOnContextDone(true)`).
- WASI preview 1 is registered via
  `wasi_snapshot_preview1.Instantiate`.
- Module is instantiated with `WithStartFunctions()` (empty) so wazero
  does not auto-run `_start`; the host then calls `_initialize`
  explicitly, matching reactor-module semantics.
- One module instance is reused for all calls. Re-instantiating per
  call would pay the `~1.7 s` compile cost again.

## Determinism

- In-process repeats (`determinism-runs=5`): `unique_hashes=1`.
- Process-restart repeats (5 independent runs): `unique_hashes=1`.
- Digest:
  `7a1dc229f814d75c3969317ceddbade75c4d5ab4e0f133da98c1dbb917381620`.

Result: deterministic outputs were confirmed across repeat runs and
restarts.

## Latency and Memory

Measured with `ROUNDS=4000` (`24,000` requests total):

| Metric                 | Value       |
|------------------------|-------------|
| Guest load (compile)   | 1699.40 ms  |
| Requests               | 24,000      |
| Avg latency            | 2022.38 us  |
| P50 latency            | 1952.17 us  |
| P95 latency            | 2464.15 us  |
| Max latency            | 12915.37 us |
| RSS after load         | 136,616 KB  |
| RSS after bench        | 109,152 KB  |
| Heap alloc after bench | 21,483 KB   |
| Heap sys after bench   | 84,832 KB   |
| Total alloc delta      | 40,812 KB   |

Per-sample risk scores (same inputs, same model as plan 64):

- `weasel-01`: 0.9743
- `weasel-02`: 0.6186
- `weasel-03`: 0.8545
- `direct-01`: 0.1893
- `direct-02`: 0.1073
- `direct-03`: 0.3308

The embedded threshold for this spike artifact is `0.20`. Classifier
weights are `cue-linear-v2` (current head), which is why these scores
differ from the plan-64 snapshot; the same weights in the go-native
harness produce identical scores.

## Binary-Size Impact

Measured by building `cmd/mdsmith` with and without the
`spike_wasm_classifier` tag:

- base mdsmith: 27,817,743 bytes
- mdsmith with embedded wasm classifier: 31,992,461 bytes
- delta: 4,174,718 bytes (~4.17 MB)
- wasm artifact alone: 3,993,759 bytes (~3.99 MB)

The ~181 KB gap between the artifact bytes and the binary delta is
the linked wazero runtime plus `wasi_snapshot_preview1` import. The
wasm artifact is Go's own runtime + classifier package compiled for
`wasip1`, which accounts for nearly all of the 3.99 MB; the classifier
logic is a small fraction of that.

## Cross-Spike Comparison

All four paths score the same corpus with the same linear weights
(except MDS029, which has its own heuristic):

| Path                | Avg latency | Startup  | RSS bench  | Binary delta |
|---------------------|-------------|----------|------------|--------------|
| MDS029 heuristic    | n/a (no ML) | 0        | baseline   | 0 B          |
| go-native (plan 64) | 3.38 us     | 0.28 ms  | 7,776 KB   | 480 B        |
| wasm + wazero       | 2,022.38 us | 1,699 ms | 109,152 KB | 4,174,718 B  |
| yzma embedded       | 51,670 us   | 199.61ms | 84,480 KB  | 524,288 B +  |

yzma also requires an 86 MB dynamic library bundle at runtime plus an
84 MB GGUF model on disk. The wasm path ships everything in-process
with no external files and no dynamic library dependency.

## Artifact Update Workflow

Safe wasm-artifact update path:

1. Update classifier source
   (`internal/rules/concisenessscoring/classifier/*.go` or
   `data/cue-linear.json`).
2. Run
   `docs/research/conciseness/spikes/wasm-embedded-inference/run.sh`.
   The script recompiles the wasm guest and refreshes both the
   harness-local copy and
   `internal/rules/concisenessscoring/wasmclassifier/classifier.wasm`.
3. Record the new `wasm_artifact_sha256` and `wasm_artifact_bytes`
   lines from `bench.txt`.
4. Pin the new SHA256 into a `const` next to the `//go:embed`
   directive in a future production integration (mirroring
   `classifier.EmbeddedArtifactSHA256`).
5. Run the spike harness to confirm the `determinism_digest` is
   stable across five process restarts before shipping.

Integrity checks:

- SHA256 pin on the embedded wasm bytes prevents silent substitution.
- wazero itself validates the WebAssembly module on instantiate; a
  corrupt artifact fails loud at startup.
- Version pinning: the classifier artifact inside the wasm retains
  its own `model_id`/`version` (echoed as `cue-linear-v2` /
  `2026-04-05` in the harness output), so drift between host and
  guest is observable at runtime.

## Fallback Boundaries

Recommended fallback rules if the wasm path were adopted:

1. Compile-time guard with a `wasm_classifier` build tag so
   non-wasm-capable builds omit the 4 MB artifact and the wazero
   dependency entirely.
2. Runtime: instantiate the module once at MDS029 first use. On
   failure (checksum mismatch, wasm validation error, wazero compile
   error), fall back to the heuristic path and emit a verbose
   diagnostic counting the event.
3. Per-call: wrap `classify` with a bounded `context.WithTimeout`
   (suggested 50 ms, well above the 2,464 us p95 observed here). On
   timeout or error, treat as a classifier failure and fall back to
   heuristic for that paragraph.
4. Surface backend selection in `--verbose` output only; keep the
   default diagnostic schema identical to the go-native backend so
   rule output is indistinguishable end-to-end.

## Recommendation

Reject this path for mdsmith at the current classifier size.

Rationale:

- The pure-Go classifier (plan 64) already delivers the same
  deterministic output with a 480 B binary cost, 3 us avg latency,
  and 7.8 MB RSS. Wasm adds no capability the Go path lacks.
- The wasm path is ~600x slower per call, uses ~14x the RSS, and
  grows the binary by ~4.17 MB. Most of that cost is the wasip1
  runtime embedded in the guest artifact, not the classifier.
- The 1.7 s wazero compile on first use would regress cold-start
  time for short `mdsmith check` invocations.
- WASI reactor support still requires `-buildmode=c-shared`, a
  `_initialize` call, and module-side pinning tricks; this is
  tractable but adds moving parts with no offsetting benefit here.

When wasm would become interesting:

- If the classifier were authored in Rust or another language and we
  wanted polyglot reuse without FFI.
- If classifier weights/topology had to be hot-swappable at runtime
  in a sandboxed way (wazero provides memory isolation that plain-Go
  embeds do not).
- If the model grew large enough that the 4 MB wasip1 runtime cost
  became negligible relative to weight bytes.

Gate for future revisit: reopen this path only if a future classifier
from plan 58 (a) is not practically authorable in pure Go, or (b)
requires the sandbox isolation wazero provides. Otherwise keep the
pure-Go classifier as the CPU fallback for MDS029.
