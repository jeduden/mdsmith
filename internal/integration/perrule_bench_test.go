package integration

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/pkg/markdown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/parser"
)

// perrule_bench_test.go is plan 2606022126's per-opt-in-rule regression
// gate. It sits ALONGSIDE TestPerRuleAllocBudget (alloc_budget_test.go)
// and the engine-level BenchmarkCheckCorpus* gates without replacing
// either.
//
// Division of labour:
//
//   - TestPerRuleAllocBudget enforces the published flat ≤ 10 allocs/op
//     ceiling (CLAUDE.md) across EVERY registered rule on the small
//     allocBudgetFixture. It is the absolute, codebase-documented bar.
//   - BenchmarkCheckCorpus* (internal/engine/bench_test.go) enforce a
//     whole-workspace p95 wall time and median allocs/op over the
//     production rule set running through the engine.
//   - This file is narrower and per-rule: for each OPT-IN rule it pins
//     a baseline (allocs/op AND a parse-normalised Check/parse time
//     ratio) measured on a larger representative doc, so a regression in
//     one opt-in rule's
//     Check trips a gate that NAMES that rule — even though opt-in
//     rules never run in BenchmarkCheckCorpus* (those use the default
//     rule set) and sit far under the flat ≤ 10 alloc ceiling.
//
// "Opt-in rule" is enumerated programmatically (never hardcoded) by
// optInRules: a rule that implements rule.Defaultable and whose
// EnabledByDefault() reports false. This mirrors the canonical
// predicate in internal/config/load.go's enabledByDefault.
//
// "Isolated" means the measurement calls r.Check(f) directly — no
// engine, no config-enable dance. Every other rule is simply not
// invoked, so isolation is structural.

// perRuleBenchDoc is the representative Markdown body each opt-in rule
// is measured against. It is deliberately LARGER (~240 lines) than
// alloc_budget_test.go's allocBudgetFixture (~20 lines): a sub-µs
// Check on a tiny file produces noisy ns/op, so the gate sizes the
// doc up until each rule's total parse+Check time sits comfortably
// above measurement jitter (parse alone is ~170µs here).
//
// The body is COMPLIANT under default settings — no default-enabled
// rule and no opt-in rule emits a diagnostic on it (verified by
// TestPerRuleBenchDocCompliant). That keeps the gate measuring each
// rule's BASE per-Check scan cost, not per-violation overhead, which
// legitimately scales with the number of diagnostics. Notably it uses
// inline links only (no reference definitions), so neither the
// default reference-label rules (MDS053/MDS054) nor the opt-in
// no-reference-style rule (MDS043) fire.
func perRuleBenchDoc() string {
	const sections = 12
	parts := make([]string, 0, sections)
	for s := 0; s < sections; s++ {
		var b strings.Builder
		fmt.Fprintf(&b, "## Section %d\n\n", s)
		b.WriteString("A short prose paragraph for the readability and structural\n")
		b.WriteString("rules to scan here. It stays one paragraph in length.\n\n")
		b.WriteString("See [the other doc](other.md) for the related details here.\n\n")
		b.WriteString("```go\nfunc f() int { return 0 }\n```\n\n")
		b.WriteString("- one item\n- two items\n- three items\n\n")
		b.WriteString("| Col | Other |\n| --- | ----- |\n| a   | b     |\n| c   | d     |")
		parts = append(parts, b.String())
	}
	return "# Document title\n\n" + strings.Join(parts, "\n\n") + "\n"
}

// perRuleBenchFS is a minimal in-memory filesystem so cross-file /
// directive rules reach real work and the link target the doc
// references resolves. ModTime is the zero Time to keep the FS map
// hash stable across runs.
func perRuleBenchFS(src []byte) fstest.MapFS {
	return fstest.MapFS{
		"doc.md": &fstest.MapFile{
			Data:    src,
			ModTime: time.Time{},
		},
		"other.md": &fstest.MapFile{
			Data:    []byte("# Other\n\nBody.\n"),
			ModTime: time.Time{},
		},
	}
}

