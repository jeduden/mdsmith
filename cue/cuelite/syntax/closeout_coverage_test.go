package syntax

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// closeout_coverage_test.go drives the syntax-package lines that the parent
// cuelite row corpus exercises but the syntax package's own tests did not, so
// the per-package coverage Codecov measures matches the in-house frontend's
// real reach (plan 240 closeout). Each test names the construct it covers.

// TestScanOperators_allBinaryAndPrefix scans every one- and two-character
// operator through ParseFile so scanTwoCharOp and scanOperatorChar resolve each
// case. The two-character comparisons appear as binary operators; the bound
// operators and the single-character prefixes appear in constraint position.
func TestScanOperators_allBinaryAndPrefix(t *testing.T) {
	for _, src := range []string{
		`x: a == b`, // EQL
		`x: a != b`, // NEQ
		`x: a <= b`, // LEQ
		`x: a >= b`, // GEQ
		`x: a =~ b`, // MAT
		`x: a !~ b`, // NMAT
		`x: a - b`,  // SUB (binary)
		`x: a * b`,  // MUL (binary, binaryPrec MUL arm)
		`x: a < b`,  // LSS
		`x: a > b`,  // GTR
		`x: !a`,     // NOT (prefix)
	} {
		_, err := ParseFile(src)
		assert.NoError(t, err, "operator source %q must parse", src)
	}
}

// TestStartsExpr_ellipsisPrefixOperand parses a `...` ellipsis whose element
// type begins with a prefix operator, driving startsExpr's tOp arm.
func TestStartsExpr_ellipsisPrefixOperand(t *testing.T) {
	f, err := ParseFile(`x: [...>=0]`)
	require.NoError(t, err)
	list := f.Decls[0].(*Field).Value.(*ListLit)
	ell := list.Elts[0].(*Ellipsis)
	require.NotNil(t, ell.Type, "the ellipsis must carry the >=0 element type")
}

// TestSelectorLabel_quotedString parses a quoted selector member
// (`fm."my-key"`), driving parseSelectorLabel's tString arm.
func TestSelectorLabel_quotedString(t *testing.T) {
	f, err := ParseFile(`x: fm."my-key"`)
	require.NoError(t, err)
	sel := f.Decls[0].(*Field).Value.(*SelectorExpr)
	lit := sel.Sel.(*BasicLit)
	assert.Equal(t, STRING, lit.Kind)
}

// TestParens_successAndError parses a parenthesized expression (the tLParen
// success path: parseExpr then the closing `take()` and ParenExpr) and a
// parenthesized form whose inner expression fails to parse (the tLParen error
// path).
func TestParens_successAndError(t *testing.T) {
	f, err := ParseFile(`x: (a)`)
	require.NoError(t, err)
	assert.IsType(t, &ParenExpr{}, f.Decls[0].(*Field).Value)

	_, err = ParseFile(`x: (=)`) // inner `=` is not an expression
	assert.Error(t, err)
}

// TestParseArgs_argError parses a call whose argument fails to parse, driving
// parseArgs's error return.
func TestParseArgs_argError(t *testing.T) {
	_, err := ParseFile(`x: f(=)`)
	assert.Error(t, err)
}

// TestParseFile_trailingScanError parses a complete field followed by a byte
// that scans to a recorded error. parseDecls breaks on the resulting tEOF, so
// parseFile's post-loop s.err check is the path that surfaces the error.
func TestParseFile_trailingScanError(t *testing.T) {
	_, err := ParseFile(`x: 1 @`)
	require.Error(t, err, "the trailing unexpected character must surface via the scanner error")
}

// TestParseFloatLiteral_decodes covers ParseFloatLiteral, the exported decoder
// the parent compile step calls (the syntax package's own ParseFile never
// reaches it).
func TestParseFloatLiteral_decodes(t *testing.T) {
	got, err := ParseFloatLiteral("1_000.5")
	require.NoError(t, err)
	assert.InDelta(t, 1000.5, got, 1e-9)

	_, err = ParseFloatLiteral("nope")
	assert.Error(t, err)

	got, err = ParseFloatLiteral("0.0")
	require.NoError(t, err)
	assert.Equal(t, 0.0, math.Abs(got))
}
