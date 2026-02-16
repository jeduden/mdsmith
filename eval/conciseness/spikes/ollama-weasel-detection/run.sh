#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
SPIKE_DIR="${SPIKE_DIR:-$ROOT_DIR/eval/conciseness/spikes/ollama-weasel-detection}"
CORPUS_DIR="${CORPUS_DIR:-$SPIKE_DIR/corpus}"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/.tmp/eval/conciseness/spikes/ollama-weasel-detection}"
API_URL="${API_URL:-http://127.0.0.1:11434/api/generate}"
CONTAINER="${CONTAINER:-ollama-plan56}"
MODELS="${MODELS:-qwen2.5:0.5b,llama3.2:1b,smollm2:360m}"
RUNS="${RUNS:-6}"
SEED="${SEED:-42}"
MAX_TOKENS="${MAX_TOKENS:-80}"
TIMEOUT_SECS="${TIMEOUT_SECS:-45}"

RESULTS_CSV="$OUT_DIR/results.csv"
RESTART_CSV="$OUT_DIR/restart.csv"
SUMMARY_TXT="$OUT_DIR/summary.txt"
RAW_DIR="$OUT_DIR/raw"

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "missing required command: $cmd" >&2
    exit 1
  fi
}

trim_and_collapse() {
  tr '\n' ' ' | sed 's/[[:space:]]\+/ /g' | sed 's/^ //; s/ $//'
}

prepare_prompt() {
  local text="$1"
  cat <<EOF
Classify whether the text is weasel language.
Return strict JSON with keys: label, confidence, rationale.
label must be either "weasel" or "direct".
confidence must be between 0 and 1.
rationale must be short (max 12 words).
Text: $text
EOF
}

call_ollama() {
  local payload="$1"
  local out_file="$2"
  curl -sS --max-time "$TIMEOUT_SECS" \
    -w '%{time_total}' \
    -H 'Content-Type: application/json' \
    -d "$payload" \
    "$API_URL" \
    -o "$out_file"
}

run_single() {
  local model="$1"
  local file="$2"
  local run="$3"
  local phase="$4"

  local base expected text prompt payload response_file
  local latency_s response_text response_hash raw_label norm_label confidence
  local eval_count eval_duration_s tokens_per_s mem_usage correct

  base="$(basename "$file" .md)"
  expected="${base%%-*}"
  text="$(tail -n +3 "$file" | trim_and_collapse)"
  prompt="$(prepare_prompt "$text")"

  payload="$(
    jq -n \
      --arg model "$model" \
      --arg prompt "$prompt" \
      --argjson seed "$SEED" \
      --argjson max_tokens "$MAX_TOKENS" \
      '{
        model: $model,
        prompt: $prompt,
        stream: false,
        format: "json",
        options: {
          temperature: 0,
          top_p: 1,
          seed: $seed,
          num_predict: $max_tokens,
          repeat_penalty: 1
        }
      }'
  )"

  response_file="$RAW_DIR/${phase}-$(echo "$model" | tr ':/' '__')-${base}-run${run}.json"
  latency_s="$(call_ollama "$payload" "$response_file")"
  response_text="$(
    jq -r '.response | if type == "string" then . else tojson end' "$response_file"
  )"
  response_hash="$(
    printf '%s' "$response_text" | LC_ALL=C shasum -a 256 | awk '{print $1}'
  )"
  raw_label="$(
    jq -r '
      .response
      | if type == "string" then (fromjson? // {}) else . end
      | .label // "parse_error"
    ' "$response_file"
  )"
  norm_label="$(printf '%s' "$raw_label" | tr '[:upper:]' '[:lower:]')"
  confidence="$(
    jq -r '
      .response
      | if type == "string" then (fromjson? // {}) else . end
      | .confidence // "parse_error"
    ' "$response_file"
  )"
  eval_count="$(jq -r '.eval_count // 0' "$response_file")"
  eval_duration_s="$(
    jq -r '((.eval_duration // 0) / 1000000000)' "$response_file"
  )"
  tokens_per_s="$(
    jq -r '
      if (.eval_duration // 0) > 0 and (.eval_count // 0) > 0
      then ((.eval_count) / ((.eval_duration) / 1000000000))
      else 0
      end
    ' "$response_file"
  )"

  mem_usage="$(
    docker stats --no-stream --format '{{.MemUsage}}' "$CONTAINER" 2>/dev/null \
      | awk -F/ '{gsub(/^[ \t]+|[ \t]+$/, "", $1); print $1}'
  )"
  if [[ -z "$mem_usage" ]]; then
    mem_usage="n/a"
  fi

  if [[ "$norm_label" == "$expected" ]]; then
    correct=1
  else
    correct=0
  fi

  printf '%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n' \
    "$model" "$base" "$expected" "$phase" "$run" "$latency_s" \
    "$eval_count" "$eval_duration_s" "$tokens_per_s" "$mem_usage" \
    "$response_hash" "$norm_label" "$confidence" "$correct" >>"$RESULTS_CSV"

  if [[ "$phase" == "restart-a" || "$phase" == "restart-b" ]]; then
    printf '%s,%s,%s,%s\n' \
      "$model" "$base" "$phase" "$response_hash" >>"$RESTART_CSV"
  fi
}

