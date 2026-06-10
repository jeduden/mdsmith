package cuelite

import (
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests below close the residual statement-coverage gaps the
// scope-threading dedup left: engine-level helpers exercised on constructed
// values, the error-propagation positions of the compiler, and the bottom/
// bytes branches that front-matter data never produces but the model must
// still handle. Each drives one branch red/green.

// TestConcreteScalarV_nonScalar covers concreteScalarV's false return for a
// non-scalar value (an atom branch surviving in a disjunction dedupe).
func TestConcreteScalarV_nonScalar(t *testing.T) {
	assert.False(t, (&engineValue{kind: kAtom, atom: akString}).concreteScalarV())
	assert.False(t, (&engineValue{kind: kStruct}).concreteScalarV())
	assert.True(t, (&engineValue{kind: kBytes, bytes: []byte("x")}).concreteScalarV())
}

// TestDedupeConcrete_keepsNonScalar drives dedupeConcrete over a disjunction
// whose surviving branches include a non-scalar, so concreteScalarV's false
// path runs inside the reduction (`string | "x"` met with `string`).
func TestDedupeConcrete_keepsNonScalar(t *testing.T) {
	got := unifyResult(t, `string | "x"`, `string`)
	require.Equal(t, kDisjoint, got.kind)
	// Both branches survive (string narrows nothing); the "x" concrete and the
	// string atom are distinct, so neither is deduped.
	require.Len(t, got.branches, 2)
}

// TestAtomKindOf_allKinds covers every concrete-kind branch of atomKindOf,
// including bytes (which front matter never carries), plus the false return.
func TestAtomKindOf_allKinds(t *testing.T) {
	cases := []struct {
		v    *engineValue
		want atomKind
		ok   bool
	}{
		{&engineValue{kind: kString}, akString, true},
		{&engineValue{kind: kInt}, akInt, true},
		{&engineValue{kind: kFloat}, akFloat, true},
		{&engineValue{kind: kBool}, akBool, true},
		{&engineValue{kind: kBytes}, akBytes, true},
		{&engineValue{kind: kNull}, 0, false},
		{&engineValue{kind: kStruct}, 0, false},
	}
	for _, tc := range cases {
		ak, ok := tc.v.atomKindOf()
		assert.Equal(t, tc.ok, ok)
		if ok {
			assert.Equal(t, tc.want, ak)
		}
	}
}

// TestNumericValue_nonNumeric covers numericValue's false return on a
// non-numeric value.
func TestNumericValue_nonNumeric(t *testing.T) {
	_, ok := (&engineValue{kind: kString, str: "x"}).numericValue()
	assert.False(t, ok)
	n, ok := (&engineValue{kind: kFloat, f: 2.5}).numericValue()
	require.True(t, ok)
	assert.Equal(t, 2.5, n)
}

// TestLiftMap covers the exported LiftMap, both its success and its
// unsupported-value error branch.
func TestLiftMap(t *testing.T) {
	v := LiftMap(map[string]any{"a": "x", "n": int64(1)})
	require.True(t, v.Exists())
	var out any
	require.NoError(t, v.Decode(&out))
	m := out.(map[string]any)
	assert.Equal(t, "x", m["a"])

	bad := LiftMap(map[string]any{"a": make(chan int)})
	assert.False(t, bad.Exists(), "an unsupported value type makes LiftMap a bottom")
}

// TestValidate_topLevelBottom covers Validate's bottom branch where isBottom
// returns a *PathError (the value's own engine node is ⊥), so the
// errors.As-true path runs.
func TestValidate_topLevelBottom(t *testing.T) {
	v := Value{v: mkBottom([]string{"a"}, "boom")}
	err := v.Validate()
	require.Error(t, err)
	var pe *PathError
	require.True(t, stderrors.As(err, &pe))
	assert.Equal(t, []string{"a"}, pe.Path())
}

// TestIsBottom_topLevelBottomEngineValue covers isBottom's v.v.isBottomV()
// branch (the engine value itself is ⊥).
func TestIsBottom_topLevelBottomEngineValue(t *testing.T) {
	v := Value{v: mkBottom([]string{"x"}, "nope")}
	reason, isB := v.isBottom()
	require.True(t, isB)
	require.Error(t, reason)
}

// TestDecodeValue_bytesAndNested covers decodeValue's bytes branch and the
// error propagation through a struct field and a list element.
func TestDecodeValue_bytesAndNested(t *testing.T) {
	b, err := decodeValue(&engineValue{kind: kBytes, bytes: []byte("ab")})
	require.NoError(t, err)
	assert.Equal(t, []byte("ab"), b)

	// A struct holding a non-concrete (atom) field errors when decoded.
	st := &engineValue{kind: kStruct, fields: []field{{name: "a", val: &engineValue{kind: kAtom, atom: akString}}}}
	_, err = decodeValue(st)
	assert.Error(t, err)

	// A list holding a non-concrete element errors when decoded.
	lst := &engineValue{kind: kList, prefix: []*engineValue{{kind: kAtom, atom: akInt}}}
	_, err = decodeValue(lst)
	assert.Error(t, err)
}

// TestIsConcrete_bytesAndDefault covers isConcrete's bytes branch and its
// default (non-concrete) return.
func TestIsConcrete_bytesAndDefault(t *testing.T) {
	assert.True(t, isConcrete(&engineValue{kind: kBytes, bytes: []byte("x")}))
	assert.False(t, isConcrete(&engineValue{kind: kAtom, atom: akString}))
	assert.False(t, isConcrete(&engineValue{kind: kThunk}))
}

// TestIsUnsatisfiedConstraint_withBottom covers isUnsatisfiedConstraint's
// firstBottom-non-nil branch: a provided optional field reduced to ⊥ is a
// real failure, not an absent constraint.
func TestIsUnsatisfiedConstraint_withBottom(t *testing.T) {
	// An optional field declared as a literal, given conflicting data, reduces
	// to ⊥; it must be reported, not skipped as "absent".
	v, err := Compile(`{a?: "x"}`)
	require.NoError(t, err)
	assert.Error(t, v.CompileMap(map[string]any{"a": "y"}).Validate())
}

// TestCollectLeaves_bytesTopBoundList covers the kBytes (concrete, no leaf),
// kTop, kBound, and kList recursion branches of collectLeaves.
func TestCollectLeaves_branches(t *testing.T) {
	// kBytes concrete: no leaf.
	assert.Empty(t, collectLeaves(&engineValue{kind: kBytes, bytes: []byte("x")}, nil, nil))
	// kTop: one incomplete leaf.
	assert.Len(t, collectLeaves(topValue(), nil, nil), 1)
	// kBound: one incomplete leaf.
	bnd := &engineValue{kind: kBound, atom: akNumber, bounds: []bound{{op: opGe, num: 0}}}
	assert.Len(t, collectLeaves(bnd, nil, nil), 1)
	// kList with a non-concrete prefix element: one leaf at index 0.
	lst := &engineValue{kind: kList, prefix: []*engineValue{{kind: kAtom, atom: akInt}}}
	assert.Len(t, collectLeaves(lst, nil, nil), 1)
}

// TestUnifyV_oThunk covers unifyV's o.kind == kThunk branch (the thunk on the
// right operand).
func TestUnifyV_oThunk(t *testing.T) {
	thunk := &engineValue{kind: kThunk, thunkExpr: func(map[string]*engineValue) *engineValue {
		return mkBottom(nil, "unresolved")
	}}
	got := unifyV(&engineValue{kind: kString, str: "x"}, thunk, nil)
	assert.NotNil(t, firstBottom(got))
}

// TestUnifyList_oneSidePrefixTailFill covers unifyList where one side's prefix
// is longer, so the shorter side fills missing elements from its tail (the
// listElemAt-returns-tail and nil-fill positions), plus the single-side tail
// branches of the result element type.
func TestUnifyList_prefixTailFill(t *testing.T) {
	// v has a 2-prefix and is open; o has a 1-prefix and is open. Index 1 on o
	// comes from o's tail; both stay open.
	got := unifyResult(t, `[int, int, ...int]`, `[int, ...int]`)
	require.Equal(t, kList, got.kind)
	require.Len(t, got.prefix, 2)
	assert.True(t, got.openTop)
}

// TestUnifyList_oElemOnly covers the result tail where only o carries an elem
// type (v.elem nil), the symmetric counterpart of TestUnify_listOneSideTail's
// v-elem case.
func TestUnifyList_oElemOnly(t *testing.T) {
	got := unifyResult(t, `[...]`, `[...int]`)
	require.NotNil(t, got.elem)
	assert.Equal(t, akInt, got.elem.atom)
}

// TestConcreteSatisfiesBound_nonStringRegex covers concreteSatisfiesBound's
// non-string regex (a numeric value against a =~ bound, base-kind mismatch
// already excluded so this drives the c.kind != kString guard) and the
// string-bound-on-non-string guard.
func TestConcreteSatisfiesBound_kindGuards(t *testing.T) {
	// A != "" bound checked against a non-string concrete returns false.
	b := bound{op: opNe, str: "", isStr: true}
	assert.False(t, concreteSatisfiesBound(&engineValue{kind: kInt, i: 1}, b))
	// A regex bound checked against a non-string concrete returns false.
	re := bound{op: opMatch}
	assert.False(t, concreteSatisfiesBound(&engineValue{kind: kInt, i: 1}, re))
	// A numeric bound checked against a non-numeric concrete returns false.
	nb := bound{op: opGe, num: 0}
	assert.False(t, concreteSatisfiesBound(&engineValue{kind: kString, str: "x"}, nb))
}

// TestCompareNum_default covers compareNum's default (an op the numeric
// comparison does not handle, e.g. a match op) and compareStr's default.
func TestCompareNum_default(t *testing.T) {
	assert.False(t, compareNum(1, opMatch, 2))
	assert.False(t, compareStr("a", opMatch, "b"))
}

// TestConcreteEqual_boolBytesNull covers concreteEqual's bool, bytes, and
// null branches directly.
func TestConcreteEqual_boolBytesNull(t *testing.T) {
	assert.True(t, concreteEqual(&engineValue{kind: kBool, b: true}, &engineValue{kind: kBool, b: true}))
	ba := &engineValue{kind: kBytes, bytes: []byte("a")}
	bb := &engineValue{kind: kBytes, bytes: []byte("a")}
	assert.True(t, concreteEqual(ba, bb))
	assert.True(t, concreteEqual(&engineValue{kind: kNull}, &engineValue{kind: kNull}))
	assert.False(t, concreteEqual(&engineValue{kind: kAtom}, &engineValue{kind: kAtom}))
}
