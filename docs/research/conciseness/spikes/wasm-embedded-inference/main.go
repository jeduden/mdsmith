//go:build ignore

// Command wasm-spike is the host harness for the wasm-embedded-inference
// spike. It embeds the wasm classifier artifact via go:embed, hosts it
// with wazero, and measures determinism + latency + memory on the same
// six-sample corpus the yzma and go-native spikes used.
package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// Embedded wasm artifact produced by the wasm/ subdirectory. run.sh
// compiles wasm/main.go with GOOS=wasip1 GOARCH=wasm and copies the
// result here before building this harness.
//
//go:embed classifier.wasm
var wasmArtifact []byte

type sample struct {
	ID   string
	Text string
}

var corpus = []sample{
	{
		ID:   "weasel-01",
		Text: "This approach may potentially improve outcomes in many situations.",
	},
	{
		ID:   "direct-01",
		Text: "Run go test ./... before submitting the pull request.",
	},
	{
		ID:   "weasel-02",
		Text: "It seems the API is kind of unreliable under heavy load.",
	},
	{
		ID:   "direct-02",
		Text: "Set timeout to 2s and retry once on HTTP 503.",
	},
	{
		ID:   "weasel-03",
		Text: "You might want to consider adding additional validation checks.",
	},
	{
		ID:   "direct-03",
		Text: "The parser accepts front matter and heading sections.",
	},
}

type runConfig struct {
	mode            string
	rounds          int
	determinismRuns int
}

type classifyResult struct {
	Label     string  `json:"label"`
	RiskScore float64 `json:"risk_score"`
	Threshold float64 `json:"threshold"`
	ModelID   string  `json:"model_id"`
	Version   string  `json:"version"`
	Cues      string  `json:"cues"`
}

type guestHandle struct {
	ctx      context.Context
	runtime  wazero.Runtime
	module   api.Module
	alloc    api.Function
	free     api.Function
	classify api.Function
	memory   api.Memory
}

func (g *guestHandle) Close() error {
	return g.runtime.Close(g.ctx)
}

func (g *guestHandle) Classify(text string) (classifyResult, error) {
	input := []byte(text)
	var ptr uint32

	if len(input) > 0 {
		allocRet, err := g.alloc.Call(g.ctx, uint64(len(input)))
		if err != nil {
			return classifyResult{}, fmt.Errorf("alloc: %w", err)
		}
		ptr = uint32(allocRet[0])
		if ptr == 0 {
			return classifyResult{}, errors.New("alloc returned null ptr")
		}
		defer func() { _, _ = g.free.Call(g.ctx, uint64(ptr)) }()

		if !g.memory.Write(ptr, input) {
			return classifyResult{}, errors.New("memory.Write failed")
		}
	}

	ret, err := g.classify.Call(g.ctx, uint64(ptr), uint64(len(input)))
	if err != nil {
		return classifyResult{}, fmt.Errorf("classify: %w", err)
	}
	packed := int64(ret[0])
	if packed < 0 {
		return classifyResult{}, errors.New("guest signaled output truncation")
	}
	outPtr := uint32(uint64(packed) >> 32)
	outLen := uint32(uint64(packed) & 0xFFFFFFFF)
	raw, ok := g.memory.Read(outPtr, outLen)
	if !ok {
		return classifyResult{}, errors.New("memory.Read failed")
	}
	buf := make([]byte, len(raw))
	copy(buf, raw)

	var result classifyResult
	if err := json.Unmarshal(buf, &result); err != nil {
		return classifyResult{}, fmt.Errorf("decode result: %w", err)
	}
	return result, nil
}

