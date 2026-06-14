package engine_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"testing"
	"time"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/engine"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

	// Production rule set, so the spike measures what `mdsmith check`
	// actually runs under the parity convention.
	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// TestSpikeBlockParseCost is the lazy-parse spike measurement harness
// (plan 2606141901). It answers the open question in the lazy-parse
// research note: is `block scan + parity rules + overhead` faster than
// gomarklint on the neutral corpus, and — if not — is the residual cost
// the parse or the rules+overhead?
//
// It is INERT by default. It runs only when MDSMITH_SPIKE_CORPUS points
// at a directory of Markdown files, so CI (which sets no such variable)
// skips it entirely. To reproduce the recorded numbers, build the
// pinned neutral corpus (the Rust Book + Reference src/ trees at the
// SHAs in internal/release/bench.go) and run:
//
//	MDSMITH_SPIKE_CORPUS=/path/to/corpus_neutral \
//	  go test -run TestSpikeBlockParseCost -v ./internal/engine/
//
// Override the iteration count with MDSMITH_SPIKE_ITERS (default 15).
//
// It times six pipelines over the corpus. Five run through the real
// engine.Runner with the parity rule set, so per-file read, config
// resolution, merge, and sort overhead are counted exactly as
// `mdsmith check -c parity` pays them; only the parse phase, the rule
// set, and source-context population differ between them:
//
//	read_only       read + front-matter strip + line split (no parse, no rules)
//	parse_block     read + goldmark BLOCK-only parse,  no rules
//	parse_full      read + goldmark full parse,        no rules
//	block+rules     read + block-only parse + parity rules + merge   <- headline
//	full_parity     read + full parse      + parity rules + merge    <- baseline
//	full_parity_ctx full_parity + per-diagnostic source-context population
//
// The block-only parse (Runner.BlockOnlyParse) is a stand-in for a
// future Layer-0 block scan. goldmark's block phase still builds a block
// node tree, so it is an UPPER BOUND on Layer 0's parse cost: a real
// flat scanner would be cheaper. If even block+rules beats gomarklint,
// Layer 0 clears the bar with room to spare.
func TestSpikeBlockParseCost(t *testing.T) {
	corpus := os.Getenv("MDSMITH_SPIKE_CORPUS")
	if corpus == "" {
		t.Skip("set MDSMITH_SPIKE_CORPUS to a Markdown corpus dir to run the lazy-parse spike")
	}
	paths := spikeCollectMarkdown(t, corpus)
	if len(paths) == 0 {
		t.Fatalf("no .md files found under %s", corpus)
	}
	pipes, nDiag := spikePipelines(corpus, spikeLoadParityConfig(t), rule.All(), paths)
	iters := spikeIters()
	med, mn := spikeMeasure(pipes, iters)
	spikeReport(t, paths, nDiag, iters, pipes, med, mn)
}

// spikePipeline is one named, timeable run over the corpus.
type spikePipeline struct {
	name string
	fn   func()
}

// spikePipelines builds the six measured pipelines plus the full-parity
// diagnostic count. Each runner is SERIAL (Concurrency=1,
// IntraFileConcurrency=1): the parse-vs-rules decomposition is about total
// CPU work per phase, and a 4-core parallel wall-clock buries that signal
// under scheduler jitter. Serial timing isolates each phase; the FRACTIONS
// transfer to the parallel CLI, which fans every phase out by the same
// 1/cores. The authoritative parallel wall-time-vs-gomarklint number comes
// from the CLI/hyperfine run that accompanies this harness, not here.
func spikePipelines(
	corpus string, cfg *config.Config, allRules []rule.Rule, paths []string,
) ([]spikePipeline, int) {
	mkRunner := func(rules []rule.Rule, blockOnly, srcCtx bool) *engine.Runner {
		return &engine.Runner{
			Config: cfg, Rules: rules, StripFrontMatter: true, RootDir: corpus,
			SkipSourceContext: !srcCtx, BlockOnlyParse: blockOnly,
			Concurrency: 1, IntraFileConcurrency: 1,
		}
	}
	run := func(rules []rule.Rule, blockOnly bool) func() {
		return func() { _ = mkRunner(rules, blockOnly, false).Run(paths) }
	}
	// One-shot: how many diagnostics full parity emits on this corpus. The
	// corpus is diagnostic-heavy, so source-context population (the
	// full_parity_ctx delta) is real CLI cost the other pipelines exclude.
	nDiag := len(mkRunner(allRules, false, false).Run(paths).Diagnostics)
	pipes := []spikePipeline{
		{"read_only", func() { spikeReadOnly(paths) }},
		{"parse_block", run(nil, true)},
		{"parse_full", run(nil, false)},
		{"block+rules", run(allRules, true)},
		{"full_parity", run(allRules, false)},
		{"full_parity_ctx", func() { _ = mkRunner(allRules, false, true).Run(paths) }},
	}
	return pipes, nDiag
}

