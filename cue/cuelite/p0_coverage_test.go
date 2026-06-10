package cuelite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcreteValueEqual_branches drives concreteValueEqual's list, struct, and
// scalar arms, including the struct field-count mismatch and a missing field.
func TestConcreteValueEqual_branches(t *testing.T) {
	one := &engineValue{kind: kInt, i: 1}
	two := &engineValue{kind: kInt, i: 2}
	mkStruct := func(fs ...field) *engineValue { return &engineValue{kind: kStruct, fields: fs} }
	// Different KIND is never equal.
	assert.False(t, concreteValueEqual(one, &engineValue{kind: kString, str: "1"}))
	// Equal and unequal lists.
	assert.True(t, concreteValueEqual(
		&engineValue{kind: kList, prefix: []*engineValue{one}},
		&engineValue{kind: kList, prefix: []*engineValue{one}}))
	assert.False(t, concreteValueEqual(
		&engineValue{kind: kList, prefix: []*engineValue{one}},
		&engineValue{kind: kList, prefix: []*engineValue{two}}))
	// An open list is never equal (the openTop guard).
	assert.False(t, concreteValueEqual(
		&engineValue{kind: kList, openTop: true},
		&engineValue{kind: kList, openTop: true}))
	// Struct field-count mismatch is unequal.
	assert.False(t, concreteValueEqual(
		mkStruct(field{name: "x", val: one}),
		mkStruct(field{name: "x", val: one}, field{name: "y", val: two})))
	// Same count, a field present in a but absent in b is unequal.
	assert.False(t, concreteValueEqual(
		mkStruct(field{name: "x", val: one}),
		mkStruct(field{name: "z", val: one})))
	// Equal structs.
	assert.True(t, concreteValueEqual(
		mkStruct(field{name: "x", val: one}),
		mkStruct(field{name: "x", val: one})))
}

// TestHasBottomLeaf_nestedPositions drives hasBottomLeaf's list-tail (elem) and
// disjunction arms: a bottom buried in an open list's tail element is found,
// and a disjunction value (a valid survivor) is never itself a failed branch.
func TestHasBottomLeaf_nestedPositions(t *testing.T) {
	bot := mkBottom(nil, "boom")
	// A bottom in an open list's tail element type is a buried bottom.
	openListWithBadTail := &engineValue{kind: kList, openTop: true, elem: bot}
	assert.True(t, hasBottomLeaf(openListWithBadTail))
	// A clean open list with a concrete tail is not.
	cleanOpenList := &engineValue{kind: kList, openTop: true, elem: &engineValue{kind: kInt, i: 1}}
	assert.False(t, hasBottomLeaf(cleanOpenList))
	// A disjunction value is a valid survivor, never a failed branch.
	disj := &engineValue{
		kind:     kDisjoint,
		branches: []*engineValue{{kind: kInt, i: 1}, {kind: kInt, i: 2}},
		modes:    []defaultMode{dfltMaybe, dfltMaybe},
	}
	assert.False(t, hasBottomLeaf(disj))
}

// TestHasBottomLeaf_listBranchPrunesOnNestedBottom drives the list-element
// branch of hasBottomLeaf end to end: a disjunction of open lists where one
// branch's tail conflicts with the data is pruned, so the clean branch decides.
func TestHasBottomLeaf_listBranchPrunesOnNestedBottom(t *testing.T) {
	// [...int] | [...string] against ["x"]: the int-tail branch's element meets
	// "x" to a bottom (nested in the list), so it is pruned and the string-tail
	// branch accepts.
	s, err := Compile(`{a: [...int] | [...string]}`)
	require.NoError(t, err)
	d, err := CompileJSON([]byte(`{"a":["x"]}`))
	require.NoError(t, err)
	assert.NoError(t, s.Unify(d).Validate())
}

// TestForceThunkFixpoint_schemaSideUnresolvable drives the final HARD force of
// the schema-side (o) struct: a schema thunk that cannot resolve after the
// fixpoint (its sibling reference is never made concrete by data) collapses to
// a ⊥ at validate rather than lingering as a silent thunk.
func TestForceThunkFixpoint_schemaSideUnresolvable(t *testing.T) {
	// n references m, but data supplies neither and m stays a bare type, so the
	// thunk never resolves: the final hard force surfaces it incomplete. The
	// DATA is the Unify receiver (d.Unify(s)), so the schema with the thunk is
	// the SECOND operand (o) — exercising the o-side final hard force.
	s, err := Compile(`{m: string, n: [if m == "p" {1}, 2][0]}`)
	require.NoError(t, err)
	d, err := CompileJSON([]byte(`{}`))
	require.NoError(t, err)
	assert.Error(t, d.Unify(s).Validate())
}

