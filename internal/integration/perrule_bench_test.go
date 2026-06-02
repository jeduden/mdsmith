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
	"github.com/stretchr/testify/require"
)

// perrule_bench_test.go is plan 215's per-opt-in-rule regression
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
//     a baseline (allocs/op AND total parse+Check ns/op) measured on a
//     larger representative doc, so a regression in one opt-in rule's
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

// perRuleCheckNsPerOp returns a CI-stable total parse+Check ns/op for
// one rule: the MINIMUM per-op wall time over several fixed-size
// repetitions, not the average testing.Benchmark reports. The minimum
// is the right gate quantity on a shared CI runner — a noisy
// neighbour, scheduler preemption, or a GC pause only ever ADDS time,
// so the least-contended repetition is the closest estimate of the
// rule's true cost and a transient spike cannot inflate it. Gating the
// average flaked the gate: MDS035 measured 1.33ms on a contended CI
// runner against a ~0.2ms uncontended cost (it clocks ~0.18ms locally).
//
// The gate times parse+Check TOGETHER rather than the parse-subtracted
// Check delta on purpose. On this corpus the goldmark parse (~170µs)
// dwarfs most rules' Check (often < 30µs), so a subtracted Check time
// is dominated by parse jitter and routinely goes negative run to run
// — useless as a gate quantity. Parse cost is constant across every
// opt-in rule (same doc, same factory), so a Check regression still
// pushes the COMBINED number past the rule's pinned ceiling, while the
// constant parse floor keeps the measurement stable.
func perRuleCheckNsPerOp(tb testing.TB, r rule.Rule, src []byte, mapFS fstest.MapFS) int64 {
	tb.Helper()
	makeFile := perRuleBenchMakeFile(tb, src, mapFS)
	_ = r.Check(makeFile()) // warm once before measuring
	// Each repetition times a fixed batch of parse+Check ops. iters
	// keeps a batch long enough (~tens of ms) to amortize per-call
	// setup and stay well above timer resolution; repeats gives
	// several short, independent windows so at least one lands in a
	// quiet scheduling slot even on a contended runner. Return the
	// minimum ns/op across the windows.
	//
	// Min, not the p95 the sibling latency gates (BenchmarkLatency*,
	// BenchmarkCheckCorpus*) use: those characterise a latency
	// DISTRIBUTION, whereas this gate estimates one rule's fixed cost,
	// where CI contention is additive noise to strip out — for which
	// the least-contended window is the right estimator and a p95 would
	// just track the worst window and re-flake. runtime.GC() before
	// each window starts it from a clean heap so a GC assist is far
	// less likely to fall inside the timed region (the high-alloc
	// rules — MDS043/037/035 — need this most).
	const (
		iters   = 100
		repeats = 12
	)
	var best int64
	for i := 0; i < repeats; i++ {
		runtime.GC()
		start := time.Now()
		for j := 0; j < iters; j++ {
			f := makeFile()
			_ = r.Check(f)
		}
		nsPerOp := time.Since(start).Nanoseconds() / iters
		if i == 0 || nsPerOp < best {
			best = nsPerOp
		}
	}
	return best
}

// perRuleBudget pairs the two hard per-rule gates: total parse+Check
// wall time and parse-subtracted allocs/op. Bundled so a new opt-in
// rule's entry cannot forget either limit.
type perRuleBudget struct {
	Time   time.Duration
	Allocs float64
}

