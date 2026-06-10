package cuelite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unifyResult compiles two schema expressions and returns the meet of their
// single field `a`, so each unify rule is driven through the public Compile
// + Unify surface. Both sources wrap the constraint in `{a: ...}`.
func unifyResult(t *testing.T, x, y string) *engineValue {
	t.Helper()
	vx, err := Compile(`{a: ` + x + `}`)
	require.NoError(t, err)
	vy, err := Compile(`{a: ` + y + `}`)
	require.NoError(t, err)
	merged := vx.Unify(vy)
	leaf, ok := merged.LookupPath(MakePath("a"))
	require.True(t, ok)
	return leaf.v
}

// TestUnify_rules drives one accept and one reject per lattice-meet rule in
// plan 218, over every operand-kind combination, exercising the symmetric
// branches in both operand orders.
func TestUnify_rules(t *testing.T) {
	cases := []struct {
		name        string
		x, y        string
		wantBottom  bool
		alsoReverse bool // also assert the reversed operand order agrees
	}{
		// ⊤ identity.
		{"top & string", `_`, `string`, false, true},
		{"top & concrete", `_`, `"x"`, false, true},
		// null.
		{"null & null", `null`, `null`, false, true},
		{"null & string", `null`, `string`, true, true},
		// concrete & concrete.
		{"equal strings", `"x"`, `"x"`, false, true},
		{"diff strings", `"x"`, `"y"`, true, true},
		{"equal ints", `3`, `3`, false, true},
		{"diff ints", `3`, `4`, true, true},
		{"int vs float same value", `3`, `3.0`, true, true},
		{"equal floats", `1.5`, `1.5`, false, true},
		{"equal bools", `true`, `true`, false, true},
		{"diff bools", `true`, `false`, true, true},
		// concrete & atom.
		{"string sat string", `"x"`, `string`, false, true},
		{"int sat number", `3`, `number`, false, true},
		{"int vs string atom", `3`, `string`, true, true},
		// concrete & bound.
		{"int sat bound", `5`, `>=0`, false, true},
		{"int fail bound", `-1`, `>=0`, true, true},
		{"string fail regex base-kind", `1`, `=~"x"`, true, true},
		// atom & atom.
		{"string & string", `string`, `string`, false, true},
		{"number & int", `number`, `int`, false, true},
		{"int & string atoms", `int`, `string`, true, true},
		// atom & bound.
		{"number & bound", `number`, `>=0`, false, true},
		{"string atom & regex", `string`, `=~"x"`, false, true},
		{"int atom & string bound", `int`, `!=""`, true, true},
		// bound & bound.
		{"two numeric bounds", `>=0`, `<=10`, false, true},
		{"two regex bounds", `=~"^a"`, `=~"b$"`, false, true},
		{"numeric bound & string bound", `>=0`, `!=""`, true, true},
		// disjunction.
		{"disjunction narrows", `"x" | "y"`, `"x"`, false, true},
		{"disjunction empty", `"x" | "y"`, `"z"`, true, true},
		// struct.
		{"struct & non-struct", `{b: int}`, `string`, true, true},
		// list.
		{"open & closed length ok", `[...int]`, `[int]`, false, true},
		{"closed length conflict", `[int, int]`, `[int]`, true, true},
		{"list & non-list", `[...int]`, `string`, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := unifyResult(t, tc.x, tc.y)
			assert.Equal(t, tc.wantBottom, got.isBottomV(), "x&y")
			if tc.alsoReverse {
				rev := unifyResult(t, tc.y, tc.x)
				assert.Equal(t, tc.wantBottom, rev.isBottomV(), "y&x")
			}
		})
	}
}

// TestUnify_disjunctionMultiBranch covers a disjunction unify that leaves
// more than one surviving branch (a still-ambiguous result).
func TestUnify_disjunctionMultiBranch(t *testing.T) {
	got := unifyResult(t, `"x" | "y" | "z"`, `string`)
	assert.Equal(t, kDisjoint, got.kind, "string narrows nothing, all branches survive")
	require.Len(t, got.branches, 3)
}

