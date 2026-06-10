package cuelite

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests drive the lifter helpers and the compiler's error-propagation
// positions. The JSON lifters only ever see decoder-produced types, so their
// "unsupported value" guards are reached by calling the helpers directly with
// an out-of-model Go value — a real contract the decoder upholds, exercised
// red/green rather than left as a dead default.

// TestLiftAny_unsupported covers liftAny's default branch and, through it, the
// error propagation in liftMap and liftSlice.
func TestLiftAny_unsupported(t *testing.T) {
	_, err := liftAny(42) // a bare int is not a decoder-produced type
	assert.Error(t, err)

	_, err = liftMap(map[string]any{"a": 42})
	assert.Error(t, err, "liftMap forwards liftAny's error")

	_, err = liftSlice([]any{42})
	assert.Error(t, err, "liftSlice forwards liftAny's error")
}

// TestLiftMapValue_jsonNumber covers liftMapValue's json.Number branch.
func TestLiftMapValue_jsonNumber(t *testing.T) {
	schema, err := Compile(`{a: int}`)
	require.NoError(t, err)
	v := schema.CompileMap(map[string]any{"a": json.Number("5")})
	assert.NoError(t, v.Validate())
}

// TestLiftNumber_intAndFloat covers liftNumber's int and float branches.
func TestLiftNumber_intAndFloat(t *testing.T) {
	v, err := liftNumber(json.Number("5"))
	require.NoError(t, err)
	assert.Equal(t, kInt, v.kind)
	f, err := liftNumber(json.Number("1.5"))
	require.NoError(t, err)
	assert.Equal(t, kFloat, f.kind)
}

// TestEvalList_comprehensionElement covers evalList's comprehension branch
// (a bare list literal with an if-comprehension, not indexed) for both a kept
// and a dropped element, at compile time.
func TestEvalList_comprehensionElement(t *testing.T) {
	// A list whose only element is a kept comprehension: [if true {1}] → [1].
	v, err := Compile(`{a: [if true {1}]}`)
	require.NoError(t, err)
	assert.NoError(t, v.CompileMap(map[string]any{"a": []any{int64(1)}}).Validate())
	// A dropped comprehension yields an empty list: [if false {1}] → [].
	w, err := Compile(`{a: [if false {1}]}`)
	require.NoError(t, err)
	assert.NoError(t, w.CompileMap(map[string]any{"a": []any{}}).Validate())
}

// TestEvalListElems_ellipsisSkipped covers evalListElems' Ellipsis branch (an
// open tail in an indexed list adds no indexable element).
func TestEvalListElems_ellipsisSkipped(t *testing.T) {
	// The list inside the index has an ellipsis tail; index 0 selects the lone
	// prefix element, the tail contributing nothing.
	v, err := Compile(`{n: int, a: [n, ...int][0]}`)
	require.NoError(t, err)
	assert.NoError(t, v.CompileMap(map[string]any{"n": int64(3), "a": int64(3)}).Validate())
}

// TestCompile_topLevelReducesToBottom covers compileSource's isBottomV branch:
// a top-level embedded expression that reduces directly to ⊥.
func TestCompile_topLevelReducesToBottom(t *testing.T) {
	_, err := Compile(`int & string`)
	assert.Error(t, err)
}

// TestCompile_topLevelNonField covers compileFile's unsupported-top-level-
// declaration branch and the top-level fieldLabel/thunk-ref error positions.
func TestCompile_topLevelErrors(t *testing.T) {
	// A bare top-level field whose value is an undeclared-reference thunk: the
	// top-level checkThunkRefs rejects it.
	_, err := Compile(`a: [if y == "z" {string}][0]`)
	assert.Error(t, err)
	// A top-level definition label is not a data field.
	_, err = Compile("#x: int")
	assert.Error(t, err)
}

// TestCompile_hardErrorPropagation drives the compiler's error-propagation
// positions with a sub-expression that fails hard (a bad regex pattern, a
// standalone star default), not an unresolved reference, so each `return nil,
// err` forwards a real compile error.
func TestCompile_hardErrorPropagation(t *testing.T) {
	bad := `=~"["` // a syntactically invalid regex: hard compile error
	cases := []string{
		`{a: ` + bad + `}`,          // struct field value
		`a: ` + bad,                 // top-level field value
		`{a: {b: ` + bad + `}}`,     // nested struct field value
		`{a: [` + bad + `]}`,        // list prefix element
		`{a: [...` + bad + `]}`,     // list ellipsis type
		`{a: ` + bad + ` & string}`, // & left operand
		`{a: string & ` + bad + `}`, // & right operand
		`{a: ` + bad + ` | string}`, // | left branch
		`{a: string | ` + bad + `}`, // | right branch
		`{a: >=` + bad + `}`,        // unary bound operand (regex value invalid)
		`{a: -` + bad + `}`,         // unary minus operand
		`{a: +` + bad + `}`,         // unary plus operand
		`{a: close(` + bad + `)}`,   // close() argument
		`{` + bad + `}`,             // embedded value of a struct
	}
	for _, schema := range cases {
		t.Run(schema, func(t *testing.T) {
			_, err := Compile(schema)
			assert.Error(t, err, "expected %q to fail compile", schema)
		})
	}
}

