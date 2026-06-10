package cuelitetest

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComputeFactor pins the ratio arithmetic and the fail-closed
// division guard: a positive baseline divides, a non-positive baseline
// returns +Inf so the budget comparison fails on a degenerate oracle
// measurement rather than reading it as infinitely fast.
func TestComputeFactor(t *testing.T) {
	assert.InDelta(t, 1.5, computeFactor(150, 100), 1e-9)
	assert.True(t, math.IsInf(computeFactor(150, 0), 1), "zero baseline must be +Inf")
	assert.True(t, math.IsInf(computeFactor(150, -5), 1), "negative baseline must be +Inf")
}

// TestEvalFactor checks the budget comparison at and across the boundary:
// a factor at the budget passes (<=), just over fails, and a zero-time
// baseline produces an +Inf factor that fails.
func TestEvalFactor(t *testing.T) {
	under := evalFactor("validate", 140, 100, 2.0)
	require.InDelta(t, 1.4, under.factor, 1e-9)
	assert.True(t, under.pass)

	atBudget := evalFactor("validate", 200, 100, 2.0)
	require.InDelta(t, 2.0, atBudget.factor, 1e-9)
	assert.True(t, atBudget.pass, "factor exactly at budget must pass")

	over := evalFactor("validate", 201, 100, 2.0)
	assert.False(t, over.pass, "factor over budget must fail")

	degenerate := evalFactor("validate", 100, 0, 2.0)
	assert.False(t, degenerate.pass, "zero baseline must fail closed")
}

// TestVerdict covers both verdict tokens so neither branch is left
// uncovered by the higher-level renderers.
func TestVerdict(t *testing.T) {
	assert.Equal(t, "PASS", verdict(true))
	assert.Equal(t, "FAIL", verdict(false))
}

// TestLogLine pins the greppable one-line format for both a pass and a
// fail, so a CI log reader (or a grep in this report) finds the factor
// and verdict on one line.
func TestLogLine(t *testing.T) {
	pass := evalFactor("validate", 143, 100, 2.0).logLine()
	assert.Equal(t, "cuelite/cue validate factor: 1.43 (budget 2.00) PASS", pass)

	fail := evalFactor("compile-validate", 300, 100, 2.0).logLine()
	assert.Equal(t, "cuelite/cue compile-validate factor: 3.00 (budget 2.00) FAIL", fail)
}

// TestRenderSummary pins the markdown table the gate appends to
// GITHUB_STEP_SUMMARY: a header, the column row, the alignment row, and
// one data row per result carrying both measurements, the factor, the
// budget, and the verdict.
func TestRenderSummary(t *testing.T) {
	out := renderSummary([]factorResult{
		evalFactor("validate", 7000, 3500, 2.5),
		evalFactor("compile-validate", 90000, 60000, 2.0),
	})
	want := "### cuelite/cue factor gate\n\n" +
		"| benchmark | cuelite ns/op | cue ns/op | factor | budget | result |\n" +
		"| --- | ---: | ---: | ---: | ---: | --- |\n" +
		"| validate | 7000 | 3500 | 2.00 | 2.50 | PASS |\n" +
		"| compile-validate | 90000 | 60000 | 1.50 | 2.00 | PASS |\n"
	assert.Equal(t, want, out)
}

// TestBudgetsAreInterim guards the documented relationship between the
// two interim budgets: the hot path tolerates the N-dependent CUE
// context growth, so its budget must stay looser than the flat cold
// path's. If a future edit inverts them, this fails and forces the doc
// comment to be revisited.
func TestBudgetsAreInterim(t *testing.T) {
	assert.Greater(t, HotFactorBudget, ColdFactorBudget,
		"hot budget must stay looser than cold while CUE-backed")
}
