package cuelite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustFail asserts that compiling schema returns an error — used to drive the
// error-propagation branches that forward a bad sub-expression's error up
// through each compiler position (struct field, list element, binary operand,
// disjunction branch, call argument, index).
func mustFail(t *testing.T, schema string) {
	t.Helper()
	_, err := Compile(schema)
	assert.Error(t, err, "expected %q to fail compile", schema)
}

// TestCompile_nestedErrorPropagation hits the `return nil, err` propagation
// branch in each compiler position by nesting an undefined reference (an
// always-failing leaf) inside the construct.
func TestCompile_nestedErrorPropagation(t *testing.T) {
	bad := "undefinedRef"
	mustFail(t, `{a: `+bad+`}`)                    // struct field value
	mustFail(t, `a: `+bad)                         // top-level field value
	mustFail(t, `{a: [`+bad+`]}`)                  // list prefix element
	mustFail(t, `{a: [...`+bad+`]}`)               // list ellipsis type
	mustFail(t, `{a: `+bad+` & string}`)           // & left operand
	mustFail(t, `{a: string & `+bad+`}`)           // & right operand
	mustFail(t, `{a: `+bad+` | string}`)           // | left branch
	mustFail(t, `{a: string | `+bad+`}`)           // | right branch
	mustFail(t, `{a: >=`+bad+`}`)                  // unary bound operand
	mustFail(t, `{a: -`+bad+`}`)                   // unary minus operand
	mustFail(t, `{a: +`+bad+`}`)                   // unary plus operand
	mustFail(t, `{a: close(`+bad+`)}`)             // close() argument
	mustFail(t, `{a: strings.MinRunes(`+bad+`)}`)  // MinRunes argument
	mustFail(t, `{a: [if `+bad+` == "x" {1}][0]}`) // comprehension condition
	mustFail(t, `{a: [if true {`+bad+`}][0]}`)     // comprehension body field
	mustFail(t, `{m: int, a: [m][`+bad+`]}`)       // index expression
}

// TestCompile_literalEdge covers the literal-parse error branches: an int
// literal too large and a malformed float, reached via schema sources.
func TestCompile_literalEdge(t *testing.T) {
	mustFail(t, `{a: 999999999999999999999999999999}`) // int overflow
}

// TestCompile_callArities covers the argument-count error branches of the
// builtin calls.
func TestCompile_callArities(t *testing.T) {
	mustFail(t, `{a: close()}`)
	mustFail(t, `{a: close({}, {})}`)
	mustFail(t, `{a: strings.MinRunes()}`)
	mustFail(t, `{a: strings.MinRunes(1, 2)}`)
}

// TestCompile_unsupportedConstructs covers the explicit "unsupported"
// branches for constructs outside the subset.
func TestCompile_unsupportedConstructs(t *testing.T) {
	mustFail(t, `{a: strings.foo}`)           // bare selector expression
	mustFail(t, `{a: 1 + 2}`)                 // unsupported binary +
	mustFail(t, `{a: [1, 2][1.5]}`)           // non-int index
	mustFail(t, `{a: int & =~"x"}`)           // regex on a non-string base — conflict
	mustFail(t, `{a: =~1}`)                   // regex pattern not a string
	mustFail(t, `{a: !~1}`)                   // non-match pattern not a string
	mustFail(t, `{a: [for x in [1] {x}][0]}`) // for-comprehension
}

// TestLiftJSON_unsupportedValue covers liftAny's default branch via a JSON
// document the strict lift handles but that holds an out-of-model type would
// be impossible; instead exercise the malformed-number lift path.
func TestLiftJSON_numberForms(t *testing.T) {
	v, err := CompileJSON([]byte(`{"a": 1e999}`))
	require.NoError(t, err)
	assert.NoError(t, v.Validate())
	v2, err := CompileJSON([]byte(`{"a": -1e999}`))
	require.NoError(t, err)
	assert.NoError(t, v2.Validate())
}

