package index

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"
)

// BenchmarkColdBuild1k measures the cost of indexing 1 000 synthetic
// files. Plan 131 sets a budget of 1 s for this size — anything
// noticeably slower would block the lazy-build path the LSP server
// runs on the first symbol request.
func BenchmarkColdBuild1k(b *testing.B) {
	if testing.Short() {
		b.Skip("benchmark skipped in -short mode")
	}
	files, loader := buildSyntheticCorpus(1000)
	const budget = time.Second

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := New("/root")
		start := time.Now()
		idx.Build(files, loader)
		elapsed := time.Since(start)
		b.ReportMetric(float64(elapsed.Milliseconds()), "build_ms")
		if elapsed > budget {
			b.Fatalf("cold build took %v (> %v) on %d files", elapsed, budget, len(files))
		}
	}
}

// TestParallelBuildSpeedup is the in-suite regression guard for the
// parallel build path. It only verifies that Build outperforms
// BuildSerial — a positive speedup. The canonical demonstration of
// plan 153's >= 2x design target lives in BenchmarkSerialBuild1k vs
// BenchmarkParallelBuild1k; run with:
//
//	go test -bench='Build1k$' -count=5 ./internal/index/
//
// to reproduce the 2x figure on an unloaded 4-core host.
//
// The reason this in-suite test enforces only a positive speedup and
// not the headline 2x is `go test ./...`: it runs many package test
// binaries concurrently, each consuming GOMAXPROCS-sized CPU
// budgets. The index package's parallel-build test routinely gets
// co-scheduled with other CPU-bound test packages, so its effective
// CPU budget drops well below 4 cores and the measured speedup
// drops with it. The benchmarks, which run in isolation, are the
// reliable place to assert magnitude.
//
// The test skips in -short mode because the 1 000-file build is
// slow on the smallest CI hardware.
func TestParallelBuildSpeedup(t *testing.T) {
	if testing.Short() {
		t.Skip("parallel speedup test skipped in -short mode")
	}
	if runtime.GOMAXPROCS(0) < 4 {
		t.Skipf("need GOMAXPROCS >= 4 for a parallel build, got %d", runtime.GOMAXPROCS(0))
	}
	files, loader := buildSyntheticCorpus(1000)

	// Warm the runtime so the first build isn't penalised by cold
	// allocator / parser caches — the comparison is between
	// already-warm runs.
	idxWarm := New("/root")
	idxWarm.Build(files, loader)

	// Compare the fastest serial and parallel sample, not the median.
	// On a busy CI runner a co-scheduled CPU-bound neighbour steals
	// cores from the parallel build for whole stretches, which drags
	// its median — and sometimes a whole attempt — below serial without
	// the fan-out being broken. The minimum sample is the run least
	// disturbed by that contention, so it reflects what the pipeline can
	// do rather than how busy the host was. Retrying covers the case
	// where one attempt is contended end to end; a real regression
	// (fan-out serialised) loses even its fastest sample every attempt.
	const attempts = 3
	var serial, parallel time.Duration
	for attempt := 1; attempt <= attempts; attempt++ {
		serial, parallel = measureBestBuild(t, files, loader)
		t.Logf("attempt %d/%d: best serial=%v best parallel=%v workers=%d speedup=%.2fx",
			attempt, attempts, serial, parallel, runtime.GOMAXPROCS(0),
			float64(serial)/float64(parallel))
		if parallel < serial {
			return
		}
	}
	t.Fatalf("parallel build never beat serial across %d attempts: "+
		"best parallel=%v >= best serial=%v (speedup %.2fx) — fan-out may be serialised",
		attempts, parallel, serial, float64(serial)/float64(parallel))
}

// BenchmarkSerialBuild1k pairs with BenchmarkParallelBuild1k to
// quantify the speedup directly. Run via:
//
//	go test -bench='Build1k$' -count=5 -benchmem \
//	  ./internal/index/
//
// On a 4-core x86_64 host this benchmark currently lands around
// 65 ms; BenchmarkParallelBuild1k lands around 30 ms — the design
// target's 2x speedup is comfortably exceeded.
func BenchmarkSerialBuild1k(b *testing.B) {
	if testing.Short() {
		b.Skip("benchmark skipped in -short mode")
	}
	files, loader := buildSyntheticCorpus(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := New("/root")
		start := time.Now()
		idx.BuildSerial(files, loader)
		elapsed := time.Since(start)
		b.ReportMetric(float64(elapsed.Milliseconds()), "build_ms")
	}
}

