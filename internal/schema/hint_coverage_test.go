package schema

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRenderHint_NonStringActualDisjunction exercises the
// `actual` non-string branch in hintForStringDisjunction: a
// numeric actual against a string-disjunction constraint
// produces no hint (the disjunction path only fires for
// string actuals).
func TestRenderHint_NonStringActualDisjunction(t *testing.T) {
	expr := `"a" | "b"`
	assert.Empty(t, RenderHint(expr, 42))
	assert.Empty(t, RenderHint(expr, float64(3.14)))
}

// TestRenderHint_SinglePartDisjunction covers the "not a
// disjunction" early return (len(parts) < 2): a bare `"x"`
// constraint cannot produce a "did you mean" hint.
func TestRenderHint_SinglePartDisjunction(t *testing.T) {
	assert.Empty(t, RenderHint(`"x"`, "y"))
}

// TestRenderHint_NonQuotedAlternative covers the
// isQuotedString=false branch: when one of the disjunction
// alternatives isn't a string literal (e.g. `int | "x"`), the
// extractor backs off rather than misreporting.
func TestRenderHint_NonQuotedAlternative(t *testing.T) {
	assert.Empty(t, RenderHint(`int | "x"`, "y"))
}

// TestRenderHint_UnquoteFailure covers the strconv.Unquote
// error branch: a malformed escape inside a string literal
// prevents the literal from being decoded, so no hint fires.
func TestRenderHint_UnquoteFailure(t *testing.T) {
	// Two alternatives so len(parts) >= 2; the bad escape
	// breaks Unquote on the second literal.
	assert.Empty(t, RenderHint(`"ok" | "\xff"`, "ok-typo"))
}

// TestRenderHint_ExactMatchSkipped covers the d==0 branch:
// the actual exactly equals one of the literals, so no hint
// fires (a "did you mean X?" message when X equals the actual
// would be confusing).
func TestRenderHint_ExactMatchSkipped(t *testing.T) {
	// The CUE constraint validator never calls RenderHint on
	// a value that satisfies the constraint, but the
	// disjunction extractor still guards against the case
	// defensively. Pass an exact match to exercise the
	// guard.
	assert.Empty(t, RenderHint(`"foo" | "bar"`, "foo"))
}

// TestRenderHint_IntRangeNonNumericActual exercises the
// toFloat64=false branch: a string actual against an int
// range produces no hint.
func TestRenderHint_IntRangeNonNumericActual(t *testing.T) {
	assert.Empty(t, RenderHint("int & >=1 & <=5", "not-a-number"))
}

// TestRenderHint_IntRangeWithIntActual exercises the int /
// int64 paths of toFloat64 that the float64 default missed.
func TestRenderHint_IntRangeWithIntActual(t *testing.T) {
	assert.Equal(t, "try 1", RenderHint("int & >=1 & <=5", int(0)))
	assert.Equal(t, "try 5", RenderHint("int & >=1 & <=5", int64(6)))
}

// TestRenderHint_IntRangeNoBoundCrossed covers the no-hint
// branch when neither bound is exceeded but the actual is
// still in range (in-range actuals already exit before hint
// rendering, but the guard inside hintForIntRange covers
// boundary-equal cases).
func TestRenderHint_IntRangeNoBoundCrossed(t *testing.T) {
	// Value equal to a bound — within range, no hint.
	assert.Empty(t, RenderHint("int & >=1 & <=5", float64(1)))
	assert.Empty(t, RenderHint("int & >=1 & <=5", float64(5)))
}