// perRuleBenchBudget pins each opt-in rule's ceiling. The trailing
// comment on every row records the approximate baseline the ceiling
// was derived from (4-core dev box, 2026-05-29, total parse+Check on
// perRuleBenchDoc, measured as an average; perRuleCheckNsPerOp now
// gates the per-window minimum, which runs a touch lower).
//
// Headroom philosophy (mirrors BenchmarkCheckCorpus*'s ~15-20% alloc /
// ~3-5x time sizing):
//
//   - Time ceiling ≈ 5x the measured baseline. perRuleCheckNsPerOp
//     gates the MINIMUM ns/op over repetitions, not the average, so a
//     slower runner and ordinary CI jitter stay well under the
//     ceiling, while a real Check-time regression (an added per-line
//     pass, a lost early-exit) still trips it. The parse floor is
//     constant, so even a cheap rule's regression shows. Floored at
//     1ms (MDS043 now sits at the floor — plan 188 removed its second
//     parse, so it parses once like every other rule). MDS035 and
//     MDS037 carry 2ms (≈8x): the
//     minimum filters TRANSIENT spikes, but a burst spanning every
//     window — go test ./... runs package binaries in parallel, so a
//     sibling's go-build can saturate all cores for the whole ~0.2s
//     measurement — the minimum cannot filter, and their higher base
//     once pushed the gate over a 1.25ms ceiling, so they get headroom.
//   - Allocs ceiling = baseline + max(20%, 4) allocs, deterministic.
//     Allocations are CPU-independent, so this is the tight gate that
//     catches an algorithmic regression (extra parse, lost memo,
//     escaped closure) the wall-time budget would have to budge for.
//
// Updating an entry: when a legitimate cost change lands (a rule
// gains a feature that adds real work), re-measure with
// `go test -run TestPerRuleBenchBudget -v ./internal/integration/`
// (the gate logs each rule's observed ns/op + allocs/op) or
// `go test -run x -bench BenchmarkOptInRule ./internal/integration/`,
// then raise that one row to the new baseline + the headroom above
// and note why in the trailing comment. A rule MISSING from this map
// fails TestPerRuleBenchBudget (see the "no pinned budget" subtest),
// so a newly-added opt-in rule must be pinned here as part of the
// change that adds it.
// MDS043's Allocs ceiling is set from mds043AllocCeiling in init()
// below. Plan 188 removed its second parse, so the arena and upstream
// build axes now allocate identically (16 in both goldmark_arena_test.go
// and goldmark_upstream_test.go); the per-axis indirection is retained
// only so a future divergence has a home. The map literal carries the
// same value so the column stays numeric.
var perRuleBenchBudget = map[string]perRuleBudget{
	"MDS024": {Time: 1000 * time.Microsecond, Allocs: 44},  // paragraph-structure: base ~192us / 36 allocs
	"MDS029": {Time: 1000 * time.Microsecond, Allocs: 30},  // conciseness-scoring: base ~178us / 24 allocs
	"MDS033": {Time: 1000 * time.Microsecond, Allocs: 4},   // directory-structure: base ~166us / 0 allocs
	"MDS034": {Time: 1000 * time.Microsecond, Allocs: 4},   // markdown-flavor: base ~197us / 0 allocs
	"MDS035": {Time: 2000 * time.Microsecond, Allocs: 102}, // toc-directive: base ~228us / 84 allocs
	"MDS036": {Time: 1000 * time.Microsecond, Allocs: 4},   // max-section-length: base ~193us / 0 allocs
	"MDS037": {Time: 2000 * time.Microsecond, Allocs: 130}, // duplicated-content: base ~241us / 108 allocs
	"MDS041": {Time: 1000 * time.Microsecond, Allocs: 4},   // no-inline-html: base ~185us / 0 allocs
	"MDS042": {Time: 1000 * time.Microsecond, Allocs: 4},   // emphasis-style: base ~176us / 0 allocs
	"MDS043": {Time: 1500 * time.Microsecond, Allocs: 16},  // no-reference-style: base ~215us / 10 allocs (plan 188)
	"MDS044": {Time: 1000 * time.Microsecond, Allocs: 4},   // horizontal-rule-style: base ~174us / 0 allocs
	"MDS045": {Time: 1000 * time.Microsecond, Allocs: 6},   // list-marker-style: base ~184us / 1 alloc
	"MDS046": {Time: 1000 * time.Microsecond, Allocs: 4},   // ordered-list-numbering: base ~175us / 0 allocs
	"MDS047": {Time: 1000 * time.Microsecond, Allocs: 4},   // ambiguous-emphasis: base ~165us / 0 allocs
	"MDS048": {Time: 1000 * time.Microsecond, Allocs: 4},   // git-hook-sync: base ~172us / 0 allocs
	"MDS049": {Time: 1000 * time.Microsecond, Allocs: 6},   // no-space-in-link-text: base ~183us / 1 alloc
	"MDS050": {Time: 1000 * time.Microsecond, Allocs: 4},   // proper-names: base ~165us / 0 allocs
	"MDS051": {Time: 1000 * time.Microsecond, Allocs: 6},   // single-h1: base ~176us / 1 alloc
	"MDS052": {Time: 1000 * time.Microsecond, Allocs: 4},   // no-space-in-code-spans: base ~177us / 0 allocs
	"MDS055": {Time: 1000 * time.Microsecond, Allocs: 4},   // forbidden-paragraph-starts: base ~179us / 0 allocs
	"MDS056": {Time: 1000 * time.Microsecond, Allocs: 4},   // forbidden-text: base ~174us / 0 allocs
	"MDS057": {Time: 1000 * time.Microsecond, Allocs: 4},   // required-text-patterns: base ~171us / 0 allocs
	"MDS058": {Time: 1000 * time.Microsecond, Allocs: 4},   // required-mentions: base ~172us / 0 allocs
	"MDS063": {Time: 1000 * time.Microsecond, Allocs: 44},  // descriptive-link-text: base ~179us / 36 allocs
	"MDS067": {Time: 1000 * time.Microsecond, Allocs: 12},  // callout-type: base ~182us / 8 allocs
	"MDS068": {Time: 1000 * time.Microsecond, Allocs: 4},   // link-style: base ~172us / 0 allocs
}