// TestCompile_basicLitParseErrors covers compileBasicLit's int-overflow and
// float-overflow error branches via numbers outside the int64/float64 subset.
// Both report the out-of-subset "unsupported" class the cross-engine fuzzer's
// strict-subset hatch keys on.
func TestCompile_basicLitParseErrors(t *testing.T) {
	// An int literal outside int64 range: the in-house engine uses int64, CUE
	// uses big.Int, so this is an out-of-subset literal, not a parse error.
	_, err := Compile(`{a: 999999999999999999999999999999}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported int literal")
	// A float literal that overflows float64 (1e999): ParseFloat returns
	// ErrRange, so the in-house engine reports the out-of-subset float class.
	_, err = Compile(`{a: 1e999}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported float literal")
}

// TestCheckEmbeddedThunkRefs_undeclared covers checkEmbeddedThunkRefs
// rejecting an embedded thunk whose reference is not a declared field, both
// as a bare embedded comparison and when the thunk is nested in an embedded
// disjunction branch — CUE rejects both eagerly with "reference X not found".
func TestCheckEmbeddedThunkRefs_undeclared(t *testing.T) {
	_, err := Compile(`{a: int, nature == "x"}`)
	assert.Error(t, err, "an embedded comparison referencing an undeclared field is rejected")
	// The thunk lives inside an embedded disjunction (A > "" | ""); the scan
	// must descend the disjunction to find the undeclared reference A, matching
	// CUE's eager "reference A not found" rather than deferring to validate.
	_, err = Compile(`{A > "" | ""}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `reference "A" not found`)
}

// TestFieldLabel_quotedAndBad covers fieldLabel's quoted-string branch and its
// non-string / bad-unquote error branches.
func TestFieldLabel_branches(t *testing.T) {
	// A quoted label that needs unquoting.
	v, err := Compile(`{"a-b": int}`)
	require.NoError(t, err)
	assert.NoError(t, v.CompileMap(map[string]any{"a-b": int64(1)}).Validate())
	// A numeric (non-string) literal label is unsupported.
	_, err = Compile(`{1: int}`)
	assert.Error(t, err)
}

// TestBoundFromOperand_nonScalar covers boundFromOperand's non-scalar-operand
// error branch (a relational bound whose operand is not a scalar).
func TestBoundFromOperand_nonScalar(t *testing.T) {
	_, err := boundFromOperand(opGe, &engineValue{kind: kStruct})
	assert.Error(t, err)
}

// TestNegateNumeric_nonNumeric covers negateNumeric's default branch.
func TestNegateNumeric_nonNumeric(t *testing.T) {
	_, err := negateNumeric(&engineValue{kind: kString, str: "x"})
	assert.Error(t, err)
}

// TestCompileCall_branches covers compileCall's unsupported-function and
// unsupported-call-target branches.
func TestCompileCall_branches(t *testing.T) {
	_, err := Compile(`{a: nope(1)}`)
	assert.Error(t, err, "an unknown bare function is unsupported")
	_, err = Compile(`{a: (1)(2)}`)
	assert.Error(t, err, "a non-ident, non-selector call target is unsupported")
}

// TestCompileSelectorCall_branches covers compileSelectorCall's
// non-ident-package and unsupported-name branches.
func TestCompileSelectorCall_branches(t *testing.T) {
	_, err := Compile(`{a: strings.Nope(1)}`)
	assert.Error(t, err, "an unsupported selector function")
	_, err = Compile(`{a: foo.bar.baz(1)}`)
	assert.Error(t, err, "a non-ident selector package")
}

// TestIsDeferrable_parenWrapped covers isDeferrable's ParenExpr recursion: a
// parenthesized relational comparison over a sibling reference defers.
func TestIsDeferrable_parenWrapped(t *testing.T) {
	// A field whose value is a parenthesized relational over a sibling: the
	// paren-wrapped comparison is deferrable, so it becomes a thunk.
	v, err := Compile(`{n: int, r: (n != 0)}`)
	require.NoError(t, err)
	// With n concrete and non-zero, the comparison resolves to true at force.
	res := v.CompileMap(map[string]any{"n": int64(5), "r": true})
	assert.NoError(t, res.Validate())
}
