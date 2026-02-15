#!/usr/bin/env bash
set -euo pipefail

ROOT_TMP=${ROOT_TMP:-/tmp/yzma-spike}
LIB_DIR="$ROOT_TMP/lib"
MODEL_DIR="$ROOT_TMP/models"
MODEL_FILE="SmolLM-135M.Q2_K.gguf"
MODEL_URL="https://huggingface.co/QuantFactory/SmolLM-135M-GGUF/resolve/main/SmolLM-135M.Q2_K.gguf"
PORT=${PORT:-8097}

mkdir -p "$LIB_DIR" "$MODEL_DIR"

echo "[1/5] install llama.cpp libraries"
go run github.com/hybridgroup/yzma/cmd/yzma@v1.9.0 install \
  -l "$LIB_DIR" -p cpu -q

echo "[2/5] download model"
go run github.com/hybridgroup/yzma/cmd/yzma@v1.9.0 model get \
  -u "$MODEL_URL" -o "$MODEL_DIR" -y

echo "[3/5] run embedded benchmark"
cat > "$ROOT_TMP/embedded_bench.go" <<'GOEOF'
package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

var corpus = []string{
	"This approach may potentially improve outcomes in many situations.",
	"Run go test ./... before submitting the pull request.",
	"It seems the API is kind of unreliable under heavy load.",
	"Set timeout to 2s and retry once on HTTP 503.",
	"You might want to consider adding additional validation checks.",
	"The parser accepts front matter and heading sections.",
}

func rssKB() int {
	pid := os.Getpid()
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return -1
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return -1
	}
	return v
}

func clearContext(ctx llama.Context) {
	llama.Synchronize(ctx)
	if mem, err := llama.GetMemory(ctx); err == nil {
		llama.MemoryClear(mem, true)
	}
}

func generate(ctx llama.Context, model llama.Model, prompt string, max int32) (string, int, error) {
	vocab := llama.ModelGetVocab(model)
	fullPrompt := "Classify as WEASEL or OK. Text: " + prompt + " Label:"
	tokens := llama.Tokenize(vocab, fullPrompt, true, false)
	batch := llama.BatchGetOne(tokens)

	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
	llama.SamplerChainAdd(sampler, llama.SamplerInitGreedy())

	out := make([]byte, 0, 128)
	generated := 0
	for pos := int32(0); pos < max; pos += batch.NTokens {
		if _, err := llama.Decode(ctx, batch); err != nil {
			return "", generated, err
		}
		token := llama.SamplerSample(sampler, ctx, -1)
		if llama.VocabIsEOG(vocab, token) {
			break
		}
		generated++
		buf := make([]byte, 64)
		n := llama.TokenToPiece(vocab, token, buf, 0, true)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		batch = llama.BatchGetOne([]llama.Token{token})
	}

	clearContext(ctx)
	return string(out), generated, nil
}

func p95(values []float64) float64 {
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	idx := int(float64(len(cp)-1) * 0.95)
	return cp[idx]
}

func main() {
	lib := os.Getenv("YZMA_LIB")
	modelPath := os.Getenv("YZMA_MODEL")
	if lib == "" || modelPath == "" {
		fmt.Println("set YZMA_LIB and YZMA_MODEL")
		os.Exit(1)
	}

	t0 := time.Now()
	llama.Load(lib)
	llama.LogSet(llama.LogSilent())
	llama.Init()
	model, err := llama.ModelLoadFromFile(modelPath, llama.ModelDefaultParams())
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer llama.ModelFree(model)
	modelLoadMS := float64(time.Since(t0).Microseconds()) / 1000.0

	cp := llama.ContextDefaultParams()
	cp.NCtx = 1024
	cp.NBatch = 1024
	cp.NThreads = 4
	cp.NThreadsBatch = 4
	ctx, err := llama.InitFromModel(model, cp)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer llama.Free(ctx)

	fmt.Printf("model_load_ms=%.2f\n", modelLoadMS)
	fmt.Printf("rss_after_model_load_kb=%d\n", rssKB())

	if _, _, err := generate(ctx, model, corpus[0], 16); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("determinism_outputs:")
	for i := 0; i < 5; i++ {
		out, _, err := generate(ctx, model, corpus[0], 16)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Printf("  run_%d=%q\n", i+1, out)
	}

	latencies := make([]float64, 0, 18)
	totalMS := 0.0
	totalTokens := 0
	for rep := 0; rep < 3; rep++ {
		for _, c := range corpus {
			start := time.Now()
			_, n, err := generate(ctx, model, c, 16)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			ms := float64(time.Since(start).Microseconds()) / 1000.0
			latencies = append(latencies, ms)
			totalMS += ms
			totalTokens += n
		}
	}

	fmt.Printf("requests=%d\n", len(latencies))
	fmt.Printf("avg_latency_ms=%.2f\n", totalMS/float64(len(latencies)))
	fmt.Printf("p95_latency_ms=%.2f\n", p95(latencies))
	fmt.Printf("tokens_generated=%d\n", totalTokens)
	fmt.Printf("tokens_per_sec=%.2f\n", (float64(totalTokens)/totalMS)*1000.0)
	fmt.Printf("rss_after_bench_kb=%d\n", rssKB())
}
GOEOF