// init pins MDS043's allocs ceiling to the active goldmark build axis.
// mds043AllocCeiling is 384 on the default arena build and 784 under
// -tags goldmark_upstream (see goldmark_arena_test.go); the map literal
// above carries the arena value, so this only differs on the non-arena
// axis.
func init() {
	b := perRuleBenchBudget["MDS043"]
	b.Allocs = mds043AllocCeiling
	perRuleBenchBudget["MDS043"] = b
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
// (deterministic) and a pinned total parse+Check ns/op ceiling
// (generous headroom for CI jitter). Each rule is its own subtest so a
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
			budget, ok := perRuleBenchBudget[r.ID()]
			if !ok {
				t.Fatalf("%s (%s) is opt-in but has no pinned budget in "+
					"perRuleBenchBudget. Add an entry: measure its baseline "+
					"with `go test -run TestPerRuleBenchBudget -v "+
					"./internal/integration/` and pin Time ≈ 5x and Allocs ≈ "+
					"baseline + max(20%%, 4).", r.ID(), r.Name())
			}

			allocs := perRuleAllocs(t, r, src, mapFS)
			if allocs > budget.Allocs {
				t.Fatalf("%s (%s) Check allocates %.1f/op, pinned ceiling = "+
					"%.0f. Either fix the regression (lost memo, extra parse, "+
					"escaped closure) or, if the new cost is justified, raise "+
					"this rule's Allocs entry in perRuleBenchBudget and note "+
					"why.", r.ID(), r.Name(), allocs, budget.Allocs)
			}

			ns := perRuleCheckNsPerOp(t, r, src, mapFS)
			got := time.Duration(ns) * time.Nanosecond
			// Log the observed numbers so a `-v` run doubles as the
			// re-measurement source when an entry needs updating.
			t.Logf("%s (%s): %v parse+Check, %.0f allocs/op "+
				"(ceilings: %v, %.0f)",
				r.ID(), r.Name(), got, allocs, budget.Time, budget.Allocs)
			if got > budget.Time {
				t.Fatalf("%s (%s) total parse+Check %v exceeds pinned ceiling "+
					"%v. A real Check-time regression is "+
					"suspected; the constant parse floor means a cheap rule's "+
					"regression still shows here. If the cost is justified, "+
					"raise this rule's Time entry in perRuleBenchBudget.",
					r.ID(), r.Name(), got, budget.Time)
			}
		})
	}
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
