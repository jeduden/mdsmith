package cuelite

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCombineMode verifies that combineMode returns max(a,b) across all
// pairings of the three defaultMode values.
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

// TestMkBottom verifies that mkBottom builds a ⊥ value carrying the
// formatted reason and the supplied path.
func TestMkBottom(t *testing.T) {
	v := mkBottom([]string{"a", "b"}, "conflict: %s vs %s", "x", "y")
	assert.True(t, v.isBottomV(), "mkBottom must produce a ⊥ value")
	assert.Equal(t, "conflict: x vs y", v.reason)
	assert.Equal(t, []string{"a", "b"}, v.path)
	assert.Equal(t, "_|_", v.describe())

	// nil path is preserved as nil.
	v2 := mkBottom(nil, "no path")
	assert.True(t, v2.isBottomV())
	assert.Nil(t, v2.path)
}

// TestTopValue verifies that topValue produces the lattice identity: not ⊥,
// and describing as "_".
func TestTopValue(t *testing.T) {
	v := topValue()
	assert.Equal(t, "_", v.describe())
	assert.False(t, v.isBottomV(), "topValue must not be ⊥")
}

// TestEngineValue_IsBottomV covers the nil receiver, a kBottom value, and
// several non-bottom values.
func TestEngineValue_IsBottomV(t *testing.T) {
	var nilV *engineValue
	assert.False(t, nilV.isBottomV(), "nil receiver must return false")
	assert.True(t, mkBottom(nil, "x").isBottomV())
	assert.False(t, topValue().isBottomV())
	assert.False(t, (&engineValue{kind: kString, str: "hello"}).isBottomV())
	assert.False(t, (&engineValue{kind: kNull}).isBottomV())
}

// TestEngineValue_DefaultValue covers no default (returns nil,false), a
// single default (returns the branch, false), and ambiguous defaults (nil,true).
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

	t.Run("single default", func(t *testing.T) {
		v := &engineValue{
			kind:     kDisjoint,
			branches: []*engineValue{a, b},
			modes:    []defaultMode{dfltIs, dfltNot},
		}
		got, ambig := v.defaultValue()
		assert.Same(t, a, got)
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

// TestEngineValue_DescribeBound verifies that describeBound renders the base
// atom type followed by each bound joined with " & ".
func TestEngineValue_DescribeBound(t *testing.T) {
	t.Run("int with two numeric bounds", func(t *testing.T) {
		v := &engineValue{
			kind: kBound,
			atom: akInt,
			bounds: []bound{
				{op: opGe, num: 0},
				{op: opLe, num: 100},
			},
		}
		assert.Equal(t, "int & >=0 & <=100", v.describeBound())
	})
	t.Run("string with regex match constraint", func(t *testing.T) {
		v := &engineValue{
			kind:   kBound,
			atom:   akString,
			bounds: []bound{{op: opMatch, src: `^[a-z]+$`}},
		}
		assert.Equal(t, `string & =~"^[a-z]+$"`, v.describeBound())
	})
	t.Run("no bounds renders atom only", func(t *testing.T) {
		v := &engineValue{kind: kBound, atom: akFloat}
		assert.Equal(t, "float", v.describeBound())
	})
}

// TestBound_Describe covers every boundOp, integral vs float numeric
// operands, and string operands.
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
