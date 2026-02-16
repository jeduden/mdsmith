package main

import (
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/jeduden/mdsmith/internal/rules/concisenessscoring/classifier"
)

type sample struct {
	ID   string
	Text string
}

type runConfig struct {
	mode            string
	rounds          int
	determinismRuns int
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

func main() {
	cfg := parseFlags()
	model, loadMS, err := loadModel()
	if err != nil {
		exitErr(err)
	}

	switch cfg.mode {
	case "digest":
		fmt.Println(corpusDigest(model))
		return
	case "bench":
		runBench(model, loadMS, cfg)
		return
	default:
		exitErr(errors.New("mode must be bench or digest"))
	}
}

func parseFlags() runConfig {
	mode := flag.String("mode", "bench", "bench or digest")
	rounds := flag.Int("rounds", 4000, "benchmark rounds over the corpus")
	determinismRuns := flag.Int("determinism-runs", 5, "in-process digest runs")
	flag.Parse()
	return runConfig{
		mode:            *mode,
		rounds:          *rounds,
		determinismRuns: *determinismRuns,
	}
}

func loadModel() (*classifier.Model, float64, error) {
	modelStart := time.Now()
	model, err := classifier.LoadEmbedded()
	if err != nil {
		return nil, 0, err
	}
	loadMS := microsToMS(float64(time.Since(modelStart).Microseconds()))
	return model, loadMS, nil
}

func runBench(model *classifier.Model, loadMS float64, cfg runConfig) {
	printModelMetadata(model, loadMS)
	digest, unique := determinismStats(model, cfg.determinismRuns)
	fmt.Printf("determinism_digest=%s\n", digest)
	fmt.Printf("determinism_unique_hashes=%d\n", unique)

	metrics := collectBenchMetrics(model, cfg.rounds)
	printBenchMetrics(metrics)
	printRiskLines(metrics.riskByID)
}

func printModelMetadata(model *classifier.Model, loadMS float64) {
	counts := model.LexiconCounts()
	fmt.Printf("model_id=%s\n", model.ModelID())
	fmt.Printf("model_version=%s\n", model.Version())
	fmt.Printf("threshold=%.4f\n", model.Threshold())
	fmt.Printf("artifact_path=%s\n", classifier.EmbeddedArtifactPath)
	fmt.Printf("artifact_sha256=%s\n", classifier.EmbeddedArtifactSHA256)
	fmt.Printf("artifact_bytes=%d\n", classifier.EmbeddedArtifactBytes())
	fmt.Printf("lexicon_filler_words=%d\n", counts.FillerWords)
	fmt.Printf("lexicon_modal_words=%d\n", counts.ModalWords)
	fmt.Printf("lexicon_vague_words=%d\n", counts.VagueWords)
	fmt.Printf("lexicon_action_words=%d\n", counts.ActionWords)
	fmt.Printf("lexicon_stop_words=%d\n", counts.StopWords)
	fmt.Printf("lexicon_hedge_phrases=%d\n", counts.HedgePhrases)
	fmt.Printf("lexicon_verbose_phrases=%d\n", counts.VerbosePhrases)
	fmt.Printf("model_load_ms=%.4f\n", loadMS)
	fmt.Printf("rss_after_load_kb=%d\n", rssKB())
}

func determinismStats(model *classifier.Model, runs int) (string, int) {
	digests := make([]string, 0, runs)
	for i := 0; i < runs; i++ {
		digests = append(digests, corpusDigest(model))
	}
	return digests[0], uniqueCount(digests)
}

func collectBenchMetrics(model *classifier.Model, rounds int) benchMetrics {
	var memBefore runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)

	latencies := make([]float64, 0, len(corpus)*rounds)
	labelCounts := map[string]int{}
	riskByID := map[string]float64{}
	for i := 0; i < rounds; i++ {
		for _, c := range corpus {
			start := time.Now()
			result := model.Classify(c.Text)
			deltaUS := float64(time.Since(start).Nanoseconds()) / 1000.0
			latencies = append(latencies, deltaUS)
			labelCounts[result.Label]++
			if i == 0 {
				riskByID[c.ID] = result.RiskScore
			}
		}
	}

	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	totalUS := sum(latencies)
	requests := len(latencies)
	return benchMetrics{
		requests:                requests,
		avgLatencyUS:            totalUS / float64(requests),
		p50LatencyUS:            percentile(latencies, 0.50),
		p95LatencyUS:            percentile(latencies, 0.95),
		maxLatencyUS:            percentile(latencies, 1.00),
		rssAfterBenchKB:         rssKB(),
		heapAllocAfterBenchKB:   memAfter.HeapAlloc / 1024,
		heapSysAfterBenchKB:     memAfter.HeapSys / 1024,
		totalAllocDeltaKB:       (memAfter.TotalAlloc - memBefore.TotalAlloc) / 1024,
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
		"labels_verbose_actionable=%d\n",
		metrics.labelsVerboseActionable,
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

func corpusDigest(model *classifier.Model) string {
	var b strings.Builder
	for _, c := range corpus {
		r := model.Classify(c.Text)
		_, _ = fmt.Fprintf(
			&b,
			"%s|%s|%.6f|%s\n",
			c.ID,
			r.Label,
			r.RiskScore,
			strings.Join(r.TriggeredCues, ","),
		)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func rssKB() int {
	out, err := exec.Command(
		"ps",
		"-o",
		"rss=",
		"-p",
		strconv.Itoa(os.Getpid()),
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

func microsToMS(v float64) float64 {
	return v / 1000.0
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