func newGuest(ctx context.Context) (*guestHandle, error) {
	r := wazero.NewRuntimeWithConfig(
		ctx,
		wazero.NewRuntimeConfig().WithCloseOnContextDone(true),
	)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		_ = r.Close(ctx)
		return nil, fmt.Errorf("wasi instantiate: %w", err)
	}
	cfg := wazero.NewModuleConfig().
		WithStdout(os.Stderr).
		WithStderr(os.Stderr).
		WithStartFunctions(). // reactor: do not auto-run _start
		WithArgs("classifier-wasm")
	mod, err := r.InstantiateWithConfig(ctx, wasmArtifact, cfg)
	if err != nil {
		_ = r.Close(ctx)
		return nil, fmt.Errorf("instantiate module: %w", err)
	}
	if initFn := mod.ExportedFunction("_initialize"); initFn != nil {
		if _, err := initFn.Call(ctx); err != nil {
			_ = r.Close(ctx)
			return nil, fmt.Errorf("_initialize: %w", err)
		}
	}
	g := &guestHandle{
		ctx:      ctx,
		runtime:  r,
		module:   mod,
		alloc:    mod.ExportedFunction("alloc"),
		free:     mod.ExportedFunction("free"),
		classify: mod.ExportedFunction("classify"),
		memory:   mod.Memory(),
	}
	if g.alloc == nil || g.free == nil || g.classify == nil {
		_ = r.Close(ctx)
		return nil, errors.New("missing exported function on wasm module")
	}
	if g.memory == nil {
		_ = r.Close(ctx)
		return nil, errors.New("wasm module has no exported memory")
	}
	return g, nil
}

func main() {
	cfg, err := parseFlags()
	if err != nil {
		exitErr(err)
	}

	ctx := context.Background()
	loadStart := time.Now()
	guest, err := newGuest(ctx)
	if err != nil {
		exitErr(err)
	}
	defer func() { _ = guest.Close() }()
	loadMS := float64(time.Since(loadStart).Microseconds()) / 1000.0

	switch cfg.mode {
	case "digest":
		fmt.Println(corpusDigest(guest))
	case "bench":
		runBench(guest, loadMS, cfg)
	default:
		exitErr(errors.New("mode must be bench or digest"))
	}
}

func parseFlags() (runConfig, error) {
	mode := flag.String("mode", "bench", "bench or digest")
	rounds := flag.Int("rounds", 4000, "benchmark rounds over the corpus")
	determinismRuns := flag.Int(
		"determinism-runs", 5, "in-process digest runs",
	)
	flag.Parse()
	if *rounds < 1 {
		return runConfig{}, fmt.Errorf("rounds must be >= 1, got %d", *rounds)
	}
	if *determinismRuns < 1 {
		return runConfig{}, fmt.Errorf(
			"determinism-runs must be >= 1, got %d", *determinismRuns,
		)
	}
	return runConfig{
		mode:            *mode,
		rounds:          *rounds,
		determinismRuns: *determinismRuns,
	}, nil
}

func runBench(guest *guestHandle, loadMS float64, cfg runConfig) {
	printArtifactMetadata(guest, loadMS)
	digest, unique := determinismStats(guest, cfg.determinismRuns)
	fmt.Printf("determinism_digest=%s\n", digest)
	fmt.Printf("determinism_unique_hashes=%d\n", unique)

	metrics := collectBenchMetrics(guest, cfg.rounds)
	printBenchMetrics(metrics)
	printRiskLines(metrics.riskByID)
}

func printArtifactMetadata(guest *guestHandle, loadMS float64) {
	// Sample once to capture metadata embedded in the guest.
	probe, err := guest.Classify(corpus[0].Text)
	if err != nil {
		exitErr(fmt.Errorf("probe classify: %w", err))
	}
	sum := sha256.Sum256(wasmArtifact)
	fmt.Printf("backend=wasm\n")
	fmt.Printf("wasm_runtime=wazero\n")
	fmt.Printf("wasm_artifact_bytes=%d\n", len(wasmArtifact))
	fmt.Printf("wasm_artifact_sha256=%s\n", hex.EncodeToString(sum[:]))
	fmt.Printf("model_id=%s\n", probe.ModelID)
	fmt.Printf("model_version=%s\n", probe.Version)
	fmt.Printf("threshold=%.4f\n", probe.Threshold)
	fmt.Printf("guest_load_ms=%.4f\n", loadMS)
	fmt.Printf("rss_after_load_kb=%d\n", rssKB())
}

func determinismStats(guest *guestHandle, runs int) (string, int) {
	digests := make([]string, 0, runs)
	for i := 0; i < runs; i++ {
		digests = append(digests, corpusDigest(guest))
	}
	return digests[0], uniqueCount(digests)
}

