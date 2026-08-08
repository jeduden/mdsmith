package discovery

import "testing"

// matchesAnyBudgetNs pins the gate's real effect: doublestar.Match
// revalidates its pattern on every call even though Discover already
// validated every pattern once via validatePatterns before the walk
// started. CI's root `test` job (the only job that runs
// internal/discovery's tests — see .github/workflows/ci.yml) runs
// under -covermode=atomic with neither -short nor -race, so the
// budget must hold there, not just under a plain `go test`. Measured
// locally with testing.Benchmark's default (adaptive-N, ~1s) run,
// stable across repeated runs: MatchUnvalidated ~155 ns/op plain,
// ~223 ns/op under -covermode=atomic; doublestar.Match ~270 ns/op
// plain, ~300-330 ns/op under -covermode=atomic. The budget sits
// between the coverage-instrumented good (~223) and bad (~300+)
// numbers, with headroom on both sides.
const matchesAnyBudgetNs = 260

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
