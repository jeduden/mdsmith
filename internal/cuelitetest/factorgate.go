package cuelitetest

import (
	"fmt"
	"math"
	"strings"
)

// Budgets bound the cuelite/cue ns-per-op ratio each benchmark arm may
// reach before the factor gate fails. They are RATIOS, not absolute
// times, so they cancel machine speed: both arms run on the same runner
// over the same workload, exactly as internal/release/benchcheck gates
// the hyperfine tool/baseline ratio. A ratio gate catches a real cuelite
// blowup while a slower CI runner moves both arms together and does not
// trip it.
//
// These are the POST-FLIP budgets. Plan 238 flipped the schema/query
// surfaces onto the in-house engine, and the interim CUE-backed-façade
// budgets (a looser hot 2.5x, cold 2.0x leaving room for the façade's
// honest overhead) were tightened HERE, at the flip, to <= 1.0x — the
// in-house path must never be slower than the CUE oracle it replaces. This
// is plan 218's "the schema validate path does not regress" acceptance
// criterion made enforceable, and plans 236/240's stated intent realized at
// the 238 flip rather than deferred. The measured factors are well under
// budget (hot ~0.26x, cold ~0.34x; see bench_test.go and the armed gate),
// so the gate now asserts the in-house engine is at least as fast as CUE,
// with the budget headroom absorbing runner noise.
const (
	// HotFactorBudget bounds BenchmarkValidate (the compile-schema-once,
	// validate-many hot path). The in-house engine compiles the schema once
	// and validates a context-free data Value each iteration with no rebuild
	// and no per-iteration context growth, so the cost is flat and the
	// measured factor sits near 0.26x. The 1.0x budget asserts the in-house
	// arm never regresses past the CUE oracle; the min/min noise-robust
	// timing (see factorGateReps) keeps a quiet-runner factor of ~0.26x
	// comfortably inside it.
	HotFactorBudget = 1.0

	// ColdFactorBudget bounds BenchmarkCompileValidate (compile schema,
	// compile data, unify, validate every iteration). The in-house engine
	// pays no cross-context rebuild on this path either; the measured factor
	// is a stable ~0.34x. The 1.0x budget holds the in-house arm to no slower
	// than CUE, with the minimum-of-reps timing absorbing runner noise.
	ColdFactorBudget = 1.0
)

// factorResult is one benchmark arm's measured ratio against its budget:
// the two ns-per-op measurements, the cuelite/cue factor between them,
// the budget it was held to, and whether it passed.
type factorResult struct {
	name      string
	cueliteNs float64
	cueNs     float64
	factor    float64
	budget    float64
	pass      bool
}

// evalFactor builds a factorResult from one arm's two measurements and
// its budget. The factor is cuelite/cue; a non-positive cue measurement
// yields a +Inf factor that always fails the budget, so a mis-measured
// (zero-time) oracle arm trips the gate loudly instead of reading as an
// infinitely fast baseline that any cuelite time would "beat".
func evalFactor(name string, cueliteNs, cueNs, budget float64) factorResult {
	factor := computeFactor(cueliteNs, cueNs)
	return factorResult{
		name:      name,
		cueliteNs: cueliteNs,
		cueNs:     cueNs,
		factor:    factor,
		budget:    budget,
		pass:      factor <= budget,
	}
}

// computeFactor returns the cuelite/cue ns-per-op ratio. A non-positive
// cue baseline returns +Inf rather than dividing by zero, so the caller's
// budget comparison fails closed on a degenerate measurement.
func computeFactor(cueliteNs, cueNs float64) float64 {
	if cueNs <= 0 {
		return math.Inf(1)
	}
	return cueliteNs / cueNs
}

// logLine renders a factorResult as one greppable line for the test log,
// e.g. "cuelite/cue validate factor: 1.43 (budget 2.50) PASS". It names
// the arm, the measured factor, and the budget so a CI log reader sees
// the verdict without opening a profile.
func (r factorResult) logLine() string {
	return fmt.Sprintf("cuelite/cue %s factor: %.2f (budget %.2f) %s",
		r.name, r.factor, r.budget, verdict(r.pass))
}

// verdict maps a pass bool to the PASS/FAIL token shared by the log line
// and the summary table, so both surfaces print one vocabulary.
func verdict(pass bool) string {
	if pass {
		return "PASS"
	}
	return "FAIL"
}

// renderSummary builds the GitHub-step-summary markdown table: one row
// per arm with its two ns-per-op measurements, the factor, the budget,
// and the verdict, so the gate's outcome is visible on the Actions run
// page without opening the job log. It returns a trailing newline so it
// appends cleanly to an existing summary file.
func renderSummary(results []factorResult) string {
	var b strings.Builder
	b.WriteString("### cuelite/cue factor gate\n\n")
	b.WriteString("| benchmark | cuelite ns/op | cue ns/op | factor | budget | result |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: | --- |\n")
	for _, r := range results {
		fmt.Fprintf(&b, "| %s | %.0f | %.0f | %.2f | %.2f | %s |\n",
			r.name, r.cueliteNs, r.cueNs, r.factor, r.budget, verdict(r.pass))
	}
	return b.String()
}
