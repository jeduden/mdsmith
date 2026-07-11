package fix

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rule"
)

// BenchmarkFixCorpus pins the allocation win from configuring each
// Configurable rule once per config signature (effectiveCachedForFix +
// configuredFor) instead of once per file — see
// docs/development/high-performance-go.md and
// TestFix_ConfiguresRulesOncePerConfigSignature. config.Defaults()
// declares no kinds or overrides, so every file in the corpus shares one
// signature: fixableRules' clone + ApplySettings work (regex compiles
// included) now runs once for the whole run instead of three times per
// file (fixableRules, plus the pre- and post-fix CheckRules calls).
//
// Baseline (2026-07-11, 60 files, config.Defaults()'s production rule
// set, before the confCache/effCache fix): ~9.3k allocs/op, because
// every file re-cloned and re-configured every enabled Configurable
// rule three times over (fixableRules, plus the pre- and post-fix
// CheckRules calls). After the fix: ~8.4k allocs/op — this corpus has
// few Configurable rules with expensive ApplySettings work, so the win
// is modest here; it grows with the number of Configurable rules a
// project enables and with corpus size, same as
// BenchmarkCheckCorpusLarge's analogous engine-side fix. The budget
// below sits at ~20% headroom over the post-fix measured count so a
// regression that bypasses the cache (a lost memoization slot, a
// signature that no longer matches) crosses the ceiling on the first
// run while machine-to-machine noise does not.
func BenchmarkFixCorpus(b *testing.B) {
	if testing.Short() {
		b.Skip("benchmark skipped in -short mode")
	}

	const files = 60
	dir := b.TempDir()
	paths := make([]string, 0, files)
	for i := 0; i < files; i++ {
		p := filepath.Join(dir, fmt.Sprintf("doc%03d.md", i))
		if err := os.WriteFile(p, []byte(fixCorpusDoc(i)), 0o644); err != nil {
			b.Fatalf("write corpus file: %v", err)
		}
		paths = append(paths, p)
	}

	cfg := config.Defaults()
	newFixer := func() *Fixer {
		return &Fixer{
			Config:  cfg,
			Rules:   rule.All(),
			RootDir: dir,
		}
	}

	// Warm the filesystem cache before measuring.
	_ = newFixer().Fix(paths)

	allocs := testing.AllocsPerRun(5, func() {
		_ = newFixer().Fix(paths)
	})

	// ~20% headroom over the measured baseline (2026-07-11, 60 files,
	// post-fix ~8.4k allocs/op), matching the project convention in
	// internal/engine/bench_test.go's BenchmarkCheckCorpus*.
	const budget = 10_500
	b.ReportMetric(allocs, "allocs_per_op")
	if allocs > budget {
		b.Fatalf("BenchmarkFixCorpus allocs/op = %.0f, budget %d — the per-signature "+
			"rule-configuration cache (effectiveCachedForFix/configuredFor) may have "+
			"regressed to per-file reconfiguration", allocs, budget)
	}
}

// fixCorpusDoc emits a small, valid Markdown document with a trailing-
// space violation so the corpus exercises at least one fixable rule
// without tripping any others.
func fixCorpusDoc(idx int) string {
	return fmt.Sprintf("# Doc %d\n\nSome content.  \n\nMore content here.\n", idx)
}
