package cuelite

import (
	"math"
	"testing"

	"github.com/jeduden/mdsmith/cue/cuelite/syntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file drives every reachable error and edge branch of the row
// evaluator to 100% statement coverage, one focused case per branch.

func TestCompileRow_RejectsMultipleExpressions(t *testing.T) {
	// `1, 2` parses to two declarations — not a single expression.
	_, err := CompileRow("1, 2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "single expression")
}

func TestRender_LiftErrorIsEncodingFrontmatter(t *testing.T) {
	// A front-matter value type the lifter cannot represent (a chan) surfaces
	// as an encoding-frontmatter error naming the field.
	tpl, err := CompileRow(`"literal"`)
	require.NoError(t, err)
	_, err = tpl.Render(map[string]any{"bad": make(chan int)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "encoding frontmatter")
	assert.Contains(t, err.Error(), `"bad"`)
}

func TestRender_NonFiniteInfIsError(t *testing.T) {
	tpl, err := CompileRow(`"literal"`)
	require.NoError(t, err)
	_, err = tpl.Render(map[string]any{"w": mustInf()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-finite")
}

func TestRender_NonFiniteNestedInStructIsError(t *testing.T) {
	tpl, err := CompileRow(`"literal"`)
	require.NoError(t, err)
	_, err = tpl.Render(map[string]any{
		"m": map[string]any{"score": mustInf()},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-finite")
}

func TestRender_NonFiniteNestedInListIsError(t *testing.T) {
	tpl, err := CompileRow(`"literal"`)
	require.NoError(t, err)
	_, err = tpl.Render(map[string]any{
		"scores": []any{mustInf()},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-finite")
}

func TestRender_UnsupportedConstruct(t *testing.T) {
	// A struct literal is not a row value.
	_, err := renderRow(t, `{a: 1}`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported row construct")
}

func TestRender_IdentBoolNullLiterals(t *testing.T) {
	got, err := renderRow(t, `[if true {"t"}, if false {"f"}][0]`, nil)
	require.NoError(t, err)
	assert.Equal(t, "t", got)
	got, err = renderRow(t, `[if x == null {"n"}, if x != null {"y"}][0]`, map[string]any{"x": nil})
	require.NoError(t, err)
	assert.Equal(t, "n", got)
}

func TestRender_UnaryNotOnNonBoolIsError(t *testing.T) {
	_, err := renderRow(t, `[if !id {"x"}][0]`, map[string]any{"id": "s"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "! requires a bool")
}

func TestRender_UnaryNegativeNumber(t *testing.T) {
	// -1 negates a numeric literal; the result is not a string, so it errors at
	// the string check — but it exercises the SUB unary arm.
	_, err := renderRow(t, `-1`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "concrete string")
}

func TestRender_UnaryNegateNonNumberIsError(t *testing.T) {
	_, err := renderRow(t, `-id`, map[string]any{"id": "s"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot negate")
}

func TestRender_UnsupportedUnaryOperator(t *testing.T) {
	// A bound-style unary (>=) is not a row construct.
	_, err := renderRow(t, `>=1`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported unary operator")
}

func TestRender_UnsupportedBinaryOperator(t *testing.T) {
	_, err := renderRow(t, `id & "x"`, map[string]any{"id": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported row operator")
}

func TestRender_NumberAddition(t *testing.T) {
	// int + int stays an int (then fails the string check) — exercises the
	// int-int add arm.
	_, err := renderRow(t, `1 + 2`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "concrete string")
}

func TestRender_FloatAddition(t *testing.T) {
	// Float `+` is rejected loudly as out-of-subset (item 8): the engine holds
	// only float64 and would diverge from CUE's decimal rendering.
	_, err := renderRow(t, `1.5 + 2`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported float arithmetic")
}

func TestRender_StructIndexNonStringIsError(t *testing.T) {
	_, err := renderRow(t, `fm[0]`, map[string]any{"id": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "struct index must be a string")
}

func TestRender_ListIndexNonIntegerIsError(t *testing.T) {
	_, err := renderRow(t, `items["k"]`, map[string]any{"items": []any{"a"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list index must be an integer")
}

func TestRender_IndexNonIndexableIsError(t *testing.T) {
	_, err := renderRow(t, `id[0]`, map[string]any{"id": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot index")
}

func TestRender_ListOpenTailIsError(t *testing.T) {
	_, err := renderRow(t, `[1, ...][0]`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open list tail")
}

func TestRender_ComprehensionMultiClauseIsError(t *testing.T) {
	_, err := renderRow(t, `[for x in items if true {x}][0]`, map[string]any{"items": []any{"a"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "single-clause comprehension")
}

func TestRender_IfConditionNonBoolIsError(t *testing.T) {
	_, err := renderRow(t, `[if id {"x"}][0]`, map[string]any{"id": "s"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "if condition must be a bool")
}

func TestRender_ForOverNonListIsError(t *testing.T) {
	_, err := renderRow(t, `[for m in id {m}][0]`, map[string]any{"id": "s"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "for-comprehension source must be a list")
}

func TestRender_ForWithKeyVariableIsError(t *testing.T) {
	_, err := renderRow(t, `[for k, m in items {m}][0]`, map[string]any{"items": []any{"a"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key variable")
}

func TestRender_ComprehensionBodyMultiFieldIsError(t *testing.T) {
	_, err := renderRow(t, `[if true {a, b}][0]`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "single expression")
}

func TestRender_ComprehensionBodyNonEmbedIsError(t *testing.T) {
	_, err := renderRow(t, `[if true {a: 1}][0]`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "embed an expression")
}

func TestRender_ComprehensionBodyNonStructIsError(t *testing.T) {
	// A comprehension value that is not a struct.
	_, err := renderRow(t, `[for m in items for n in items {m}][0]`, map[string]any{"items": []any{"a"}})
	require.Error(t, err)
	// Two for-clauses → multi-clause rejection (a single clause whose value is
	// non-struct is hard to author, so the multi-clause arm guards first).
	assert.Contains(t, err.Error(), "single-clause")
}

func TestRender_UnsupportedFunction(t *testing.T) {
	_, err := renderRow(t, `strings.ToUpper("x")`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported function")
}

func TestRender_CallArgError(t *testing.T) {
	_, err := renderRow(t, `len(missing)`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRender_CallTargetNonIdentPkgIsError(t *testing.T) {
	// A selector whose base is not an identifier package.
	_, err := renderRow(t, `items[0].Join([], "")`, map[string]any{
		"items": []any{map[string]any{"id": "x"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported call target")
}

func TestRender_CallTargetNonCallableIsError(t *testing.T) {
	// Calling a parenthesized expression.
	_, err := renderRow(t, `("x")("y")`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported call target")
}

func TestRender_StringsJoinNonListIsError(t *testing.T) {
	_, err := renderRow(t, `strings.Join("x", ",")`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a list")
}

func TestRender_StringsJoinNonStringSepIsError(t *testing.T) {
	_, err := renderRow(t, `strings.Join(["a"], 1)`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "separator must be a string")
}

func TestRender_StringsJoinNonStringElementIsError(t *testing.T) {
	_, err := renderRow(t, `strings.Join([1], ",")`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "element 0 must be a string")
}

func TestRender_LenArityError(t *testing.T) {
	_, err := renderRow(t, `len("a", "b")`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "len takes one argument")
}

func TestRender_LenNonStringOrListIsError(t *testing.T) {
	_, err := renderRow(t, `len(n)`, map[string]any{"n": 5})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "string or list")
}

func TestRender_EqualListLengthMismatch(t *testing.T) {
	got, err := renderRow(t, `[if a == b {"eq"}, if a != b {"ne"}][0]`, map[string]any{
		"a": []any{"x"},
		"b": []any{"x", "y"},
	})
	require.NoError(t, err)
	assert.Equal(t, "ne", got)
}

func TestRender_EqualListElementMismatch(t *testing.T) {
	got, err := renderRow(t, `[if a == b {"eq"}, if a != b {"ne"}][0]`, map[string]any{
		"a": []any{"x"},
		"b": []any{"y"},
	})
	require.NoError(t, err)
	assert.Equal(t, "ne", got)
}

func TestRender_EqualListMatch(t *testing.T) {
	got, err := renderRow(t, `[if a == b {"eq"}, if a != b {"ne"}][0]`, map[string]any{
		"a": []any{"x"},
		"b": []any{"x"},
	})
	require.NoError(t, err)
	assert.Equal(t, "eq", got)
}

func TestRender_ParenExprAtTopLevel(t *testing.T) {
	got, err := renderRow(t, `("x" + "y")`, nil)
	require.NoError(t, err)
	assert.Equal(t, "xy", got)
}

func TestRender_UnaryOperandError(t *testing.T) {
	_, err := renderRow(t, `!missing`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRender_BoolNullLiteralDirect(t *testing.T) {
	// true/false/null parse as basic literals (not identifiers); interpolating
	// them exercises the bool/null literal rendering.
	got, err := renderRow(t, `"\(true)\(false)"`, nil)
	require.NoError(t, err)
	assert.Equal(t, "truefalse", got)
	// null in a direct value position errors at interpolation, but the ident
	// arm is reached first.
	_, err = renderRow(t, `"\(null)"`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid interpolation")
}

func TestRender_BinaryLeftOperandError(t *testing.T) {
	_, err := renderRow(t, `missing + "x"`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRender_BinaryRightOperandError(t *testing.T) {
	_, err := renderRow(t, `"x" + missing`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRender_SelectorBaseError(t *testing.T) {
	_, err := renderRow(t, `missing.id`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRender_IndexBaseError(t *testing.T) {
	_, err := renderRow(t, `missing[0]`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRender_IndexIndexError(t *testing.T) {
	_, err := renderRow(t, `items[missing]`, map[string]any{"items": []any{"a"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRender_ListElementError(t *testing.T) {
	_, err := renderRow(t, `[missing][0]`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRender_ForClauseSourceError(t *testing.T) {
	_, err := renderRow(t, `[for m in missing {m}][0]`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRender_ForClauseBodyError(t *testing.T) {
	_, err := renderRow(t, `[for m in items {missing}][0]`, map[string]any{"items": []any{"a"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRender_IfClauseBodyError(t *testing.T) {
	_, err := renderRow(t, `[if true {missing}][0]`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// mustInf returns positive infinity as a float64, the value yaml.v3 decodes
// `.inf` to.
func mustInf() float64 {
	return math.Inf(1)
}

// TestRowSelectorName_UnquoteError covers rowSelectorName's unquote-error
// branch via a constructed malformed string-label node. The CUE parser never
// produces this shape for a selector, so the branch is reached only by a direct
// AST construction.
func TestRowSelectorName_UnquoteError(t *testing.T) {
	_, _, err := rowSelectorName(&syntax.BasicLit{Kind: syntax.STRING, Value: `"\x"`})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "selector label")
}

// TestEvalRowSelector_BadLabel covers evalRowSelector's error path when the
// selector label cannot resolve, via a constructed malformed string-label node
// the CUE parser never emits for a selector.
func TestEvalRowSelector_BadLabel(t *testing.T) {
	rs, err := newRowScope(map[string]any{"fm2": map[string]any{"k": "v"}})
	require.NoError(t, err)
	sel := &syntax.SelectorExpr{
		X:   &syntax.Ident{Name: "fm2"},
		Sel: &syntax.BasicLit{Kind: syntax.STRING, Value: `"\x"`},
	}
	_, err = evalRowSelector(sel, rs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "selector label")
}

// TestRowLen_StructIsRejected covers rowLen's struct branch: CUE's len(struct)
// is the field count, but the row subset rejects it loudly as out-of-subset.
func TestRowLen_StructIsRejected(t *testing.T) {
	_, err := renderRow(t, `"\(len(m))"`, map[string]any{"m": map[string]any{"k": "v"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported len of a struct")
}

// TestEvalRowInterpolation_BytesRejected covers evalRowInterpolation's
// bytes-dialect rejection: the in-house parser flags a single-quote
// interpolation on the node (IsBytes), which the row subset (no bytes kind)
// rejects loudly. Driven directly so the branch is covered without relying on
// the parser emitting a bytes interpolation (which the row corpus never does).
func TestEvalRowInterpolation_BytesRejected(t *testing.T) {
	rs, err := newRowScope(map[string]any{"x": "v"})
	require.NoError(t, err)
	n := &syntax.Interpolation{IsBytes: true, Elts: []syntax.Expr{
		&syntax.BasicLit{Value: "a"},
		&syntax.Ident{Name: "x"},
		&syntax.BasicLit{Value: "b"},
	}}
	_, err = evalRowInterpolation(n, rs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported bytes interpolation")
}

// TestEvalRowInterpolation_NonStringEmbedded covers the embedded-expression
// error path: an interpolation whose embedded value is not stringable (a null)
// is rejected with "invalid interpolation".
func TestEvalRowInterpolation_NonStringEmbedded(t *testing.T) {
	_, err := renderRow(t, `"a\(x)b"`, map[string]any{"x": nil})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid interpolation")
}

// TestNewRowScope_EmptyKeyNotBare covers isBareIdentifier's empty-string
// branch: a front-matter key that is the empty string binds nowhere as a bare
// identifier (it is reachable only via fm[""]).
func TestNewRowScope_EmptyKeyNotBare(t *testing.T) {
	got, err := renderRow(t, `fm[""]`, map[string]any{"": "v"})
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}
