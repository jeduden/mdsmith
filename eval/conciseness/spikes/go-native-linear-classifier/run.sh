#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../../../.." && pwd)"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/.tmp/spikes/go-native-linear-classifier}"
ROUNDS="${ROUNDS:-4000}"
DETERMINISM_RUNS="${DETERMINISM_RUNS:-5}"
GOCACHE="${GOCACHE:-$OUT_DIR/.gocache}"

mkdir -p "$OUT_DIR"
mkdir -p "$GOCACHE"
export GOCACHE
cd "$ROOT_DIR"

echo "[1/4] build go-native spike harness binary"
go build -o "$OUT_DIR/go-native-spike" \
  ./eval/conciseness/spikes/go-native-linear-classifier

echo "[2/4] determinism check across process restarts"
HASHES_FILE="$OUT_DIR/determinism-hashes.txt"
: >"$HASHES_FILE"
for _ in $(seq 1 "$DETERMINISM_RUNS"); do
  "$OUT_DIR/go-native-spike" -mode digest >>"$HASHES_FILE"
done
determinism_hash=$(head -n 1 "$HASHES_FILE")
determinism_unique_hashes=$(
  sort -u "$HASHES_FILE" | wc -l | awk '{print $1}'
)
echo "determinism_hash=$determinism_hash"
echo "determinism_unique_hashes=$determinism_unique_hashes"

echo "[3/4] run latency and memory benchmark"
"$OUT_DIR/go-native-spike" \
  -mode bench \
  -rounds "$ROUNDS" \
  -determinism-runs "$DETERMINISM_RUNS" \
  | tee "$OUT_DIR/bench.txt"

echo "[4/4] measure binary-size delta"
go build -o "$OUT_DIR/mdsmith-base" ./cmd/mdsmith
go build -tags spike_gonative_classifier \
  -o "$OUT_DIR/mdsmith-go-native" ./cmd/mdsmith
base_bytes=$(wc -c <"$OUT_DIR/mdsmith-base" | awk '{print $1}')
go_native_bytes=$(wc -c <"$OUT_DIR/mdsmith-go-native" | awk '{print $1}')
delta_bytes=$((go_native_bytes - base_bytes))

echo "mdsmith_base_bytes=$base_bytes" | tee "$OUT_DIR/size.txt"
echo "mdsmith_go_native_bytes=$go_native_bytes" | tee -a "$OUT_DIR/size.txt"
echo "mdsmith_delta_bytes=$delta_bytes" | tee -a "$OUT_DIR/size.txt"

cat >"$OUT_DIR/summary.txt" <<EOF
determinism_hash=$determinism_hash
determinism_unique_hashes=$determinism_unique_hashes
mdsmith_base_bytes=$base_bytes
mdsmith_go_native_bytes=$go_native_bytes
mdsmith_delta_bytes=$delta_bytes
EOF

echo "results_dir=$OUT_DIR"