type benchMetrics struct {
	requests                int
	avgLatencyUS            float64
	p50LatencyUS            float64
	p95LatencyUS            float64
	maxLatencyUS            float64
	rssAfterBenchKB         int
	heapAllocAfterBenchKB   uint64
	heapSysAfterBenchKB     uint64
	totalAllocDeltaKB       uint64
	labelsVerboseActionable int
	labelsAcceptable        int
	riskByID                map[string]float64
}

func collectBenchMetrics(guest *guestHandle, rounds int) benchMetrics {
	var before runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	latencies := make([]float64, 0, len(corpus)*rounds)
	labelCounts := map[string]int{}
	riskByID := map[string]float64{}
	for i := 0; i < rounds; i++ {
		for _, c := range corpus {
			start := time.Now()
			result, err := guest.Classify(c.Text)
			if err != nil {
				exitErr(err)
			}
			delta := float64(time.Since(start).Nanoseconds()) / 1000.0
			latencies = append(latencies, delta)
			labelCounts[result.Label]++
			if i == 0 {
				riskByID[c.ID] = result.RiskScore
			}
		}
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	totalUS := sum(latencies)
	requests := len(latencies)
	return benchMetrics{
		requests:                requests,
		avgLatencyUS:            totalUS / float64(requests),
		p50LatencyUS:            percentile(latencies, 0.50),
		p95LatencyUS:            percentile(latencies, 0.95),
		maxLatencyUS:            percentile(latencies, 1.00),
		rssAfterBenchKB:         rssKB(),
		heapAllocAfterBenchKB:   after.HeapAlloc / 1024,
		heapSysAfterBenchKB:     after.HeapSys / 1024,
		totalAllocDeltaKB:       (after.TotalAlloc - before.TotalAlloc) / 1024,
		labelsVerboseActionable: labelCounts["verbose-actionable"],
		labelsAcceptable:        labelCounts["acceptable"],
		riskByID:                riskByID,
	}
}

func printBenchMetrics(metrics benchMetrics) {
	fmt.Printf("requests=%d\n", metrics.requests)
	fmt.Printf("avg_latency_us=%.4f\n", metrics.avgLatencyUS)
	fmt.Printf("p50_latency_us=%.4f\n", metrics.p50LatencyUS)
	fmt.Printf("p95_latency_us=%.4f\n", metrics.p95LatencyUS)
	fmt.Printf("max_latency_us=%.4f\n", metrics.maxLatencyUS)
	fmt.Printf("rss_after_bench_kb=%d\n", metrics.rssAfterBenchKB)
	fmt.Printf("heap_alloc_after_bench_kb=%d\n", metrics.heapAllocAfterBenchKB)
	fmt.Printf("heap_sys_after_bench_kb=%d\n", metrics.heapSysAfterBenchKB)
	fmt.Printf("total_alloc_delta_kb=%d\n", metrics.totalAllocDeltaKB)
	fmt.Printf(
		"labels_verbose_actionable=%d\n", metrics.labelsVerboseActionable,
	)
	fmt.Printf("labels_acceptable=%d\n", metrics.labelsAcceptable)
}

func printRiskLines(riskByID map[string]float64) {
	keys := make([]string, 0, len(riskByID))
	for id := range riskByID {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		fmt.Printf("risk_%s=%.4f\n", id, riskByID[id])
	}
}

func corpusDigest(guest *guestHandle) string {
	var b strings.Builder
	for _, c := range corpus {
		r, err := guest.Classify(c.Text)
		if err != nil {
			exitErr(err)
		}
		_, _ = fmt.Fprintf(
			&b, "%s|%s|%.6f|%s\n",
			c.ID, r.Label, r.RiskScore, r.Cues,
		)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func rssKB() int {
	out, err := exec.Command(
		"ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid()),
	).Output()
	if err != nil {
		return heapSysKB()
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return heapSysKB()
	}
	return value
}

func heapSysKB() int {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return int(stats.HeapSys / 1024)
}

func uniqueCount(values []string) int {
	set := map[string]struct{}{}
	for _, v := range values {
		set[v] = struct{}{}
	}
	return len(set)
}

func percentile(values []float64, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	if q <= 0 {
		return cp[0]
	}
	if q >= 1 {
		return cp[len(cp)-1]
	}
	idx := int(q * float64(len(cp)-1))
	return cp[idx]
}

func sum(values []float64) float64 {
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