// TestParseRenderedBounds_Edges exercises the error branches
// in parseRenderedBounds. These fire when the rendered string
// is malformed; in practice renderIntRange produces a
// well-formed string, so hintForIntRange short-circuits on
// the renderIntRange ok=false path. Direct calls cover the
// defensive branches for future callers.
func TestParseRenderedBounds_Edges(t *testing.T) {
	t.Run("malformed between drops the and separator", func(t *testing.T) {
		_, _, hasLo, hasHi := parseRenderedBounds("int between 1 5")
		assert.False(t, hasLo)
		assert.False(t, hasHi)
	})

	t.Run("non-integer lower bound", func(t *testing.T) {
		_, _, hasLo, _ := parseRenderedBounds("int between abc and 5")
		assert.False(t, hasLo)
	})

	t.Run("non-integer upper bound", func(t *testing.T) {
		_, _, _, hasHi := parseRenderedBounds("int between 1 and zzz")
		assert.False(t, hasHi)
	})

	t.Run("non-integer half-open lower", func(t *testing.T) {
		_, _, hasLo, _ := parseRenderedBounds("int >= xyz")
		assert.False(t, hasLo)
	})

	t.Run("non-integer half-open upper", func(t *testing.T) {
		_, _, _, hasHi := parseRenderedBounds("int <= xyz")
		assert.False(t, hasHi)
	})

	t.Run("unknown rendered form", func(t *testing.T) {
		_, _, hasLo, hasHi := parseRenderedBounds("string")
		assert.False(t, hasLo)
		assert.False(t, hasHi)
	})
}

// TestLevenshtein_InlineGuardKicksIn exercises the
// CodeQL-visible inline guard path inside levenshtein() that
// the over-cap helper would normally short-circuit before.
// The test calls levenshtein directly with strings sized at
// the cap to confirm the inner branch path is reached.
func TestLevenshtein_InlineGuardKicksIn(t *testing.T) {
	// Inputs at maxLevInput rune count stay inside the DP
	// branch; smaller of the two governs the row size.
	short := strings.Repeat("a", maxLevInput)
	tiny := "abc"
	// short ↔ tiny: 1024 - 3 = 1021 deletions, 0 substitutions
	// from the 3-char overlap, so distance ~ 1021.
	got := levenshtein(short, tiny)
	assert.Greater(t, got, 1000)
}

// TestLevenshtein_OneEmpty exercises the early-return branches
// for empty operands that the typo tests don't hit.
func TestLevenshtein_OneEmpty(t *testing.T) {
	assert.Equal(t, 4, levenshtein("", "abcd"))
	assert.Equal(t, 4, levenshtein("abcd", ""))
}

// TestLevenshtein_BothOverCapReturnsLonger exercises both
// branches of the over-cap fallback: when b is longer than a,
// the helper returns len(b)'s capped count; when a is
// longer, it returns len(a). The first call below covers
// the `return cb` branch the typo tests miss.
func TestLevenshtein_BothOverCapReturnsLonger(t *testing.T) {
	short := strings.Repeat("a", maxLevInput+5)
	longer := strings.Repeat("a", maxLevInput+50)
	// b longer → returns cb (= maxLevInput+1).
	got := levenshtein(short, longer)
	assert.Equal(t, maxLevInput+1, got)
	// a longer → returns ca.
	got = levenshtein(longer, short)
	assert.Equal(t, maxLevInput+1, got)
}

// TestRuneCountAtMost_Cap exercises the early-exit branch of
// runeCountAtMost: a string longer than the cap returns
// exactly the cap, not the true rune count.
func TestRuneCountAtMost_Cap(t *testing.T) {
	s := strings.Repeat("a", maxLevInput+10)
	got := runeCountAtMost(s, maxLevInput)
	assert.Equal(t, maxLevInput, got)
}

// TestTooLongForLevInput_Bound exercises the boundary branch:
// exactly maxLevInput runes is not "too long", maxLevInput+1
// is.
func TestTooLongForLevInput_Bound(t *testing.T) {
	exact := strings.Repeat("a", maxLevInput)
	tooLong := strings.Repeat("a", maxLevInput+1)
	assert.False(t, tooLongForLevInput(exact))
	assert.True(t, tooLongForLevInput(tooLong))
}

// TestRenderHint_IntRangeOverflowFalsePositive guards the
// hint extractor against suggesting the rendered exclusive
// form's bound (`int > MaxInt`) for an in-range value. With
// the inclusive shift skipped, parseRenderedBounds doesn't
// match the rendered form, so hintForIntRange returns no
// hint.
func TestRenderHint_IntRangeOverflowFalsePositive(t *testing.T) {
	maxStr := math.MaxInt
	assert.Empty(t, RenderHint("int & >"+itoa(maxStr), float64(0)))
}

// itoa is a tiny wrapper so the test reads naturally without
// importing strconv at the call site.
func itoa(n int) string {
	if n == math.MaxInt {
		return "9223372036854775807"
	}
	return ""
}