(
  cd "$ROOT_TMP"
  if [ ! -f go.mod ]; then
    cat > go.mod <<'GOEOF'
module yzmaspike

go 1.24.4

require github.com/hybridgroup/yzma v1.9.0
GOEOF
  fi
  GOCACHE="$ROOT_TMP/.gocache" go mod tidy >/dev/null
  GOCACHE="$ROOT_TMP/.gocache" YZMA_LIB="$LIB_DIR" \
    YZMA_MODEL="$MODEL_DIR/$MODEL_FILE" go run embedded_bench.go \
    | tee "$ROOT_TMP/embedded-results.txt"
)

echo "[4/5] run service benchmark"
LAT="$ROOT_TMP/service-latencies.txt"
MODELMS="$ROOT_TMP/service-modelms.txt"
OUTS="$ROOT_TMP/service-outs.txt"
: >"$LAT"
: >"$MODELMS"
: >"$OUTS"

start_ts=$(python3 - <<'PY'
import time
print(time.time())
PY
)
"$LIB_DIR/llama-server" -m "$MODEL_DIR/$MODEL_FILE" --port "$PORT" \
  -n 16 -c 1024 -t 4 -tb 4 --no-mmap --log-disable \
  >"$ROOT_TMP/server.log" 2>&1 &
PID=$!
trap 'kill $PID 2>/dev/null || true' EXIT

for _ in {1..60}; do
  if curl -sf "http://127.0.0.1:$PORT/health" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
ready_ts=$(python3 - <<'PY'
import time
print(time.time())
PY
)
startup_ms=$(python3 - <<PY
s=$start_ts
r=$ready_ts
print(f"{(r-s)*1000:.2f}")
PY
)
rss_load=$(ps -o rss= -p "$PID" | tr -d ' ')

prompts=(
"Classify as WEASEL or OK. Text: This approach may potentially improve outcomes in many situations. Label:"
"Classify as WEASEL or OK. Text: Run go test ./... before submitting the pull request. Label:"
"Classify as WEASEL or OK. Text: It seems the API is kind of unreliable under heavy load. Label:"
"Classify as WEASEL or OK. Text: Set timeout to 2s and retry once on HTTP 503. Label:"
"Classify as WEASEL or OK. Text: You might want to consider adding additional validation checks. Label:"
"Classify as WEASEL or OK. Text: The parser accepts front matter and heading sections. Label:"
)

for i in 1 2 3 4 5; do
  payload=$(jq -n --arg p "${prompts[0]}" '{prompt:$p,n_predict:16,temperature:0}')
  tmp=$(mktemp)
  curl -sS -o "$tmp" -X POST "http://127.0.0.1:$PORT/completion" \
    -H 'Content-Type: application/json' -d "$payload" >/dev/null
  jq -r '.content' "$tmp" | sed -n '1p' >>"$OUTS"
  rm -f "$tmp"
done

for rep in 1 2 3; do
  for prompt in "${prompts[@]}"; do
    payload=$(jq -n --arg p "$prompt" '{prompt:$p,n_predict:16,temperature:0}')
    tmp=$(mktemp)
    total=$(curl -sS -w '%{time_total}' -o "$tmp" -X POST \
      "http://127.0.0.1:$PORT/completion" -H 'Content-Type: application/json' \
      -d "$payload")
    echo "$total" >>"$LAT"
    jq -r '(.timings.prompt_ms + .timings.predicted_ms)' "$tmp" >>"$MODELMS"
    rm -f "$tmp"
  done
done

rss_end=$(ps -o rss= -p "$PID" | tr -d ' ')

python3 - <<PY | tee "$ROOT_TMP/service-results.txt"
import statistics
from pathlib import Path
lat = [float(x.strip()) for x in Path('$LAT').read_text().splitlines() if x.strip()]
mod = [float(x.strip()) for x in Path('$MODELMS').read_text().splitlines() if x.strip()]
outs = Path('$OUTS').read_text().splitlines()
lat_s = sorted(lat)
mod_s = sorted(mod)
idx = int((len(lat_s)-1)*0.95)
idxm = int((len(mod_s)-1)*0.95)
print(f"startup_ms={float('$startup_ms'):.2f}")
print(f"rss_after_model_load_kb={int('$rss_load')}")
print("determinism_outputs:")
for i, line in enumerate(outs[:5], 1):
    print(f"  run_{i}={line!r}")
print(f"requests={len(lat)}")
print(f"avg_latency_ms={statistics.mean(lat)*1000:.2f}")
print(f"p95_latency_ms={lat_s[idx]*1000:.2f}")
print(f"avg_model_timing_ms={statistics.mean(mod):.2f}")
print(f"p95_model_timing_ms={mod_s[idxm]:.2f}")
print(f"tokens_generated={len(lat)*16}")
print(f"tokens_per_sec={16/statistics.mean(lat):.2f}")
print(f"rss_after_bench_kb={int('$rss_end')}")
PY

kill "$PID"
wait "$PID" || true

echo "[5/5] done"
echo "Embedded results: $ROOT_TMP/embedded-results.txt"
echo "Service results:  $ROOT_TMP/service-results.txt"
