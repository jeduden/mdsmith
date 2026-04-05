#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SPIKE_DIR="${SPIKE_DIR:-$ROOT_DIR/spikes/localai-weasel-detection}"
CORPUS_DIR="${CORPUS_DIR:-$SPIKE_DIR/corpus}"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/.tmp/spikes/localai-weasel-detection}"
LOCALAI_URL="${LOCALAI_URL:-http://127.0.0.1:18080/v1/chat/completions}"
MODEL="${MODEL:-llama-3.2-1b-instruct:q4_k_m}"
RUNS="${RUNS:-5}"
SEED="${SEED:-42}"
LOCALAI_CONTAINER="${LOCALAI_CONTAINER:-}"

mkdir -p "$OUT_DIR/raw"
RESULTS_CSV="$OUT_DIR/results.csv"
SUMMARY_TXT="$OUT_DIR/summary.txt"

printf 'file,run,latency_s,mem_usage,response_sha256,label,confidence\n' \
  >"$RESULTS_CSV"

for file in "$CORPUS_DIR"/*.md; do
  base="$(basename "$file" .md)"
  text="$(tail -n +3 "$file" | tr '\n' ' ' | sed 's/[[:space:]]\+/ /g')"

  for run in $(seq 1 "$RUNS"); do
    payload="$(
      jq -n \
        --arg model "$MODEL" \
        --arg text "$text" \
        --argjson seed "$SEED" \
        '{
          model: $model,
          temperature: 0,
          top_p: 1,
          seed: $seed,
          max_tokens: 120,
          presence_penalty: 0,
          frequency_penalty: 0,
          messages: [
            {
              role: "system",
              content: "Classify whether the paragraph uses weasel language. Return compact JSON with keys label, confidence, rationale. label must be weasel or direct."
            },
            {role: "user", content: $text}
          ]
        }'
    )"

    response_file="$OUT_DIR/raw/${base}-run${run}.json"
    latency="$(
      curl -s -o "$response_file" -w '%{time_total}' \
        -H 'Content-Type: application/json' \
        -d "$payload" \
        "$LOCALAI_URL"
    )"

    content="$(jq -r '.choices[0].message.content // ""' "$response_file")"
    hash="$(
      printf '%s' "$content" | LC_ALL=C shasum -a 256 | awk '{print $1}'
    )"
    label="$(
      printf '%s' "$content" | jq -r '.label // "parse_error"' \
        2>/dev/null || echo "parse_error"
    )"
    confidence="$(
      printf '%s' "$content" | jq -r '.confidence // "parse_error"' \
        2>/dev/null || echo "parse_error"
    )"

    mem_usage="n/a"
    if [[ -n "$LOCALAI_CONTAINER" ]]; then
      mem_usage="$(
        docker stats --no-stream --format '{{.MemUsage}}' \
          "$LOCALAI_CONTAINER" 2>/dev/null \
          | awk -F'/' '{gsub(/^[ \t]+|[ \t]+$/, "", $1); print $1}'
      )"
      if [[ -z "$mem_usage" ]]; then
        mem_usage="n/a"
      fi
    fi

    printf '%s,%s,%s,%s,%s,%s,%s\n' \
      "$base" "$run" "$latency" "$mem_usage" "$hash" "$label" "$confidence" \
      >>"$RESULTS_CSV"
  done
done

{
  printf 'model=%s\n' "$MODEL"
  printf 'seed=%s\n' "$SEED"
  printf 'runs_per_file=%s\n\n' "$RUNS"
  awk -F, '
    NR > 1 {
      count[$1]++
      sum[$1] += $3
      if ($3 > max[$1]) {
        max[$1] = $3
      }
      hash_seen[$1 SUBSEP $5] = 1
    }
    END {
      for (f in count) {
        unique = 0
        for (k in hash_seen) {
          split(k, parts, SUBSEP)
          if (parts[1] == f) {
            unique++
          }
        }
        printf "%s unique_hashes=%d avg_latency_s=%.4f max_latency_s=%.4f\n",
          f, unique, sum[f] / count[f], max[f]
      }
    }
  ' "$RESULTS_CSV" | sort
} >"$SUMMARY_TXT"

printf 'Wrote %s\n' "$RESULTS_CSV"
printf 'Wrote %s\n' "$SUMMARY_TXT"
