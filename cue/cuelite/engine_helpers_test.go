package cuelite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCombineMode(t *testing.T) {
	assert.Equal(t, dfltMaybe, combineMode(dfltMaybe, dfltMaybe))
	assert.Equal(t, dfltNot, combineMode(dfltMaybe, dfltNot))
	assert.Equal(t, dfltNot, combineMode(dfltNot, dfltMaybe)) // commutative
	assert.Equal(t, dfltNot, combineMode(dfltNot, dfltNot))
	assert.Equal(t, dfltIs, combineMode(dfltMaybe, dfltIs))
	assert.Equal(t, dfltIs, combineMode(dfltIs, dfltMaybe)) // commutative
	assert.Equal(t, dfltIs, combineMode(dfltNot, dfltIs))
	assert.Equal(t, dfltIs, combineMode(dfltIs, dfltNot)) // commutative
	assert.Equal(t, dfltIs, combineMode(dfltIs, dfltIs))
}

func TestMkBottom(t *testing.T) {
	v := mkBottom([]string{"a", "b"}, "conflict: %s vs %s", "x", "y")
	require.True(t, v.isBottomV())
	assert.Equal(t, "conflict: x vs y", v.reason)
	assert.Equal(t, []string{"a", "b"}, v.path)
	assert.Equal(t, "_|_", v.describe())

	v2 := mkBottom(nil, "no path")
	require.True(t, v2.isBottomV())
	assert.Nil(t, v2.path)
}

func TestTopValue(t *testing.T) {
	v := topValue()
	assert.Equal(t, "_", v.describe())
	assert.False(t, v.isBottomV())
}

// TestEngineValue_IsBottomV pins the nil-safe method contract: a nil receiver
// must return false, not panic.
func TestEngineValue_IsBottomV(t *testing.T) {
	var nilV *engineValue
	assert.False(t, nilV.isBottomV())
	assert.True(t, mkBottom(nil, "x").isBottomV())
	assert.False(t, topValue().isBottomV())
	assert.False(t, (&engineValue{kind: kString, str: "hello"}).isBottomV())
	assert.False(t, (&engineValue{kind: kNull}).isBottomV())
}

// TestEngineValue_DefaultValue pins the ambig bool semantics: false means
// "zero or one default" while true means "more than one dfltIs branch",
// which CUE treats as ambiguous and non-concrete.
func TestEngineValue_DefaultValue(t *testing.T) {
	a := &engineValue{kind: kString, str: "a"}
	b := &engineValue{kind: kString, str: "b"}

	t.Run("no default", func(t *testing.T) {
		v := &engineValue{
			kind:     kDisjoint,
			branches: []*engineValue{a, b},
			modes:    []defaultMode{dfltMaybe, dfltNot},
		}
		got, ambig := v.defaultValue()
		assert.Nil(t, got)
		assert.False(t, ambig)
	})

	t.Run("first branch is default", func(t *testing.T) {
		v := &engineValue{
			kind:     kDisjoint,
			branches: []*engineValue{a, b},
			modes:    []defaultMode{dfltIs, dfltNot},
		}
		got, ambig := v.defaultValue()
		assert.Same(t, a, got)
		assert.False(t, ambig)
	})

	t.Run("second branch is default", func(t *testing.T) {
		v := &engineValue{
			kind:     kDisjoint,
			branches: []*engineValue{a, b},
			modes:    []defaultMode{dfltNot, dfltIs},
		}
		got, ambig := v.defaultValue()
		assert.Same(t, b, got)
		assert.False(t, ambig)
	})

	t.Run("ambiguous defaults", func(t *testing.T) {
		v := &engineValue{
			kind:     kDisjoint,
			branches: []*engineValue{a, b},
			modes:    []defaultMode{dfltIs, dfltIs},
		}
		got, ambig := v.defaultValue()
		assert.Nil(t, got)
		assert.True(t, ambig)
	})
}

func TestEngineValue_DescribeBound(t *testing.T) {
	cases := []struct {
		name string
		v    *engineValue
		want string
	}{
		{
			"int with two numeric bounds",
			&engineValue{kind: kBound, atom: akInt, bounds: []bound{{op: opGe, num: 0}, {op: opLe, num: 100}}},
			"int & >=0 & <=100",
		},
		{
			"string with regex match constraint",
			&engineValue{kind: kBound, atom: akString, bounds: []bound{{op: opMatch, src: `^[a-z]+$`}}},
			`string & =~"^[a-z]+$"`,
		},
		{
			"float atom no bounds",
			&engineValue{kind: kBound, atom: akFloat},
			"float",
		},
		{
			"number atom no bounds",
			&engineValue{kind: kBound, atom: akNumber},
			"number",
		},
		{
			"bool atom no bounds",
			&engineValue{kind: kBound, atom: akBool},
			"bool",
		},
		{
			"bytes atom no bounds",
			&engineValue{kind: kBound, atom: akBytes},
			"bytes",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.v.describeBound())
		})
	}
}

func TestBound_Describe(t *testing.T) {
	cases := []struct {
		name string
		b    bound
		want string
	}{
		{"ge int", bound{op: opGe, num: 0}, ">=0"},
		{"le int", bound{op: opLe, num: 10}, "<=10"},
		{"gt int", bound{op: opGt, num: 0}, ">0"},
		{"lt int", bound{op: opLt, num: 100}, "<100"},
		{"ne string", bound{op: opNe, isStr: true, str: ""}, `!=""`},
		{"ne int", bound{op: opNe, num: 5}, "!=5"},
		{"match", bound{op: opMatch, src: `^[a-z]+$`}, `=~"^[a-z]+$"`},
		{"not match", bound{op: opNotMatch, src: `^[0-9]+$`}, `!~"^[0-9]+$"`},
		{"min runes", bound{op: opMinRunes, num: 5}, "strings.MinRunes(5)"},
		{"float operand", bound{op: opGe, num: 1.5}, ">=1.5"},
		{"negative int", bound{op: opGt, num: -1}, ">-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.b.describe())
		})
	}
}
