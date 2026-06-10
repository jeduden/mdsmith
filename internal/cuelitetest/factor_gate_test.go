package cuelitetest

import (
	"os"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/jeduden/mdsmith/cue/cuelite"
)

// factorGateIters is the fixed per-arm loop count the gate times each
// repetition over. It is held CONSTANT (not auto-scaled like
// testing.Benchmark) on purpose: the CUE-backed cuelite Validate arm
// accumulates one compiled document in its long-lived schema context per
// iteration, so a larger loop inflates the cuelite arm against the oracle
// and the measured factor would depend on the loop length. A small fixed
// count keeps the ratio stable and the gate fast.
const factorGateIters = 30

// factorGateReps is how many times each arm is timed; the gate takes the
// MINIMUM ns/op across them. Minimum is the standard noise-robust choice
// for a ratio gate: the fastest run is the one least perturbed by a GC
// pause, a scheduler preemption, or a neighbour process on a shared CI
// runner, so min/min cancels machine noise the way the ratio cancels
// machine speed.
const factorGateReps = 5

// armTimer times one benchmark arm: it runs n iterations of body and
// returns the elapsed nanoseconds per iteration. body closes over its
// own compiled state so the schema-compile cost is hoisted out of the
// hot-path arms exactly as bench_test.go hoists it.
type armTimer func(n int) float64

// minNsPerOp runs t once as a discarded warmup (so JIT-cold allocation
// and first-touch GC do not skew the first timed rep) then returns the
// minimum ns-per-op across factorGateReps timed runs.
func minNsPerOp(t armTimer) float64 {
	t(factorGateIters) // warmup, discarded
	best := t(factorGateIters)
	for i := 1; i < factorGateReps; i++ {
		if ns := t(factorGateIters); ns < best {
			best = ns
		}
	}
	return best
}

// timeLoop runs body n times and returns ns-per-op, the shared timing
// core every arm uses.
func timeLoop(n int, body func()) float64 {
	start := time.Now()
	for i := 0; i < n; i++ {
		body()
	}
	return float64(time.Since(start).Nanoseconds()) / float64(n)
}

// validateLiteArm times the cuelite hot path: schema compiled once, data
// compiled and validated each iteration — mirroring BenchmarkValidate's
// cuelite arm.
func validateLiteArm(c Case) armTimer {
	return func(n int) float64 {
		schema, err := cuelite.Compile(c.Schema)
		if err != nil {
			panic(err)
		}
		data := []byte(c.Data)
		return timeLoop(n, func() {
			d, _ := cuelite.CompileJSON(data)
			_ = schema.Unify(d).Validate()
		})
	}
}

// validateCueArm times the oracle hot path, symmetric to validateLiteArm.
func validateCueArm(c Case) armTimer {
	return func(n int) float64 {
		ctx := cuecontext.New()
		schema := ctx.CompileString(c.Schema)
		data := []byte(c.Data)
		return timeLoop(n, func() {
			d, _ := oracleData(ctx, data)
			_ = schema.Unify(d).Validate(cue.Concrete(true))
		})
	}
}

// compileValidateLiteArm times the cuelite cold path: compile schema,
// compile data, unify, validate every iteration — BenchmarkCompileValidate's
// cuelite arm, run through the harness path so the two stay in lockstep.
func compileValidateLiteArm(c Case) armTimer {
	return func(n int) float64 {
		return timeLoop(n, func() { _ = CueLitePath(c) })
	}
}

// compileValidateCueArm times the oracle cold path, symmetric to
// compileValidateLiteArm.
func compileValidateCueArm(c Case) armTimer {
	return func(n int) float64 {
		return timeLoop(n, func() { _ = OraclePath(c) })
	}
}

// measureArm returns the budgeted result for one named arm from its
// cuelite and cue timers.
func measureArm(name string, lite, cue armTimer, budget float64) factorResult {
	return evalFactor(name, minNsPerOp(lite), minNsPerOp(cue), budget)
}

// TestFactorGate is the CI factor gate: it measures the cuelite/cue
// ns-per-op ratio for the hot (validate) and cold (compile-validate)
// paths and fails when either exceeds its interim budget. The ratio is
// only meaningful on a quiet runner: inside the parallel `go test ./...`
// job the packages contend for CPU and the allocation-heavier cuelite
// arm degrades more than the oracle arm, inflating the factor (observed
// 3.46x under contention vs ~2.0x quiet). So the gate runs only when
// CUELITE_FACTOR_GATE=1, which the dedicated cuelite-bench CI job sets;
// everywhere else it skips. It logs one greppable line per arm and, when
// GITHUB_STEP_SUMMARY is set, appends a markdown table so the factors
// show on the Actions run page.
func TestFactorGate(t *testing.T) {
	if os.Getenv("CUELITE_FACTOR_GATE") != "1" {
		t.Skip("factor gate needs a quiet runner; set CUELITE_FACTOR_GATE=1 (the cuelite-bench CI job does)")
	}
	if testing.Short() {
		t.Skip("factor gate measures benchmarks; skipped under -short")
	}
	c := benchCase()
	results := []factorResult{
		measureArm("validate", validateLiteArm(c), validateCueArm(c), HotFactorBudget),
		measureArm("compile-validate", compileValidateLiteArm(c), compileValidateCueArm(c), ColdFactorBudget),
	}
	for _, r := range results {
		t.Log(r.logLine())
	}
	writeStepSummary(t, renderSummary(results))
	for _, r := range results {
		if !r.pass {
			t.Errorf("%s", r.logLine())
		}
	}
}

// writeStepSummary appends summary to the file named by
// GITHUB_STEP_SUMMARY when that env var is set, so the factor table lands
// on the Actions run page. A missing var (local runs) is a no-op; a
// write error fails the test so a broken summary surfaces rather than
// silently dropping the gate's only at-a-glance output.
func writeStepSummary(t *testing.T, summary string) {
	t.Helper()
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("open GITHUB_STEP_SUMMARY: %v", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			t.Fatalf("close GITHUB_STEP_SUMMARY: %v", cerr)
		}
	}()
	if _, err := f.WriteString(summary); err != nil {
		t.Fatalf("write GITHUB_STEP_SUMMARY: %v", err)
	}
}
