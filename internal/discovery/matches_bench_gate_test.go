package discovery

import "testing"

// matchesAnyBudgetNs pins the gate's real effect: doublestar.Match
// revalidates its pattern on every call even though Discover already
// validated every pattern once via validatePatterns before the walk
// started. Measured locally: doublestar.Match ~270 ns/op,
// MatchUnvalidated ~155 ns/op on matchesAnyBenchPatterns (6 patterns,
// no match). The budget sits between the two so a regression back to
// Match fails the gate while leaving headroom for machine noise.
const matchesAnyBudgetNs = 220

// TestMatchesAny_NoRevalidationBudget pins the ns/op regression gate
// under a normal `go test` run, mirroring
// noreferencestyle.TestCheckFootnotes_NoNeedleBudget's rationale: CI's
// -bench jobs don't cover this package, so testing.Benchmark runs the
// benchmark programmatically and lands the assertion in a Test that
// `go test ./...` actually runs.
func TestMatchesAny_NoRevalidationBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("perf gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("perf gate skipped under -race; the race detector's " +
			"instrumentation overhead perturbs the ns/op measurement")
	}
	result := testing.Benchmark(BenchmarkMatchesAny)
	perOp := float64(result.NsPerOp())
	t.Logf("matchesAny (no match, 6 patterns) = %.0f ns/op (budget = %d)",
		perOp, matchesAnyBudgetNs)
	if perOp > matchesAnyBudgetNs {
		t.Fatalf("matchesAny: %.0f ns/op, budget = %d; MatchUnvalidated may "+
			"have regressed back to Match's per-call pattern revalidation",
			perOp, matchesAnyBudgetNs)
	}
}
