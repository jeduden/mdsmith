package schema

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRenderExpected_Edges exercises the early-return branches
// in RenderExpected and renderStringDisjunction / renderRegex /
// renderNonEmptyString that previously had no test coverage.
func TestRenderExpected_Edges(t *testing.T) {
	t.Run("empty input returns empty", func(t *testing.T) {
		assert.Equal(t, "", RenderExpected(""))
		// Whitespace-only is trimmed to "" before any shape
		// matches; the renderer returns the trimmed value.
		assert.Equal(t, "", RenderExpected("   "))
	})

	t.Run("disjunction with non-quoted alternative falls back", func(t *testing.T) {
		// `int | "x"` has a non-string-literal alternative; the
		// renderer should fall back to the raw expression
		// rather than misreport.
		assert.Equal(t, `int | "x"`, RenderExpected(`int | "x"`))
	})

	t.Run("regex with bad unquote falls back", func(t *testing.T) {
		// A regex pattern that uses single-quote delimiters
		// (CUE accepts double-quoted only) can't be unquoted;
		// the renderer falls through to the raw form.
		assert.Equal(t, `=~'foo'`, RenderExpected(`=~'foo'`))
	})

	t.Run("regex without quoted pattern falls back", func(t *testing.T) {
		assert.Equal(t, `=~foo`, RenderExpected(`=~foo`))
	})
}

// TestRenderExpected_IntRangeMixed covers the mixed-exclusive
// render branch: when one bound can be converted to inclusive
// (`<= N-1`) but the other can't (`>MaxInt`), the renderer
// emits both with explicit comparison operators so the
// asymmetry stays visible.
func TestRenderExpected_IntRangeMixed(t *testing.T) {
	maxStr := strconv.Itoa(math.MaxInt)
	got := RenderExpected("int & >" + maxStr + " & <=10")
	assert.Contains(t, got, "int > "+maxStr)
	assert.Contains(t, got, "and <= 10")
}

// TestRenderExpected_IntRangeOnlyExclusiveUpper exercises the
// half-open upper-bound rendering with an exclusive `<` form
// that the overflow guard preserves intact.
func TestRenderExpected_IntRangeOnlyExclusiveUpper(t *testing.T) {
	minStr := strconv.Itoa(math.MinInt)
	got := RenderExpected("int & <" + minStr)
	assert.Equal(t, "int < "+minStr, got)
}

// TestRenderExpected_IntRangeNoIntKeyword regresses the early
// false return when the `int` keyword is missing from the
// constraint (e.g. `>=1 & <=5` without `int &`). The
// constraint isn't recognised as an int range, so the renderer
// falls back to the raw expression.
func TestRenderExpected_IntRangeNoIntKeyword(t *testing.T) {
	got := RenderExpected(">=1 & <=5")
	assert.Equal(t, ">=1 & <=5", got)
}

// TestRenderExpected_IntRangeUnknownOperand exercises the
// fallback when a `&`-joined part doesn't fit the small
// grammar (int / >=, >, <=, <). The renderer aborts and falls
// back to the raw expression so a partial constraint never
// reaches the user.
func TestRenderExpected_IntRangeUnknownOperand(t *testing.T) {
	got := RenderExpected("int & some-other & >=1")
	assert.Equal(t, "int & some-other & >=1", got)
}

// TestRenderExpected_RegexNoWhitespace regresses a Copilot
// review observation: `string&=~"^A$"` (no spaces around `&`)
// is semantically equivalent to `string & =~"^A$"`. Both now
// render as `string matching <pattern>` instead of falling
// through to the raw expression.
func TestRenderExpected_RegexNoWhitespace(t *testing.T) {
	cases := map[string]string{
		`string&=~"^A$"`:  "string matching ^A$",
		`string &=~"^B$"`: "string matching ^B$",
		`string& =~"^C$"`: "string matching ^C$",
		`=~"^D$"`:         "string matching ^D$",
	}
	for in, want := range cases {
		assert.Equal(t, want, RenderExpected(in), "input: %q", in)
	}
}

// TestRenderExpected_RegexUnquoteFailure covers the
// strconv.Unquote error branch in renderRegex: a pattern
// with a malformed escape sequence (e.g. `\q` which Go does
// not recognise) is rejected after isQuotedString accepts
// the outer quotes.
func TestRenderExpected_RegexUnquoteFailure(t *testing.T) {
	assert.Equal(t, `=~"\q"`, RenderExpected(`=~"\q"`))
}

// TestRenderExpected_IntRangeUpperHalfOpen covers the
// half-open upper-bound render branch (`int <= N`) that the
// other tests miss when the lower bound is unspecified.
func TestRenderExpected_IntRangeUpperHalfOpen(t *testing.T) {
	assert.Equal(t, "int <= 5", RenderExpected("int & <=5"))
}

// TestRenderIntRange_NoBoundsFallsThrough exercises the
// final fallback inside (*intRangeBounds).render: an
// `int & int` constraint has the int keyword twice but no
// bounds on either side, so render returns ("", false) and
// RenderExpected falls back to the raw expression.
func TestRenderIntRange_NoBoundsFallsThrough(t *testing.T) {
	assert.Equal(t, "int & int", RenderExpected("int & int"))
}