wait_for_ollama() {
  local i
  for i in $(seq 1 30); do
    if curl -sS --max-time 2 "http://127.0.0.1:11434/api/tags" \
      >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "ollama endpoint did not become ready in time" >&2
  return 1
}

require_cmd jq
require_cmd curl
require_cmd docker
require_cmd shasum

rm -rf "$RAW_DIR"
mkdir -p "$RAW_DIR"
printf 'model,file,expected,phase,run,latency_s,eval_count,eval_duration_s,tokens_per_s,mem_usage,response_sha256,label,confidence,correct\n' \
  >"$RESULTS_CSV"
printf 'model,file,phase,response_sha256\n' >"$RESTART_CSV"

IFS=',' read -r -a model_list <<<"$MODELS"

for model in "${model_list[@]}"; do
  for file in "$CORPUS_DIR"/*.md; do
    for run in $(seq 1 "$RUNS"); do
      run_single "$model" "$file" "$run" "steady"
    done
  done
done

for model in "${model_list[@]}"; do
  for file in "$CORPUS_DIR"/*.md; do
    run_single "$model" "$file" "1" "restart-a"
  done
done

docker restart "$CONTAINER" >/dev/null
wait_for_ollama

for model in "${model_list[@]}"; do
  for file in "$CORPUS_DIR"/*.md; do
    run_single "$model" "$file" "1" "restart-b"
  done
done

{
  printf 'models=%s\n' "$MODELS"
  printf 'seed=%s\n' "$SEED"
  printf 'runs_per_file=%s\n\n' "$RUNS"

  printf 'aggregate_metrics\n'
  awk -F, '
    NR > 1 && $4 == "steady" {
      key = $1
      n[key]++
      latency_sum[key] += $6
      tok_sum[key] += $9
      if ($6 > latency_max[key]) {
        latency_max[key] = $6
      }
      if ($12 != "parse_error") {
        parse_ok[key]++
      }
      correct[key] += $14
    }
    END {
      for (k in n) {
        printf "%s avg_latency_s=%.4f max_latency_s=%.4f avg_tokens_per_s=%.2f parse_rate=%.3f accuracy=%.3f\n",
          k, latency_sum[k] / n[k], latency_max[k], tok_sum[k] / n[k],
          parse_ok[k] / n[k], correct[k] / n[k]
      }
    }
  ' "$RESULTS_CSV" | sort

  printf '\nunique_hashes_per_model_file\n'
  awk -F, '
    NR > 1 && $4 == "steady" {
      pair = $1 SUBSEP $2
      seen[pair] = 1
      h = $1 SUBSEP $2 SUBSEP $11
      hashes[h] = 1
    }
    END {
      for (pair in seen) {
        split(pair, p, SUBSEP)
        count = 0
        for (h in hashes) {
          split(h, hs, SUBSEP)
          if (hs[1] == p[1] && hs[2] == p[2]) {
            count++
          }
        }
        printf "%s,%s unique_hashes=%d\n", p[1], p[2], count
      }
    }
  ' "$RESULTS_CSV" | sort

  printf '\nrestart_determinism\n'
  awk -F, '
    NR > 1 {
      key = $1 SUBSEP $2
      if ($3 == "restart-a") {
        a[key] = $4
      } else if ($3 == "restart-b") {
        b[key] = $4
      }
    }
    END {
      for (k in a) {
        split(k, p, SUBSEP)
        status = "mismatch"
        if (a[k] == b[k]) {
          status = "match"
        }
        printf "%s,%s %s\n", p[1], p[2], status
      }
    }
  ' "$RESTART_CSV" | sort
} >"$SUMMARY_TXT"

printf 'Wrote %s\n' "$RESULTS_CSV"
printf 'Wrote %s\n' "$RESTART_CSV"
printf 'Wrote %s\n' "$SUMMARY_TXT"