// optInRules returns every registered rule that is opt-in: it
// implements rule.Defaultable and reports EnabledByDefault() == false.
// Enumerated from rule.All() so the suite cannot drift as rules are
// added, removed, or flip their default. Sorted by ID for stable
// subtest / sub-benchmark ordering.
func optInRules() []rule.Rule {
	all := rule.All()
	out := make([]rule.Rule, 0, len(all))
	for _, r := range all {
		if d, ok := r.(rule.Defaultable); ok && !d.EnabledByDefault() {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// perRuleBenchMakeFile returns a factory that builds a fresh lint.File
// per call so per-File memos (LinkReferences, ProseRanges, newline
// offsets) start cold, matching what the engine sees in production
// (one File per Check). The FS and RunCache are wired so cross-file
// rules reach their real work.
func perRuleBenchMakeFile(tb testing.TB, src []byte, mapFS fstest.MapFS) func() *lint.File {
	tb.Helper()
	return func() *lint.File {
		// Always "doc.md": the name the FS maps to src and the name
		// TestPerRuleBenchDocCompliant verifies, so the gate measures the
		// exact file proven diagnostic-free. A filename- or FS-presence-
		// sensitive rule (e.g. directory-structure) then cannot diverge
		// between the compliance guard and the measured baseline.
		f, err := lint.NewFile("doc.md", src)
		require.NoError(tb, err)
		f.FS = mapFS
		f.RootDir = "."
		f.RunCache = lint.NewRunCache()
		return f
	}
}

// perRuleAllocs returns the parse-subtracted allocs/op for r.Check on
// the larger perRuleBenchDoc. Mirrors allocsForRule in
// alloc_budget_test.go (fresh File per iteration, warm once, subtract
// the parse-only baseline) but on the larger doc so the alloc gate and
// the time gate share one representative input.
func perRuleAllocs(tb testing.TB, r rule.Rule, src []byte, mapFS fstest.MapFS) float64 {
	tb.Helper()
	makeFile := perRuleBenchMakeFile(tb, src, mapFS)
	// Warm: prime package-level singletons (regex compile, tokenizer
	// init) the first Check would otherwise charge to the measured
	// frame.
	_ = r.Check(makeFile())

	const runs = 100
	parse := testing.AllocsPerRun(runs, func() {
		_ = makeFile()
	})
	full := testing.AllocsPerRun(runs, func() {
		f := makeFile()
		_ = r.Check(f)
	})
	delta := full - parse
	if delta < 0 {
		delta = 0
	}
	return delta
}

// ruleTiming holds an opt-in rule's parse-normalised Check cost on
// perRuleBenchDoc: the median per-iteration (parse+Check)/parse ratio,
// plus mean parse-only and parse+Check times for log context.
type ruleTiming struct {
	Ratio   float64 // median of per-iteration (parse+Check)/parse ratios
	ParseNs int64   // mean parse-only ns/op, for log context
	FullNs  int64   // mean parse+Check ns/op, for log context
}

// perRuleTiming measures one rule's Check cost as a fraction of the parse
// floor on perRuleBenchDoc: each iteration it times a parse-only op and a
// parse+Check op BACK-TO-BACK and records their ratio; it returns the
// median ratio (plus mean times for logging).
//
// Pairing the two ops per ITERATION is what makes the gate
// runner-independent. They run a few hundred microseconds apart -- well
// inside one scheduler quantum -- so both see the same contention factor
// k (a sibling go-build saturating every core slows both equally) and k
// cancels in that iteration's ratio. The median over many iterations is
// robust to the iterations a GC pause or a context switch lands inside.
//
// Earlier approaches timed the two as SEPARATE 100-op batches and reduced
// them independently across windows (first min, then median). Batches sit
// tens of milliseconds apart -- many scheduler quanta -- so the two
// reductions could reflect different k and the ratio still scaled with
// load: a no-op Check measured 0.80 on a contended CI runner. The goldmark
// parse dwarfs most rules' Check, so an absolute ns ceiling would track
// machine speed too; the ratio removes that factor. The deterministic
// allocs budget stays the tight per-rule algorithmic-regression catcher.
func perRuleTiming(tb testing.TB, r rule.Rule, src []byte, mapFS fstest.MapFS) ruleTiming {
	tb.Helper()
	makeFile := perRuleBenchMakeFile(tb, src, mapFS)
	_ = r.Check(makeFile()) // warm once before measuring
	const iters = 301       // odd: the median is a single iteration's ratio
	ratios := make([]float64, iters)
	var parseSum, fullSum int64
	runtime.GC() // start from a clean heap so fewer GC assists land inside
	for i := range ratios {
		s := time.Now()
		_ = makeFile()
		parseNs := time.Since(s).Nanoseconds()
		if parseNs <= 0 {
			// A ns-resolution clock never times the ~170µs parse at 0, but
			// guard the divisor so a degenerate measurement fails loudly
			// instead of making the ratio +Inf.
			tb.Fatalf("parse op measured %dns; need >0 to form the ratio", parseNs)
		}

		s = time.Now()
		f := makeFile()
		_ = r.Check(f)
		fullNs := time.Since(s).Nanoseconds()

		ratios[i] = float64(fullNs) / float64(parseNs)
		parseSum += parseNs
		fullSum += fullNs
	}
	sort.Float64s(ratios)
	return ruleTiming{
		Ratio:   ratios[iters/2],
		ParseNs: parseSum / int64(iters),
		FullNs:  fullSum / int64(iters),
	}
}

// maxTimeRatio caps every opt-in rule's parse+Check time as a MULTIPLE of
// the parse-only floor, both timed in the SAME iteration (see
// perRuleTiming). One uniform bar fits all rules because the ratio divides
// out machine speed and CI contention: a sibling go-build that saturates
// every core slows the parse op and the parse+Check op together, so the
// ratio reads the same on a quiet box or a saturated batch runner --
// validated by holding ~1.0 for a no-op Check across a 2.6-4.3x swing in
// raw parse time under all-core load. That sustained contention is exactly
// what an absolute ns ceiling could not filter (it only ADDS time, across
// every window), so MDS035/MDS037 once tripped a 1.25ms ceiling and had to
// carry ~8x headroom; the ratio removes the machine-speed factor entirely.
//
// On perRuleBenchDoc (2026-06-02) every rule but one sits at 0.99-1.28x;
// the stable outlier is MDS043 (no-reference-style) at ~1.65x, whose
// reference-style link scan is the most Check work relative to parse -- it
// is CPU, not allocation (MDS043's Check allocates ~10/op). 2.5x clears
// that outlier with ~50% headroom while still binding the cheap rules
// tighter than the old absolute gate (1ms vs their ~175µs ≈ 5.7x) and
// tripping a Check that adds a full extra parse (ratio +~1.0). The
// deterministic perRuleAllocCeiling gate below is the tight per-rule
// algorithmic-regression catch; this is the coarse CPU backstop. If a
// legitimate cost change pushes a rule over the bar, re-measure with
// `go test -run TestPerRuleBenchBudget -v ./internal/integration/` (the
// gate logs every rule's ratio) and raise this, noting why.
const maxTimeRatio = 2.5

// perRuleAllocCeiling pins each opt-in rule's parse-subtracted allocs/op
// ceiling on perRuleBenchDoc. Allocations are CPU-independent, so this
// is the tight, deterministic gate that catches an algorithmic
// regression (extra parse, lost memo, escaped closure); maxTimeRatio
// above is the coarse CPU backstop. Ceiling = baseline + max(20%, 4)
// allocs; the trailing comment records the approximate baseline (4-core
// dev box).
//
// A rule MISSING from this map fails TestPerRuleBenchBudget (the "no
// pinned ceiling" path), so a newly-added opt-in rule must be pinned
// here as part of the change that adds it. Re-measure with `go test -run
// TestPerRuleBenchBudget -v ./internal/integration/` (the gate logs each
// rule's observed allocs/op).
//
// MDS043's ceiling is set from mds043AllocCeiling in init() below. Plan
// 188 removed its second parse, so the arena and upstream build axes now
// allocate identically (16 in both goldmark_arena_test.go and
// goldmark_upstream_test.go); the init() indirection is retained only so
// a future divergence has a home.
var perRuleAllocCeiling = map[string]float64{
	"MDS024": 44,  // paragraph-structure: ~36 allocs
	"MDS029": 30,  // conciseness-scoring: ~24 allocs
	"MDS033": 4,   // directory-structure: 0 allocs
	"MDS034": 4,   // markdown-flavor: 0 allocs
	"MDS035": 102, // toc-directive: ~84 allocs
	"MDS036": 4,   // max-section-length: 0 allocs
	"MDS037": 130, // duplicated-content: ~108 allocs
	"MDS041": 4,   // no-inline-html: 0 allocs
	"MDS042": 4,   // emphasis-style: 0 allocs
	"MDS043": 16,  // no-reference-style: ~10 allocs (plan 188)
	"MDS044": 4,   // horizontal-rule-style: 0 allocs
	"MDS045": 6,   // list-marker-style: ~1 alloc
	"MDS046": 4,   // ordered-list-numbering: 0 allocs
	"MDS047": 4,   // ambiguous-emphasis: 0 allocs
	"MDS048": 4,   // git-hook-sync: 0 allocs
	"MDS049": 6,   // no-space-in-link-text: ~1 alloc
	"MDS050": 4,   // proper-names: 0 allocs
	"MDS051": 6,   // single-h1: ~1 alloc
	"MDS052": 4,   // no-space-in-code-spans: 0 allocs
	"MDS055": 4,   // forbidden-paragraph-starts: 0 allocs
	"MDS056": 4,   // forbidden-text: 0 allocs
	"MDS057": 4,   // required-text-patterns: 0 allocs
	"MDS058": 4,   // required-mentions: 0 allocs
	"MDS063": 44,  // descriptive-link-text: ~36 allocs
	"MDS067": 12,  // callout-type: ~8 allocs
	"MDS068": 4,   // link-style: 0 allocs
}

// init pins MDS043's allocs ceiling from the build-tagged
// mds043AllocCeiling constant (see goldmark_arena_test.go /
// goldmark_upstream_test.go for the per-axis values, and the
// perRuleAllocCeiling note above for why the indirection is kept even
// when the two axes match).
func init() {
	perRuleAllocCeiling["MDS043"] = mds043AllocCeiling
}

// TestPerRuleBenchDocCompliant guards the invariant perRuleBenchDoc
// relies on: no default-enabled rule and no opt-in rule may fire on
// the doc, so the gate measures base scan cost rather than
// per-violation work. If a future rule change makes the doc
// non-compliant, this names the offending rule so the doc can be
// adjusted (or the rule examined). Cheap — runs even under -short.
func TestPerRuleBenchDocCompliant(t *testing.T) {
	src := []byte(perRuleBenchDoc())
	mapFS := perRuleBenchFS(src)
	makeFile := perRuleBenchMakeFile(t, src, mapFS)
	all := rule.All()
	sort.Slice(all, func(i, j int) bool { return all[i].ID() < all[j].ID() })
	for _, r := range all {
		r := r
		t.Run(r.ID()+"_"+r.Name(), func(t *testing.T) {
			ds := r.Check(makeFile())
			if len(ds) != 0 {
				t.Fatalf("%s (%s) fires %d diagnostic(s) on perRuleBenchDoc "+
					"(first: %q at line %d); the per-rule bench doc must stay "+
					"compliant so the gate measures base scan cost, not "+
					"per-violation overhead. Adjust perRuleBenchDoc.",
					r.ID(), r.Name(), len(ds), ds[0].Message, ds[0].Line)
			}
		})
	}
}

// TestPerRuleBenchBudget is the per-opt-in-rule regression gate. For
// each opt-in rule it asserts BOTH a pinned allocs/op ceiling
// (deterministic, perRuleAllocCeiling) and the uniform parse-normalised
// time-ratio ceiling (maxTimeRatio, runner-independent). Each rule is
// its own subtest so a
// failure names the offending rule and the rest of the matrix stays
// visible.
//
// Skipped under -short (the AllocsPerRun loops and the repeated
// timing windows are expensive) and under -race (the race detector
// perturbs both allocation counts and timing) — mirroring
// TestPerRuleAllocBudget.
func TestPerRuleBenchBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("per-rule bench gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("per-rule bench gate skipped under -race; the race " +
			"detector perturbs allocation counts and timing")
	}
	src := []byte(perRuleBenchDoc())
	mapFS := perRuleBenchFS(src)
	for _, r := range optInRules() {
		r := r
		t.Run(r.ID()+"_"+r.Name(), func(t *testing.T) {
			allocCeiling, ok := perRuleAllocCeiling[r.ID()]
			if !ok {
				t.Fatalf("%s (%s) is opt-in but has no pinned alloc ceiling "+
					"in perRuleAllocCeiling. Add an entry: measure its "+
					"baseline with `go test -run TestPerRuleBenchBudget -v "+
					"./internal/integration/` and pin allocs to baseline + "+
					"max(20%%, 4).", r.ID(), r.Name())
			}

			allocs := perRuleAllocs(t, r, src, mapFS)
			if allocs > allocCeiling {
				t.Fatalf("%s (%s) Check allocates %.1f/op, pinned ceiling = "+
					"%.0f. Either fix the regression (lost memo, extra parse, "+
					"escaped closure) or, if the new cost is justified, raise "+
					"this rule's entry in perRuleAllocCeiling and note why.",
					r.ID(), r.Name(), allocs, allocCeiling)
			}

			tm := perRuleTiming(t, r, src, mapFS)
			ratio := tm.Ratio
			// Log the observed numbers so a `-v` run doubles as the
			// re-measurement source when a ceiling needs updating.
			t.Logf("%s (%s): ratio %.2f, %.0f allocs/op (ceilings: ratio "+
				"%.2f, allocs %.0f; mean parse %v, parse+Check %v)",
				r.ID(), r.Name(), ratio, allocs,
				maxTimeRatio, allocCeiling,
				time.Duration(tm.ParseNs), time.Duration(tm.FullNs))
			if ratio > maxTimeRatio {
				t.Fatalf("%s (%s) Check/parse ratio %.2f exceeds the %.2fx "+
					"maxTimeRatio ceiling (mean parse %v, parse+Check %v). A "+
					"real Check-time regression is suspected; the per-iteration "+
					"ratio cancels CI contention, so this is machine-"+
					"independent. If the cost is justified, raise maxTimeRatio "+
					"(noting why); the deterministic allocs gate above is the "+
					"finer catch.",
					r.ID(), r.Name(), ratio, maxTimeRatio,
					time.Duration(tm.ParseNs), time.Duration(tm.FullNs))
			}
		})
	}
}

// noopTimingRule and extraParseRule are synthetic rules used only by
// TestPerRuleTiming to prove perRuleTiming.Ratio responds to Check cost:
// a no-op Check leaves the ratio at the parse floor (~1.0), while a Check
// that does one extra full parse (the canonical "added a parse"
// regression the gate guards against) roughly doubles it (~2.0),
// independent of CPU speed.
type noopTimingRule struct{}

func (noopTimingRule) ID() string                         { return "MDSTEST-NOOP" }
func (noopTimingRule) Name() string                       { return "noop-timing" }
func (noopTimingRule) Category() string                   { return "test" }
func (noopTimingRule) Check(*lint.File) []lint.Diagnostic { return nil }

type extraParseRule struct{}

func (extraParseRule) ID() string       { return "MDSTEST-PARSE" }
func (extraParseRule) Name() string     { return "extra-parse-timing" }
func (extraParseRule) Category() string { return "test" }
func (extraParseRule) Check(f *lint.File) []lint.Diagnostic {
	// One more parse of f.Source via the same canonical parser
	// lint.NewFile uses, so the full parse+Check batch does ~2 parses and
	// the ratio lands near 2.0. ParseContext returns no error, so there is
	// nothing to swallow.
	node := markdown.ParseContext(f.Source, parser.NewContext())
	runtime.KeepAlive(node)
	return nil
}

// TestPerRuleTiming verifies the parse-normalised measurement that
// TestPerRuleBenchBudget gates on: the ratio sits near 1.0 for a no-op
// Check and climbs well past it when Check adds a full parse's worth of
// work. Because each ratio divides out the parse floor timed in the same
// iteration, the assertions hold regardless of how fast or contended the
// host is. Skipped under -short and -race like the gate itself.
func TestPerRuleTiming(t *testing.T) {
	if testing.Short() {
		t.Skip("per-rule timing test skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("per-rule timing test skipped under -race; the race " +
			"detector perturbs timing")
	}
	src := []byte(perRuleBenchDoc())
	mapFS := perRuleBenchFS(src)

	noop := perRuleTiming(t, noopTimingRule{}, src, mapFS)
	require.Positive(t, noop.ParseNs, "parse floor must be measurable")
	require.Positive(t, noop.FullNs, "parse+Check must be measurable")
	// A free Check cannot truly push the ratio below ~1.0; a dip is pure
	// measurement noise (one parse sample beating another), so the lower
	// bound only rules out absurdity. The meaningful checks are that a
	// free Check is NOT inflated toward 2.0 and that busy clearly exceeds
	// it.
	assert.Greater(t, noop.Ratio, 0.8,
		"a no-op Check should leave the ratio near the parse floor (~1.0)")
	assert.Less(t, noop.Ratio, 1.3,
		"a no-op Check should not inflate the ratio")

	busy := perRuleTiming(t, extraParseRule{}, src, mapFS)
	assert.Greater(t, busy.Ratio, 1.5,
		"a Check doing one extra full parse should ~double the ratio")
	assert.Greater(t, busy.Ratio, noop.Ratio,
		"the ratio must respond to Check cost, not just parse")
	t.Logf("noop ratio %.2f (mean parse %dns, full %dns); extra-parse "+
		"ratio %.2f (mean parse %dns, full %dns)",
		noop.Ratio, noop.ParseNs, noop.FullNs,
		busy.Ratio, busy.ParseNs, busy.FullNs)
}

// BenchmarkOptInRule reports each opt-in rule's isolated total
// parse+Check ns/op and allocs/op as its own sub-benchmark, so
// `go test -run x -bench BenchmarkOptInRule ./internal/integration/`
// lists every opt-in rule's time and allocation cost. The sub-bench
// name is `<ID>_<name>` to match TestPerRuleBenchBudget's subtests.
//
// Each iteration rebuilds a fresh lint.File so per-File memos start
// cold (the production one-File-per-Check shape); the warm Check
// before ResetTimer keeps package-level first-touch cost out of the
// measured frame.
func BenchmarkOptInRule(b *testing.B) {
	if testing.Short() {
		b.Skip("benchmark skipped in -short mode")
	}
	src := []byte(perRuleBenchDoc())
	mapFS := perRuleBenchFS(src)
	for _, r := range optInRules() {
		r := r
		b.Run(r.ID()+"_"+r.Name(), func(b *testing.B) {
			makeFile := perRuleBenchMakeFile(b, src, mapFS)
			_ = r.Check(makeFile())
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				f := makeFile()
				_ = r.Check(f)
			}
		})
	}
}
