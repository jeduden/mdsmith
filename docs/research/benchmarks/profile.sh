#!/usr/bin/env bash
# Find *where* `mdsmith check` spends time — the companion to the
# gate. The gate (BenchmarkCheckCorpus{Small,Large} + check-bench)
# tells you it regressed; this tells you which function.
#
# Two profiles, because they cover different code:
#   1. In-process bench profile — exact gated path, function- and
#      line-level, deterministic synthetic corpus.
#   2. Real-CLI profile via the env hook — adds process startup,
#      workspace walk, and config resolution that the in-process
#      bench under-exercises.
#
# Usage: docs/research/benchmarks/profile.sh [workdir] [target]
#   target defaults to "." (lint this repo with the real CLI)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WORK="${1:-/tmp/mdsmith-prof}"
TARGET="${2:-$REPO_ROOT}"
mkdir -p "$WORK"
cd "$REPO_ROOT"

HOT='Check|Lint|Walk|Run|Rule|parse|Parse'

echo "== 1. In-process gate profile (internal/engine) =="
# -o keeps the compiled test binary in $WORK, not the repo root
# (go test drops <pkg>.test in cwd when profiling flags are set).
go test -run='^$' -bench=BenchmarkCheckCorpusLarge -benchtime=10x \
  -o "$WORK/engine.test" \
  -cpuprofile "$WORK/cpu.bench.out" \
  -memprofile "$WORK/mem.bench.out" \
  ./internal/engine/ >/dev/null
echo "-- CPU, hottest functions --"
go tool pprof -nodecount=20 -top "$WORK/cpu.bench.out" 2>/dev/null \
  | sed -n '1,28p'
echo "-- CPU, per-line for $HOT --"
go tool pprof -list="$HOT" "$WORK/cpu.bench.out" 2>/dev/null \
  | sed -n '1,60p'
echo "-- Allocated bytes, hottest --"
go tool pprof -nodecount=15 -alloc_space -top "$WORK/mem.bench.out" \
  2>/dev/null | sed -n '1,22p'

echo
echo "== 2. Real-CLI profile (env hook, end to end) =="
go build -o "$WORK/mdsmith" ./cmd/mdsmith
MDSMITH_CPUPROFILE="$WORK/cpu.cli.out" \
  "$WORK/mdsmith" check "$TARGET" >/dev/null 2>&1 || true
echo "-- CPU, hottest functions (includes walk + config) --"
go tool pprof -nodecount=20 -top "$WORK/cpu.cli.out" 2>/dev/null \
  | sed -n '1,28p'

echo
echo "Profiles in $WORK. Drill in with:"
echo "  go tool pprof $WORK/cpu.cli.out      # then 'top', 'list <fn>', 'web'"
echo "  go tool pprof -http=: $WORK/cpu.bench.out"
