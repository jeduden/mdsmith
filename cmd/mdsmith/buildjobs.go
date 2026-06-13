package main

import (
	"io"
	"path/filepath"
	"sync"
	"time"

	buildexec "github.com/jeduden/mdsmith/internal/build"
	"github.com/jeduden/mdsmith/internal/config"
)

// concurrentResult is one target's outcome plus the cache entry to apply.
// Each worker writes its result into the slot at its target index, so the
// post-dispatch loop reads them back in declared order.
type concurrentResult struct {
	outcome targetOutcome
	entry   *buildexec.CacheEntry
}

// runConcurrent dispatches targets through a worker pool of opts.jobs
// goroutines. Each worker computes its verdict and runs its recipe; the
// recipe streams are written through a mutex-synchronized writer so lines
// stay coherent across concurrent recipes. Cache entries are collected
// and applied serially in declared order after every recipe finishes, so
// the shared cache is never touched concurrently. Plan 103 rejects
// overlapping outputs at load, so the targets' writes are disjoint.
func runConcurrent(
	builder buildexec.Builder, targets []buildTarget, cfg *config.Config,
	opts buildPassOpts, cache *buildexec.Cache, timeout time.Duration,
	w io.Writer, fold func(targetOutcome),
) {
	sw := &syncWriter{w: w}

	// Collect all finals for concurrent-safe post-condition checks.
	allFinals := make([]string, 0, len(targets))
	for _, bt := range targets {
		for _, rel := range bt.target.Outputs {
			allFinals = append(allFinals, filepath.Join(bt.target.Root, filepath.FromSlash(rel)))
		}
	}

	// Verdicts read the shared cache; compute them serially up front so the
	// concurrent workers touch no shared cache state.
	stins := make([]buildexec.StalenessInput, len(targets))
	verdicts := make([]buildexec.Verdict, len(targets))
	verdictErrs := make([]error, len(targets))
	for i, bt := range targets {
		stins[i] = stalenessFor(bt, cfg)
		verdicts[i], verdictErrs[i] = targetVerdict(stins[i], cache, opts)
	}

	results := make([]concurrentResult, len(targets))
	sem := make(chan struct{}, opts.jobs)
	var wg sync.WaitGroup
	for i, bt := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, bt buildTarget) {
			defer wg.Done()
			defer func() { <-sem }()
			outcome, entry := decideAndRun(
				builder, bt, opts, stins[i], verdicts[i], verdictErrs[i], timeout, allFinals, sw)
			results[i] = concurrentResult{outcome: outcome, entry: entry}
		}(i, bt)
	}
	wg.Wait()

	for _, r := range results {
		fold(r.outcome)
		if r.entry != nil {
			cache.Put(*r.entry)
		}
	}
}

// syncWriter serializes Write calls so concurrent recipe streams produce
// line-coherent output. os/exec hands each Write a chunk; the recipe's
// own newline framing keeps lines whole, and the mutex keeps two recipes'
// chunks from interleaving mid-write.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

var _ io.Writer = (*syncWriter)(nil)