// TestRawHasLoneSurrogateEscape_truncatedAtEnd drives the i+6 > len bound: a
// `\u` escape with fewer than four trailing hex digits at the very end of the
// raw bytes is not a complete escape and is not a lone surrogate.
func TestRawHasLoneSurrogateEscape_truncatedAtEnd(t *testing.T) {
	assert.False(t, rawHasLoneSurrogateEscape([]byte(`"\ud8`)))
	assert.False(t, rawHasLoneSurrogateEscape([]byte(`"x\u`)))
}

// TestNestedThunkRefCheck_openListAndDisjunction drives checkThunkRefsIn's
// open-list (elem) and disjunction-branch descents: an undeclared reference in
// each nested position is a compile-time "reference not found".
func TestNestedThunkRefCheck_openListAndDisjunction(t *testing.T) {
	t.Run("open list tail element", func(t *testing.T) {
		_, err := Compile(`{xs: [...undeclared != ""]}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
	t.Run("disjunction branch", func(t *testing.T) {
		_, err := Compile(`{x: (undeclared == "a") | "z"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
	t.Run("declared open-list reference compiles", func(t *testing.T) {
		_, err := Compile(`{mech: string, xs: [...mech != ""]}`)
		require.NoError(t, err)
	})
}

// TestEmptyIntervalStringBounds drives emptyInterval's string branch in both
// the strict-equal and lower-exceeds-upper shapes, complementing the numeric
// cases in TestBoundIntersectionConflict.
func TestEmptyIntervalStringBounds(t *testing.T) {
	t.Run("string lower exceeds upper", func(t *testing.T) {
		_, err := Compile(`{x: >="z" & <="a"}`)
		require.Error(t, err)
	})
	t.Run("string equal and strict is empty", func(t *testing.T) {
		_, err := Compile(`{x: >"m" & <="m"}`)
		require.Error(t, err)
	})
	t.Run("string equal non-strict is the singleton", func(t *testing.T) {
		_, err := Compile(`{x: >="m" & <="m"}`)
		require.NoError(t, err)
	})
}

// TestNestedThunkInOpenListTail forces a thunk that sits in an open list's tail
// element type, exercising forceThunkValue's elem branch and hasThunkValue's
// elem branch.
func TestNestedThunkInOpenListTail(t *testing.T) {
	s, err := Compile(`{mech: string, xs: [...mech != ""]}`)
	require.NoError(t, err)
	t.Run("conforms", func(t *testing.T) {
		assert.NoError(t, s.CompileMap(map[string]any{"mech": "p", "xs": []any{true, true}}).Validate())
	})
	t.Run("rejects a false tail element", func(t *testing.T) {
		assert.Error(t, s.CompileMap(map[string]any{"mech": "p", "xs": []any{false}}).Validate())
	})
}

// TestForceThunkValue_nonThunkPassthrough covers forceThunkValue's default arm
// (a value with no thunk is returned unchanged) and the early returns when a
// list or disjunction carries no thunk: a struct whose fields are plain values
// and whose list/disjunction members hold no thunk forces to itself.
func TestForceThunkValue_nonThunkPassthrough(t *testing.T) {
	scope := map[string]*engineValue{}
	plain := &engineValue{kind: kInt, i: 1}
	assert.Same(t, plain, forceThunkValue(plain, scope, false))
	list := &engineValue{kind: kList, prefix: []*engineValue{plain}}
	assert.Same(t, list, forceThunkValue(list, scope, false))
	disj := &engineValue{
		kind:     kDisjoint,
		branches: []*engineValue{plain, {kind: kInt, i: 2}},
		modes:    []defaultMode{dfltMaybe, dfltMaybe},
	}
	assert.Same(t, disj, forceThunkValue(disj, scope, false))
}

// TestForcedDisjunctionKeepsDefault drives a disjunction whose DEFAULT branch
// is a thunk: forcing preserves the branch's default mode, so the resolved
// default survives the force pass. The schema `x: *(m == "a") | "z"` defaults
// to the comparison; with m="a" the default resolves to true and an absent x
// takes it.
func TestForcedDisjunctionKeepsDefault(t *testing.T) {
	s, err := Compile(`{m: string, x?: *(m == "a") | "z"}`)
	require.NoError(t, err)
	// x is optional and absent, so it takes its default. With m == "a" the
	// default thunk resolves to the concrete true, so the document is concrete.
	assert.NoError(t, s.CompileMap(map[string]any{"m": "a"}).Validate())
}