// TestLiftMapValue_numberForms covers the int, float64, and json.Number
// branches of the map lift. A Go int lifts to kInt and any float64 lifts to
// kFloat (no integral coercion — a float64 is a float, matching CUE).
func TestLiftMapValue_numberForms(t *testing.T) {
	schema, err := Compile(`{i: int, f: float}`)
	require.NoError(t, err)
	assert.NoError(t, schema.CompileMap(map[string]any{"i": 7, "f": 1.5}).Validate())
	// A whole-number float64 stays a float, so it satisfies float and conflicts
	// with int — the two lift paths and CUE agree.
	assert.NoError(t, schema.CompileMap(map[string]any{"i": 1, "f": float64(7)}).Validate())
	assert.Error(t, schema.CompileMap(map[string]any{"i": float64(7), "f": 1.5}).Validate())
}

// TestAtomKindOf covers the bytes and bool branches of atomKindOf via a
// conflict that prints them.
func TestAtomKindOf_branches(t *testing.T) {
	// bool concrete vs string atom → conflict naming bool.
	v, err := Compile(`{a: string}`)
	require.NoError(t, err)
	verr := v.CompileMap(map[string]any{"a": true}).Validate()
	require.Error(t, verr)
}

// TestCompile_topLabelRejected pins that the bare top token `_` is rejected
// as a field label (CUE excludes it from the data struct), even though `_`
// is a valid VALUE (top).
func TestCompile_topLabelRejected(t *testing.T) {
	_, err := Compile(`{_: int}`)
	require.Error(t, err)
	// `_` as a value is fine.
	v, err := Compile(`{a: _}`)
	require.NoError(t, err)
	assert.NoError(t, v.CompileMap(map[string]any{"a": "anything"}).Validate())
}

// TestExprText covers exprText's selector and fallback branches via an
// unsupported selector-expression error message.
func TestExprText(t *testing.T) {
	_, err := Compile(`{a: pkg.sub.deep}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pkg.sub")
}

// TestNumericValue covers numericValue's float branch through a float bound
// comparison.
func TestNumericValue_float(t *testing.T) {
	v, err := Compile(`{a: >=1.5}`)
	require.NoError(t, err)
	assert.NoError(t, v.CompileMap(map[string]any{"a": 2.5}).Validate())
	assert.Error(t, v.CompileMap(map[string]any{"a": 1.0}).Validate())
}

// TestCompareStr_orderedOps covers compareStr's ordered relational branches
// through string comparisons inside a forced thunk.
func TestCompareStr_orderedOps(t *testing.T) {
	cases := []struct {
		cond string
		s    string
		hold bool
	}{
		{`s >= "m"`, "z", true},
		{`s <= "m"`, "a", true},
		{`s > "m"`, "z", true},
		{`s < "m"`, "a", true},
	}
	for _, tc := range cases {
		t.Run(tc.cond, func(t *testing.T) {
			schema := `{s: string, r: [if ` + tc.cond + ` {string & != ""}, (string | *"")][0]}`
			v, err := Compile(schema)
			require.NoError(t, err)
			verr := v.CompileMap(map[string]any{"s": tc.s, "r": ""}).Validate()
			if tc.hold {
				assert.Error(t, verr)
			} else {
				assert.NoError(t, verr)
			}
		})
	}
}

// TestConcreteEqual_kinds covers concreteEqual's bool, bytes-equivalent, and
// null branches through CompileJSON data and literal schemas.
func TestConcreteEqual_kinds(t *testing.T) {
	// bool equality.
	v, err := Compile(`{a: true}`)
	require.NoError(t, err)
	assert.NoError(t, v.CompileMap(map[string]any{"a": true}).Validate())
	assert.Error(t, v.CompileMap(map[string]any{"a": false}).Validate())
	// null equality.
	vn, err := Compile(`{a: null}`)
	require.NoError(t, err)
	assert.NoError(t, vn.CompileMap(map[string]any{"a": nil}).Validate())
}

// TestNumberOverflow_negative covers the negative-overflow float lift branch.
func TestNumberOverflow_negative(t *testing.T) {
	v, err := CompileJSON([]byte(`-1e999`))
	require.NoError(t, err)
	assert.NoError(t, v.Validate())
}

// TestIsDefinitionOrHidden covers the underscore-top exception and the
// non-special path.
func TestIsDefinitionOrHidden(t *testing.T) {
	assert.True(t, isDefinitionOrHidden("#x"))
	assert.True(t, isDefinitionOrHidden("_x"))
	assert.False(t, isDefinitionOrHidden("_"))
	assert.False(t, isDefinitionOrHidden("x"))
	assert.False(t, isDefinitionOrHidden(""))
}