// measureBestBuild builds the corpus serially and in parallel `samples`
// times and returns the fastest (minimum) wall-clock time of each. The
// minimum is the sample least disturbed by a co-scheduled neighbour, so
// it isolates the build pipeline's own behaviour from host contention.
// It fails the test if the serial and parallel builds ever disagree on
// the file count.
func measureBestBuild(
	t *testing.T,
	files []string,
	loader func(string) ([]byte, error),
) (serial, parallel time.Duration) {
	t.Helper()
	const samples = 11
	serial, parallel = math.MaxInt64, math.MaxInt64
	for i := 0; i < samples; i++ {
		idxSerial := New("/root")
		startSerial := time.Now()
		idxSerial.BuildSerial(files, loader)
		serial = min(serial, time.Since(startSerial))

		idxParallel := New("/root")
		startParallel := time.Now()
		idxParallel.Build(files, loader)
		parallel = min(parallel, time.Since(startParallel))

		// Sanity-check both variants agree on file count before
		// comparing wall-clock numbers.
		if len(idxSerial.Files()) != len(idxParallel.Files()) {
			t.Fatalf("serial built %d files, parallel built %d",
				len(idxSerial.Files()), len(idxParallel.Files()))
		}
	}
	return serial, parallel
}

// BenchmarkParallelBuild1k matches BenchmarkColdBuild1k but uses the
// parallel-by-default Build. Plan 153 expects this to beat the serial
// baseline; the benchmark exists so `go test -bench` lets us track
// regressions.
func BenchmarkParallelBuild1k(b *testing.B) {
	if testing.Short() {
		b.Skip("benchmark skipped in -short mode")
	}
	files, loader := buildSyntheticCorpus(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := New("/root")
		start := time.Now()
		idx.Build(files, loader)
		elapsed := time.Since(start)
		b.ReportMetric(float64(elapsed.Milliseconds()), "build_ms")
	}
}

// BenchmarkIncrementalUpdate measures one Update on an established
// index. Plan 131 sets a 20 ms budget per `didChange`.
func BenchmarkIncrementalUpdate(b *testing.B) {
	if testing.Short() {
		b.Skip("benchmark skipped in -short mode")
	}
	files, loader := buildSyntheticCorpus(1000)
	idx := New("/root")
	idx.Build(files, loader)
	const budget = 20 * time.Millisecond

	src := []byte(syntheticBody(0))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		idx.Update(files[i%len(files)], src)
		elapsed := time.Since(start)
		if elapsed > budget {
			b.Fatalf("update took %v (> %v)", elapsed, budget)
		}
	}
}

func buildSyntheticCorpus(n int) ([]string, func(string) ([]byte, error)) {
	files := make([]string, 0, n)
	bodies := make(map[string][]byte, n)
	for i := 0; i < n; i++ {
		path := fmt.Sprintf("docs/file_%05d.md", i)
		files = append(files, path)
		bodies[path] = []byte(syntheticBody(i))
	}
	return files, func(p string) ([]byte, error) {
		if b, ok := bodies[p]; ok {
			return b, nil
		}
		return nil, fmt.Errorf("not found: %s", p)
	}
}

func syntheticBody(seed int) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: File %d\nkinds:\n  - reference\n", seed)
	b.WriteString("---\n")
	fmt.Fprintf(&b, "# Top heading %d\n\n", seed)
	for s := 0; s < 5; s++ {
		fmt.Fprintf(&b, "## Section %d-%d\n\n", seed, s)
		next := (seed + 1) % 1000
		fmt.Fprintf(&b,
			"Body for section %d.%d with [a link](./file_%05d.md#top-heading-%d).\n\n",
			seed, s, next, next)
	}
	return b.String()
}