// TestUnify_nestedListElements covers element-wise list unification with a
// prefix on each side and a tail meet.
func TestUnify_nestedListElements(t *testing.T) {
	v, err := Compile(`{a: [>=0, ...int]}`)
	require.NoError(t, err)
	d, err := CompileJSON([]byte(`{"a": [5, 6, 7]}`))
	require.NoError(t, err)
	assert.NoError(t, v.Unify(d).Validate())

	bad, err := CompileJSON([]byte(`{"a": [-1, 6]}`))
	require.NoError(t, err)
	assert.Error(t, v.Unify(bad).Validate())
}

// TestUnify_listTailMeet covers two open lists whose tail element types meet.
func TestUnify_listTailMeet(t *testing.T) {
	got := unifyResult(t, `[...number]`, `[...int]`)
	require.Equal(t, kList, got.kind)
	require.True(t, got.openTop)
	require.NotNil(t, got.elem)
	assert.Equal(t, akInt, got.elem.atom)
}

// TestUnify_listOneSideTail covers an open list meeting an open list where
// only one side carries a tail element type.
func TestUnify_listOneSideTail(t *testing.T) {
	a := unifyResult(t, `[...int]`, `[...]`)
	require.Equal(t, kList, a.kind)
	require.NotNil(t, a.elem)
	b := unifyResult(t, `[...]`, `[...int]`)
	require.NotNil(t, b.elem)
}

// TestUnify_structClosedBothDirections covers a closed struct rejecting an
// extra field declared only on the other side, in both operand orders.
func TestUnify_structClosedBothDirections(t *testing.T) {
	closedFirst := unifyResult(t, `close({})`, `{extra: int}`)
	assert.NotNil(t, firstBottom(closedFirst))
	closedSecond := unifyResult(t, `{extra: int}`, `close({})`)
	assert.NotNil(t, firstBottom(closedSecond))
}

// TestUnify_structOptionalMerge covers two structs that both declare an
// optional field: the merged field stays optional only when both are.
func TestUnify_structOptionalMerge(t *testing.T) {
	v, err := Compile(`{a?: string}`)
	require.NoError(t, err)
	w, err := Compile(`{a?: string}`)
	require.NoError(t, err)
	// Both optional and absent in data → no error.
	assert.NoError(t, v.Unify(w).Validate())
}

// TestUnify_thunkOutsideStruct covers a thunk reaching unifyV outside a
// struct's force pass (forced with an empty scope, yielding ⊥).
func TestUnify_thunkOutsideStruct(t *testing.T) {
	// A bare thunk value, unified directly, has no sibling scope; it forces to
	// ⊥. Build one via the registry ternary field, then unify the field itself.
	schema := `{mechanism: "push" | "pull", ` +
		`registry: [if mechanism == "push" {string & != ""}, (string | *"")][0]}`
	v, err := Compile(schema)
	require.NoError(t, err)
	reg, ok := v.LookupPath(MakePath("registry"))
	require.True(t, ok)
	require.Equal(t, kThunk, reg.v.kind)
	// Unify the lone thunk against a concrete; the thunk forces with no scope
	// and yields ⊥, so the result is a bottom.
	other, err := Compile(`"x"`)
	require.NoError(t, err)
	assert.NotNil(t, firstBottom(reg.Unify(other).v))
}

// TestUnify_disjunctionPreservesDefault covers a disjunction unify where the
// default branch survives.
func TestUnify_disjunctionPreservesDefault(t *testing.T) {
	got := unifyResult(t, `*"a" | "b"`, `string`)
	require.Equal(t, kDisjoint, got.kind)
	def, ambiguous := got.defaultValue()
	require.NotNil(t, def)
	require.False(t, ambiguous)
	require.Equal(t, "a", def.str)
}

// TestConcreteSatisfiesBound_minRunesInt64 pins the int64-space
// strings.MinRunes comparison: a rune count wider than 32 bits must
// stay unsatisfied on every platform rather than truncate on 32-bit
// targets and invert the check.
func TestConcreteSatisfiesBound_minRunesInt64(t *testing.T) {
	schema, err := Compile(`close({ x: strings.MinRunes(4294967297) })`)
	require.NoError(t, err)
	data, err := CompileJSON([]byte(`{"x": "short"}`))
	require.NoError(t, err)
	verr := data.Unify(schema).Validate()
	require.Error(t, verr)
}