// spikeMeasure warms then times each pipeline iters times, returning the
// median and min per pipeline. GC is pinned high for the loop: the
// gomarklint-architecture note found the ~16% GC in a back-to-back
// in-process bench is an artifact the single-shot CLI never pays, so
// raising GOGC lets the min approach the true uninterfered CPU cost.
func spikeMeasure(pipes []spikePipeline, iters int) (med, mn map[string]time.Duration) {
	prevGC := debug.SetGCPercent(800)
	defer debug.SetGCPercent(prevGC)
	med = map[string]time.Duration{}
	mn = map[string]time.Duration{}
	for _, p := range pipes {
		p.fn() // warm
		samples := make([]time.Duration, 0, iters)
		for i := 0; i < iters; i++ {
			start := time.Now()
			p.fn()
			samples = append(samples, time.Since(start))
		}
		med[p.name] = percentileDur(samples, 0.5)
		mn[p.name] = percentileDur(samples, 0.0)
	}
	return med, mn
}

// spikeReport logs the pipeline table and the per-phase decomposition.
// Each phase is a difference of two pipelines that share all earlier
// phases, so the shared cost cancels and the delta isolates one phase
// (min is the cleanest CPU-bound signal).
func spikeReport(
	t *testing.T, paths []string, nDiag, iters int,
	pipes []spikePipeline, med, mn map[string]time.Duration,
) {
	t.Helper()
	var totalBytes int64
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil {
			totalBytes += fi.Size()
		}
	}
	t.Logf("lazy-parse spike (SERIAL CPU): %d files, %.2f MiB, %d diags, %d iters, GOMAXPROCS=%d",
		len(paths), float64(totalBytes)/(1<<20), nDiag, iters, runtime.GOMAXPROCS(0))
	t.Logf("%-15s  %10s  %10s", "pipeline", "median", "min")
	for _, p := range pipes {
		t.Logf("%-15s  %10s  %10s", p.name,
			med[p.name].Round(time.Microsecond), mn[p.name].Round(time.Microsecond))
	}
	full := mn["full_parity"]
	r := mn["read_only"]
	pb := mn["parse_block"] - r                   // block parse alone
	pi := mn["parse_full"] - mn["parse_block"]    // inline parse alone
	ruMo := mn["block+rules"] - mn["parse_block"] // parity rules + merge/sort
	srcCtx := mn["full_parity_ctx"] - full        // source-context population
	parseFreeFloor := r + ruMo                    // rules + overhead, free parse
	us := func(d time.Duration) time.Duration { return d.Round(time.Microsecond) }
	pct := func(d time.Duration) float64 { return 100 * float64(d) / float64(full) }
	t.Logf("decomposition (min, as %% of full_parity):")
	t.Logf("  R  read+split            = %9s  %5.1f%%", us(r), pct(r))
	t.Logf("  Pb block parse           = %9s  %5.1f%%", us(pb), pct(pb))
	t.Logf("  Pi inline parse          = %9s  %5.1f%%", us(pi), pct(pi))
	t.Logf("  Ru+Mo rules+merge        = %9s  %5.1f%%", us(ruMo), pct(ruMo))
	t.Logf("  parse-free floor R+Ru+Mo = %9s  %5.1f%%", us(parseFreeFloor), pct(parseFreeFloor))
	t.Logf("  src-context populate     = %9s  %5.1f%%", us(srcCtx), pct(srcCtx))
	t.Logf("  block+rules/full      = %.3f (inline parse removed)", ratio(mn["block+rules"], full))
	t.Logf("  parse-free floor/full = %.3f (all parse removed)", ratio(parseFreeFloor, full))
}

func ratio(a, b time.Duration) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// spikeReadOnly mirrors the engine's per-file read (read, strip front
// matter, split lines) with no parse and no rules, serially — matching
// the serial pipelines so the per-file I/O floor R is on the same
// footing as Pb/Pi/Ru+Mo in the decomposition.
func spikeReadOnly(paths []string) {
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		_, content := lint.StripFrontMatter(b)
		_ = bytes.Split(content, []byte("\n"))
	}
}

// spikeCollectMarkdown returns every *.md path under root.
func spikeCollectMarkdown(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(p) == ".md" {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk corpus %s: %v", root, err)
	}
	return paths
}

// spikeLoadParityConfig loads the rule config the spike runs. It defaults
// to the committed benchmark parity profile (so the spike matches
// `mdsmith check -c parity`), but honours MDSMITH_SPIKE_CONFIG to point at
// any other config — e.g. a single-rule profile for a per-rule bottleneck
// study. The defaults are merged in exactly as cmd/mdsmith's loadConfigRaw
// does, so the structural rules keep their real (enabled) state.
func spikeLoadParityConfig(t *testing.T) *config.Config {
	t.Helper()
	path := os.Getenv("MDSMITH_SPIKE_CONFIG")
	if path == "" {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("runtime.Caller failed")
		}
		root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
		path = filepath.Join(root, "docs", "research", "benchmarks", "bench-parity.mdsmith.yml")
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("load spike config %s: %v", path, err)
	}
	return config.Merge(config.Defaults(), loaded)
}

// spikeIters reads MDSMITH_SPIKE_ITERS (default 15).
func spikeIters() int {
	if v := os.Getenv("MDSMITH_SPIKE_ITERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 15
}
