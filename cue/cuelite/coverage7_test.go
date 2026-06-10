package cuelite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests drive the per-kind discriminant disjuncts inside the unify and
// classification switches that statement coverage already counts (the case is
// reached via a sibling disjunct) but branch coverage (gobco) tracks
// separately. Each feeds a value of the specific kind — including kBytes and
// kFloat, which front-matter data does not produce but the model must still
// unify — through the relevant path with a constructed engine value.

// TestIsReferenceName_literals covers the null/true/false and float/number/
// bool/bytes keyword disjuncts of isReferenceName.
func TestIsReferenceName_literals(t *testing.T) {
	for _, kw := range []string{"_", "null", "true", "false", "string", "int", "float", "number", "bool", "bytes"} {
		assert.False(t, isReferenceName(kw), "%q is a keyword, not a reference", kw)
	}
	assert.True(t, isReferenceName("mechanism"))
}

// TestUnifyV_bytesLeftAndFloatLeft covers the kBytes and kFloat left-operand
// disjuncts of unifyV's concrete-scalar case.
func TestUnifyV_concreteScalarDisjuncts(t *testing.T) {
	// kBytes on the left, unified with an equal bytes value.
	b1 := &engineValue{kind: kBytes, bytes: []byte("x")}
	b2 := &engineValue{kind: kBytes, bytes: []byte("x")}
	got := unifyV(b1, b2, nil)
	assert.False(t, got.isBottomV())
	// kFloat on the left.
	f := unifyV(&engineValue{kind: kFloat, f: 1.5}, &engineValue{kind: kFloat, f: 1.5}, nil)
	assert.False(t, f.isBottomV())
}

// TestUnifyConcrete_bytesRight covers unifyConcrete's kBytes right-operand
// disjunct (two equal bytes values).
func TestUnifyConcrete_bytesRight(t *testing.T) {
	ba := &engineValue{kind: kBytes, bytes: []byte("a")}
	bb := &engineValue{kind: kBytes, bytes: []byte("a")}
	assert.False(t, unifyConcrete(ba, bb, nil).isBottomV())
}

// TestUnifyAtomLeft_floatBytesConcrete covers unifyAtomLeft's kFloat and
// kBytes concrete-right disjuncts (a typed atom narrowing to a concrete).
func TestUnifyAtomLeft_floatBytesConcrete(t *testing.T) {
	// float atom & concrete float.
	got := unifyAtomLeft(&engineValue{kind: kAtom, atom: akFloat}, &engineValue{kind: kFloat, f: 1.5}, nil)
	require.Equal(t, kFloat, got.kind)
	// bytes atom & concrete bytes.
	gb := unifyAtomLeft(&engineValue{kind: kAtom, atom: akBytes}, &engineValue{kind: kBytes, bytes: []byte("z")}, nil)
	assert.Equal(t, kBytes, gb.kind)
}

// TestUnifyBoundLeft_concreteDisjuncts covers unifyBoundLeft's float/bool/
// bytes/null right-operand disjuncts: a bound met by a concrete of each kind
// (the base-kind check then decides accept or conflict).
func TestUnifyBoundLeft_concreteDisjuncts(t *testing.T) {
	numBound := &engineValue{kind: kBound, atom: akNumber, bounds: []bound{{op: opGe, num: 0}}}
	// A concrete float satisfies a numeric bound.
	assert.False(t, unifyBoundLeft(numBound, &engineValue{kind: kFloat, f: 1.5}, nil).isBottomV())
	// A concrete bool conflicts with a numeric bound (base-kind mismatch).
	assert.True(t, unifyBoundLeft(numBound, &engineValue{kind: kBool, b: true}, nil).isBottomV())
	// A concrete null conflicts with a numeric bound.
	assert.True(t, unifyBoundLeft(numBound, &engineValue{kind: kNull}, nil).isBottomV())
	// A concrete bytes conflicts with a numeric bound.
	assert.True(t, unifyBoundLeft(numBound, &engineValue{kind: kBytes, bytes: []byte("x")}, nil).isBottomV())
}

// TestConcreteScalarV_floatNull covers concreteScalarV's kInt/kFloat/kNull
// disjuncts directly.
func TestConcreteScalarV_floatNull(t *testing.T) {
	assert.True(t, (&engineValue{kind: kInt, i: 1}).concreteScalarV())
	assert.True(t, (&engineValue{kind: kFloat, f: 1.5}).concreteScalarV())
	assert.True(t, (&engineValue{kind: kNull}).concreteScalarV())
}

// TestIsDeferrable_allRelationalOps covers each relational-operator disjunct of
// isDeferrable via a parenthesized comparison over a sibling reference.
func TestIsDeferrable_allRelationalOps(t *testing.T) {
	for _, op := range []string{">=", "<=", ">", "<", "!=", "==", "=~", "!~"} {
		rhs := "0"
		if op == "=~" || op == "!~" {
			rhs = `"x"`
		}
		e := parseExpr(t, `(n `+op+` `+rhs+`)`)
		assert.True(t, isDeferrable(e), "op %q over a sibling is deferrable", op)
	}
	// A non-relational binary (a bare reference) is not deferrable.
	assert.False(t, isDeferrable(parseExpr(t, `n`)))
}
