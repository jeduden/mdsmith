package cuelite

import (
	"testing"

	"github.com/jeduden/mdsmith/cue/cuelite/syntax"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Final coverage pass. A handful of branches are total-function guards or
// kind-exhaustive defaults that the public surface cannot reach (the value
// model never builds a value of the offending kind on a real path). Per the
// repo rule, each is driven directly with a constructed value rather than left
// uncovered; the remaining branches are reachable from a crafted schema.

// TestUnifyV_unknownKind covers unifyV's exhaustive-switch default with a
// value of an out-of-model kind.
func TestUnifyV_unknownKind(t *testing.T) {
	got := unifyV(&engineValue{kind: 99}, &engineValue{kind: 99}, nil)
	assert.True(t, got.isBottomV())
}

// TestUnifyConcrete_againstStruct covers unifyConcrete's default branch: a
// concrete scalar unified with a struct conflicts.
func TestUnifyConcrete_againstStruct(t *testing.T) {
	got := unifyResult(t, `"x"`, `{b: int}`)
	assert.True(t, got.isBottomV())
}

// TestUnifyBoundLeft_againstStruct covers unifyBoundLeft's default branch: a
// bound unified with a struct conflicts.
func TestUnifyBoundLeft_againstStruct(t *testing.T) {
	got := unifyResult(t, `>=0`, `{b: int}`)
	assert.True(t, got.isBottomV())
}

// TestCompareNum_notEqual covers compareNum's opNe branch via a numeric != bound.
func TestCompareNum_notEqual(t *testing.T) {
	v, err := Compile(`{a: !=5}`)
	require.NoError(t, err)
	assert.NoError(t, v.CompileMap(map[string]any{"a": int64(6)}).Validate())
	assert.Error(t, v.CompileMap(map[string]any{"a": int64(5)}).Validate())
}

// TestBoundOpOf_outOfDomain covers boundOpOf's ok=false return for a token
// outside the relational set.
func TestBoundOpOf_outOfDomain(t *testing.T) {
	_, ok := boundOpOf(syntax.ADD)
	assert.False(t, ok)
	op, ok := boundOpOf(syntax.GEQ)
	require.True(t, ok)
	assert.Equal(t, opGe, op)
}

// TestListElemAt_pastClosedPrefix covers listElemAt's nil return for an index
// past a closed list's prefix (the total-function fallback).
func TestListElemAt_pastClosedPrefix(t *testing.T) {
	closed := &engineValue{kind: kList, prefix: []*engineValue{{kind: kInt, i: 1}}}
	assert.Nil(t, listElemAt(closed, 5))
	open := &engineValue{kind: kList, openTop: true, elem: topValue()}
	assert.NotNil(t, listElemAt(open, 5))
}

// TestCollectLeaves_unknownKind covers collectLeaves' exhaustive-switch
// default with an out-of-model kind.
func TestCollectLeaves_unknownKind(t *testing.T) {
	out := collectLeaves(&engineValue{kind: 99}, nil, nil)
	require.Len(t, out, 1)
}

// TestIsUnsatisfiedConstraint_direct covers both returns of
// isUnsatisfiedConstraint directly.
func TestIsUnsatisfiedConstraint_direct(t *testing.T) {
	// A ⊥ value: a provided-and-conflicting field, not an absent constraint.
	assert.False(t, isUnsatisfiedConstraint(mkBottom(nil, "boom")))
	// A bare atom with no ⊥: an absent optional field's unsatisfied constraint.
	assert.True(t, isUnsatisfiedConstraint(&engineValue{kind: kAtom, atom: akString}))
	// A concrete value: satisfied, so not "unsatisfied".
	assert.False(t, isUnsatisfiedConstraint(&engineValue{kind: kString, str: "x"}))
}

// TestIsConcrete_structSkipsAbsentOptional covers isConcrete's
// optional-unsatisfied-skip branch directly.
func TestIsConcrete_structSkipsAbsentOptional(t *testing.T) {
	// A struct with one absent optional (a bare atom constraint) and one
	// concrete field is concrete: the optional is skipped.
	st := &engineValue{kind: kStruct, fields: []field{
		{name: "opt", val: &engineValue{kind: kAtom, atom: akString}, optional: true},
		{name: "got", val: &engineValue{kind: kInt, i: 1}},
	}}
	assert.True(t, isConcrete(st))
}

// TestUnifyStruct_thunkOnLeft covers unifyStruct's hasThunkField(v) branch:
// the thunk-bearing schema struct is the left (v) operand of the meet.
func TestUnifyStruct_thunkOnLeft(t *testing.T) {
	schema, err := Compile(`{mechanism: "push" | "pull", ` +
		`registry: [if mechanism == "push" {string & != ""}, (string | *"")][0]}`)
	require.NoError(t, err)
	data, err := CompileJSON([]byte(`{"mechanism": "push", "registry": "npm"}`))
	require.NoError(t, err)
	// schema is the receiver (v side), so unifyStruct forces v's thunk field.
	assert.NoError(t, schema.Unify(data).Validate())
}

// TestEvalStruct_fieldPlusEmbedded covers evalStruct's field-plus-embedded
// result (a struct with both a named field and an embedded constraint).
func TestEvalStruct_fieldPlusEmbedded(t *testing.T) {
	// {x: int, >=0}: a field x plus an embedded numeric bound on the struct
	// itself. The embed makes the struct conflict with being a struct, so it
	// reduces to ⊥ — but the field+embedded unify position runs either way.
	v, err := Compile(`{a: {x: int, {x: >=0}}}`)
	require.NoError(t, err)
	assert.NoError(t, v.CompileMap(map[string]any{"a": map[string]any{"x": int64(5)}}).Validate())
	assert.Error(t, v.CompileMap(map[string]any{"a": map[string]any{"x": int64(-1)}}).Validate())
}

// TestFreeRefs_nilChild covers freeRefs' nil-node walk branch via a field with
// a nil value.
func TestFreeRefs_nilChild(t *testing.T) {
	refs := freeRefs(&syntax.UnaryExpr{Op: syntax.GEQ, X: nil})
	assert.Empty(t, refs)
}

// TestCompileBasicLit_directErrors covers compileBasicLit's string-unquote,
// float-parse, and unsupported-kind branches via constructed literals.
func TestCompileBasicLit_directErrors(t *testing.T) {
	_, err := compileBasicLit(&syntax.BasicLit{Kind: syntax.STRING, Value: `"\x"`})
	assert.Error(t, err, "a malformed string literal fails to unquote")
	_, err = compileBasicLit(&syntax.BasicLit{Kind: syntax.FLOAT, Value: "1.2.3"})
	assert.Error(t, err, "a malformed float literal fails to parse")
	_, err = compileBasicLit(&syntax.BasicLit{Kind: syntax.NoToken, Value: "x"})
	assert.Error(t, err, "an unsupported literal kind is rejected")
}

// TestCheckEmbeddedThunkRefs_declared covers checkEmbeddedThunkRefs' trailing
// nil return when the embedded thunk's references are all declared fields.
func TestCheckEmbeddedThunkRefs_declared(t *testing.T) {
	s := &engineValue{kind: kStruct, fields: []field{{name: "mode", val: &engineValue{kind: kAtom, atom: akString}}}}
	embedded := &engineValue{kind: kThunk, thunkRefs: []string{"mode"}}
	assert.NoError(t, checkEmbeddedThunkRefs(s, embedded), "all refs declared")
	// An undeclared reference is rejected.
	bad := &engineValue{kind: kThunk, thunkRefs: []string{"absent"}}
	assert.Error(t, checkEmbeddedThunkRefs(s, bad))
}

// TestEvalIdent_nonConcreteInScope covers evalIdent's non-concrete-in-scope
// branch directly.
func TestEvalIdent_nonConcreteInScope(t *testing.T) {
	scope := map[string]*engineValue{"n": {kind: kAtom, atom: akInt}}
	_, err := evalIdent(&syntax.Ident{Name: "n"}, scope)
	assert.ErrorIs(t, err, errUnresolved, "a non-concrete scope value defers")
}
