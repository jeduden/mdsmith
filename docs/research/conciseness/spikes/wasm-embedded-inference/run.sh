#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../../../../.." && pwd)"
SPIKE_DIR="$ROOT_DIR/docs/research/conciseness/spikes/wasm-embedded-inference"
WASM_GUEST_DIR="$SPIKE_DIR/wasm"
EMBED_PKG_DIR="$ROOT_DIR/internal/rules/concisenessscoring/wasmclassifier"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/.tmp/spikes/wasm-embedded-inference}"
ROUNDS="${ROUNDS:-4000}"
DETERMINISM_RUNS="${DETERMINISM_RUNS:-5}"
GOCACHE="${GOCACHE:-$OUT_DIR/.gocache}"

mkdir -p "$OUT_DIR" "$GOCACHE"
export GOCACHE
cd "$ROOT_DIR"

echo "[1/5] compile wasm guest (GOOS=wasip1 GOARCH=wasm reactor)"
GOOS=wasip1 GOARCH=wasm go build \
  -buildmode=c-shared \
  -o "$SPIKE_DIR/classifier.wasm" \
  ./docs/research/conciseness/spikes/wasm-embedded-inference/wasm
cp "$SPIKE_DIR/classifier.wasm" "$EMBED_PKG_DIR/classifier.wasm"

echo "[2/5] build host harness binary"
go build -o "$OUT_DIR/wasm-spike" \
  "$SPIKE_DIR/main.go"

echo "[3/5] determinism check across process restarts"
HASHES_FILE="$OUT_DIR/determinism-hashes.txt"
: >"$HASHES_FILE"
for _ in $(seq 1 "$DETERMINISM_RUNS"); do
  "$OUT_DIR/wasm-spike" -mode digest >>"$HASHES_FILE"
done
determinism_hash=$(head -n 1 "$HASHES_FILE")
determinism_unique_hashes=$(
  sort -u "$HASHES_FILE" | wc -l | awk '{print $1}'
)
echo "determinism_hash=$determinism_hash"
echo "determinism_unique_hashes=$determinism_unique_hashes"

echo "[4/5] run latency and memory benchmark"
"$OUT_DIR/wasm-spike" \
  -mode bench \
  -rounds "$ROUNDS" \
  -determinism-runs "$DETERMINISM_RUNS" \
  | tee "$OUT_DIR/bench.txt"

echo "[5/5] measure binary-size delta"
go build -o "$OUT_DIR/mdsmith-base" ./cmd/mdsmith
go build -tags spike_wasm_classifier \
  -o "$OUT_DIR/mdsmith-wasm" ./cmd/mdsmith
base_bytes=$(wc -c <"$OUT_DIR/mdsmith-base" | awk '{print $1}')
wasm_bytes=$(wc -c <"$OUT_DIR/mdsmith-wasm" | awk '{print $1}')
delta_bytes=$((wasm_bytes - base_bytes))
wasm_artifact_bytes=$(wc -c <"$SPIKE_DIR/classifier.wasm" | awk '{print $1}')

{
  echo "mdsmith_base_bytes=$base_bytes"
  echo "mdsmith_wasm_bytes=$wasm_bytes"
  echo "mdsmith_delta_bytes=$delta_bytes"
  echo "wasm_artifact_bytes=$wasm_artifact_bytes"
} | tee "$OUT_DIR/size.txt"

cat >"$OUT_DIR/summary.txt" <<EOF
determinism_hash=$determinism_hash
determinism_unique_hashes=$determinism_unique_hashes
mdsmith_base_bytes=$base_bytes
mdsmith_wasm_bytes=$wasm_bytes
mdsmith_delta_bytes=$delta_bytes
wasm_artifact_bytes=$wasm_artifact_bytes
EOF

echo "results_dir=$OUT_DIR"
